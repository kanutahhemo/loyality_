package main

import (
	"flag"
	"fmt"
	"github.com/kanutahhemo/loyality_/internal/config"
	"github.com/kanutahhemo/loyality_/internal/storage/database"
	"github.com/kanutahhemo/loyality_/internal/transport/server"
	"github.com/sirupsen/logrus"
	"log"
	"os"
)

func main() {
	flag.Parse()
	cfg := config.GetCfg()
	fmt.Println("config: ", cfg)
	logFile, err := os.OpenFile("loyality.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("failed to create log file: %s", err)
	}
	defer logFile.Close()

	// Создаем логгер
	logger := logrus.New()

	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	switch cfg.LogLevel {
	case "ERROR":
		logger.SetLevel(logrus.ErrorLevel)
	case "WARNING":
		logger.SetLevel(logrus.WarnLevel)
	case "INFO":
		logger.SetLevel(logrus.InfoLevel)
	case "DEBUG":
		logger.SetLevel(logrus.DebugLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.SetOutput(logFile)
	logger.Debugf("Loglevel is %s", logger.GetLevel())

	logger.Debugf("dns is: %s", cfg.DatabaseDSN)
	if err := database.ApplyMigrations(cfg.DatabaseDSN); err != nil {
		logger.Fatalf("failed to apply migrations: %s", err)
	}

	db, err := database.NewPgDatabase(cfg.DatabaseDSN)
	if err != nil {
		logger.Fatal("Failed to connect to the database")
	}
	defer db.CancelFunc()
	defer db.Close()

	server.RunServer(cfg, db, logger)
}
