package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	BaseURL       string
	DBPath        string
	RedisURL      string
	RootEmails    []string
	SessionSecret string
	MagicLinkTTL  time.Duration
	SessionTTL    time.Duration
	CookieSecure  bool
	ResendAPIKey  string
	EmailFrom     string
	EmailTo       []string
	EmailRetries  int
}

func Load() Config {
	_ = godotenv.Load()
	return Config{
		Port:          getenv("PORT", "8080"),
		BaseURL:       must("BASE_URL"),
		DBPath:        getenv("DB_PATH", "./data/beacon.db"),
		RedisURL:      getenv("REDIS_URL", "redis://localhost:6379"),
		RootEmails:    splitCSV(must("ROOT_EMAILS")),
		SessionSecret: must("SESSION_SECRET"),
		MagicLinkTTL:  parseDur("MAGIC_LINK_TTL", 15*time.Minute),
		SessionTTL:    parseDur("SESSION_TTL", 15*24*time.Hour),
		CookieSecure:  getenv("COOKIE_SECURE", "false") == "true",
		ResendAPIKey:  must("RESEND_API_KEY"),
		EmailFrom:     must("EMAIL_FROM"),
		EmailTo:       emailRecipients(),
		EmailRetries:  parseInt("EMAIL_RETRY_ATTEMPTS", 3),
	}
}

// emailRecipients defaults to ROOT_EMAILS unless EMAIL_TO is explicitly set,
// allowing notifications to be routed to a different address than the sign-in allowlist.
func emailRecipients() []string {
	if v := os.Getenv("EMAIL_TO"); v != "" {
		return splitCSV(v)
	}
	return splitCSV(os.Getenv("ROOT_EMAILS"))
}

func (c Config) IsRootEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, e := range c.RootEmails {
		if strings.ToLower(e) == email {
			return true
		}
	}
	return false
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func must(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required env: %s", k)
	}
	return v
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDur(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func parseInt(k string, def int) int {
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
