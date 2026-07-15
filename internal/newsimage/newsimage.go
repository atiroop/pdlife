// Package newsimage generates the AI feature image for one news article
// and uploads it to R2 — shared by cmd/news_ingest_pubmed,
// cmd/news_ingest_nephrothai, and the /admin/content-queue "regenerate"
// button, so the prompt/style rules can't drift between call sites.
//
// # Content policy (do not weaken without discussing first)
//
// The prompt in BuildPrompt strictly forbids depicting internal human
// anatomy, organs, or any body part showing disease/infection/
// inflammation/wounds/pathology, whether realistic or stylized — medical
// concepts must be represented symbolically (e.g. a shield blocking a
// bacteria icon) rather than shown happening inside/on a body. Simple
// cartoon-proportioned human characters (doctor, patient) are allowed in
// the scene itself, just never with anatomical detail. This is a hard
// requirement from the feature spec, not a style preference.
//
// # Never blocks article ingestion
//
// GenerateAndStore never returns an error the caller must fail on — a
// failed image (rate limit, content-policy rejection, timeout, missing
// OPENAI_API_KEY) always resolves to a Result with
// Status=NewsArticleFeatureImageFailed and a nil URL. The calling
// ingestion command inserts the article regardless; the moderation queue
// UI shows a placeholder for articles without an image.
package newsimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/atiroop/pdlife/internal/llmprovider"
	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/r2store"
)

const (
	imageModel    = "gpt-image-2"
	imageEndpoint = "https://api.openai.com/v1/images/generations"
	imageSize     = "1536x1024" // 3:2 — closest the API offers to 16:9
	imageQuality  = "medium"    // cost control; revisit if quality is insufficient
	maxAttempts   = 3
)

var imageHTTPClient = &http.Client{Timeout: 120 * time.Second} // image generation is slow

// promptTemplate is the fixed wrapper around a per-article mini-scene
// description (see DescribeScene). Only %s (the scene) varies between
// articles — everything else, including the anatomy restriction, is
// constant. Semi-3D vector/editorial-illustration style (think modern
// fintech/crypto journalism art), replacing the earlier neon-cyberpunk
// style — that style's topic phrases were short and generic enough
// ("peritoneal dialysis infection management") that unrelated articles
// converged on near-identical compositions. Grounding the prompt in a
// literal mini-scene specific to each article's actual finding (see
// DescribeScene) is what fixes that at the source.
const promptTemplate = `Semi-3D vector illustration in a bold, editorial news style (similar to modern fintech/crypto journalism illustration): chunky rounded character/object design with flat color fills and subtle gradient highlights for depth, clean bold outlines, warm studio-style lighting on objects. Depict a SPECIFIC LITERAL MINI-SCENE that represents this article's actual finding/story: %s. Background: solid or softly gradient color (not photographic, not cityscape), color chosen to suit the scene's mood. Simple friendly character design is fine if a person appears (doctor, patient, generic figure) — but keep characters simplified/cartoon-proportioned, never anatomically detailed.

STRICT RULE (non-negotiable): never depict internal human anatomy, organs, or any body part showing disease, infection, inflammation, wounds, or pathological conditions — whether realistic or stylized. If the article involves infection/illness, represent it symbolically (e.g. a shield blocking a bacteria icon, a warning sign, a medicine bottle) rather than showing it happening inside/on a body. No text in the image. Aspect ratio 3:2 landscape.`

// BuildPrompt fills the fixed template with a per-article mini-scene
// description — see DescribeScene for how that description is produced
// from Thai source text.
func BuildPrompt(scene string) string {
	return fmt.Sprintf(promptTemplate, scene)
}

const sceneSystemPrompt = `คุณจะได้รับข้อความภาษาไทยที่สรุปเนื้อหาบทความทางการแพทย์/สุขภาพเกี่ยวกับโรคไต จงอ่านแล้วคิดฉากภาพประกอบข่าว 1 ฉากที่สื่อถึง "ประเด็นเฉพาะ" ของเรื่องนี้ (ไม่ใช่หัวข้อกว้างๆ) เช่นถ้าเป็นเรื่องรักษาติดเชื้อสำเร็จโดยไม่ต้องถอดสายสวน ให้คิดฉากที่สื่อถึง "ความสำเร็จที่ไม่ต้องผ่าตัด" ไม่ใช่แค่ "มีเชื้อโรค" ตอบเป็นภาษาอังกฤษ 2-3 ประโยค อธิบายฉาก, วัตถุ/สัญลักษณ์หลักในฉาก, และอารมณ์ของภาพ ห้ามอธิบายอวัยวะภายใน/พยาธิสภาพร่างกายในคำตอบเด็ดขาด ตอบกลับเป็น JSON เท่านั้น รูปแบบ {"scene": "..."} ห้ามมีข้อความอื่น`

