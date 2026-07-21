// Command uptime_check is a standalone, cron-run tool (NOT an HTTP
// endpoint, NOT wired into the deployed web binary — same category as
// cmd/db_backup and cmd/purge_deleted_accounts). It runs every 5 minutes
// and:
//
//  1. GETs https://pdlife.app/healthz with a 10-second timeout. Any
//     non-200 response or timeout/connection error counts as "down".
//  2. Persists the last known status ("up"/"down") plus when that status
//     started in a small JSON state file (stateFilePath below) so a
//     sustained outage does NOT re-send the down alert every 5 minutes —
//     the alert email only fires on the transition into "down", and a
//     second "recovered" email only fires on the transition back to "up".
//  3. Emails both events to admin@pdlife.app via the existing Resend SMTP
//     mailer.
//
// # Running
//
//	go run ./cmd/uptime_check
//
// Cron on the VPS (every 5 minutes):
//
//	*/5 * * * *  cd /home/pdlife/web/pdlife.app/public_html && ./uptime_check >> /var/log/pdlife/uptime_check.log 2>&1
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/mailer"
)

const (
	targetURL     = "https://pdlife.app/healthz"
	checkTimeout  = 10 * time.Second
	stateFilePath = "/root/pdlife_uptime_state.json"
)

type state struct {
	Status string    `json:"status"` // "up" or "down"
	Since  time.Time `json:"since"`  // when Status last changed
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("uptime_check: %v", err)
	}
	adminEmail := cfg.AdminAlertEmail

	m, err := mailer.New(cfg)
	if err != nil {
		log.Fatalf("uptime_check: mailer init failed: %v", err)
	}

	down, detail := checkHealthz()
	now := time.Now()
	prev := loadState(stateFilePath)

	switch {
	case down && prev.Status != "down":
		log.Printf("uptime_check: transition up->down: %s", detail)
		next := state{Status: "down", Since: now}
		if err := m.SendUptimeDownAlert(adminEmail, mailer.UptimeAlertData{
			URL:       targetURL,
			Detail:    detail,
			Since:     now.Format("2 Jan 2006 15:04:05"),
			CheckedAt: now.Format("2 Jan 2006 15:04:05"),
		}); err != nil {
			log.Printf("uptime_check: FAILED to send down alert email: %v", err)
		} else {
			log.Printf("uptime_check: down alert email sent to %s", adminEmail)
		}
		saveState(stateFilePath, next)

	case down:
		log.Printf("uptime_check: still down since %s: %s", prev.Since.Format(time.RFC3339), detail)

	case !down && prev.Status == "down":
		log.Printf("uptime_check: transition down->up: %s", detail)
		if err := m.SendUptimeRecoveredAlert(adminEmail, mailer.UptimeAlertData{
			URL:       targetURL,
			Detail:    detail,
			Since:     prev.Since.Format("2 Jan 2006 15:04:05"),
			CheckedAt: now.Format("2 Jan 2006 15:04:05"),
		}); err != nil {
			log.Printf("uptime_check: FAILED to send recovered alert email: %v", err)
		} else {
			log.Printf("uptime_check: recovered alert email sent to %s", adminEmail)
		}
		saveState(stateFilePath, state{Status: "up", Since: now})

	default:
		log.Printf("uptime_check: OK: %s", detail)
	}
}

// checkHealthz returns whether the target is down and a human-readable
// detail string (HTTP status, or the timeout/connection error).
func checkHealthz() (down bool, detail string) {
	client := &http.Client{Timeout: checkTimeout}
	resp, err := client.Get(targetURL)
	if err != nil {
		return true, fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// loadState reads the persisted state, defaulting to "up" (as of now) if
// the file doesn't exist yet or can't be parsed — the safest default,
// since it means a first-ever run only alerts if the target is actually
// down right now rather than assuming a fictitious prior outage.
func loadState(path string) state {
	data, err := os.ReadFile(path)
	if err != nil {
		return state{Status: "up", Since: time.Now()}
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("uptime_check: WARNING: state file unreadable (%v), assuming up", err)
		return state{Status: "up", Since: time.Now()}
	}
	return s
}

func saveState(path string, s state) {
	data, err := json.Marshal(s)
	if err != nil {
		log.Printf("uptime_check: WARNING: marshaling state failed: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("uptime_check: WARNING: writing state file failed: %v", err)
	}
}
