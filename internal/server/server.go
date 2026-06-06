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

func NewServer(db database.Service) *http.Server {
	s := &Server{
		addr: config.RunAddress,
		db:   db,
	}
	return &http.Server{
		Addr:         s.addr,
		Handler:      s.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}
