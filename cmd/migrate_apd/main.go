// Command migrate_apd is a one-off, run-once-by-hand tool. It is NOT wired
// to any HTTP route and is not part of the deployed web binary (deploy.sh
// only ships ./bin/pdlife). It:
//
//  1. Creates the admin@pdlife.app account (role=Admin, pre-verified) +
//     an APD patient_profiles record.
//  2. Copies every APDPrescription / APDDailyLog row belonging to
//     admin@jocky.website in the legacy jocky_cms database (read-only —
//     nothing there is ever modified or deleted) into the new
//     apd_prescriptions / apd_log_entries tables, preserving original
//     timestamps.
//
// Run once, from the production server, after migrations/20260708_create_apd_log_book.sql
// has been applied:
//
//	SEED_ADMIN_PASSWORD='...' \
//	JOCKY_DB_HOST=localhost JOCKY_DB_PORT=3306 JOCKY_DB_NAME=jocky_cms \
//	JOCKY_DB_USER=jocky_cms_user JOCKY_DB_PASSWORD='...' \
//	go run ./cmd/migrate_apd
//
// The admin password is read only from SEED_ADMIN_PASSWORD at runtime,
// hashed with bcrypt immediately, and never logged or persisted anywhere
// in plaintext. Re-running after a successful migration is refused (the
// tool checks for an existing admin@pdlife.app account first).
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/models"
)

const sourceUserEmail = "admin@jocky.website"

