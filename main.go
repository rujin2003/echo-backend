package main

import (
	"log"
	"net/http"
	"os"
)



var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}
	jwtSecret = []byte(secret)
}

func main() {
	manager := NewManager()

	http.HandleFunc("/ws", manager.serveWs)

	log.Printf("WebSocket server starting on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
