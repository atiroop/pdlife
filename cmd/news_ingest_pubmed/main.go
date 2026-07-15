// Command news_ingest_pubmed is a standalone, cron-run tool (same category
// as cmd/migrate_apd and cmd/foodcheck_diffcheck — NOT an HTTP endpoint,
// NOT wired into the deployed web binary). It fetches new PubMed articles
// about peritoneal dialysis, asks an LLM to translate the title and write
// a short Thai summary, and inserts them into news_articles as
// status='pending' for a human to review before anything reaches patients.
//
// # Why PubMed only, for now
//
// See docs/news_sources_survey.md. PubMed/NCBI E-utilities is the only
// one of the four surveyed sources with an unambiguous green light for
// automated fetch + AI summarization — kidney.org and healio.com both
// explicitly forbid automated access and AI use of their content in their
// Terms of Use (healio.com's robots.txt even blocks Claude by name).
// nephrothai.org has verbal permission (see that doc) but its content is
// mostly PDF/video downloads rather than summarizable prose, so it isn't
// wired up here yet.
//
// # Query
//
// `"peritoneal dialysis" AND CAPD AND "Review"[Publication Type]` — the
// query the survey recommended: it cuts the ~12,000-result unfiltered
// baseline down to ~1,100 review-type articles, which read as practical
// overviews rather than single-patient case reports. This still is NOT a
// perfect filter (a "review" can still be written for nephrologists, not
// patients) — that's exactly why every row lands as status='pending'
// rather than being auto-published.
//
// # Content policy (do not weaken without re-reading the survey)
//
//   - Never store or display the source abstract verbatim — abstracts are
//     the copyright of the originating journal, not NCBI's to relicense.
//     The abstract is fetched only as ephemeral input to the LLM prompt.
//   - The LLM is instructed to translate/summarize ONLY what the abstract
//     says — no added medical advice, no embellishment.
//   - content_html is always left NULL for pubmed rows (no full-text
//     reuse permission exists for journal articles).
//   - credit_source_name + credit_url are always populated and must be
//     shown by any UI that renders these rows.
//
// # Required configuration
//
// Every one of these must be set before this runs; the program refuses
// to guess or silently skip translation if any are missing (see
// requireProviderConfig below) — DB_* vars are handled by internal/config
// as usual, the rest are specific to this command:
//
//	PROVIDER=openai              # primary LLM provider (openai|groq)
//	FALLBACK_PROVIDER=groq       # tried only if PROVIDER's call fails
//	OPENAI_API_KEY=...
//	OPENAI_MODEL=...             # e.g. gpt-5-mini
//	GROQ_API_KEY=...             # required because FALLBACK_PROVIDER=groq
//	GROQ_MODEL=...               # optional, defaults to qwen/qwen3-32b
//
// # Running
//
//	go run ./cmd/news_ingest_pubmed
//
// Cron on the VPS (daily, matches the frequency docs/news_sources_survey.md
// recommends for PubMed):
//
//	0 4 * * *  cd /home/pdlife/web/pdlife.app/public_html && ./news_ingest_pubmed >> /var/log/pdlife/news_ingest_pubmed.log 2>&1
package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/llmprovider"
	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/newsimage"
	"github.com/atiroop/pdlife/internal/r2store"
)

const (
	pubmedQuery       = `"peritoneal dialysis" AND CAPD AND "Review"[Publication Type]`
	pubmedRetMax      = 20
	eutilsRequestGap  = 400 * time.Millisecond // stays well under the 3 req/sec anonymous limit
	httpTimeout       = 30 * time.Second
	newsArticleSource = "pubmed"
)

// ---- PubMed E-utilities ----

var httpClient = &http.Client{Timeout: httpTimeout}

// eutilsIdentity returns the tool+email query params NCBI's usage
// guidelines recommend (not required at our request volume, but good
// practice — see docs/news_sources_survey.md). Derived from SMTP_FROM if
// it contains a parseable address; omitted otherwise rather than guessed.
func eutilsIdentity() string {
	params := "tool=pdlife-news-ingest"
	if addr, err := mail.ParseAddress(os.Getenv("SMTP_FROM")); err == nil && addr.Address != "" {
		params += "&email=" + addr.Address
	}
	return params
}

type esearchResponse struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

func pubmedSearch(query string, retmax int) ([]string, error) {
	url := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&term=%s&retmax=%d&sort=pub_date&retmode=json&%s",
		httpQueryEscape(query), retmax, eutilsIdentity(),
	)
	body, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("esearch: %w", err)
	}
	var parsed esearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("esearch: parse response: %w", err)
	}
	return parsed.ESearchResult.IDList, nil
}

type esummaryRecord struct {
	Title           string `json:"title"`
	FullJournalName string `json:"fulljournalname"`
	PubDate         string `json:"pubdate"`
}

