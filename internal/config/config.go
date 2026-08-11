package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	CookieSecure     bool
	AudioDir         string
	// TelephonyBackend selects the calling implementation: "ami" (classic,
	// dialplan contexts on the PBX) or "ari" (Stasis, no dialplan).
	TelephonyBackend string
	// AudioMediaBaseURL enables serving prompts to Asterisk over HTTP
	// (res_http_media_cache). Usually SITE_DOMAIN. Empty = disabled.
	AudioMediaBaseURL string
	AMI              AMIConfig
	ARI              ARIConfig
	PBXSSH           PBXSSHConfig
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

type ARIConfig struct {
	URL      string // e.g. http://pbx:8088/ari
	Username string
	Password string
}

// PBXSSHConfig lets the audio-upload page auto-push replaced prompts to the
// PBX sounds dir (used when Asterisk plays local sound files).
type PBXSSHConfig struct {
	Host      string
	User      string
	Password  string
	KeyPath   string
	SoundsDir string
}

type EskizConfig struct {
	Email    string
	Password string
	BaseURL  string
	DryRun   bool
}

// Load reads the full config. Missing required vars are collected and
// returned as one error (no panic) so callers can print a usable message.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var missing []string
	req := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			missing = append(missing, k)
		}
		return v
	}

	backend := envStr("TELEPHONY_BACKEND", "ami")
	if backend != "ami" && backend != "ari" {
		return nil, fmt.Errorf("TELEPHONY_BACKEND must be ami or ari, got %q", backend)
	}
	// Backend-conditional requirements: an ARI deployment doesn't need AMI
	// credentials and vice versa.
	reqAMI, reqARI := req, envStrEmpty
	if backend == "ari" {
		reqAMI, reqARI = envStrEmpty, req
	}

	cfg := &Config{
		DatabaseURL:      req("DATABASE_URL"),
		DBPoolMaxConns:   int32(envInt("DB_POOL_MAX_CONNS", 100)),
		DBPoolMinConns:   int32(envInt("DB_POOL_MIN_CONNS", 10)),
		HTTPAddr:         envStr("HTTP_ADDR", ":8000"),
		SiteDomain:       envStr("SITE_DOMAIN", "http://localhost:8000"),
		SessionSecret:    padKey(req("SESSION_SECRET"), 32),
		CSRFKey:          padKey(req("CSRF_KEY"), 32),
		CookieSecure:     envStr("COOKIE_SECURE", "false") == "true",
		TelephonyBackend: backend,
		// Where the Asterisk WAV prompts live. On a co-located box this is the
		// Asterisk sounds dir the admin audio page writes to; Asterisk plays
		// straight from here so a replaced file takes effect on the next call.
		AudioDir:          envStr("AUDIO_DIR", "audios"),
		AudioMediaBaseURL: envStr("AUDIO_MEDIA_BASE_URL", ""),
		RiverMaxWorkers:   envInt("RIVER_MAX_WORKERS", 5),
		AMI: AMIConfig{
			Host:             reqAMI("AMI_HOST"),
			Port:             envInt("AMI_PORT", 5038),
			Username:         reqAMI("AMI_USERNAME"),
			Secret:           reqAMI("AMI_SECRET"),
			CallerID:         envStr("AMI_CALLER_ID", `"Ambulance" <103>`),
			OperatorQueue:    envStr("AMI_OPERATOR_QUEUE", "777"),
			CallTimeout:      time.Duration(envInt("AMI_CALL_TIMEOUT", 60)) * time.Second,
			RatingRetryLimit: envInt("AMI_RATING_RETRY_LIMIT", 3),
			RatingTimeout:    time.Duration(envInt("AMI_RATING_TIMEOUT", 10)) * time.Second,
		},
		ARI: ARIConfig{
			URL:      reqARI("ARI_URL"),
			Username: reqARI("ARI_USERNAME"),
			Password: reqARI("ARI_PASSWORD"),
		},
		PBXSSH: PBXSSHConfig{
			Host:      envStr("PBX_SSH_HOST", ""),
			User:      envStr("PBX_SSH_USER", ""),
			Password:  envStr("PBX_SSH_PASSWORD", ""),
			KeyPath:   envStr("PBX_SSH_KEY", ""),
			SoundsDir: envStr("PBX_SOUNDS_DIR", "/var/lib/asterisk/sounds/en"),
		},
		Eskiz: EskizConfig{
			Email:    envStr("ESKIZ_EMAIL", ""),
			Password: envStr("ESKIZ_PASSWORD", ""),
			BaseURL:  envStr("ESKIZ_BASE_URL", "https://notify.eskiz.uz/api"),
			DryRun:   envStr("ESKIZ_DRY_RUN", "false") == "true",
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("required env vars not set: %s (see .env.example)", strings.Join(missing, ", "))
	}
	return cfg, nil
}

// LoadCore reads only what DB-facing subcommands (migrate, createuser, seed)
// need — DATABASE_URL and pool sizes. No AMI/session vars required.
func LoadCore() (*Config, error) {
	_ = godotenv.Load()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("required env var DATABASE_URL is not set (see .env.example)")
	}
	return &Config{
		DatabaseURL:    url,
		DBPoolMaxConns: int32(envInt("DB_POOL_MAX_CONNS", 100)),
		DBPoolMinConns: int32(envInt("DB_POOL_MIN_CONNS", 10)),
	}, nil
}

// envStrEmpty reads an optional var (used for the inactive backend's keys).
func envStrEmpty(k string) string { return os.Getenv(k) }

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
