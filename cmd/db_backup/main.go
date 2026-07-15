// Command db_backup is a standalone, cron-run tool (NOT an HTTP endpoint,
// NOT wired into the deployed web binary — same category as
// cmd/purge_deleted_accounts and cmd/foodcheck_diffcheck). It runs once a
// day and:
//
//  1. mysqldumps the full pdlife database and gzip-compresses it in
//     memory (no dependency on the system `gzip` binary).
//  2. Writes the result to /root/pdlife_backups/pdlife_backup_{YYYYMMDD}.sql.gz
//     — the same directory used for manual backups during the APD/Food
//     Check migration (backup_before_*.sql files already there); this tool
//     only ever touches its own pdlife_backup_*.sql.gz files, never those.
//  3. Uploads the same bytes to R2 under pdlife/backups/, as an offsite
//     copy independent of the VPS. This MUST go to a private, non-CDN-
//     fronted bucket (see r2store.BackupConfigFromEnv) — the bucket used
//     elsewhere in this codebase for news/article images is served
//     publicly via cdn.pdlife.app, and a full DB dump (password hashes,
//     patient health data) must never be reachable there.
//  4. Deletes local pdlife_backup_*.sql.gz files older than 30 days.
//  5. Deletes R2 objects under pdlife/backups/ older than 90 days.
//  6. Logs every step's outcome (stdout — cron redirects this to a file).
//  7. On ANY failure in steps 1-3 (the actual backup), emails
//     admin@pdlife.app immediately via the existing Resend SMTP mailer —
//     a backup that fails silently is more dangerous than no backup at
//     all, since it looks safe while not being one.
//
// Retention pruning (steps 4-5) failing does NOT trigger an alert email —
// only logged as a warning — since disk/R2 space slowly growing from a
// pruning hiccup is far less dangerous than an actually-missing backup,
// and conflating the two would train admin@pdlife.app to ignore alerts.
//
// # Running
//
//	go run ./cmd/db_backup
//
// Cron on the VPS (nightly at 3am, lowest-traffic hour):
//
//	0 3 * * *  cd /home/pdlife/web/pdlife.app/public_html && ./db_backup >> /var/log/pdlife/db_backup.log 2>&1
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/r2store"
)

const (
	backupDir      = "/root/pdlife_backups"
	r2Prefix       = "pdlife/backups/"
	filenamePrefix = "pdlife_backup_"
	filenameSuffix = ".sql.gz"

	localRetention = 30 * 24 * time.Hour
	r2Retention    = 90 * 24 * time.Hour
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("db_backup: %v", err)
	}
	adminEmail := getEnvOr("ADMIN_ALERT_EMAIL", "admin@pdlife.app")

	m, err := mailer.New(cfg)
	if err != nil {
		log.Fatalf("db_backup: mailer init failed: %v", err)
	}

	if err := run(cfg); err != nil {
		log.Printf("db_backup: FAILED: %v", err)
		alertErr := m.SendBackupFailureAlert(adminEmail, mailer.BackupFailureAlertData{
			Step:    stepOf(err),
			Error:   err.Error(),
			RanAt:   time.Now().Format("2 Jan 2006 15:04:05"),
			LogHint: "/var/log/pdlife/db_backup.log",
		})
		if alertErr != nil {
			log.Printf("db_backup: ALSO FAILED to send failure alert email: %v", alertErr)
		} else {
			log.Printf("db_backup: failure alert email sent to %s", adminEmail)
		}
		os.Exit(1)
	}
	log.Println("db_backup: DONE")
}

func run(cfg *config.Config) error {
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return wrapStep("mkdir backup dir", err)
	}

	filename := filenamePrefix + time.Now().Format("20060102") + filenameSuffix
	localPath := filepath.Join(backupDir, filename)

	gz, err := dumpAndCompress(cfg)
	if err != nil {
		return wrapStep("mysqldump", err)
	}
	log.Printf("db_backup: dump complete, compressed size=%d bytes", len(gz))

	if err := os.WriteFile(localPath, gz, 0600); err != nil {
		return wrapStep("write local file", err)
	}
	log.Printf("db_backup: wrote local backup %s (%d bytes)", localPath, len(gz))

	r2Client, err := r2store.New(r2store.BackupConfigFromEnv())
	if err != nil {
		return wrapStep("r2 client init", err)
	}
	ctx := context.Background()
	r2Key := r2Prefix + filename
	if _, err := r2Client.Upload(ctx, r2Key, gz, "application/gzip"); err != nil {
		return wrapStep("r2 upload", err)
	}
	log.Printf("db_backup: uploaded to R2 key=%s (%d bytes)", r2Key, len(gz))

	if err := pruneLocal(backupDir); err != nil {
		log.Printf("db_backup: WARNING: local retention pruning failed (today's backup itself succeeded): %v", err)
	}
	if err := pruneR2(ctx, r2Client); err != nil {
		log.Printf("db_backup: WARNING: R2 retention pruning failed (today's backup itself succeeded): %v", err)
	}

	return nil
}

