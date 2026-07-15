// Command seed_dev creates a ready-to-use account in the local development
// database so the app can be driven end to end without going through
// registration — which would send a real verification email through the
// production Resend account.
//
// The account it writes is already past every gate the log book sits
// behind: email verified, role Member, terms accepted, profile complete
// and health-data consent given.
//
// Usage (from the repo root, after scripts/setup-dev-db.ps1):
//
//	go run ./cmd/seed_dev
//	go run ./cmd/seed_dev -treatment CAPD
//	go run ./cmd/seed_dev -email admin@pdlife.test -admin
//
// Re-running updates the existing account rather than failing, so it
// doubles as a password reset when a dev forgets which one they used.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/handler"
	"github.com/atiroop/pdlife/internal/models"
)

func main() {
	email := flag.String("email", "dev@pdlife.test", "email address for the seeded account")
	password := flag.String("password", "devpassword123", "password for the seeded account")
	nickname := flag.String("nickname", "คุณทดสอบ", "nickname for the seeded account")
	treatment := flag.String("treatment", "APD", "treatment type: APD, CAPD or HD")
	admin := flag.Bool("admin", false, "give the account the Admin role")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("seed_dev: %v", err)
	}

	// This tool mints a login that bypasses email verification. It must
	// never be able to do that anywhere but a local throwaway database, so
	// the host is checked rather than trusted — same guard as
	// scripts/setup-dev-db.ps1.
	if cfg.DBHost != "localhost" && cfg.DBHost != "127.0.0.1" {
		log.Fatalf("seed_dev: DB_HOST is %q, not localhost — refusing to seed an account into a non-local database", cfg.DBHost)
	}

	tt := models.TreatmentType(strings.ToUpper(*treatment))
	switch tt {
	case models.TreatmentAPD, models.TreatmentCAPD, models.TreatmentHD:
	default:
		log.Fatalf("seed_dev: -treatment must be APD, CAPD or HD, got %q", *treatment)
	}

	if err := auth.ValidatePasswordStrength(*password); err != nil {
		log.Fatalf("seed_dev: -password is not accepted by the app's own rules: %v", err)
	}

	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("seed_dev: %v", err)
	}

	if err := seed(db, *email, *password, *nickname, tt, *admin); err != nil {
		log.Fatalf("seed_dev: %v", err)
	}

	role := "Member"
	if *admin {
		role = "Admin"
	}
	fmt.Println()
	fmt.Println("  Seeded account ready:")
	fmt.Printf("    email:     %s\n", *email)
	fmt.Printf("    password:  %s\n", *password)
	fmt.Printf("    role:      %s\n", role)
	fmt.Printf("    treatment: %s\n", tt)
	fmt.Println()
	fmt.Println("  Log in at http://localhost:8085/login")
}

func seed(db *gorm.DB, email, password, nickname string, tt models.TreatmentType, admin bool) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	stamp, err := auth.NewRandomToken()
	if err != nil {
		return fmt.Errorf("generate security stamp: %w", err)
	}

	now := time.Now()
	termsVersion := handler.LegalContentUpdatedDate
	consentVersion := handler.HealthDataConsentVersion

	role := models.RoleMember
	if admin {
		role = models.RoleAdmin
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		err := tx.Where("email = ?", email).First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			user = models.User{Email: email}
		case err != nil:
			return fmt.Errorf("look up user: %w", err)
		}

		user.PasswordHash = hash
		user.SecurityStamp = stamp
		user.Nickname = nickname
		user.Role = role
		user.IsActive = true
		user.EmailVerifiedAt = &now
		user.TermsAcceptedAt = &now
		user.TermsAcceptedVersion = &termsVersion
		// Clear the two states that block login, so re-running the tool
		// recovers an account that was suspended or deletion-flagged while
		// testing those flows.
		user.AccountDeletionRequestedAt = nil
		user.SuspendedAt = nil
		user.SuspendedReason = nil

		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("save user: %w", err)
		}

		var profile models.PatientProfile
		err = tx.Where("user_id = ?", user.ID).First(&profile).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			profile = models.PatientProfile{UserID: user.ID}
		case err != nil:
			return fmt.Errorf("look up patient profile: %w", err)
		}

		hospital := "โรงพยาบาลทดสอบ"
		coverage := models.CoverageGoldCard
		profile.TreatmentType = &tt
		profile.HospitalName = &hospital
		profile.CoverageType = &coverage
		profile.ProfileCompletedAt = &now
		profile.HealthDataConsentAt = &now
		profile.HealthDataConsentVersion = &consentVersion

		if err := tx.Save(&profile).Error; err != nil {
			return fmt.Errorf("save patient profile: %w", err)
		}
		return nil
	})
}
