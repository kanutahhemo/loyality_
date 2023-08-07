package config

import (
	"flag"
	"github.com/caarlos0/env/v6"
	"log"
	"os"
)

type envConfig struct {
	ServerAddress        string `env:"RUN_ADDRESS" envDefault:"localhost:8080"`
	DatabaseDSN          string `env:"DATABASE_URI"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
}

type argsConfig struct {
	ServerAddress        string
	DatabaseDSN          string
	AccrualSystemAddress string
}

type Config struct {
	ServerAddress        string
	DatabaseDSN          string
	AccrualSystemAddress string
	SecretKey            string
	LogLevel             string
}

var Cfg Config
var envCfg envConfig
var argsCfg argsConfig
var F string

func init() {

	flag.StringVar(&argsCfg.ServerAddress, "a", "localhost:8080", "Server Address")
	flag.StringVar(&argsCfg.DatabaseDSN, "d", "", "Database DSN")
	flag.StringVar(&argsCfg.AccrualSystemAddress, "r", "", "ACCRUAL_SYSTEM_ADDRESS")
	flag.StringVar(&F, "f", "", "echo string")

}

func EchoString() string {
	return F
}

func GetCfg() Config {
	err := env.Parse(&envCfg)
	if err != nil {
		log.Fatal(err)
	}

	var present bool

	_, present = os.LookupEnv("SERVER_ADDRESS")
	if present && envCfg.ServerAddress != "" {
		Cfg.ServerAddress = envCfg.ServerAddress
	} else {
		Cfg.ServerAddress = argsCfg.ServerAddress
	}

	_, present = os.LookupEnv("DATABASE_DSN")
	if present && envCfg.DatabaseDSN != "" {
		Cfg.DatabaseDSN = envCfg.DatabaseDSN
	} else {
		Cfg.DatabaseDSN = argsCfg.DatabaseDSN
	}

	_, present = os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS")
	if present && envCfg.DatabaseDSN != "" {
		Cfg.AccrualSystemAddress = envCfg.AccrualSystemAddress
	} else {
		Cfg.AccrualSystemAddress = argsCfg.AccrualSystemAddress
	}

	Cfg.SecretKey, present = os.LookupEnv("SECRET")
	if !present {
		log.Print("SECRET is not specified. Set it to 111")
		Cfg.LogLevel = "111"
	}

	Cfg.LogLevel, present = os.LookupEnv("LOGLEVEL")
	if !present {
		log.Print("LOGLEVEL is not specified. Set it to error")
		Cfg.LogLevel = "ERROR"
	}

	return Cfg
}
