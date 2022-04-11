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

	path := ""
	flag.StringVar(&path, "p", path, "input file to load")
	flag.Parse()

	resp, err := http.Post(
		fmt.Sprintf("http://%s/load", addr),
		"application/x-www-form-urlencoded",
		strings.NewReader(fmt.Sprintf("path=%s", path)),
	)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(body))
}
