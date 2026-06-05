package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/server"
)

func gracefulShutdown(apiServer *http.Server, done chan bool) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down gracefully, press Ctrl+C again to force")
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown with error: %v", err)
	}

	log.Println("Server exiting")

	done <- true
}

func main() {
	config.Init()

	if config.DbUri == "" {
		godotenv.Load()
		config.Init()
	}

	log.Printf("Config: addr=%s db=%s", config.RunAddress, config.DbUri)

	srv := server.NewServer()
	log.Printf("Starting server on %s", srv.Addr)

	done := make(chan bool, 1)
	go gracefulShutdown(srv, done)

	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %s", err)
	}

	<-done
	log.Println("Graceful shutdown complete.")
}
