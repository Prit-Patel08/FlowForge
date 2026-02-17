package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	_, err := http.Get("http://localhost:8080/incidents")
	if err != nil {
		fmt.Printf("Healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
