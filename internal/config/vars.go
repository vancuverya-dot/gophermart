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
	var runAddress, dbUri, accrualAddress string
	flag.StringVar(&runAddress, "a", "", "address and port to run server")
	flag.StringVar(&dbUri, "d", "", "database uri")
	flag.StringVar(&accrualAddress, "r", "", "address of accrual system")
	flag.Parse()

	if runAddress != "" {
		RunAddress = runAddress
	} else {
		RunAddress = os.Getenv("RUN_ADDRESS")
	}

	if dbUri != "" {
		DbUri = dbUri
	} else {
		DbUri = os.Getenv("DATABASE_URI")
	}

	if accrualAddress != "" {
		AccrualSystemAddress = accrualAddress
	} else {
		AccrualSystemAddress = os.Getenv("ACCRUAL_SYSTEM_ADDRESS")
	}

	TokenKey = os.Getenv("TOKEN_KEY")
}
