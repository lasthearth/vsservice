package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config represents the configuration for the application.
type Config struct {
	AppEnv string `default:"dev"`

	// DisableAuthMatcher disables interceptor for auth token.
	DisableAuthMatcher bool `envconfig:"DISABLE_AUTH_MATCHER" default:"false"`

	// LogLevel is the minimum level emitted. Accepts debug, info, warn, error.
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// LogFormat forces the encoder: json or console. Empty derives it from
	// AppEnv (console in dev, json otherwise).
	LogFormat string `envconfig:"LOG_FORMAT"`
	// LogSampleReads keeps 1 record in N for successful high-volume reads
	// (List*/Get* and the world-status RPCs). 0 or 1 disables sampling.
	// Errors, warnings and writes are never sampled.
	LogSampleReads int `envconfig:"LOG_SAMPLE_READS" default:"10"`

	JWKS_URL string `envconfig:"JWKS_URL"`
	// Issuer and Audience are required because golang-jwt treats an empty
	// expected value as "skip this check": leaving either unset silently
	// accepts every token the identity provider ever minted, for any audience.
	Issuer   string `envconfig:"ISSUER" required:"true"`
	Audience string `envconfig:"AUDIENCE" required:"true"`

	GrpcPort     int `envconfig:"GRPC_PORT" default:"50051"`
	GateAwayPort int `envconfig:"GATEAWAY_PORT" default:"6969"`
	// MetricsPort serves the Prometheus scrape endpoint. It is NOT routed
	// through the gateway or Traefik and carries no auth, so the listener must
	// never sit on a public interface.
	MetricsPort int `envconfig:"METRICS_PORT" default:"6970"`
	// MetricsBindAddr is the interface that endpoint binds. The loopback default
	// fits a local run. A container deployment sets 0.0.0.0: a listener on the
	// container's own loopback is unreachable from the Prometheus container,
	// which scrapes over the private overlay network; the port stays
	// unpublished, so all-interfaces inside the container is still not public.
	MetricsBindAddr string `envconfig:"METRICS_BIND_ADDR" default:"127.0.0.1"`

	MongoUrlFile string `envconfig:"MONGO_URL_FILE" required:"true"`
	// MongoSlowMS is the command duration above which the driver monitor logs
	// mongo.command.slow. Failed commands are always logged.
	MongoSlowMS int64 `envconfig:"MONGO_SLOW_MS" default:"250"`

	CdnUrl string `envconfig:"CDN_URL"`

	// CorsAllowedOrigins lists the exact browser origins allowed to make
	// credentialed cross-origin requests. A single leading `*.` wildcard label
	// is supported (rs/cors matches it as prefix+suffix), so
	// `https://*.lasthearth.ru` is safe but `http://localhost*` is not: that
	// form matches any origin merely STARTING with the string, including
	// `http://localhost.attacker.com`.
	CorsAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"https://lasthearth.ru,https://*.lasthearth.ru"`

	// CorsDevOrigins is appended to CorsAllowedOrigins when AppEnv is not
	// "prod", so local frontends work without loosening the deployed list.
	CorsDevOrigins []string `envconfig:"CORS_DEV_ORIGINS" default:"http://localhost:3000,http://localhost:5173,http://127.0.0.1:3000,http://127.0.0.1:5173"`

	// MediaAllowedHosts lists external hosts (besides the CDN) that image URLs
	// may point to, e.g. i.imgur.com.
	MediaAllowedHosts []string `envconfig:"MEDIA_ALLOWED_HOSTS"`

	SsoUrl       string   `envconfig:"SSO_URL"`
	ClientID     string   `envconfig:"CLIENT_ID"`
	ClientSecret string   `envconfig:"CLIENT_SECRET"`
	TokenUrl     string   `envconfig:"TOKEN_URL"`
	Resource     string   `envconfig:"RESOURCE"`
	Scopes       []string `envconfig:"SCOPES"`

	MinioEndpoint  string `envconfig:"MINIO_ENDPOINT"`
	MinioAccessKey string `envconfig:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `envconfig:"MINIO_SECRET_KEY"`
	MinioUseSSL    bool   `envconfig:"MINIO_USE_SSL"`

	NatsUrl string `envconfig:"NATS_URL"`
	// NatsMaxDeliverLog is the delivery attempt above which a redelivery is
	// logged at Warn instead of Error.
	NatsMaxDeliverLog uint64 `envconfig:"NATS_MAX_DELIVER_LOG" default:"3"`

	// HTTPClientLog enables the outbound HTTP client hooks (request, response,
	// final failure after retries).
	HTTPClientLog bool `envconfig:"HTTP_CLIENT_LOG" default:"true"`

	DiscordBotToken        string   `envconfig:"DISCORD_BOT_TOKEN" required:"true"`
	DiscordBaseURL         string   `envconfig:"DISCORD_BASE_URL" default:"https://discord.com/api/v10"`
	DiscordNewsWebhook     string   `envconfig:"DISCORD_NEWS_WEBHOOK_URL"`
	DiscordAllowedChannels []string `envconfig:"DISCORD_ALLOWED_CHANNELS" required:"true"`

	LogtoWebhookSecret string `envconfig:"LOGTO_WEBHOOK_SECRET"`

	TelegramToken string `envconfig:"TELEGRAM_TOKEN"`
	GroupId       string `envconfig:"GROUP_ID"`

	// ReferralCoinsReward is the number of donate-coins awarded to a referrer
	// when someone uses their referral code.
	ReferralCoinsReward int64 `envconfig:"REFERRAL_COINS_REWARD" default:"100"`
	// ReferralRefereeCoinsReward is the number of donate-coins awarded to the
	// player who applied a referral code (the referee).
	ReferralRefereeCoinsReward int64 `envconfig:"REFERRAL_REFEREE_COINS_REWARD" default:"50"`

	// ModsArchiveDir is the folder the "download everything" mods archive is
	// built in; empty means <os temp dir>/vsservice-mods. The service keeps at
	// most one finished archive there and sweeps its own files on start.
	ModsArchiveDir string `envconfig:"MODS_ARCHIVE_DIR"`
	// ModsArchiveMaxBytes caps that archive (1 GiB by default); a larger set of
	// required mods is not packed and the site hides the button. 0 — no cap.
	ModsArchiveMaxBytes int64 `envconfig:"MODS_ARCHIVE_MAX_BYTES" default:"1073741824"`
	// ModsBucket is where the game server uploads the mod files and the
	// manifest. It is created public-read at startup, because the CDN serves
	// those files straight to players.
	ModsBucket string `envconfig:"MODS_BUCKET" default:"server-mods"`
	// ModsManifestKey is the object the game server writes the mod list to.
	ModsManifestKey string `envconfig:"MODS_MANIFEST_KEY" default:"manifest.json"`
	// ModsSyncInterval is how often the manifest is read again, and how old the
	// lazy refresh in ListMods lets the cached list get.
	ModsSyncInterval time.Duration `envconfig:"MODS_SYNC_INTERVAL" default:"1m"`
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

	// envconfig's required tag only rejects an ABSENT variable — `ISSUER=` is
	// "present and empty" and passes it. golang-jwt reads an empty expected
	// issuer or audience as "skip this check", so an empty value here disables
	// JWT validation instead of tightening it. Fail closed at startup.
	if cfg.Issuer == "" {
		return cfg, errors.New("ISSUER must be a non-empty value: an empty issuer disables JWT issuer validation")
	}
	if cfg.Audience == "" {
		return cfg, errors.New("AUDIENCE must be a non-empty value: an empty audience disables JWT audience validation")
	}

	return cfg, nil
}
