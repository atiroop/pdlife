// Command foodcheck_diffcheck is a standalone, cron-run tool (NOT an HTTP
// endpoint, NOT wired into the deployed web binary — same category as
// cmd/migrate_apd and cmd/migrate_foodcheck). It checks once a month
// whether the two upstream sources behind Food Check (INMU Mahidol's Thai
// FCD and thaifcd.anamai.moph.go.th) have changed since the last check,
// and emails admin@pdlife.app if so. See docs/foodcheck_survey.md for the
// data pipeline this monitors and internal/foodrisk for the feature it
// backs.
//
// # What it does NOT do
//
// It never writes to foodcheck_foods, foodcheck_anamai_foods, or any
// other Food Check data table — only to foodcheck_source_snapshots (its
// own bookkeeping table, migrations/20260710_create_foodcheck_source_snapshots.sql).
// Re-importing data after a detected change is always a manual, human step
// — see the alert email this sends. main() asserts the row counts of the
// two main data tables are unchanged before/after every run as a runtime
// guard against that ever regressing.
//
// # How it checks each source
//
// Both sources' "list everything" endpoints were verified by hand against
// the live sites before writing this (2026-07-08):
//   - Anamai's /nss/search.php with an empty keyword returns all ~1,484
//     items in one response (confirmed: no server-side pagination).
//   - INMU's /thaifcd/foodsearch/food_group_result must be queried once
//     per food group (17 groups, ids from docs/foodcheck_survey.md); its
//     page_no parameter currently returns byte-identical content on every
//     page for a group that fits on one page (confirmed for group A), so
//     this tool pages through page_no per group only until a page adds no
//     new ids, rather than assuming a fixed page count.
//
// Item identity is INMU's internal numeric id / Anamai's fid — never a
// full nutrient re-fetch (item_count + a hash of the sorted id list is
// enough to detect additions/removals; this is not a re-scrape of
// scraper/fetch_nutrients.py's job).
//
// # Politeness
//
// Checks robots.txt on both domains before requesting anything (both
// returned 404 — no restrictions declared — when last checked by hand,
// but this is re-checked live every run in case that changes), identifies
// itself with a contactable User-Agent, and sleeps requestDelay between
// requests to the same host — same 1.5s floor the source scraper docs
// mandate (docs/foodcheck_survey.md 5), never reduced.
//
// # -source flag
//
// -source=inmu|anamai|all (default all) restricts which source(s) this
// run checks. Added because the production VPS cannot reach
// thaifcd.anamai.moph.go.th at all (confirmed 2026-07-08: 100% ping
// packet loss, TCP connect timeout — very likely that site blocking
// foreign/non-Thai-ISP IP ranges, INMU and general internet both reachable
// fine from the same VPS). The VPS cron therefore always runs
// -source=inmu; a real Thai residential/office IP (e.g. a developer's own
// machine, via -source=anamai) is needed to check Anamai at all until
// that connectivity gap is resolved some other way — see
// docs/foodcheck_anamai_manual_check.md for that manual workflow.
//
// # Running
//
// Manual (first run ever, per source, creates a baseline — no email is
// sent for a first-ever baseline, there's nothing to compare against):
//
//	go run ./cmd/foodcheck_diffcheck -source=all
//
// Cron on the VPS (source=inmu only — see the flag doc above for why):
//
//	0 3 1 * *  cd /home/pdlife/web/pdlife.app/public_html && ./foodcheck_diffcheck -source=inmu >> /var/log/pdlife/foodcheck_diffcheck.log 2>&1
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/models"
)

const (
	userAgent    = "pdlife.app-diffcheck/1.0 (contact: admin@pdlife.app)"
	requestDelay = 1500 * time.Millisecond
	httpTimeout  = 30 * time.Second

	inmuBaseURL       = "https://inmu.mahidol.ac.th"
	inmuGroupListPath = "/thaifcd/foodsearch/food_group_result"

	anamaiBaseURL    = "https://thaifcd.anamai.moph.go.th"
	anamaiSearchPath = "/nss/search.php"

	maxPagesPerGroup = 50 // safety cap; see doc comment above on why page_no is looped rather than assumed
)

