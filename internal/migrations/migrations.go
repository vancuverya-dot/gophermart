package migrations

import (
	"database/sql"
	"embed"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed sql/*.sql
var migrationsFS embed.FS

// Up — применяет все up миграции которые ещё не были применены.
func Up(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "sql")
	if err != nil {
		log.Printf("failed to create migration source: %v", err)
		return err
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Printf("failed to create migration driver: %v", err)
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		log.Printf("failed to create migrate instance: %v", err)
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Printf("failed to apply migrations: %v", err)
		return err
	}

	log.Println("migrations applied successfully")
	return nil
}
