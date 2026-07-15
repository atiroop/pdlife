// Package handler: PDLife Editorial Articles — admin-authored rich-text
// articles (distinct from the AI-summarized News & Research pipeline in
// news_list.go/admin_content_queue.go). This file covers the admin side:
// media upload, create/update/delete/list. See editorial_public.go for
// the public /articles, /articles/:slug pages.
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/r2store"
	"github.com/atiroop/pdlife/internal/sanitize"
)

const (
	editorialMaxImageBytes = 10 << 20  // 10 MB
	editorialMaxVideoBytes = 200 << 20 // 200 MB
)

// editorialAllowedTypes maps a sniffed MIME type (from the file's actual
// bytes, not its extension/filename) to the extension used for its R2
// key and the max size allowed for that kind. Extension checks alone are
// trivially spoofed (rename evil.exe to evil.jpg) — http.DetectContentType
// reads the file's magic bytes instead.
var editorialAllowedTypes = map[string]struct {
	ext     string
	maxSize int64
}{
	"image/jpeg": {"jpg", editorialMaxImageBytes},
	"image/png":  {"png", editorialMaxImageBytes},
	"image/webp": {"webp", editorialMaxImageBytes},
	"video/mp4":  {"mp4", editorialMaxVideoBytes},
	"video/webm": {"webm", editorialMaxVideoBytes},
}

// ---- POST /admin/editorial/upload-media ----

func (h *AuthHandler) EditorialUploadMedia(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	// Cap the raw request body before even touching the multipart
	// parser — without this, ParseMultipartForm will happily spill an
	// arbitrarily large body to disk temp files before any size check
	// below ever runs.
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, editorialMaxVideoBytes+1<<20)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "ไม่พบไฟล์ที่อัปโหลด"})
	}

	src, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "เปิดไฟล์ไม่สำเร็จ"})
	}
	defer src.Close()

	sniff := make([]byte, 512)
	n, err := io.ReadFull(src, sniff)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "อ่านไฟล์ไม่สำเร็จ"})
	}
	contentType := http.DetectContentType(sniff[:n])

	rule, ok := editorialAllowedTypes[contentType]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "ประเภทไฟล์ไม่รองรับ (รองรับเฉพาะรูปภาพ JPG/PNG/WEBP หรือวิดีโอ MP4/WEBM)",
		})
	}
	if fileHeader.Size > rule.maxSize {
		limit := "10MB"
		if strings.HasPrefix(contentType, "video/") {
			limit = "200MB"
		}
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("ไฟล์ใหญ่เกินกำหนด (จำกัด %s)", limit),
		})
	}

	rest, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "อ่านไฟล์ไม่สำเร็จ"})
	}
	data := append(sniff[:n], rest...)
	if int64(len(data)) > rule.maxSize {
		limit := "10MB"
		if strings.HasPrefix(contentType, "video/") {
			limit = "200MB"
		}
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("ไฟล์ใหญ่เกินกำหนด (จำกัด %s)", limit),
		})
	}

	r2Client, err := r2store.New(r2store.ConfigFromEnv())
	if err != nil {
		log.Printf("editorial: R2 not configured: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "ระบบอัปโหลดไม่พร้อมใช้งาน กรุณาลองใหม่ภายหลัง"})
	}

	key := fmt.Sprintf("pdlife/editorial/%d/%d-%s.%s", user.ID, time.Now().Unix(), randomHex(8), rule.ext)
	url, err := r2Client.Upload(c.Request().Context(), key, data, contentType)
	if err != nil {
		log.Printf("editorial: upload failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "อัปโหลดไม่สำเร็จ กรุณาลองใหม่"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"url": url})
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing is effectively unrecoverable (no
		// system entropy source) — fall back to a timestamp so the
		// upload still succeeds rather than 500ing on this alone.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// ---- slug generation ----

var slugUnsafeRe = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// slugify turns a title into a URL-safe slug. Thai has no case, so
// ToLower only affects any ASCII portions of the title; non-letter/digit
// runs (spaces, punctuation) become a single hyphen.
func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugUnsafeRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "article"
	}
	return s
}

// uniqueSlug appends -2, -3, ... to base until no other row has that
// slug. excludeID is the article's own ID when checking during an
// update (so an article doesn't collide with itself); pass 0 for a new,
// not-yet-created article.
func (h *AuthHandler) uniqueSlug(base string, excludeID uint64) string {
	slug := base
	for i := 2; ; i++ {
		q := h.DB.Model(&models.EditorialArticle{}).Where("slug = ?", slug)
		if excludeID != 0 {
			q = q.Where("id != ?", excludeID)
		}
		var count int64
		q.Count(&count)
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// ---- save request shape shared by create/update/autosave ----

type editorialSaveRequest struct {
	Title         string `json:"title"`
	ContentHTML   string `json:"content_html"`
	CoverImageURL string `json:"cover_image_url"`
	// Action is "draft" (save without changing publish state — the
	// default for a brand-new article), "publish" (ensure published,
	// stamp PublishedAt only the first time), or "autosave" (silent
	// background save from the editor's 30s debounce timer — never
	// touches status/PublishedAt at all, regardless of current state).
	Action string `json:"action"`
}

// ---- POST /admin/editorial/new ----

func (h *AuthHandler) EditorialCreate(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	var req editorialSaveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "กรุณากรอกชื่อบทความ"})
	}

	article := models.EditorialArticle{
		AuthorID:    user.ID,
		Title:       title,
		Slug:        h.uniqueSlug(slugify(title), 0),
		ContentHTML: sanitize.HTML(req.ContentHTML),
		Status:      models.EditorialArticleDraft,
	}
	if req.CoverImageURL != "" {
		article.CoverImageURL = &req.CoverImageURL
	}
	if req.Action == "publish" {
		now := time.Now()
		article.Status = models.EditorialArticlePublished
		article.PublishedAt = &now
	}

	if err := h.DB.Create(&article).Error; err != nil {
		log.Printf("editorial: create failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "บันทึกไม่สำเร็จ กรุณาลองใหม่"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id": article.ID, "slug": article.Slug, "status": article.Status,
	})
}

// ---- POST /admin/editorial/:id/edit ----

func (h *AuthHandler) EditorialUpdate(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	id, perr := strconv.ParseUint(c.Param("id"), 10, 64)
	if perr != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "invalid id"})
	}
	var article models.EditorialArticle
	if err := h.DB.First(&article, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"error": "ไม่พบบทความ"})
	}

	var req editorialSaveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "กรุณากรอกชื่อบทความ"})
	}

	updates := map[string]interface{}{
		"title":        title,
		"content_html": sanitize.HTML(req.ContentHTML),
	}
	if req.CoverImageURL != "" {
		updates["cover_image_url"] = req.CoverImageURL
	}
	// "draft" and "autosave" both leave status/published_at completely
	// untouched — neither downgrades an already-published article back
	// to draft (that would be a surprising, easy-to-trigger-by-accident
	// unpublish with no explicit action asking for it).
	if req.Action == "publish" {
		updates["status"] = models.EditorialArticlePublished
		if article.PublishedAt == nil {
			updates["published_at"] = time.Now()
		}
	}

	if err := h.DB.Model(&article).Updates(updates).Error; err != nil {
		log.Printf("editorial: update id=%d failed: %v", id, err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "บันทึกไม่สำเร็จ กรุณาลองใหม่"})
	}

	status := article.Status
	if req.Action == "publish" {
		status = models.EditorialArticlePublished
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id": article.ID, "slug": article.Slug, "status": status,
	})
}