// inmuFoodGroupIDs mirrors scraper/config.py's FOOD_GROUP_IDS from the
// source system (docs/foodcheck_survey.md) — the numeric ids the current
// version of the INMU site expects, not the A-Z letters themselves.
var inmuFoodGroupIDs = map[string]int{
	"A": 70, "B": 71, "C": 73, "D": 74, "E": 75,
	"F": 76, "G": 77, "H": 78, "J": 79, "K": 80,
	"M": 81, "N": 72, "Q": 82, "S": 83, "T": 84,
	"U": 85, "Z": 69,
}

var inmuIDPattern = regexp.MustCompile(`food_name/\?[^"]*\bid=(\d+)`)
var anamaiFidPattern = regexp.MustCompile(`fID=([0-9A-Za-z]+)`)

func main() {
	sourceFlag := flag.String("source", "all", `which source to check: "inmu", "anamai", or "all"`)
	flag.Parse()
	runINMU, runAnamai, err := parseSourceFlag(*sourceFlag)
	if err != nil {
		log.Fatalf("invalid -source: %v", err)
	}

	adminEmail := getEnvOr("ADMIN_ALERT_EMAIL", "admin@pdlife.app")

	cfg := config.Load()
	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	m, err := mailer.New(cfg)
	if err != nil {
		log.Fatalf("mailer init failed: %v", err)
	}

	client := &http.Client{Timeout: httpTimeout}

	var foodsBefore, anamaiFoodsBefore int64
	db.Model(&models.FoodCheckFood{}).Count(&foodsBefore)
	db.Model(&models.FoodCheckAnamaiFood{}).Count(&anamaiFoodsBefore)
	log.Printf("main data tables before run: foodcheck_foods=%d foodcheck_anamai_foods=%d", foodsBefore, anamaiFoodsBefore)

	if runINMU {
		runSource(db, m, client, adminEmail, sourceConfig{
			snapshotSource: models.SnapshotSourceINMU,
			displayName:    "Thai FCD (INMU มหิดล)",
			siteURL:        inmuBaseURL + "/thaifcd",
			robotsBaseURL:  inmuBaseURL,
			robotsPath:     inmuGroupListPath,
			fetch:          func() ([]string, error) { return fetchINMUIDs(client) },
		})
	} else {
		log.Println("[inmu] SKIPPED: not requested via -source")
	}

	if runAnamai {
		runSource(db, m, client, adminEmail, sourceConfig{
			snapshotSource: models.SnapshotSourceAnamai,
			displayName:    "กรมอนามัย (thaifcd.anamai.moph.go.th)",
			siteURL:        anamaiBaseURL + "/nss/search.php",
			robotsBaseURL:  anamaiBaseURL,
			robotsPath:     anamaiSearchPath,
			fetch:          func() ([]string, error) { return fetchAnamaiIDs(client) },
		})
	} else {
		log.Println("[anamai] SKIPPED: not requested via -source")
	}

	var foodsAfter, anamaiFoodsAfter int64
	db.Model(&models.FoodCheckFood{}).Count(&foodsAfter)
	db.Model(&models.FoodCheckAnamaiFood{}).Count(&anamaiFoodsAfter)
	if foodsAfter != foodsBefore || anamaiFoodsAfter != anamaiFoodsBefore {
		log.Fatalf("ASSERTION FAILED: main data tables changed during this run (foodcheck_foods %d->%d, foodcheck_anamai_foods %d->%d) — this tool must never write to those tables, investigate immediately",
			foodsBefore, foodsAfter, anamaiFoodsBefore, anamaiFoodsAfter)
	}
	log.Printf("ASSERTION OK: main data tables untouched (foodcheck_foods=%d foodcheck_anamai_foods=%d)", foodsAfter, anamaiFoodsAfter)

	log.Println("DONE")
}

