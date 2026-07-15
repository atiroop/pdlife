package models

import "time"

type EditorialArticleStatus string

const (
	EditorialArticleDraft     EditorialArticleStatus = "draft"
	EditorialArticlePublished EditorialArticleStatus = "published"
)

// EditorialArticle is an admin-authored rich-text article ("PDLife
// Editorial Articles" — distinct from the AI-summarized NewsArticle
// pipeline). ContentHTML is always sanitized (internal/sanitize) before
// it reaches this struct's way into the DB — see internal/handler's
// editorial handlers — so templates render it directly as template.HTML
// without re-sanitizing on every page view.
type EditorialArticle struct {
	ID          uint64 `gorm:"column:id;primaryKey"`
	AuthorID    uint64 `gorm:"column:author_id;not null"`
	Title       string `gorm:"column:title;not null"`
	Slug        string `gorm:"column:slug;unique;not null"`
	ContentHTML string `gorm:"column:content_html;not null"`
	// CoverImageURL nullability mirrors NewsArticle.FeatureImageURL —
	// nil means "show a placeholder", not "generation failed" (there's
	// no generation here, it's just optional).
	CoverImageURL *string                `gorm:"column:cover_image_url"`
	Status        EditorialArticleStatus `gorm:"column:status;type:enum('draft','published');not null;default:draft"`
	// PublishedAt is set exactly once, the first time an article moves
	// to Published — republishing (editing an already-published article)
	// never overwrites it. See internal/handler's editorial update logic.
	PublishedAt *time.Time `gorm:"column:published_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (EditorialArticle) TableName() string {
	return "editorial_articles"
}
