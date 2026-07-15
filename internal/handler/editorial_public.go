package handler

import (
	"html/template"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
	"github.com/atiroop/pdlife/internal/sanitize"
)

const editorialExcerptRunes = 200

// editorialAuthorFallback is shown when an article's author account no
// longer resolves (e.g. deleted) — content_html itself is never lost,
// only the byline degrades.
const editorialAuthorFallback = "ทีมงาน PDLife"

// editorialListItem is the view model for one card on /articles —
// ExcerptText is precomputed server-side (stripped/truncated) rather
// than in the template, since there's no "strip tags" template func.
type editorialListItem struct {
	models.EditorialArticle
	ExcerptText string
	AuthorName  string
}

// ---- GET /articles ----
//
// Fully public — no login required (unlike /news, which sits behind
// requireOnboardedUser). Uses the plain site-header/site-footer shell
// (same as /terms, /privacy, index.html), not the authenticated
// app-shell, so there's no session lookup or nav-info wiring here at all.

func (h *AuthHandler) ArticlesList(c echo.Context) error {
	var articles []models.EditorialArticle
	h.DB.Where("status = ?", models.EditorialArticlePublished).
		Order("published_at DESC").Find(&articles)

	items := h.editorialListItemsWithAuthors(articles)

	return c.Render(http.StatusOK, "articles_list.html", map[string]interface{}{"Items": items})
}

// ---- GET /articles/:slug ----

func (h *AuthHandler) ArticleDetail(c echo.Context) error {
	// Slugs are Thai UTF-8, not ASCII — some proxies/environments hand
	// this route param to Echo still percent-encoded (e.g. "%e0%b8%81...")
	// instead of decoded, which would otherwise never match the decoded
	// UTF-8 slug stored in the DB. PathUnescape on an already-decoded
	// string (the normal case) is a safe no-op, since a real slug never
	// contains a literal "%XX" sequence (slugify only emits letters,
	// digits, hyphens).
	slug := c.Param("slug")
	if decoded, err := url.PathUnescape(slug); err == nil {
		slug = decoded
	}
	var article models.EditorialArticle
	if err := h.DB.Where("slug = ? AND status = ?", slug, models.EditorialArticlePublished).First(&article).Error; err != nil {
		return c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title": "ไม่พบบทความ", "Message": "ไม่พบบทความที่ต้องการ หรือบทความนี้ยังไม่ได้เผยแพร่",
		})
	}

	authorName := editorialAuthorFallback
	var author models.User
	if err := h.DB.First(&author, article.AuthorID).Error; err == nil {
		authorName = author.Nickname
	}

	return c.Render(http.StatusOK, "article_detail.html", map[string]interface{}{
		"Article":         article,
		"AuthorName":      authorName,
		"ContentHTMLSafe": template.HTML(article.ContentHTML), // already sanitized at save time — see internal/sanitize
	})
}

// editorialListItemsWithAuthors batches author lookups into one query
// instead of one-per-article, then builds each card's excerpt.
func (h *AuthHandler) editorialListItemsWithAuthors(articles []models.EditorialArticle) []editorialListItem {
	authorIDs := make([]uint64, 0, len(articles))
	seen := map[uint64]bool{}
	for _, a := range articles {
		if !seen[a.AuthorID] {
			seen[a.AuthorID] = true
			authorIDs = append(authorIDs, a.AuthorID)
		}
	}
	var authors []models.User
	if len(authorIDs) > 0 {
		h.DB.Where("id IN ?", authorIDs).Find(&authors)
	}
	nameByID := map[uint64]string{}
	for _, a := range authors {
		nameByID[a.ID] = a.Nickname
	}

	items := make([]editorialListItem, 0, len(articles))
	for _, a := range articles {
		name := nameByID[a.AuthorID]
		if name == "" {
			name = editorialAuthorFallback
		}
		items = append(items, editorialListItem{
			EditorialArticle: a,
			ExcerptText:      sanitize.Excerpt(a.ContentHTML, editorialExcerptRunes),
			AuthorName:       name,
		})
	}
	return items
}
