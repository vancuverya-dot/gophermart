package server

import (
	"net/http"
	"time"

	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/database"
)

type Server struct {
	addr string
	db   database.Service
}

func NewServer() *http.Server {
	NewServer := &Server{
		addr: config.RunAddress,
		db:   database.New(),
	}

	server := &http.Server{
		Addr:         NewServer.addr,
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}
