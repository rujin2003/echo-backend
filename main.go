package main

import (
	"fmt"
	"net/http"
)

func main() {
	setupApi()
}

func setupApi() {
	http.HandleFunc("/ws", NewManager().serveWs)
	fmt.Println("Server started on :8080")
	http.ListenAndServe(":8080", nil)
}
