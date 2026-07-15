package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/labrange"
	"github.com/atiroop/pdlife/internal/models"
)

// ---- GET / ----

// LandingPage sends an already-logged-in visitor straight to /dashboard
// instead of showing the public marketing page again; everyone else sees
// the normal landing page unchanged.
func (h *AuthHandler) LandingPage(c echo.Context) error {
	if user, err := h.currentSession(c); err == nil && user != nil {
		return c.Redirect(http.StatusSeeOther, "/dashboard")
	}
	return c.Render(http.StatusOK, "index.html", nil)
}

// requireOnboardedUser gates /dashboard, /news, /profile: valid session +
// completed onboarding. Unlike requireApdPatient/requireCapdPatient this
// does not check treatment_type — /dashboard is the landing page for
// every treatment type, including HD (placeholder card).
func (h *AuthHandler) requireOnboardedUser(c echo.Context) (*models.User, *models.PatientProfile, error) {
	user, err := h.currentSession(c)
	if err != nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role == models.RoleUnverified {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ยืนยันอีเมลก่อน",
			"Message": "กรุณายืนยันอีเมลก่อนใช้งาน",
		})
	}
	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	return user, &profile, nil
}

// ---- GET /dashboard ----

func (h *AuthHandler) Dashboard(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}
	nav := h.navInfoFromProfile(user, profile)

	var newsItems []models.NewsArticle
	h.DB.Where("status = ?", models.NewsArticleStatusPublished).
		Order("published_at DESC, id DESC").Limit(4).Find(&newsItems)

	// /dashboard itself doesn't require health-data consent (see
	// requireOnboardedUser) since it's also the landing page for
	// news/admin — but the KPI summary card below IS health data, so it
	// must stay hidden behind a "ยืนยันความยินยอม" prompt if consent was
	// never given or was withdrawn, same as the actual /apd, /capd pages.
	needsConsent := profile.HealthDataConsentAt == nil
	var treatmentCard map[string]interface{}
	if !needsConsent && profile.TreatmentType != nil {
		switch *profile.TreatmentType {
		case models.TreatmentAPD:
			treatmentCard = h.apdSummaryCard(profile.ID)
		case models.TreatmentCAPD:
			treatmentCard = h.capdSummaryCard(profile.ID)
		case models.TreatmentHD:
			treatmentCard = h.hdSummaryCard(profile.ID)
		}
	}

	// Lab results apply to every treatment type (unlike TreatmentCard
	// above), so this isn't gated on profile.TreatmentType — only on the
	// same health-data consent check.
	var labAbnormalItems []LabAbnormalItem
	var labHasData bool
	if !needsConsent {
		labAbnormalItems, labHasData = h.labSummaryData(profile.ID, isHDProfile(profile))
	}

	var adminPendingCount int64
	var adminEditorialDraftCount int64
	var adminUserCount int64
	if user.Role == models.RoleAdmin {
		h.DB.Model(&models.NewsArticle{}).
			Where("status = ?", models.NewsArticleStatusPending).
			Count(&adminPendingCount)
		h.DB.Model(&models.EditorialArticle{}).
			Where("status = ?", models.EditorialArticleDraft).
			Count(&adminEditorialDraftCount)
		h.DB.Model(&models.User{}).Count(&adminUserCount)
	}

	var editorialArticles []models.EditorialArticle
	h.DB.Where("status = ?", models.EditorialArticlePublished).
		Order("published_at DESC").Limit(3).Find(&editorialArticles)
	editorialItems := h.editorialListItemsWithAuthors(editorialArticles)

	data := map[string]interface{}{
		"NewsItems":                newsItems,
		"TreatmentCard":            treatmentCard,
		"NeedsConsent":             needsConsent,
		"AdminPendingCount":        adminPendingCount,
		"AdminEditorialDraftCount": adminEditorialDraftCount,
		"AdminUserCount":           adminUserCount,
		"EditorialItems":           editorialItems,
		"LabAbnormalItems":         labAbnormalItems,
		"LabHasData":               labHasData,
		"LabDisclaimer":            labrange.Disclaimer,
		"KpiDisclaimer":            kpi.Disclaimer,
	}
	return c.Render(http.StatusOK, "dashboard.html", withNav(data, user, nav, "/dashboard"))
}

// apdSummaryCard reuses apdKPICards (the exact same function ApdDashboard
// calls for /apd) so the dashboard's card set and status thresholds can
// never drift from the full page's.
func (h *AuthHandler) apdSummaryCard(profileID uint64) map[string]interface{} {
	cards, hasLatest, _ := h.apdKPICards(profileID)
	if !hasLatest {
		return map[string]interface{}{"Type": "APD", "HasData": false}
	}
	return map[string]interface{}{
		"Type":    "APD",
		"HasData": true,
		"Cards":   cards,
	}
}

// capdSummaryCard reuses capdKPICards (the exact same function
// CapdDashboard calls for /capd) — the peritonitis alert must stay just
// as prominent here as on /capd itself, never quietly folded away.
func (h *AuthHandler) capdSummaryCard(profileID uint64) map[string]interface{} {
	cards, hasLatest, _, peritonitisAlert, peritonitisMeta := h.capdKPICards(profileID)
	if !hasLatest {
		return map[string]interface{}{"Type": "CAPD", "HasData": false}
	}
	return map[string]interface{}{
		"Type":             "CAPD",
		"HasData":          true,
		"Cards":            cards,
		"PeritonitisAlert": peritonitisAlert,
		"PeritonitisMeta":  peritonitisMeta,
	}
}

// hdSummaryCard reuses hdKPICards (the exact same function HdDashboard
// calls for /hd) so the dashboard's card set and status thresholds can
// never drift from the full page's.
func (h *AuthHandler) hdSummaryCard(profileID uint64) map[string]interface{} {
	cards, hasLatest, _ := h.hdKPICards(profileID)
	if !hasLatest {
		return map[string]interface{}{"Type": "HD", "HasData": false}
	}
	return map[string]interface{}{
		"Type":    "HD",
		"HasData": true,
		"Cards":   cards,
	}
}