func main() {
	adminEmail := getEnvOr("SEED_ADMIN_EMAIL", "admin@pdlife.app")
	adminPassword := os.Getenv("SEED_ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Fatal("SEED_ADMIN_PASSWORD is required (export it before running, never hardcode it)")
	}
	if err := auth.ValidatePasswordStrength(adminPassword); err != nil {
		log.Fatalf("SEED_ADMIN_PASSWORD does not meet the password policy: %v", err)
	}

	sourceDSN, err := sourceDSNFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("migrate_apd: %v", err)
	}
	dest, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("destination (pdlife) DB connection failed: %v", err)
	}

	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		log.Fatalf("source (jocky_cms) DB connection failed: %v", err)
	}
	defer source.Close()
	if err := source.Ping(); err != nil {
		log.Fatalf("source (jocky_cms) DB ping failed: %v", err)
	}

	var existing int64
	if err := dest.Model(&models.User{}).Where("email = ?", adminEmail).Count(&existing).Error; err != nil {
		log.Fatalf("checking for existing admin account failed: %v", err)
	}
	if existing > 0 {
		log.Fatalf("refusing to run: a user with email %q already exists in pdlife — this migration has already run", adminEmail)
	}

	sourceUserID, err := lookupSourceUserID(source, sourceUserEmail)
	if err != nil {
		log.Fatalf("looking up source user %q failed: %v", sourceUserEmail, err)
	}
	log.Printf("source user %q found (id=%d)", sourceUserEmail, sourceUserID)

	passwordHash, err := auth.HashPassword(adminPassword)
	if err != nil {
		log.Fatalf("hashing admin password failed: %v", err)
	}

	prescriptions, err := fetchSourcePrescriptions(source, sourceUserID)
	if err != nil {
		log.Fatalf("reading source prescriptions failed: %v", err)
	}
	dailyLogs, err := fetchSourceDailyLogs(source, sourceUserID)
	if err != nil {
		log.Fatalf("reading source daily logs failed: %v", err)
	}
	log.Printf("read %d prescription(s) and %d daily log(s) from jocky_cms", len(prescriptions), len(dailyLogs))

	var migratedPrescriptions, migratedLogs, failedLogs int

	err = dest.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		user := models.User{
			Email:           adminEmail,
			PasswordHash:    passwordHash,
			Nickname:        "ผู้ดูแลระบบ",
			Role:            models.RoleAdmin,
			IsActive:        true,
			EmailVerifiedAt: &now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating admin user: %w", err)
		}

		treatment := models.TreatmentAPD
		profile := models.PatientProfile{
			UserID:             user.ID,
			TreatmentType:      &treatment,
			ProfileCompletedAt: &now,
		}
		if err := tx.Create(&profile).Error; err != nil {
			return fmt.Errorf("creating patient profile: %w", err)
		}

		prescriptionIDMap := make(map[int64]uint64, len(prescriptions))
		for _, p := range prescriptions {
			row := models.ApdPrescription{
				PatientProfileID:   profile.ID,
				Name:               p.Name,
				SolutionBag1:       p.SolutionBag1,
				SolutionBag2:       p.SolutionBag2,
				TotalVolumeML:      p.TotalVolumeMl,
				TherapyTimeMinutes: p.TherapyTimeMinutes,
				FillVolumeML:       p.FillVolumeMl,
				Cycles:             p.Cycles,
				DwellTimeMinutes:   p.DwellTimeMinutes,
				LastFillML:         p.LastFillMl,
				ManualExchange:     p.ManualExchange,
				IsDefaultProfile:   p.IsDefaultProfile,
				CreatedAt:          p.CreatedAt,
				UpdatedAt:          p.UpdatedAt,
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("creating prescription (source id=%d): %w", p.ID, err)
			}
			prescriptionIDMap[p.ID] = row.ID
			migratedPrescriptions++
		}

		for _, l := range dailyLogs {
			var prescriptionID *uint64
			if l.PrescriptionID != nil {
				if newID, ok := prescriptionIDMap[*l.PrescriptionID]; ok {
					prescriptionID = &newID
				} else {
					log.Printf("WARN: daily log (source id=%d, date=%s) referenced unknown prescription id=%d — leaving prescription_id NULL",
						l.ID, l.Date.Format("2006-01-02"), *l.PrescriptionID)
				}
			}
			row := models.ApdLogEntry{
				PatientProfileID:   profile.ID,
				EntryDate:          l.Date,
				TreatmentStartTime: l.TreatmentStartTime,
				WeightKG:           l.WeightKg,
				BPSystolic:         l.SystolicBp,
				BPDiastolic:        l.DiastolicBp,
				Pulse:              l.Pulse,
				BloodGlucoseMgDL:   l.BloodGlucoseMgDl,
				IDrainVolumeML:     l.IDrainVolumeMl,
				TotalUFML:          l.TotalUfMl,
				UrineAvgDayML:      l.UrineAvgDayMl,
				DrainageAppearance: l.DrainageAppearance,
				Remark:             l.Remark,
				PrescriptionID:     prescriptionID,
				CreatedAt:          l.CreatedAt,
				UpdatedAt:          l.UpdatedAt,
			}
			if err := tx.Create(&row).Error; err != nil {
				failedLogs++
				log.Printf("FAILED: daily log (source id=%d, date=%s): %v", l.ID, l.Date.Format("2006-01-02"), err)
				continue
			}
			migratedLogs++
		}

		return nil
	})
	if err != nil {
		log.Fatalf("migration transaction rolled back: %v", err)
	}

	log.Printf("DONE — admin account %q created; prescriptions migrated: %d/%d; daily logs migrated: %d/%d (failed: %d)",
		adminEmail, migratedPrescriptions, len(prescriptions), migratedLogs, len(dailyLogs), failedLogs)
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func sourceDSNFromEnv() (string, error) {
	host := getEnvOr("JOCKY_DB_HOST", "localhost")
	port := getEnvOr("JOCKY_DB_PORT", "3306")
	name := getEnvOr("JOCKY_DB_NAME", "jocky_cms")
	user := os.Getenv("JOCKY_DB_USER")
	password := os.Getenv("JOCKY_DB_PASSWORD")
	if user == "" || password == "" {
		return "", fmt.Errorf("JOCKY_DB_USER and JOCKY_DB_PASSWORD are required (read-only credentials for the legacy jocky_cms database)")
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC", user, password, host, port, name), nil
}

func lookupSourceUserID(db *sql.DB, email string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM User WHERE email = ?", email).Scan(&id)
	return id, err
}

type sourcePrescription struct {
	ID                 int64
	Name               string
	SolutionBag1       string
	SolutionBag2       string
	TotalVolumeMl      int
	TherapyTimeMinutes int
	FillVolumeMl       int
	Cycles             int
	DwellTimeMinutes   int
	LastFillMl         *int
	ManualExchange     *string
	IsDefaultProfile   bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func fetchSourcePrescriptions(db *sql.DB, userID int64) ([]sourcePrescription, error) {
	rows, err := db.Query(`
		SELECT id, name, solutionBag1, solutionBag2, totalVolumeMl, therapyTimeMinutes,
		       fillVolumeMl, cycles, dwellTimeMinutes, lastFillMl, manualExchange,
		       isDefaultProfile, createdAt, updatedAt
		FROM APDPrescription WHERE userId = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []sourcePrescription
	for rows.Next() {
		var p sourcePrescription
		var lastFillMl sql.NullInt64
		var manualExchange sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.SolutionBag1, &p.SolutionBag2, &p.TotalVolumeMl,
			&p.TherapyTimeMinutes, &p.FillVolumeMl, &p.Cycles, &p.DwellTimeMinutes,
			&lastFillMl, &manualExchange, &p.IsDefaultProfile, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if lastFillMl.Valid {
			v := int(lastFillMl.Int64)
			p.LastFillMl = &v
		}
		if manualExchange.Valid {
			p.ManualExchange = &manualExchange.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type sourceDailyLog struct {
	ID                 int64
	Date               time.Time
	TreatmentStartTime string
	WeightKg           float64
	SystolicBp         int
	DiastolicBp        int
	Pulse              int
	BloodGlucoseMgDl   *int
	IDrainVolumeMl     int
	TotalUfMl          int
	UrineAvgDayMl      int
	DrainageAppearance *string
	Remark             *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	PrescriptionID     *int64
}

func fetchSourceDailyLogs(db *sql.DB, userID int64) ([]sourceDailyLog, error) {
	rows, err := db.Query(`
		SELECT id, date, treatmentStartTime, weightKg, systolicBp, diastolicBp, pulse,
		       bloodGlucoseMgDl, iDrainVolumeMl, totalUfMl, urineAvgDayMl,
		       drainageAppearance, remark, createdAt, updatedAt, prescriptionId
		FROM APDDailyLog WHERE userId = ? ORDER BY date ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []sourceDailyLog
	for rows.Next() {
		var l sourceDailyLog
		var bloodGlucose sql.NullInt64
		var drainageAppearance, remark sql.NullString
		var prescriptionID sql.NullInt64
		if err := rows.Scan(&l.ID, &l.Date, &l.TreatmentStartTime, &l.WeightKg, &l.SystolicBp,
			&l.DiastolicBp, &l.Pulse, &bloodGlucose, &l.IDrainVolumeMl, &l.TotalUfMl,
			&l.UrineAvgDayMl, &drainageAppearance, &remark, &l.CreatedAt, &l.UpdatedAt, &prescriptionID); err != nil {
			return nil, err
		}
		if bloodGlucose.Valid {
			v := int(bloodGlucose.Int64)
			l.BloodGlucoseMgDl = &v
		}
		if drainageAppearance.Valid {
			l.DrainageAppearance = &drainageAppearance.String
		}
		if remark.Valid {
			l.Remark = &remark.String
		}
		if prescriptionID.Valid {
			v := prescriptionID.Int64
			l.PrescriptionID = &v
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
