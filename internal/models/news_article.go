package models

import "time"

type NewsArticleStatus string

const (
	// NewsArticleStatusPending is the only status the ingestion pipeline
	// ever writes — a human must promote a row to Published before any
	// patient-facing page shows it.
	NewsArticleStatusPending   NewsArticleStatus = "pending"
	NewsArticleStatusPublished NewsArticleStatus = "published"
	NewsArticleStatusRejected  NewsArticleStatus = "rejected"
)

// NewsArticleFeatureImageStatus tracks the AI feature-image pipeline
// separately from the article's own review Status — a failed image never
// blocks an article from reaching the pending queue (see
// internal/newsimage).
type NewsArticleFeatureImageStatus string

const (
	NewsArticleFeatureImagePending   NewsArticleFeatureImageStatus = "pending"
	NewsArticleFeatureImageGenerated NewsArticleFeatureImageStatus = "generated"
	NewsArticleFeatureImageFailed    NewsArticleFeatureImageStatus = "failed"
)

// NewsArticle is one ingested item (PubMed article or nephrothai.org post)
// for the Phase 4 News & Research section. For pubmed, SummaryTH is an
// AI-generated paraphrase grounded in the source abstract — never a
// verbatim copy of the source's own copyrighted text; ContentHTML is
// always nil. For nephrothai, ContentHTML holds the full permitted
// content and SummaryTH is a plain mechanical excerpt (no AI call — the
// content is already Thai and reuse is separately permitted, see
// docs/news_sources_survey.md). See docs/schema_spec.md for field notes.
type NewsArticle struct {
	ID               uint64     `gorm:"column:id;primaryKey"`
	Source           string     `gorm:"column:source;not null"`
	ExternalID       string     `gorm:"column:external_id;not null"`
	Title            string     `gorm:"column:title;not null"`
	TitleTH          string     `gorm:"column:title_th;not null"`
	SummaryTH        string     `gorm:"column:summary_th;not null"`
	ContentHTML      *string    `gorm:"column:content_html"`
	JournalName      *string    `gorm:"column:journal_name"`
	PublishedAt      *time.Time `gorm:"column:published_at;type:date"`
	CreditSourceName string     `gorm:"column:credit_source_name;not null"`
	CreditURL        string     `gorm:"column:credit_url;not null"`
	// FeatureImageURL/FeatureImageStatus are populated by
	// internal/newsimage after insert — a failed or still-pending image
	// never blocks the article's own review Status below.
	FeatureImageURL    *string                       `gorm:"column:feature_image_url"`
	FeatureImageStatus NewsArticleFeatureImageStatus `gorm:"column:feature_image_status;type:enum('pending','generated','failed');not null;default:pending"`
	Status             NewsArticleStatus             `gorm:"column:status;type:enum('pending','published','rejected');not null;default:pending"`
	// ReviewedBy/ReviewedAt are set together, once, by the approve/reject
	// handlers in /admin/content-queue — ingestion commands always insert
	// status=pending with both of these nil.
	ReviewedBy *uint64    `gorm:"column:reviewed_by"`
	ReviewedAt *time.Time `gorm:"column:reviewed_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (NewsArticle) TableName() string {
	return "news_articles"
}