type sourceConfig struct {
	snapshotSource models.FoodCheckSourceSnapshotSource
	displayName    string
	siteURL        string
	robotsBaseURL  string
	robotsPath     string
	fetch          func() ([]string, error)
}

func runSource(db *gorm.DB, m *mailer.Mailer, client *http.Client, adminEmail string, sc sourceConfig) {
	allowed, err := checkRobotsAllowed(client, sc.robotsBaseURL, sc.robotsPath)
	if err != nil {
		log.Printf("[%s] WARNING: robots.txt check failed (%v) — proceeding cautiously since absence of a robots.txt traditionally means no restriction, but investigate if this keeps happening", sc.snapshotSource, err)
	} else if !allowed {
		log.Printf("[%s] SKIPPED: robots.txt disallows %s — not fetching this source this run", sc.snapshotSource, sc.robotsPath)
		return
	}

	ids, err := sc.fetch()
	if err != nil {
		log.Printf("[%s] ERROR: fetching current data failed: %v", sc.snapshotSource, err)
		return
	}
	sort.Strings(ids)
	count := len(ids)
	hash := computeHash(ids)
	now := time.Now()

	var previous models.FoodCheckSourceSnapshot
	err = db.Where("source = ?", sc.snapshotSource).Order("checked_at DESC").First(&previous).Error
	hasPrevious := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[%s] ERROR: looking up previous snapshot failed: %v", sc.snapshotSource, err)
		return
	}

	if !hasPrevious {
		log.Printf("[%s] BASELINE: no previous snapshot — recording count=%d as the first baseline, no alert sent", sc.snapshotSource, count)
	} else if previous.ItemCount != count || previous.ContentHash != hash {
		log.Printf("[%s] DIFF: count %d -> %d (hash changed: %v) — sending alert to %s", sc.snapshotSource, previous.ItemCount, count, previous.ContentHash != hash, adminEmail)
		alertErr := m.SendFoodCheckDiffAlert(adminEmail, mailer.FoodCheckDiffAlertData{
			SourceName:        sc.displayName,
			SourceURL:         sc.siteURL,
			OldCount:          previous.ItemCount,
			NewCount:          count,
			PreviousCheckedAt: previous.CheckedAt.Format("2 Jan 2006 15:04"),
			CheckedAt:         now.Format("2 Jan 2006 15:04"),
		})
		if alertErr != nil {
			log.Printf("[%s] ERROR: sending alert email failed: %v", sc.snapshotSource, alertErr)
		} else {
			log.Printf("[%s] alert email sent successfully", sc.snapshotSource)
		}
	} else {
		log.Printf("[%s] OK: no change (count=%d)", sc.snapshotSource, count)
	}

	rawJSON, err := json.Marshal(ids)
	if err != nil {
		log.Printf("[%s] WARNING: marshaling raw snapshot failed: %v (saving snapshot without it)", sc.snapshotSource, err)
	}
	rawStr := string(rawJSON)
	snapshot := models.FoodCheckSourceSnapshot{
		Source:      sc.snapshotSource,
		ItemCount:   count,
		ContentHash: hash,
		CheckedAt:   now,
		RawSnapshot: &rawStr,
	}
	if err := db.Create(&snapshot).Error; err != nil {
		log.Printf("[%s] ERROR: saving new snapshot failed: %v", sc.snapshotSource, err)
	}
}

func computeHash(sortedIDs []string) string {
	sum := sha256.Sum256([]byte(strings.Join(sortedIDs, "\n")))
	return hex.EncodeToString(sum[:])
}

