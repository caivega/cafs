//  BitWrk - A Bitcoin-friendly, anonymous marketplace for computing power
//  Copyright (C) 2013-2019 Jonas Eschenburg <jonas@bitwrk.net>
//
//  This program is free software: you can redistribute it and/or modify
//  it under the terms of the GNU General Public License as published by
//  the Free Software Foundation, either version 3 of the License, or
//  (at your option) any later version.
//
//  This program is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of
//  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//  GNU General Public License for more details.
//
//  You should have received a copy of the GNU General Public License
//  along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package httpsync implements methods for requesting and serving files via CAFS
package httpsync

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/indyjo/cafs"
	"github.com/indyjo/cafs/remotesync"
	"github.com/indyjo/cafs/remotesync/shuffle"
)

// Struct FileHandler implements the http.Handler interface and serves a file over HTTP.
// The protocol used matches with function SyncFrom.
// Create using the New... functions.
type FileHandler struct {
	m        sync.Mutex
	source   chunksSource
	syncinfo *remotesync.SyncInfo
	log      cafs.Printer
}

// It is the owner's responsibility to correctly dispose of FileHandler instances.
func (handler *FileHandler) Dispose() {
	handler.m.Lock()
	s := handler.source
	handler.source = nil
	handler.syncinfo = nil
	handler.m.Unlock()
	if s != nil {
		s.Dispose()
	}
}

// Function NewFileHandlerFromFile creates a FileHandler that serves chunks of a File.
func NewFileHandlerFromFile(file cafs.File, perm shuffle.Permutation) *FileHandler {
	result := &FileHandler{
		m:        sync.Mutex{},
		source:   fileBasedChunksSource{file: file.Duplicate()},
		syncinfo: &remotesync.SyncInfo{Perm: perm},
		log:      cafs.NewWriterPrinter(ioutil.Discard),
	}
	result.syncinfo.SetChunksFromFile(file)
	return result
}

// Function NewFileHandlerFromSyncInfo creates a FileHandler that serves chunks as
// specified in a FileInfo. It doesn't necessarily require all of the chunks to be present
// and will block waiting for a missing chunk to become available.
// As a specialty, a FileHander created using this function needs not be disposed.
func NewFileHandlerFromSyncInfo(syncinfo *remotesync.SyncInfo, storage cafs.FileStorage) *FileHandler {
	result := &FileHandler{
		m: sync.Mutex{},
		source: syncInfoChunksSource{
			syncinfo: syncinfo,
			storage:  storage,
		},
		syncinfo: syncinfo,
		log:      cafs.NewWriterPrinter(ioutil.Discard),
	}
	return result
}

// Sets the FileHandler's log Printer.
func (handler *FileHandler) WithPrinter(printer cafs.Printer) *FileHandler {
	handler.log = printer
	return handler
}

func (handler *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if err := json.NewEncoder(w).Encode(handler.syncinfo); err != nil {
			handler.log.Printf("Error serving SyncInfo: R%v", err)
		}
		return
	} else if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Require a Connection: close header that will trick Go's HTTP server into allowing bi-directional streams.
	if r.Header.Get("Connection") != "close" {
		http.Error(w, "Connection: close required", http.StatusBadRequest)
		return
	}

	chunks, err := handler.source.GetChunks()
	if err != nil {
		handler.log.Printf("GetChunks() failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer chunks.Dispose()

	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()

	var bytesSkipped, bytesTransferred int64
	cb := func(toTransfer, transferred int64) {
		bytesSkipped = -toTransfer
		bytesTransferred = transferred
	}
	handler.log.Printf("Calling WriteChunkData")
	start := time.Now()
	err = remotesync.WriteChunkData(chunks, 0, bufio.NewReader(r.Body), handler.syncinfo.Perm,
		remotesync.SimpleFlushWriter{w, w.(http.Flusher)}, cb)
	duration := time.Since(start)
	speed := float64(bytesTransferred) / duration.Seconds()
	handler.log.Printf("WriteChunkData took %v. KBytes transferred: %v (%.2f/s) skipped: %v",
		duration, bytesTransferred>>10, speed/1024, bytesSkipped>>10)
	if err != nil {
		handler.log.Printf("Error in WriteChunkData: %v", err)
		return
	}
}

// Function SyncFrom uses an HTTP client to connect to some URL and download a fie into the
// given FileStorage.
func SyncFrom(ctx context.Context, storage cafs.FileStorage, client *http.Client, url, info string) (file cafs.File, err error) {
	// Fetch SyncInfo from remote
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET returned status %v", resp.Status)
	}
	var syncinfo remotesync.SyncInfo
	err = json.NewDecoder(resp.Body).Decode(&syncinfo)
	if err != nil {
		return
	}

	// Create Builder and establish a bidirectional POST connection
	builder := remotesync.NewBuilder(storage, &syncinfo, 32, info)
	defer builder.Dispose()

	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, url, pr)
	if err != nil {
		return
	}

	// Enable cancelation
	req = req.WithContext(ctx)

	// Trick Go's HTTP server implementation into allowing bi-directional data flow
	req.Header.Set("Connection", "close")

	go func() {
		if err := builder.WriteWishList(remotesync.NopFlushWriter{pw}); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("error in WriteWishList: %v", err))
			return
		}
		_ = pw.Close()
	}()

	res, err := client.Do(req)
	if err != nil {
		return
	}
	file, err = builder.ReconstructFileFromRequestedChunks(res.Body)
	return
}

func SyncFile(fileStorage cafs.FileStorage, source string) error {
	log.Printf("Sync from %v", source)
	if file, err := SyncFrom(context.Background(), fileStorage, http.DefaultClient, source, "synced from "+source); err != nil {
		return err
	} else {
		log.Printf("Successfully received %v (%v bytes)", file.Key(), file.Size())
		file.Dispose()
	}
	return nil
}

func SaveFile(storage cafs.FileStorage, hash string, path string) error {
	key, err := cafs.ParseKey(hash)
	if err != nil {
		return err
	}
	f, err := storage.Get(key)
	if err != nil {
		return err
	}
	log.Printf("Load data: %v (chunked: %v, %v chunks)", path, f.IsChunked(), f.NumChunks())

	_, err = os.Stat(path)
	if err == nil {
		return errors.New(path + " exists")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	tf, err := os.Create(path)
	if err != nil {
		return err
	}
	defer tf.Close()

	rf := f.Open()
	defer rf.Close()
	if n, err := io.Copy(tf, rf); err != nil {
		return err
	} else {
		log.Printf("Write file: %v (%v bytes, chunked: %v, %v chunks)", path, n, f.IsChunked(), f.NumChunks())
	}
	return nil
}

func LoadFile(storage cafs.FileStorage, path string) (cafs.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	tmp := storage.Create(path)
	defer tmp.Dispose()
	n, err := io.Copy(tmp, f)
	if err != nil {
		return nil, fmt.Errorf("error after copying %v bytes: %v", n, err)
	}

	err = tmp.Close()
	if err != nil {
		return nil, err
	}

	file := tmp.File()
	log.Printf("Read file: %v (%v, %v bytes, chunked: %v, %v chunks)", path, file.Key().String(), n, file.IsChunked(), file.NumChunks())

	return file, nil
}
