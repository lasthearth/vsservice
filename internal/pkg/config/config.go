package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"os"
	"path/filepath"
)

type Config struct {
	AppEnv string `default:"dev"`

	GrpcPort     int `envconfig:"GPRC_PORT" default:"50051"`
	GateAwayPort int `envconfig:"GATEAWAY_PORT" default:"6969"`

	MongoUrlFile string `envconfig:"MONGO_URL_FILE" required:"true"`

	VsAPIUrl string `envconfig:"VSAPI_URL"`

	StatsFetchingIntervalSecs int  `envconfig:"STATS_FETCHING_INTERVAL_SECS"`
	StatsFetchingEnable       bool `envconfig:"STATS_FETCHING_ENABLE"`
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

	return cfg, nil
}