// ---- POST /admin/editorial/:id/delete ----

func (h *AuthHandler) EditorialDelete(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	id, perr := strconv.ParseUint(c.Param("id"), 10, 64)
	if perr != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/editorial")
	}
	if err := h.DB.Delete(&models.EditorialArticle{}, id).Error; err != nil {
		log.Printf("editorial: delete id=%d failed: %v", id, err)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/editorial")
}

// ---- GET /admin/editorial ----

func (h *AuthHandler) EditorialList(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	var articles []models.EditorialArticle
	h.DB.Order("updated_at DESC").Find(&articles)

	data := map[string]interface{}{"Articles": articles}
	return c.Render(http.StatusOK, "editorial_list.html", withNav(data, user, h.navInfoForUser(user), "/admin/editorial"))
}

// ---- GET /admin/editorial/new ----

func (h *AuthHandler) EditorialNewForm(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}
	data := map[string]interface{}{"Article": nil}
	return c.Render(http.StatusOK, "editorial_editor.html", withNav(data, user, h.navInfoForUser(user), "/admin/editorial"))
}

// ---- GET /admin/editorial/:id/edit ----

func (h *AuthHandler) EditorialEditForm(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	id, perr := strconv.ParseUint(c.Param("id"), 10, 64)
	if perr != nil {
		return c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title": "ไม่พบบทความ", "Message": "ไม่พบบทความที่ต้องการแก้ไข",
		})
	}
	var article models.EditorialArticle
	if err := h.DB.First(&article, id).Error; err != nil {
		return c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title": "ไม่พบบทความ", "Message": "ไม่พบบทความที่ต้องการแก้ไข",
		})
	}

	data := map[string]interface{}{
		"Article":         article,
		"ContentHTMLSafe": template.HTML(article.ContentHTML), // already sanitized at save time — see internal/sanitize
	}
	return c.Render(http.StatusOK, "editorial_editor.html", withNav(data, user, h.navInfoForUser(user), "/admin/editorial"))
}
