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
//  along with this program.  If not, see <http://www.gnu.org/licenses/>.package main

package cmd

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime/pprof"

	"github.com/indyjo/cafs"
	"github.com/indyjo/cafs/ram"
	"github.com/indyjo/cafs/remotesync/httpsync"
)

var storage cafs.FileStorage = ram.NewRamStorage(1 << 30)
var fileHandlers = make(map[string]*httpsync.FileHandler)

func Service(addr string, preloads []string) {
	for _, preload := range preloads {
		if _, err := loadFile(preload); err != nil {
			log.Fatalf("Error loading '[%v]: %v", preload, err)
		}
	}

	http.HandleFunc("/load", handleLoad)
	http.HandleFunc("/save", handleSave)
	http.HandleFunc("/sync", handleSync)
	http.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		storage.DumpStatistics(cafs.NewWriterPrinter(w))
	})
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		storage = ram.NewRamStorage(1 << 30)
		fileHandlers = make(map[string]*httpsync.FileHandler)

		log.Println("reset done")
		_, _ = w.Write([]byte("reset done"))
	})
	http.HandleFunc("/dump", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		if len(name) == 0 {
			name = "goroutine"
		}
		profile := pprof.Lookup(name)
		if profile == nil {
			_, _ = w.Write([]byte("No such profile"))
			return
		}
		err := profile.WriteTo(w, 1)
		if err != nil {
			log.Printf("Error in profile.WriteTo: %v\n", err)
		}
	})

	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatalf("Error in ListenAndServe: %v", err)
	}
}

func loadFile(path string) (string, error) {
	file, err := httpsync.LoadFile(storage, path)
	if err != nil {
		return "", err
	}
	defer file.Dispose()

	_, ok := fileHandlers[file.Key().String()]
	if !ok {
		printer := log.New(os.Stderr, "", log.LstdFlags)
		handler := httpsync.NewFileHandlerFromFile(file, rand.Perm(256)).WithPrinter(printer)
		fileHandlers[file.Key().String()] = handler

		path = fmt.Sprintf("/file/%v", file.Key().String()[:16])
		http.Handle(path, handler)
		log.Printf("serving under %v", path)
	} else {
		log.Printf("serving exists %v", path)
	}

	return file.Key().String(), nil
}

func handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	path := r.FormValue("path")

	hash, err := loadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte(hash))
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	hash := r.FormValue("hash")
	path := r.FormValue("path")

	err := httpsync.SaveFile(storage, hash, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	source := r.FormValue("source")
	if err := httpsync.SyncFile(storage, source); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
