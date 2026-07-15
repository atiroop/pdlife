package handler

import (
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/llmprovider"
	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/newsimage"
	"github.com/atiroop/pdlife/internal/r2store"
)

// requireAdmin gates every /admin/* route: valid session + role == Admin.
// If user is nil the guard has already written a response (redirect or
// 403 render) and the caller should return err as-is.
func (h *AuthHandler) requireAdmin(c echo.Context) (*models.User, error) {
	user, err := h.currentSession(c)
	if err != nil {
		return nil, c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role != models.RoleAdmin {
		return nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ไม่มีสิทธิ์เข้าถึง",
			"Message": "หน้านี้ใช้ได้เฉพาะผู้ดูแลระบบ (Admin) เท่านั้น",
		})
	}
	return user, nil
}

// ---- GET /admin/content-queue ----

func (h *AuthHandler) AdminContentQueue(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}

	sourceFilter := c.QueryParam("source")
	if sourceFilter != "nephrothai" && sourceFilter != "pubmed" {
		sourceFilter = ""
	}

	var totalPending int64
	h.DB.Model(&models.NewsArticle{}).
		Where("status = ?", models.NewsArticleStatusPending).
		Count(&totalPending)

	query := h.DB.Where("status = ?", models.NewsArticleStatusPending)
	if sourceFilter != "" {
		query = query.Where("source = ?", sourceFilter)
	}
	var items []models.NewsArticle
	query.Order("published_at DESC, id DESC").Find(&items)

	rows := make([]map[string]interface{}, len(items))
	for i, item := range items {
		var contentHTML template.HTML
		if item.ContentHTML != nil {
			// Trusted admin-only preview of content already covered by
			// the nephrothai.org reuse permission (see
			// docs/news_sources_survey.md) — never rendered unescaped on
			// any patient-facing page.
			contentHTML = template.HTML(*item.ContentHTML)
		}
		rows[i] = map[string]interface{}{
			"Item":        item,
			"ContentHTML": contentHTML,
		}
	}

	data := map[string]interface{}{
		"Rows":         rows,
		"TotalPending": totalPending,
		"SourceFilter": sourceFilter,
	}
	return c.Render(http.StatusOK, "admin_content_queue.html", withNav(data, user, h.navInfoForUser(user), "/admin/content-queue"))
}

// adminReviewAction is shared by approve/reject: both are a status
// transition away from pending that must record who/when, and both must
// be idempotent (a second click on an already-reviewed row is a no-op
// 404, never a silent overwrite of the first reviewer's decision).
func (h *AuthHandler) adminReviewAction(c echo.Context, newStatus models.NewsArticleStatus) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}
	id, parseErr := strconv.ParseUint(c.Param("id"), 10, 64)
	if parseErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid id"})
	}

	var article models.NewsArticle
	if err := h.DB.Where("id = ? AND status = ?", id, models.NewsArticleStatusPending).First(&article).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"success": false, "error": "not found or already reviewed"})
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":      newStatus,
		"reviewed_by": user.ID,
		"reviewed_at": now,
	}
	if err := h.DB.Model(&article).Updates(updates).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "update failed"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// ---- POST /admin/content-queue/:id/approve ----

func (h *AuthHandler) AdminApproveContent(c echo.Context) error {
	return h.adminReviewAction(c, models.NewsArticleStatusPublished)
}

// ---- POST /admin/content-queue/:id/reject ----

func (h *AuthHandler) AdminRejectContent(c echo.Context) error {
	return h.adminReviewAction(c, models.NewsArticleStatusRejected)
}

// ---- POST /admin/content-queue/:id/regenerate-image ----

// AdminRegenerateImage lets a reviewer discard an unsatisfactory
// AI-generated image and try again. Unlike cmd/news_ingest_pubmed and
// cmd/news_ingest_nephrothai (which call llmprovider.Require and exit the
// whole process on missing config), this is a request handler inside the
// always-running web server — a config problem here must return a JSON
// error to this one admin click, never take down the server for every
// other user. That's why this resolves providers with llmprovider.Resolve
// directly instead of Require.
func (h *AuthHandler) AdminRegenerateImage(c echo.Context) error {
	user, err := h.requireAdmin(c)
	if user == nil {
		return err
	}
	id, parseErr := strconv.ParseUint(c.Param("id"), 10, 64)
	if parseErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid id"})
	}

	var article models.NewsArticle
	if err := h.DB.First(&article, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"success": false, "error": "not found"})
	}

	primary, perr := llmprovider.Resolve(strings.TrimSpace(os.Getenv("PROVIDER")))
	if perr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "provider config: " + perr.Error()})
	}
	fallback, ferr := llmprovider.Resolve(strings.TrimSpace(os.Getenv("FALLBACK_PROVIDER")))
	if ferr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "fallback provider config: " + ferr.Error()})
	}
	providers := llmprovider.List(primary, fallback)

	// SummaryTH is populated for both sources (AI translation for pubmed,
	// a mechanical excerpt of content_html for nephrothai — see
	// cmd/news_ingest_nephrothai) and gives DescribeScene real detail to
	// work with beyond the title alone.
	sceneInput := article.TitleTH + "\n\n" + article.SummaryTH
	scene, terr := newsimage.DescribeScene(providers, sceneInput)
	if terr != nil {
		return c.JSON(http.StatusBadGateway, map[string]interface{}{"success": false, "error": "scene description failed: " + terr.Error()})
	}

	r2Client, r2err := r2store.New(r2store.ConfigFromEnv())
	if r2err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "R2 config: " + r2err.Error()})
	}

	result := newsimage.Regenerate(c.Request().Context(), r2Client, os.Getenv("OPENAI_API_KEY"), scene, article.Source, article.ExternalID, article.FeatureImageURL)

	if err := h.DB.Model(&article).Updates(map[string]interface{}{
		"feature_image_url":    result.URL,
		"feature_image_status": result.Status,
	}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "database update failed"})
	}

	if result.Status != models.NewsArticleFeatureImageGenerated {
		return c.JSON(http.StatusOK, map[string]interface{}{"success": false, "error": "generation failed — see server logs"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "image_url": *result.URL})
}