// dumpAndCompress shells out to mysqldump and gzips its stdout in memory.
// Credentials are passed via MYSQL_PWD (not -p on the command line) so
// they never show up in `ps` output. --no-defaults is required because
// this runs as root via cron, and root's own /root/.my.cnf on this VPS
// carries a [client] password= for root's unrelated MySQL account —
// without --no-defaults that option-file password silently overrides
// MYSQL_PWD and mysqldump authenticates (and fails) as the wrong
// password entirely (confirmed 2026-07-12: MYSQL_PWD alone was clobbered
// by ~/.my.cnf, producing a misleading "Access denied" error).
// mysqldump's "Deprecated program name" stderr warning (it's a MariaDB
// mariadb-dump symlink on this VPS) is benign and deliberately not
// treated as failure — only cmd.Wait's exit status is.
func dumpAndCompress(cfg *config.Config) ([]byte, error) {
	cmd := exec.Command("mysqldump",
		"--no-defaults",
		"-h", cfg.DBHost,
		"-P", cfg.DBPort,
		"-u", cfg.DBUser,
		"--single-transaction",
		"--routines",
		"--triggers",
		cfg.DBName,
	)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mysqldump: %w", err)
	}
	if _, err := io.Copy(gzWriter, stdout); err != nil {
		_ = cmd.Wait()
		return nil, fmt.Errorf("compress dump output: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("mysqldump exited with error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	if gzBuf.Len() == 0 {
		return nil, fmt.Errorf("mysqldump produced empty output")
	}
	return gzBuf.Bytes(), nil
}

// pruneLocal deletes this tool's own backups older than localRetention.
// It only ever touches files matching pdlife_backup_*.sql.gz — the old
// manual backup_before_*.sql files and stray migration .sql files already
// in backupDir are never even considered.
func pruneLocal(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read backup dir: %w", err)
	}
	cutoff := time.Now().Add(-localRetention)
	var deleted, kept int
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), filenamePrefix) || !strings.HasSuffix(e.Name(), filenameSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			log.Printf("db_backup: WARNING: stat %s failed: %v", e.Name(), err)
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, e.Name())
			if err := os.Remove(path); err != nil {
				log.Printf("db_backup: WARNING: deleting old local backup %s failed: %v", path, err)
				continue
			}
			log.Printf("db_backup: deleted old local backup %s (modified %s)", path, info.ModTime().Format("2006-01-02"))
			deleted++
		} else {
			kept++
		}
	}
	log.Printf("db_backup: local retention: deleted=%d kept=%d (cutoff=%s)", deleted, kept, cutoff.Format("2006-01-02"))
	return nil
}

// pruneR2 deletes objects under r2Prefix older than r2Retention (kept
// longer than local, per spec, so an offsite copy still exists for a
// while even after the local one has rotated out).
func pruneR2(ctx context.Context, client *r2store.Client) error {
	objects, err := client.ListObjects(ctx, r2Prefix)
	if err != nil {
		return fmt.Errorf("list R2 objects: %w", err)
	}
	cutoff := time.Now().Add(-r2Retention)
	var deleted, kept int
	for _, obj := range objects {
		if obj.LastModified.Before(cutoff) {
			if err := client.Delete(ctx, obj.Key); err != nil {
				log.Printf("db_backup: WARNING: deleting old R2 backup %s failed: %v", obj.Key, err)
				continue
			}
			log.Printf("db_backup: deleted old R2 backup %s (modified %s)", obj.Key, obj.LastModified.Format("2006-01-02"))
			deleted++
		} else {
			kept++
		}
	}
	log.Printf("db_backup: R2 retention: deleted=%d kept=%d (cutoff=%s)", deleted, kept, cutoff.Format("2006-01-02"))
	return nil
}

// stepError names which step of run() failed, so the failure alert email
// can say e.g. "mysqldump" or "r2 upload" instead of just a raw error.
type stepError struct {
	step string
	err  error
}

func (e *stepError) Error() string { return e.err.Error() }
func (e *stepError) Unwrap() error { return e.err }

func wrapStep(step string, err error) error {
	if err == nil {
		return nil
	}
	return &stepError{step: step, err: err}
}

func stepOf(err error) string {
	var se *stepError
	if errors.As(err, &se) {
		return se.step
	}
	return "unknown"
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
