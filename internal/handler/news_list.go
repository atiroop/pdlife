package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// ---- GET /news ----

func (h *AuthHandler) NewsList(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}
	nav := h.navInfoFromProfile(user, profile)

	var items []models.NewsArticle
	h.DB.Where("status = ?", models.NewsArticleStatusPublished).
		Order("published_at DESC, id DESC").Find(&items)

	data := map[string]interface{}{"Items": items}
	return c.Render(http.StatusOK, "news_list.html", withNav(data, user, nav, "/news"))
}

// ---- GET /profile ----

func (h *AuthHandler) ProfilePlaceholder(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}
	nav := h.navInfoFromProfile(user, profile)

	data := map[string]interface{}{}
	return c.Render(http.StatusOK, "profile_placeholder.html", withNav(data, user, nav, "/profile"))
}
