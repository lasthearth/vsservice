package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"os"
	"path/filepath"
)

type Config struct {
	AppEnv string `default:"dev"`

	GrpcPort     int `envconfig:"GPRC_PORT" default:"50051"`
	GateAwayPort int `envconfig:"GATEAWAY_PORT" default:"6969"`
}

func New() (Config, error) {
	cfg := Config{}

	wd, err := os.Getwd()
	if err != nil {
		return cfg, err
	}

	envPath := filepath.Join(wd, ".env")

	_ = godotenv.Load(envPath)

	if err := envconfig.Process("", &cfg); err != nil {
		return cfg, err
	}

	fmt.Println(cfg)

	return cfg, nil
}
