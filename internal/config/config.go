package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	JWTSecret string

	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	AppBaseURL  string
	LogoURL     string
	LogoURLDark string
}

// MinJWTSecretLen is the shortest JWT_SECRET we accept. HS256 keys shorter
// than the 32-byte hash output add no security over a 32-byte one but do
// make brute force cheaper, so a short secret is treated as a config error
// rather than a warning.
const MinJWTSecretLen = 32

// Load reads configuration from the environment (and .env, if present).
//
// It fails rather than returning a half-configured Config: every value in
// requiredNonEmpty below has no safe default, and getEnv's fallback would
// otherwise paper over a missing one. JWT_SECRET is the dangerous case —
// falling back to "" would HMAC-sign every session token with an empty
// key, letting anyone forge a session for any patient, and the app would
// look perfectly healthy while doing it. That silent-fallback-to-unsafe
// shape is exactly what leaked a database dump to the public CDN bucket
// in docs/incidents/2026-07-12-backup-bucket-exposure.md; refusing to
// start is the cheap way to make that class of mistake impossible.
//
// Values that do have a safe default (APP_BASE_URL, SMTP host/port/from)
// are deliberately not required — production relies on those defaults.
func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", ""),

		JWTSecret: getEnv("JWT_SECRET", ""),

		SMTPHost:     getEnv("SMTP_HOST", "smtp.resend.com"),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUser:     getEnv("SMTP_USER", "resend"),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", "pdlife.app <noreply@pdlife.app>"),

		AppBaseURL:  getEnv("APP_BASE_URL", "https://pdlife.app"),
		LogoURL:     getEnv("LOGO_URL", ""),
		LogoURLDark: getEnv("LOGO_URL_DARK", ""),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	requiredNonEmpty := []struct {
		name  string
		value string
	}{
		{"DB_USER", c.DBUser},
		{"DB_PASSWORD", c.DBPassword},
		{"DB_NAME", c.DBName},
		{"JWT_SECRET", c.JWTSecret},
	}

	var missing []string
	for _, v := range requiredNonEmpty {
		if strings.TrimSpace(v.value) == "" {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required environment variables are unset or empty: %s (see .env.example)",
			strings.Join(missing, ", "))
	}

	if len(c.JWTSecret) < MinJWTSecretLen {
		return errors.New("JWT_SECRET is too short: it must be at least " +
			fmt.Sprint(MinJWTSecretLen) + " characters (generate one with: openssl rand -base64 32)")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
