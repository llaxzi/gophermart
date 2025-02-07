package main

import (
	"flag"
	"os"
)

var runAddr string
var databaseDSN string
var accrualAddr string

func parseVars() {
	if envRunAddr := os.Getenv("RUN_ADDRESS"); envRunAddr != "" {
		runAddr = envRunAddr
	}
	if envDatabaseDSN := os.Getenv("DATABASE_URI"); envDatabaseDSN != "" {
		databaseDSN = envDatabaseDSN
	}
	if envAccrualAddr := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); envAccrualAddr != "" {
		accrualAddr = envAccrualAddr
	}

	flagRunAddr := flag.String("a", "", "run address")
	if *flagRunAddr != "" {
		runAddr = *flagRunAddr
	}
	flagDatabaseDSN := flag.String("d", "", "database dsn")
	if *flagDatabaseDSN != "" {
		runAddr = *flagDatabaseDSN
	}
	flagAccrualAddr := flag.String("r", "", "accrual system address")
	if *flagAccrualAddr != "" {
		runAddr = *flagAccrualAddr
	}
}
