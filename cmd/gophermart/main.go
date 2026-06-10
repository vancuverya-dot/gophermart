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

func gracefulShutdown(apiServer *http.Server, done chan struct{}) {
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
	close(done)
}

func main() {
	cfg := config.New()

	db, err := database.New(cfg.DBURI)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := worker.New(db, cfg.AccrualSystemAddress)
	workerDone := make(chan struct{})
	go func() {
		w.Run(ctx, 2*time.Second)
		close(workerDone)
	}()

	srv := server.NewServerWithDB(db, cfg)
	log.Printf("Starting server on %s", srv.Addr)

	done := make(chan struct{})
	go gracefulShutdown(srv, done)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %s", err)
	}

	<-done
	cancel()
	<-workerDone

	if err := db.Close(); err != nil {
		log.Printf("error closing database: %v", err)
	}
	log.Println("Graceful shutdown complete.")
}