func pubmedSummary(pmid string) (*esummaryRecord, error) {
	url := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi?db=pubmed&id=%s&retmode=json&%s",
		pmid, eutilsIdentity(),
	)
	body, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("esummary: %w", err)
	}
	var parsed struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("esummary: parse response: %w", err)
	}
	raw, ok := parsed.Result[pmid]
	if !ok {
		return nil, fmt.Errorf("esummary: no record for PMID %s in response", pmid)
	}
	var record esummaryRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("esummary: parse record: %w", err)
	}
	return &record, nil
}

type efetchXML struct {
	PubmedArticle []struct {
		MedlineCitation struct {
			Article struct {
				Abstract struct {
					AbstractText []struct {
						Text string `xml:",chardata"`
					} `xml:"AbstractText"`
				} `xml:"Abstract"`
			} `xml:"Article"`
		} `xml:"MedlineCitation"`
	} `xml:"PubmedArticle"`
}

// pubmedAbstract fetches the abstract text for one PMID. The text is used
// ONLY as ephemeral LLM input in this program — never stored or displayed
// as-is (see the content policy note at the top of this file).
func pubmedAbstract(pmid string) (string, error) {
	url := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi?db=pubmed&id=%s&rettype=abstract&retmode=xml&%s",
		pmid, eutilsIdentity(),
	)
	body, err := httpGet(url)
	if err != nil {
		return "", fmt.Errorf("efetch: %w", err)
	}
	var parsed efetchXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("efetch: parse response: %w", err)
	}
	if len(parsed.PubmedArticle) == 0 {
		return "", fmt.Errorf("efetch: no article in response for PMID %s", pmid)
	}
	var parts []string
	for _, seg := range parsed.PubmedArticle[0].MedlineCitation.Article.Abstract.AbstractText {
		if t := strings.TrimSpace(seg.Text); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("efetch: no abstract text for PMID %s", pmid)
	}
	return strings.Join(parts, " "), nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func httpQueryEscape(s string) string {
	return strings.NewReplacer(" ", "+", "\"", "%22").Replace(s)
}

// parsePubDate handles the handful of date formats PubMed's esummary
// actually returns ("2026 Jul 8", "2026 Jul", "2026"). Returns nil (not an
// error) for anything it can't parse — a missing published_at is fine,
// guessing a wrong one is not.
func parsePubDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"2006 Jan 2", "2006 Jan", "2006"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}

// ---- LLM translate + summarize ----

type translationResult struct {
	TitleTH   string `json:"title_th"`
	SummaryTH string `json:"summary_th"`
}

const systemPrompt = `คุณเป็นผู้ช่วยแปลและสรุปบทคัดย่องานวิจัยทางการแพทย์ (ภาษาอังกฤษ) เป็นภาษาไทย
สำหรับแอปติดตามสุขภาพผู้ป่วยโรคไต กฎเคร่งครัดที่ต้องทำตามทุกข้อ:
1. แปล/สรุปให้ตรงกับเนื้อหาในบทคัดย่อต้นฉบับเท่านั้น ห้ามเพิ่มเติมข้อมูล ตีความเกินจริง หรือคาดเดา
2. ห้ามให้คำแนะนำทางการแพทย์ใดๆ เพิ่มเติมนอกเหนือจากสิ่งที่บทคัดย่อระบุไว้ตรงๆ
3. summary_th ต้องเป็นภาษาไทยธรรมชาติ อ่านง่าย ความยาว 3-5 ประโยค ห้ามคัดลอกประโยคภาษาอังกฤษมาแปลตรงตัวทั้งดุ้น ให้เรียบเรียงใหม่เป็นภาษาไทย
4. title_th คือชื่อบทความแปลเป็นภาษาไทย
5. ตอบกลับเป็น JSON เท่านั้น รูปแบบ {"title_th": "...", "summary_th": "..."} ห้ามมีข้อความอื่นนอกเหนือจาก JSON นี้`

func translate(providers []*llmprovider.Provider, title, abstract string) (*translationResult, error) {
	userContent := fmt.Sprintf("Title: %s\n\nAbstract: %s", title, abstract)
	content, used, err := llmprovider.CallChatJSON(providers, systemPrompt, userContent, 0.3)
	if err != nil {
		return nil, err
	}
	var result translationResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("%s: could not parse JSON result: %w", used.Name, err)
	}
	if result.TitleTH == "" || result.SummaryTH == "" {
		return nil, fmt.Errorf("%s: response missing title_th or summary_th", used.Name)
	}
	return &result, nil
}

// ---- main ----