// DescribeScene asks the given providers (same list an ingestion command
// already resolved for translation) to turn Thai source text into a
// short, vivid, article-specific mini-scene description (2-3 English
// sentences) suitable as BuildPrompt's scene argument. This is what makes
// each article's image genuinely different from the others, rather than
// converging on a handful of generic topic phrases — see promptTemplate's
// doc comment.
func DescribeScene(providers []*llmprovider.Provider, thaiText string) (string, error) {
	content, used, err := llmprovider.CallChatJSON(providers, sceneSystemPrompt, thaiText, 0.3)
	if err != nil {
		return "", err
	}
	var result struct {
		Scene string `json:"scene"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return "", fmt.Errorf("%s: could not parse scene JSON: %w", used.Name, err)
	}
	if strings.TrimSpace(result.Scene) == "" {
		return "", fmt.Errorf("%s: empty scene", used.Name)
	}
	return result.Scene, nil
}

// Result is always safe to use even on failure — URL is nil and Status is
// Failed rather than an error being returned.
type Result struct {
	URL    *string
	Status models.NewsArticleFeatureImageStatus
}

// GenerateAndStore generates one feature image for (source, externalID)
// from scene (see DescribeScene), uploads it to R2 at
// pdlife/news/{source}/{externalID}.png, and returns the outcome.
// openAIAPIKey is checked directly here — image generation always
// requires OpenAI specifically regardless of which provider
// PROVIDER/FALLBACK_PROVIDER select for text.
func GenerateAndStore(ctx context.Context, r2 *r2store.Client, openAIAPIKey, scene, source, externalID string) Result {
	if strings.TrimSpace(openAIAPIKey) == "" {
		log.Printf("newsimage: OPENAI_API_KEY not set, skipping image generation for %s/%s", source, externalID)
		return Result{Status: models.NewsArticleFeatureImageFailed}
	}

	prompt := BuildPrompt(scene)
	imgBytes, err := generateImageBytes(openAIAPIKey, prompt)
	if err != nil {
		log.Printf("newsimage: generation failed for %s/%s: %v", source, externalID, err)
		return Result{Status: models.NewsArticleFeatureImageFailed}
	}

	key := fmt.Sprintf("pdlife/news/%s/%s.png", source, externalID)
	url, err := r2.Upload(ctx, key, imgBytes, "image/png")
	if err != nil {
		log.Printf("newsimage: R2 upload failed for %s/%s: %v", source, externalID, err)
		return Result{Status: models.NewsArticleFeatureImageFailed}
	}

	log.Printf("newsimage: generated+uploaded %s/%s -> %s", source, externalID, url)
	return Result{URL: &url, Status: models.NewsArticleFeatureImageGenerated}
}

// Regenerate is GenerateAndStore plus best-effort deletion of the
// previous image first, for the /admin/content-queue "regenerate" button.
// Deletion failure is logged but does not stop regeneration — leaving one
// orphaned R2 object is a far smaller problem than losing the ability to
// regenerate at all.
func Regenerate(ctx context.Context, r2 *r2store.Client, openAIAPIKey, scene, source, externalID string, oldURL *string) Result {
	if oldURL != nil {
		if key := r2.KeyFromURL(*oldURL); key != "" {
			if err := r2.Delete(ctx, key); err != nil {
				log.Printf("newsimage: failed to delete old image %s (continuing anyway): %v", key, err)
			}
		}
	}
	return GenerateAndStore(ctx, r2, openAIAPIKey, scene, source, externalID)
}

// generateImageBytes calls the OpenAI Images API with exponential
// backoff + jitter, per OpenAI's own retry guidance — covers rate
// limits, transient timeouts, and (less usefully, but harmless) content
// policy rejections.
func generateImageBytes(apiKey, prompt string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		data, err := callImageAPI(apiKey, prompt)
		if err == nil {
			return data, nil
		}
		lastErr = err
		log.Printf("newsimage: image API attempt %d/%d failed: %v", attempt, maxAttempts, err)
		if attempt < maxAttempts {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 2s, 4s
			jitter := time.Duration(rand.Int64N(int64(750 * time.Millisecond)))
			time.Sleep(backoff + jitter)
		}
	}
	return nil, fmt.Errorf("all %d attempts failed, last error: %w", maxAttempts, lastErr)
}

func callImageAPI(apiKey, prompt string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"model":      imageModel,
		"prompt":     prompt,
		"size":       imageSize,
		"quality":    imageQuality,
		"n":          1,
		"moderation": "auto",
	})

	req, err := http.NewRequest(http.MethodPost, imageEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai images API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("no image data in response")
	}

	imgBytes, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return imgBytes, nil
}
