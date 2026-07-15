// Command news_ingest_nephrothai is a standalone, cron-run tool (same
// category as cmd/news_ingest_pubmed, cmd/migrate_apd, cmd/foodcheck_diffcheck
// — NOT an HTTP endpoint, NOT wired into the deployed web binary). It
// parses the "สาระความรู้" category RSS feed on nephrothai.org and inserts
// new posts into news_articles as status='pending' for a human to review.
//
// # Legal basis
//
// See docs/news_sources_survey.md — the project owner called the
// nephrothai.org site admin directly and got verbal permission to reuse
// content with credit always attached. That's why credit_source_name
// below is a fixed literal, not derived per-post: the permission was
// blanket, tied to that one credit line.
//
// # Why (mostly) no AI call here (unlike cmd/news_ingest_pubmed)
//
// Two reasons this command's text handling is much simpler than the
// pubmed one:
//   - The content is already Thai — nothing to translate.
//   - There's no copyright reason to avoid storing the full content (the
//     phone-call permission covers reuse), so ContentHTML is populated
//     directly from the feed's <content:encoded> and that's what
//     /admin/content-queue shows reviewers, not an AI summary. SummaryTH
//     is still populated (the column is NOT NULL) but as a plain
//     mechanical excerpt of the stripped text — no LLM call, no cost.
//
// One AI call does happen, though: internal/newsimage needs a short
// English topic phrase for the feature-image prompt, summarized from
// title_th via the same PROVIDER/FALLBACK_PROVIDER config
// cmd/news_ingest_pubmed uses — see "Required configuration" below.
//
// # The pdf/video-only problem (see docs/news_sources_survey.md)
//
// Most posts in this category turned out to be a single "ดาวน์โหลด PDF"
// button or an embedded video/infographic, not prose — confirmed by hand
// before writing this. There is no reliable structural signal for this in
// the feed (both real articles and PDF-only posts use the same Elementor
// wrapper markup), so this command uses a length heuristic instead: strip
// all HTML tags from <content:encoded>, and skip the post if the
// remaining plain text is under minContentRuneLength runes — that's
// short enough to only ever be a button caption like "ดาวน์โหลดคำแนะนำ
// สำหรับการดูแล และรักษาโรคไต" (see the real example in the survey doc),
// never an actual article body. Every run logs how many posts were
// skipped this way, so the real prose-coverage percentage of this
// category is visible over time rather than assumed.
//
// # Required configuration
//
// Same as cmd/news_ingest_pubmed (needed for the topic-summarization
// call feeding image generation, plus OPENAI_API_KEY specifically for
// the image API itself — see internal/newsimage):
//
//	PROVIDER=openai
//	FALLBACK_PROVIDER=groq
//	OPENAI_API_KEY=...
//	OPENAI_MODEL=...
//	GROQ_API_KEY=...
//	GROQ_MODEL=...          # optional, defaults to qwen/qwen3-32b
//	R2_ENDPOINT / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY / R2_BUCKET / R2_CDN_BASE
//
// Unlike the provider vars (hard requirement, this command exits if
// missing), incomplete R2 config only disables image generation for the
// run — see the r2Client nil-check in main().
//
// # Running
//
//	go run ./cmd/news_ingest_nephrothai
//
// Cron on the VPS (daily, matches docs/news_sources_survey.md's
// recommended frequency for this source):
//
//	0 5 * * *  cd /home/pdlife/web/pdlife.app/public_html && ./news_ingest_nephrothai >> /var/log/pdlife/news_ingest_nephrothai.log 2>&1
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/llmprovider"
	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/newsimage"
	"github.com/atiroop/pdlife/internal/r2store"
)

const (
	// The "สาระความรู้" (knowledge) category feed — confirmed working
	// and scoped correctly by hand, see docs/news_sources_survey.md.
	nephrothaiFeedURL = "https://www.nephrothai.org/category/%E0%B8%AA%E0%B8%B2%E0%B8%A3%E0%B8%B0%E0%B8%84%E0%B8%A7%E0%B8%B2%E0%B8%A1%E0%B8%A3%E0%B8%B9%E0%B9%89/feed/"

	minContentRuneLength = 150
	summaryExcerptRunes  = 200
	sceneInputRunes      = 1200 // more context than SummaryTH's excerpt so DescribeScene has real detail to draw a specific scene from

	newsArticleSource = "nephrothai"
	creditSourceName  = "สมาคมโรคไตแห่งประเทศไทย (nephrothai.org)"

	httpTimeout = 30 * time.Second
)

// ---- RSS parsing ----

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	// content:encoded — WordPress's full rendered post HTML.
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

var httpClient = &http.Client{Timeout: httpTimeout}

func fetchFeed(url string) (*rssFeed, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pdlife.app-news-ingest/1.0 (+https://pdlife.app)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}
	return &feed, nil
}

// ---- content heuristics ----

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`\s+`)
	postIDRe     = regexp.MustCompile(`[?&]p=(\d+)`)
)

// stripHTML reduces post HTML to plain visible text, used only to measure
// length for the pdf/video-only heuristic above.
func stripHTML(s string) string {
	stripped := htmlTagRe.ReplaceAllString(s, " ")
	stripped = html.UnescapeString(stripped)
	stripped = whitespaceRe.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(stripped)
}

