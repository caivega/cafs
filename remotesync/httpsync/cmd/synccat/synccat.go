package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func main() {
	addr := "127.0.0.1:8080"
	flag.StringVar(&addr, "l", addr, "which port to connect")

	source := "127.0.0.1:8080"
	flag.StringVar(&source, "s", source, "which source to connect")

	hash := ""
	flag.StringVar(&hash, "h", hash, "hash file to sync")

	path := ""
	flag.StringVar(&path, "p", path, "the file to save")
	flag.Parse()

	if addr != source {
		sync(addr, source, hash)
	}
	save(addr, hash, path)
}

func sync(addr string, source string, hash string) {
	resp, err := http.Post(
		fmt.Sprintf("http://%s/sync", addr),
		"application/x-www-form-urlencoded",
		strings.NewReader(fmt.Sprintf("source=%s", fmt.Sprintf("http://%s/file/%s", source, hash[:16]))),
	)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("sync %s from %s, done\n", hash, source)
}

func save(addr string, hash string, path string) {
	resp, err := http.Post(
		fmt.Sprintf("http://%s/save", addr),
		"application/x-www-form-urlencoded",
		strings.NewReader(fmt.Sprintf("hash=%s&path=%s", hash, path)),
	)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("save %s to %s, done\n", hash, path)
}
