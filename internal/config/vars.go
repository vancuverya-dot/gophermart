package config

import (
	"flag"
	"os"
)

// Config — конфигурация приложения.
type Config struct {
	TokenKey             string
	DBURI                string
	RunAddress           string
	AccrualSystemAddress string
}

// New — создаёт конфиг из флагов командной строки и переменных окружения.
// Флаги имеют приоритет над переменными окружения.
func New() *Config {
	var runAddress, dbURI, accrualAddress string
	flag.StringVar(&runAddress, "a", "", "address and port to run server")
	flag.StringVar(&dbURI, "d", "", "database uri")
	flag.StringVar(&accrualAddress, "r", "", "address of accrual system")
	flag.Parse()

	cfg := &Config{
		TokenKey: os.Getenv("TOKEN_KEY"),
	}

	if runAddress != "" {
		cfg.RunAddress = runAddress
	} else {
		cfg.RunAddress = os.Getenv("RUN_ADDRESS")
	}

	if dbURI != "" {
		cfg.DBURI = dbURI
	} else {
		cfg.DBURI = os.Getenv("DATABASE_URI")
	}

	if accrualAddress != "" {
		cfg.AccrualSystemAddress = accrualAddress
	} else {
		cfg.AccrualSystemAddress = os.Getenv("ACCRUAL_SYSTEM_ADDRESS")
	}

	return cfg
}
