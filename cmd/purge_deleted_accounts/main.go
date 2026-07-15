// Command purge_deleted_accounts is a standalone, cron-run tool (NOT an
// HTTP endpoint, NOT wired into the deployed web binary — same category as
// cmd/migrate_apd and cmd/foodcheck_diffcheck). It hard-deletes/anonymizes
// every account whose deletion was requested (via /profile — see
// internal/handler.AccountDeletionGraceDays and
// internal/handler/profile.go's ProfileDeleteAccount) more than
// AccountDeletionGraceDays (90) days ago.
//
// # What "purge" means per table
//
//   - editorial_articles: published articles are reassigned to a
//     permanent placeholder author ("ผู้ใช้ที่ถูกลบ", find-or-created on
//     every run) rather than deleted, so public /articles/:slug links
//     never 404. Never-published drafts are deleted outright — nothing
//     public depends on them.
//   - patient_profiles + everything keyed off it (apd_log_entries,
//     apd_prescriptions, capd_log_entries, foodcheck_search_history):
//     deleted.
//   - refresh_tokens, password_reset_tokens, email_verifications: deleted.
//   - the users row itself: hard-deleted (Unscoped — users.deleted_at is a
//     GORM soft-delete column, but PDPA erasure means actually gone, not
//     just excluded from default queries).
//
// Every step runs inside one transaction per user, and every outcome
// (success or failure) is logged — this must never silently no-op.
//
// # Running
//
//	go run ./cmd/purge_deleted_accounts
//
// Cron on the VPS (nightly):
//
//	0 2 * * *  cd /home/pdlife/web/pdlife.app/public_html && ./purge_deleted_accounts >> /var/log/pdlife/purge_deleted_accounts.log 2>&1
package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/handler"
	"github.com/atiroop/pdlife/internal/models"
)

const (
	deletedUserEmail    = "deleted-user@pdlife.app"
	deletedUserNickname = "ผู้ใช้ที่ถูกลบ"
)

func main() {
	cfg := config.Load()
	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("purge_deleted_accounts: db connection failed: %v", err)
	}

	cutoff := time.Now().AddDate(0, 0, -handler.AccountDeletionGraceDays)

	var users []models.User
	if err := db.Where("account_deletion_requested_at IS NOT NULL AND account_deletion_requested_at < ?", cutoff).
		Find(&users).Error; err != nil {
		log.Fatalf("purge_deleted_accounts: query failed: %v", err)
	}

	log.Printf("purge_deleted_accounts: found %d account(s) past the %d-day grace period (cutoff=%s)",
		len(users), handler.AccountDeletionGraceDays, cutoff.Format(time.RFC3339))

	if len(users) == 0 {
		log.Println("purge_deleted_accounts: nothing to do")
		return
	}

	deletedUserID, err := ensureDeletedUserPlaceholder(db)
	if err != nil {
		log.Fatalf("purge_deleted_accounts: ensure placeholder user failed: %v", err)
	}

	var purged, failed int
	for _, u := range users {
		if err := purgeUser(db, u, deletedUserID); err != nil {
			log.Printf("purge_deleted_accounts: user_id=%d (%s): FAILED: %v", u.ID, u.Email, err)
			failed++
			continue
		}
		log.Printf("purge_deleted_accounts: user_id=%d (%s): purged successfully", u.ID, u.Email)
		purged++
	}

	log.Printf("purge_deleted_accounts: done — %d purged, %d failed", purged, failed)
}

// ensureDeletedUserPlaceholder finds or creates the permanent account that
// absorbs authorship of published editorial articles from purged users.
// PasswordHash is deliberately not a valid bcrypt hash and Role is
// Unverified — this account must never be able to log in.
func ensureDeletedUserPlaceholder(db *gorm.DB) (uint64, error) {
	var existing models.User
	err := db.Where("email = ?", deletedUserEmail).First(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	placeholder := models.User{
		Email:        deletedUserEmail,
		PasswordHash: "!purge-placeholder-never-a-valid-bcrypt-hash!",
		Nickname:     deletedUserNickname,
		Role:         models.RoleUnverified,
		IsActive:     false,
	}
	if err := db.Create(&placeholder).Error; err != nil {
		return 0, err
	}
	log.Printf("purge_deleted_accounts: created placeholder user_id=%d (%s)", placeholder.ID, deletedUserEmail)
	return placeholder.ID, nil
}

func purgeUser(db *gorm.DB, u models.User, deletedUserID uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Editorial articles first, and always before deleting the user
		// row — editorial_articles.author_id has ON DELETE CASCADE, so if
		// any row still pointed at this user when we reached the final
		// delete below, it would be silently destroyed instead of
		// reassigned/intentionally-deleted.
		if err := tx.Model(&models.EditorialArticle{}).
			Where("author_id = ? AND status = ?", u.ID, models.EditorialArticlePublished).
			Update("author_id", deletedUserID).Error; err != nil {
			return fmt.Errorf("reassign published articles: %w", err)
		}
		if err := tx.Where("author_id = ? AND status = ?", u.ID, models.EditorialArticleDraft).
			Delete(&models.EditorialArticle{}).Error; err != nil {
			return fmt.Errorf("delete draft articles: %w", err)
		}

		var profile models.PatientProfile
		perr := tx.Where("user_id = ?", u.ID).First(&profile).Error
		switch {
		case perr == nil:
			if err := tx.Where("patient_profile_id = ?", profile.ID).Delete(&models.ApdLogEntry{}).Error; err != nil {
				return fmt.Errorf("delete apd log entries: %w", err)
			}
			if err := tx.Where("patient_profile_id = ?", profile.ID).Delete(&models.ApdPrescription{}).Error; err != nil {
				return fmt.Errorf("delete apd prescriptions: %w", err)
			}
			if err := tx.Where("patient_profile_id = ?", profile.ID).Delete(&models.CapdLogEntry{}).Error; err != nil {
				return fmt.Errorf("delete capd log entries: %w", err)
			}
			if err := tx.Where("patient_profile_id = ?", profile.ID).Delete(&models.FoodCheckSearchHistory{}).Error; err != nil {
				return fmt.Errorf("delete food search history: %w", err)
			}
			if err := tx.Delete(&profile).Error; err != nil {
				return fmt.Errorf("delete patient profile: %w", err)
			}
		case !errors.Is(perr, gorm.ErrRecordNotFound):
			return fmt.Errorf("load patient profile: %w", perr)
		}

		if err := tx.Where("user_id = ?", u.ID).Delete(&models.RefreshToken{}).Error; err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&models.PasswordResetToken{}).Error; err != nil {
			return fmt.Errorf("delete password reset tokens: %w", err)
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&models.EmailVerification{}).Error; err != nil {
			return fmt.Errorf("delete email verifications: %w", err)
		}

		if err := tx.Unscoped().Delete(&models.User{}, u.ID).Error; err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		return nil
	})
}