// fetchINMUIDs queries every food group once, paging within a group only
// until a page contributes no new ids (see the package doc comment for
// why — the current site version doesn't reliably paginate at all).
func fetchINMUIDs(client *http.Client) ([]string, error) {
	all := map[string]struct{}{}

	groups := make([]string, 0, len(inmuFoodGroupIDs))
	for g := range inmuFoodGroupIDs {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	for _, group := range groups {
		groupID := inmuFoodGroupIDs[group]
		seenInGroup := map[string]struct{}{}

		for page := 1; page <= maxPagesPerGroup; page++ {
			url := fmt.Sprintf("%s%s?food_group_id=%d&mode=food_group_result&page_no=%d", inmuBaseURL, inmuGroupListPath, groupID, page)
			body, err := politeGet(client, url)
			if err != nil {
				return nil, fmt.Errorf("fetching INMU group %s page %d: %w", group, page, err)
			}

			matches := inmuIDPattern.FindAllStringSubmatch(body, -1)
			newInThisPage := 0
			for _, m := range matches {
				id := m[1]
				if _, ok := seenInGroup[id]; !ok {
					seenInGroup[id] = struct{}{}
					newInThisPage++
				}
			}

			if newInThisPage == 0 {
				break // this page added nothing new — stop paging this group
			}
		}

		for id := range seenInGroup {
			all[group+":"+id] = struct{}{} // prefix with group letter: INMU ids are unique per-group in this scheme, not guaranteed globally unique across groups
		}
	}

	out := make([]string, 0, len(all))
	for id := range all {
		out = append(out, id)
	}
	return out, nil
}

// fetchAnamaiIDs makes a single request — confirmed by hand (2026-07-08)
// that an empty-keyword search returns every item in one response, no
// pagination needed.
func fetchAnamaiIDs(client *http.Client) ([]string, error) {
	url := fmt.Sprintf("%s%s?keyword=&nutrient=00&foodgroup=00", anamaiBaseURL, anamaiSearchPath)
	body, err := politeGet(client, url)
	if err != nil {
		return nil, fmt.Errorf("fetching Anamai search: %w", err)
	}

	matches := anamaiFidPattern.FindAllStringSubmatch(body, -1)
	seen := map[string]struct{}{}
	for _, m := range matches {
		seen[m[1]] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

// politeGet issues one GET with an identifying User-Agent and sleeps
// requestDelay afterward — every call site pays this delay, which is what
// keeps a 17-group INMU crawl from hammering the server (docs/foodcheck_survey.md 5
// mandates this same 1.5s floor for the original scraper; never reduced here either).
func politeGet(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	defer time.Sleep(requestDelay)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// checkRobotsAllowed fetches robots.txt fresh every run (not cached from
// any prior check) and returns whether targetPath is allowed under the
// User-agent: * group. A non-200 response (both source sites returned 404
// when checked by hand) means no robots.txt was published, which by
// convention means no restriction — this function returns (true, nil) in
// that case, matching standard robots-exclusion behavior.
func checkRobotsAllowed(client *http.Client, baseURL, targetPath string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/robots.txt", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return true, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	disallowed := parseRobotsDisallow(string(body))
	for _, prefix := range disallowed {
		if prefix != "" && strings.HasPrefix(targetPath, prefix) {
			return false, nil
		}
	}
	return true, nil
}

// parseRobotsDisallow extracts Disallow rules from the User-agent: *
// group of a robots.txt body. Deliberately minimal (no wildcard/$ support,
// no per-UA-specific groups) — sufficient to honor a real disallow rule
// without implementing the full RFC 9309 grammar for a tool that only
// ever targets two known, hand-verified endpoints.
func parseRobotsDisallow(body string) []string {
	var disallowed []string
	inWildcardGroup := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "user-agent:"):
			ua := strings.TrimSpace(line[len("User-agent:"):])
			inWildcardGroup = ua == "*"
		case inWildcardGroup && strings.HasPrefix(lower, "disallow:"):
			path := strings.TrimSpace(line[len("Disallow:"):])
			disallowed = append(disallowed, path)
		}
	}
	return disallowed
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseSourceFlag validates -source and returns which of the two sources
// to run this invocation.
func parseSourceFlag(v string) (runINMU, runAnamai bool, err error) {
	switch v {
	case "all":
		return true, true, nil
	case "inmu":
		return true, false, nil
	case "anamai":
		return false, true, nil
	default:
		return false, false, fmt.Errorf(`%q is not one of "inmu", "anamai", "all"`, v)
	}
}
