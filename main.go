package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

var jwtSecret []byte

func init() {
	// .env is OPTIONAL (Render does not use it)
	_ = godotenv.Load()

	// JWT secret is REQUIRED
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}
	jwtSecret = []byte(secret)
}

func main() {
	// Panic safety
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in main: %v", r)
		}
	}()

	// Render-injected PORT (MANDATORY)
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is not set (Render injects this automatically)")
	}

	manager := NewManager()

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", manager.serveWs)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Printf("WebSocket server starting on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server exited gracefully")
	}
}
