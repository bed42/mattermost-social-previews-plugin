package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOGTags_FullOG(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Article Title">
		<meta property="og:description" content="A great article about things.">
		<meta property="og:image" content="https://example.com/image.jpg">
		<meta property="og:url" content="https://example.com/article">
		<meta property="og:site_name" content="Example News">
		<title>Fallback Title</title>
	</head></html>`

	preview := parseOGTags(html)

	assert.Equal(t, "Article Title", preview.Title)
	assert.Equal(t, "A great article about things.", preview.Description)
	assert.Equal(t, "https://example.com/image.jpg", preview.ImageURL)
	assert.Equal(t, "https://example.com/article", preview.URL)
	assert.Equal(t, "Example News", preview.SiteName)
}

func TestParseOGTags_FallbackToHTMLTitle(t *testing.T) {
	html := `<html><head>
		<title>Page Title From Title Tag</title>
		<meta name="description" content="Description from meta tag.">
	</head></html>`

	preview := parseOGTags(html)

	assert.Equal(t, "Page Title From Title Tag", preview.Title)
	assert.Equal(t, "Description from meta tag.", preview.Description)
	assert.Empty(t, preview.ImageURL)
}

func TestParseOGTags_OGTakesPriorityOverFallback(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Description">
		<title>HTML Title</title>
		<meta name="description" content="HTML Description">
	</head></html>`

	preview := parseOGTags(html)

	assert.Equal(t, "OG Title", preview.Title)
	assert.Equal(t, "OG Description", preview.Description)
}

func TestParseOGTags_NoMetadata(t *testing.T) {
	html := `<html><head></head><body>Just content</body></html>`

	preview := parseOGTags(html)

	assert.Empty(t, preview.Title)
	assert.Empty(t, preview.Description)
}

func TestParseOGTags_HTMLEntities(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Biden &amp; Trump&#39;s &quot;Deal&quot;">
	</head></html>`

	preview := parseOGTags(html)

	assert.Equal(t, "Biden & Trump's \"Deal\"", preview.Title)
}

func TestBuildOGAttachment_WithSiteName(t *testing.T) {
	preview := &OGPreview{
		Title:       "Article Title",
		Description: "Some description",
		ImageURL:    "https://example.com/img.jpg",
		SiteName:    "NPR",
	}

	att := buildOGAttachment(preview, "https://www.npr.org/article/123")

	assert.Equal(t, "Article Title", att.Title)
	assert.Equal(t, "https://www.npr.org/article/123", att.TitleLink)
	assert.Equal(t, "Some description", att.Text)
	assert.Equal(t, "https://example.com/img.jpg", att.ImageURL)
	assert.Equal(t, "#808080", att.Color)
	assert.Equal(t, "🔗 NPR", att.Footer)
}

func TestBuildOGAttachment_FallbackToDomain(t *testing.T) {
	preview := &OGPreview{
		Title:       "Page Title",
		Description: "Desc",
	}

	att := buildOGAttachment(preview, "https://www.example.com/page")

	assert.Equal(t, "🔗 www.example.com", att.Footer)
}

func TestBuildOGAttachment_LongDescription(t *testing.T) {
	longDesc := ""
	for i := 0; i < 400; i++ {
		longDesc += "x"
	}
	preview := &OGPreview{
		Title:       "Title",
		Description: longDesc,
	}

	att := buildOGAttachment(preview, "https://example.com")

	assert.Len(t, att.Text, 303) // 300 + "..."
	assert.True(t, len(att.Text) < len(longDesc))
}

func TestExtractGenericURLs_Basic(t *testing.T) {
	text := "Check this out https://www.npr.org/article/123 and also https://example.com"
	urls := extractGenericURLs(text, nil, "")

	assert.Equal(t, []string{"https://www.npr.org/article/123", "https://example.com"}, urls)
}

func TestExtractGenericURLs_ExcludesHandled(t *testing.T) {
	text := "https://mastodon.social/@user/123 https://www.npr.org/article"
	excluded := []string{"https://mastodon.social/@user/123"}

	urls := extractGenericURLs(text, excluded, "")

	assert.Equal(t, []string{"https://www.npr.org/article"}, urls)
}

func TestExtractGenericURLs_StripsPunctuation(t *testing.T) {
	text := "See https://example.com/page, and https://other.com/thing."
	urls := extractGenericURLs(text, nil, "")

	assert.Equal(t, []string{"https://example.com/page", "https://other.com/thing"}, urls)
}

func TestExtractGenericURLs_NoDuplicates(t *testing.T) {
	text := "https://example.com https://example.com"
	urls := extractGenericURLs(text, nil, "")

	assert.Equal(t, []string{"https://example.com"}, urls)
}
