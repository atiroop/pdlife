package handler

import (
	"fmt"
	"time"

	"github.com/atiroop/pdlife/internal/models"
)

// FormatDateThai renders t as a Thai Buddhist-era date ("10 ก.ค. 2569") —
// shared by main.go's "formatDate" template func and any Go code (e.g.
// profile.go's account-deletion email) that needs the same format outside
// a template.
func FormatDateThai(t time.Time) string {
	thaiMonths := []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonths[int(t.Month())], t.Year()+543)
}

// navInfo bundles the per-request bits every authenticated page's header
// needs: which treatment-type log book shortcut to show (empty for
// no-profile-yet — the header renders a disabled "เร็วๆ นี้" label
// instead) and whether to show the admin menu item.
type navInfo struct {
	TreatmentLabel string
	TreatmentLink  string
	IsAdmin        bool
}

func (h *AuthHandler) navInfoFromProfile(user *models.User, profile *models.PatientProfile) navInfo {
	info := navInfo{IsAdmin: user.Role == models.RoleAdmin}
	if profile == nil || profile.TreatmentType == nil {
		return info
	}
	switch *profile.TreatmentType {
	case models.TreatmentAPD:
		info.TreatmentLabel = "APD ของฉัน"
		info.TreatmentLink = "/apd"
	case models.TreatmentCAPD:
		info.TreatmentLabel = "CAPD ของฉัน"
		info.TreatmentLink = "/capd"
	case models.TreatmentHD:
		info.TreatmentLabel = "HD ของฉัน"
		info.TreatmentLink = "/hd"
	}
	return info
}

// navInfoForUser is the by-user variant for handlers that don't already
// have the PatientProfile loaded (e.g. Food Check, admin content-queue).
func (h *AuthHandler) navInfoForUser(user *models.User) navInfo {
	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		return navInfo{IsAdmin: user.Role == models.RoleAdmin}
	}
	return h.navInfoFromProfile(user, &profile)
}

// navDict builds the template data map fragment every page must merge
// into its render data so "app-header" has what it needs.
func navDict(user *models.User, nav navInfo, active string) map[string]interface{} {
	return map[string]interface{}{
		"User":              user,
		"NavTreatmentLabel": nav.TreatmentLabel,
		"NavTreatmentLink":  nav.TreatmentLink,
		"IsAdmin":           nav.IsAdmin,
		"Active":            active,
	}
}

// withNav merges navDict's keys into a page's own data map so templates
// can render "app-header" with plain `{{template "app-header" .}}` — the
// nav fields sit at the same top level as the page-specific ones.
func withNav(data map[string]interface{}, user *models.User, nav navInfo, active string) map[string]interface{} {
	for k, v := range navDict(user, nav, active) {
		data[k] = v
	}
	return data
}
