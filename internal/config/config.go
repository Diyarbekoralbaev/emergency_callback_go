package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL      string
	DBPoolMaxConns   int32
	DBPoolMinConns   int32
	HTTPAddr         string
	SiteDomain       string
	SessionSecret    []byte
	CSRFKey          []byte
	AMI              AMIConfig
	Eskiz            EskizConfig
	RiverMaxWorkers  int
}

type AMIConfig struct {
	Host             string
	Port             int
	Username         string
	Secret           string
	CallerID         string
	OperatorQueue    string
	CallTimeout      time.Duration
	RatingRetryLimit int
	RatingTimeout    time.Duration
}

type EskizConfig struct {
	Email    string
	Password string
	BaseURL  string
	DryRun   bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:     mustEnv("DATABASE_URL"),
		DBPoolMaxConns:  int32(envInt("DB_POOL_MAX_CONNS", 100)),
		DBPoolMinConns:  int32(envInt("DB_POOL_MIN_CONNS", 10)),
		HTTPAddr:        envStr("HTTP_ADDR", ":8000"),
		SiteDomain:      envStr("SITE_DOMAIN", "http://localhost:8000"),
		SessionSecret:   padKey(mustEnv("SESSION_SECRET"), 32),
		CSRFKey:         padKey(mustEnv("CSRF_KEY"), 32),
		RiverMaxWorkers: envInt("RIVER_MAX_WORKERS", 5),
		AMI: AMIConfig{
			Host:             mustEnv("AMI_HOST"),
			Port:             envInt("AMI_PORT", 5038),
			Username:         mustEnv("AMI_USERNAME"),
			Secret:           mustEnv("AMI_SECRET"),
			CallerID:         envStr("AMI_CALLER_ID", `"Ambulance" <103>`),
			OperatorQueue:    envStr("AMI_OPERATOR_QUEUE", "777"),
			CallTimeout:      time.Duration(envInt("AMI_CALL_TIMEOUT", 60)) * time.Second,
			RatingRetryLimit: envInt("AMI_RATING_RETRY_LIMIT", 3),
			RatingTimeout:    time.Duration(envInt("AMI_RATING_TIMEOUT", 10)) * time.Second,
		},
		Eskiz: EskizConfig{
			Email:    envStr("ESKIZ_EMAIL", ""),
			Password: envStr("ESKIZ_PASSWORD", ""),
			BaseURL:  envStr("ESKIZ_BASE_URL", "https://notify.eskiz.uz/api"),
			DryRun:   envStr("ESKIZ_DRY_RUN", "false") == "true",
		},
	}

	return cfg, nil
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", k))
	}
	return v
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func padKey(s string, n int) []byte {
	b := []byte(s)
	if len(b) >= n {
		return b[:n]
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}
