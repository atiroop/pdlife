package config

import (
	"strings"
	"testing"
)

// validConfig is the minimum that must pass, mirroring what production's
// .env actually sets. Anything a test wants to reject, it breaks from here.
func validConfig() *Config {
	return &Config{
		DBUser:     "pdlife_pdlife-db-admin",
		DBPassword: "hunter2",
		DBName:     "pdlife_pdlife-db",
		JWTSecret:  strings.Repeat("k", MinJWTSecretLen),
	}
}

func TestValidateAcceptsCompleteConfig(t *testing.T) {
	if err := validConfig().validate(); err != nil {
		t.Fatalf("validate() rejected a complete config: %v", err)
	}
}

func TestValidateRejectsEmptyRequiredVars(t *testing.T) {
	cases := map[string]func(*Config){
		"DB_USER":     func(c *Config) { c.DBUser = "" },
		"DB_PASSWORD": func(c *Config) { c.DBPassword = "" },
		"DB_NAME":     func(c *Config) { c.DBName = "" },
		"JWT_SECRET":  func(c *Config) { c.JWTSecret = "" },
	}

	for name, blank := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			blank(cfg)

			err := cfg.validate()
			if err == nil {
				t.Fatalf("validate() accepted a config with %s empty", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error should name the missing var %q, got: %v", name, err)
			}
		})
	}
}

// A whitespace-only value is the shape a hand-edited .env produces, and
// it is just as unusable as an empty one.
func TestValidateRejectsWhitespaceOnlyRequiredVar(t *testing.T) {
	cfg := validConfig()
	cfg.DBPassword = "   "

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() accepted a whitespace-only DB_PASSWORD")
	}
}

func TestValidateReportsEveryMissingVarAtOnce(t *testing.T) {
	cfg := validConfig()
	cfg.DBUser = ""
	cfg.JWTSecret = ""

	err := cfg.validate()
	if err == nil {
		t.Fatal("validate() accepted a config missing two required vars")
	}
	for _, want := range []string{"DB_USER", "JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list %q so one run fixes both, got: %v", want, err)
		}
	}
}

// The empty-secret case is the one that matters: getEnv's fallback used to
// leave JWTSecret == "", which signs every session token with an empty key
// and lets anyone forge a session for any patient.
func TestValidateRejectsShortJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecret = strings.Repeat("k", MinJWTSecretLen-1)

	err := cfg.validate()
	if err == nil {
		t.Fatalf("validate() accepted a %d-character JWT_SECRET", MinJWTSecretLen-1)
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error should name JWT_SECRET, got: %v", err)
	}
}

// Production's real secret is a 43-character base64 encoding of 32 random
// bytes; guard against a bound that would reject it.
func TestValidateAcceptsBase64EncodedThirtyTwoByteSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecret = strings.Repeat("k", 43)

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() rejected a 43-character secret: %v", err)
	}
}

// APP_BASE_URL is unset in production and relies on Load's default, so it
// must not be treated as required.
func TestValidateDoesNotRequireDefaultedVars(t *testing.T) {
	cfg := validConfig()
	cfg.AppBaseURL = ""
	cfg.SMTPPassword = ""

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() required a var that has a safe default: %v", err)
	}
}
