package config

import (
	"flag"
	"os"
)

var (
	TokenKey             string
	DbUri                string
	RunAddress           string
	AccrualSystemAddress string
)

func Init() {
	flag.StringVar(&RunAddress, "a", os.Getenv("RUN_ADDRESS"), "address and port to run server")
	flag.StringVar(&DbUri, "d", os.Getenv("DB_URI"), "database uri")
	flag.StringVar(&AccrualSystemAddress, "r", os.Getenv("ACCRUAL_SYSTEM_ADDRESS"), "address of accrual system")
	flag.Parse()

	TokenKey = os.Getenv("TOKEN_KEY")
}
