package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/database"
	"github.com/vancuverya-dot/gophermart/internal/server"
	"github.com/vancuverya-dot/gophermart/internal/worker"
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

	db := database.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.New(db.DB()).Run(ctx)

	srv := server.NewServer(db)
	log.Printf("Starting server on %s", srv.Addr)

	done := make(chan bool, 1)
	go gracefulShutdown(srv, done)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %s", err)
	}
	<-done
	log.Println("Graceful shutdown complete.")
}