func main() {
	// config.Load() loads .env into the process environment (via
	// godotenv) as a side effect — must happen before any os.Getenv call,
	// including inside llmprovider.Require below.
	cfg := config.Load()

	primary, fallback := llmprovider.Require("news_ingest_pubmed")
	providers := llmprovider.List(primary, fallback)
	log.Printf("news_ingest_pubmed: primary provider=%s (%s), fallback provider=%s (%s)",
		primary.Name, primary.Model, fallback.Name, fallback.Model)

	// R2/image generation is best-effort — a missing/broken config here
	// must never stop articles from being ingested (see content policy
	// note in internal/newsimage), so this only logs, never fatals.
	r2Client, r2Err := r2store.New(r2store.ConfigFromEnv())
	if r2Err != nil {
		log.Printf("news_ingest_pubmed: R2 not configured, feature images will be skipped: %v", r2Err)
		r2Client = nil
	}
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("news_ingest_pubmed: database connection failed: %v", err)
	}

	pmids, err := pubmedSearch(pubmedQuery, pubmedRetMax)
	if err != nil {
		log.Fatalf("news_ingest_pubmed: pubmed search failed: %v", err)
	}
	log.Printf("news_ingest_pubmed: query returned %d PMIDs", len(pmids))

	var inserted, skippedExisting, failed int
	for _, pmid := range pmids {
		var existingCount int64
		if err := db.Model(&models.NewsArticle{}).
			Where("source = ? AND external_id = ?", newsArticleSource, pmid).
			Count(&existingCount).Error; err != nil {
			log.Printf("news_ingest_pubmed: PMID %s: duplicate check failed: %v", pmid, err)
			failed++
			continue
		}
		if existingCount > 0 {
			skippedExisting++
			continue
		}

		time.Sleep(eutilsRequestGap)
		summary, err := pubmedSummary(pmid)
		if err != nil {
			log.Printf("news_ingest_pubmed: PMID %s: %v", pmid, err)
			failed++
			continue
		}

		time.Sleep(eutilsRequestGap)
		abstract, err := pubmedAbstract(pmid)
		if err != nil {
			log.Printf("news_ingest_pubmed: PMID %s: %v", pmid, err)
			failed++
			continue
		}

		result, err := translate(providers, summary.Title, abstract)
		if err != nil {
			log.Printf("news_ingest_pubmed: PMID %s: translation failed: %v", pmid, err)
			failed++
			continue
		}

		var journalName *string
		if summary.FullJournalName != "" {
			journalName = &summary.FullJournalName
		}
		creditSourceName := "PubMed/NCBI"
		if summary.FullJournalName != "" {
			creditSourceName = fmt.Sprintf("PubMed/NCBI — %s", summary.FullJournalName)
		}

		// Feature image: best-effort, never blocks the insert below. See
		// internal/newsimage's package doc for the content-policy rules
		// this must follow.
		featureImageStatus := models.NewsArticleFeatureImageFailed
		var featureImageURL *string
		scene, sceneErr := newsimage.DescribeScene(providers, result.TitleTH+"\n\n"+result.SummaryTH)
		if sceneErr != nil {
			log.Printf("news_ingest_pubmed: PMID %s: scene description failed, image generation skipped: %v", pmid, sceneErr)
		} else if r2Client == nil {
			log.Printf("news_ingest_pubmed: PMID %s: R2 not configured, image generation skipped", pmid)
		} else {
			imgResult := newsimage.GenerateAndStore(context.Background(), r2Client, openAIAPIKey, scene, newsArticleSource, pmid)
			featureImageURL = imgResult.URL
			featureImageStatus = imgResult.Status
		}

		article := models.NewsArticle{
			Source:             newsArticleSource,
			ExternalID:         pmid,
			Title:              summary.Title,
			TitleTH:            result.TitleTH,
			SummaryTH:          result.SummaryTH,
			ContentHTML:        nil, // never store full text for pubmed — see content policy above
			JournalName:        journalName,
			PublishedAt:        parsePubDate(summary.PubDate),
			CreditSourceName:   creditSourceName,
			CreditURL:          fmt.Sprintf("https://pubmed.ncbi.nlm.nih.gov/%s/", pmid),
			FeatureImageURL:    featureImageURL,
			FeatureImageStatus: featureImageStatus,
			Status:             models.NewsArticleStatusPending,
		}
		if err := db.Create(&article).Error; err != nil {
			log.Printf("news_ingest_pubmed: PMID %s: insert failed: %v", pmid, err)
			failed++
			continue
		}
		inserted++
		log.Printf("news_ingest_pubmed: PMID %s: inserted (%q)", pmid, result.TitleTH)
	}

	log.Printf("news_ingest_pubmed: done — inserted=%d skipped_existing=%d failed=%d", inserted, skippedExisting, failed)
	if failed > 0 && inserted == 0 && skippedExisting == 0 {
		os.Exit(1)
	}
}
