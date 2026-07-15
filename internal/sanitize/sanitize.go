// Package sanitize is the single place editorial article HTML gets
// cleaned before it's allowed into the database — every save path
// (create, update, autosave) in internal/handler's editorial handlers
// must call HTML() on content_html, no exceptions. Content is stored
// already-sanitized and rendered directly by templates afterward.
package sanitize

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// policy is a strict whitelist for rich-text article bodies: paragraph
// and heading structure, basic inline formatting, lists, quotes, links,
// and the two media types the editor can insert (img/video, both always
// pointing at R2-hosted URLs). Everything else — script, iframe, style,
// on* event attributes, class/id attributes, anything not explicitly
// listed below — is stripped by default, since bluemonday policies are
// whitelists: nothing passes through unless named here.
var policy = newPolicy()

// stripPolicy keeps text content only — used for the /articles list
// excerpt, not for anything that gets saved back to the DB.
var stripPolicy = bluemonday.StripTagsPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "h1", "h2", "h3", "h4", "strong", "em", "u", "ul", "ol", "li", "blockquote", "br")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowAttrs("src", "alt", "loading").OnElements("img")
	p.AllowAttrs("src", "controls", "preload").OnElements("video")
	return p
}

// HTML sanitizes raw editor HTML down to the whitelist above. Safe to
// call on already-sanitized content (idempotent).
func HTML(raw string) string {
	return policy.Sanitize(raw)
}

// Excerpt strips all HTML tags and truncates to maxRunes, for the
// /articles list page's first-paragraph preview. Never write the result
// back to the DB — it's a display-only derivative of ContentHTML.
func Excerpt(html string, maxRunes int) string {
	text := strings.TrimSpace(stripPolicy.Sanitize(html))
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= maxRunes {
		return text
	}
	return string(r[:maxRunes]) + "…"
}