// excerptSummary is a plain (non-AI) truncation — see the "why no AI
// call" note at the top of this file.
func excerptSummary(plain string, maxRunes int) string {
	r := []rune(plain)
	if len(r) <= maxRunes {
		return plain
	}
	return string(r[:maxRunes]) + "…"
}

// externalID prefers the numeric WordPress post ID out of <guid> (stable
// across URL/slug edits); falls back to a hash of the link — short and
// safely under the external_id VARCHAR(100) column even though nephrothai
// permalinks are long, percent-encoded Thai text.
func externalID(guid, link string) string {
	if m := postIDRe.FindStringSubmatch(guid); len(m) == 2 {
		return "p" + m[1]
	}
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:])[:16]
}

func parsePubDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}

// ---- main ----

func main() {
	// config.Load() loads .env into the process environment (via
	// godotenv) as a side effect — must happen before any os.Getenv call,
	// including inside llmprovider.Require below.
	cfg := config.Load()

	primary, fallback := llmprovider.Require("news_ingest_nephrothai")
	providers := llmprovider.List(primary, fallback)
	log.Printf("news_ingest_nephrothai: primary provider=%s (%s), fallback provider=%s (%s)",
		primary.Name, primary.Model, fallback.Name, fallback.Model)

	// R2/image generation is best-effort — a missing/broken config here
	// must never stop posts from being ingested (see content policy note
	// in internal/newsimage), so this only logs, never fatals.
	r2Client, r2Err := r2store.New(r2store.ConfigFromEnv())
	if r2Err != nil {
		log.Printf("news_ingest_nephrothai: R2 not configured, feature images will be skipped: %v", r2Err)
		r2Client = nil
	}
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("news_ingest_nephrothai: database connection failed: %v", err)
	}

	feed, err := fetchFeed(nephrothaiFeedURL)
	if err != nil {
		log.Fatalf("news_ingest_nephrothai: fetch feed failed: %v", err)
	}
	log.Printf("news_ingest_nephrothai: feed returned %d items", len(feed.Channel.Items))

	var inserted, skippedExisting, skippedThin, failed int
	for _, item := range feed.Channel.Items {
		extID := externalID(item.GUID, item.Link)

		var existingCount int64
		if err := db.Model(&models.NewsArticle{}).
			Where("source = ? AND external_id = ?", newsArticleSource, extID).
			Count(&existingCount).Error; err != nil {
			log.Printf("news_ingest_nephrothai: %q: duplicate check failed: %v", item.Title, err)
			failed++
			continue
		}
		if existingCount > 0 {
			skippedExisting++
			continue
		}

		plain := stripHTML(item.ContentEncoded)
		if utf8.RuneCountInString(plain) < minContentRuneLength {
			skippedThin++
			log.Printf("news_ingest_nephrothai: skipped (pdf/video-only, %d runes of text): %q", utf8.RuneCountInString(plain), item.Title)
			continue
		}

		title := strings.TrimSpace(item.Title)
		contentHTML := item.ContentEncoded

		// Feature image: best-effort, never blocks the insert below. See
		// internal/newsimage's package doc for the content-policy rules
		// this must follow.
		featureImageStatus := models.NewsArticleFeatureImageFailed
		var featureImageURL *string
		sceneInput := title + "\n\n" + excerptSummary(plain, sceneInputRunes)
		scene, sceneErr := newsimage.DescribeScene(providers, sceneInput)
		if sceneErr != nil {
			log.Printf("news_ingest_nephrothai: %q: scene description failed, image generation skipped: %v", title, sceneErr)
		} else if r2Client == nil {
			log.Printf("news_ingest_nephrothai: %q: R2 not configured, image generation skipped", title)
		} else {
			imgResult := newsimage.GenerateAndStore(context.Background(), r2Client, openAIAPIKey, scene, newsArticleSource, extID)
			featureImageURL = imgResult.URL
			featureImageStatus = imgResult.Status
		}

		article := models.NewsArticle{
			Source:             newsArticleSource,
			ExternalID:         extID,
			Title:              title,
			TitleTH:            title, // already Thai — no translation needed
			SummaryTH:          excerptSummary(plain, summaryExcerptRunes),
			ContentHTML:        &contentHTML,
			JournalName:        nil,
			PublishedAt:        parsePubDate(item.PubDate),
			CreditSourceName:   creditSourceName,
			CreditURL:          item.Link,
			FeatureImageURL:    featureImageURL,
			FeatureImageStatus: featureImageStatus,
			Status:             models.NewsArticleStatusPending,
		}
		if err := db.Create(&article).Error; err != nil {
			log.Printf("news_ingest_nephrothai: %q: insert failed: %v", title, err)
			failed++
			continue
		}
		inserted++
		log.Printf("news_ingest_nephrothai: inserted (%q, %d runes)", title, utf8.RuneCountInString(plain))
	}

	log.Printf("news_ingest_nephrothai: done — inserted=%d skipped_existing=%d skipped_pdf_video_only=%d failed=%d (feed total=%d)",
		inserted, skippedExisting, skippedThin, failed, len(feed.Channel.Items))
}
