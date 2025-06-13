package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config represents the configuration for the application.
type Config struct {
	AppEnv string `default:"dev"`

	JWKS_URL string `envconfig:"JWKS_URL"`
	Issuer   string `envconfig:"ISSUER"`
	Audience string `envconfig:"AUDIENCE"`

	GrpcPort     int `envconfig:"GPRC_PORT" default:"50051"`
	GateAwayPort int `envconfig:"GATEAWAY_PORT" default:"6969"`

	MongoUrlFile string `envconfig:"MONGO_URL_FILE" required:"true"`

	VsAPIUrl string `envconfig:"VSAPI_URL"`

	CdnUrl string `envconfig:"CDN_URL"`

	SsoUrl       string   `envconfig:"SSO_URL"`
	ClientID     string   `envconfig:"CLIENT_ID"`
	ClientSecret string   `envconfig:"CLIENT_SECRET"`
	TokenUrl     string   `envconfig:"TOKEN_URL"`
	Resource     string   `envconfig:"RESOURCE"`
	Scopes       []string `envconfig:"SCOPES"`

	StatsFetchingIntervalSecs int  `envconfig:"STATS_FETCHING_INTERVAL_SECS"`
	StatsFetchingEnable       bool `envconfig:"STATS_FETCHING_ENABLE"`

	MinioEndpoint  string `envconfig:"MINIO_ENDPOINT"`
	MinioAccessKey string `envconfig:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `envconfig:"MINIO_SECRET_KEY"`
	MinioUseSSL    bool   `envconfig:"MINIO_USE_SSL"`
}

// New initializes from .env and returns a new Config instance.
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
