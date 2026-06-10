package database

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var postgresDSN string

func mustStartPostgresContainer() (string, func(context.Context, ...testcontainers.TerminateOption) error, error) {
	var (
		dbName = "database"
		dbPwd  = "password"
		dbUser = "user"
	)

	dbContainer, err := postgres.Run(
		context.Background(),
		"postgres:latest",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPwd),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		return "", nil, err
	}

	host, err := dbContainer.Host(context.Background())
	if err != nil {
		return "", dbContainer.Terminate, err
	}

	port, err := dbContainer.MappedPort(context.Background(), "5432/tcp")
	if err != nil {
		return "", dbContainer.Terminate, err
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPwd, host, port.Port(), dbName)

	return dsn, dbContainer.Terminate, nil
}

func TestMain(m *testing.M) {
	dsn, teardown, err := mustStartPostgresContainer()
	if err != nil {
		log.Fatalf("could not start postgres container: %v", err)
	}
	postgresDSN = dsn

	m.Run()

	if teardown != nil {
		if err := teardown(context.Background()); err != nil {
			log.Fatalf("could not teardown postgres container: %v", err)
		}
	}
}

func TestNew(t *testing.T) {
	srv, err := New(postgresDSN)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("New() returned nil")
	}
}

func TestClose(t *testing.T) {
	srv, err := New(postgresDSN)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("expected Close() to return nil, got %v", err)
	}
}
