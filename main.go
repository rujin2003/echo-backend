package main

import (
	"log"
	"net/http"
	"os"
	"github.com/joho/godotenv"
)



var jwtSecret []byte
func init() {
	
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, relying on system environment")
	}

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
