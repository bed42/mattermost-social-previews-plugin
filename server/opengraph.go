package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// OGPreview holds data extracted from Open Graph meta tags (with fallbacks).
type OGPreview struct {
	Title       string
	Description string
	ImageURL    string
	SiteName    string
	URL         string
}

var (
	ogSiteNameRe = regexp.MustCompile(`<meta\s+(?:property|name)="og:site_name"\s+content="([^"]*)"`)
	htmlTitleRe  = regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	metaDescRe   = regexp.MustCompile(`<meta\s+name="description"\s+content="([^"]*)"`)
)

// fetchOGPreview fetches a URL and extracts Open Graph metadata.
func fetchOGPreview(rawURL string) (*OGPreview, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MattermostPlugin/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Only parse HTML responses
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml") {
		return nil, fmt.Errorf("not an HTML page: %s", contentType)
	}

	// Limit read to 512KB to avoid downloading huge pages
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	preview := parseOGTags(string(body))
	if preview.Title == "" && preview.Description == "" {
		return nil, fmt.Errorf("no metadata found for URL")
	}

	if preview.URL == "" {
		preview.URL = rawURL
	}

	return preview, nil
}

// parseOGTags extracts Open Graph meta tags from raw HTML, falling back to
// <title> and <meta name="description"> when OG tags are absent.
func parseOGTags(rawHTML string) *OGPreview {
	preview := &OGPreview{}

	// OG tags (primary)
	if m := ogTitleRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		preview.Title = html.UnescapeString(m[1])
	}
	if m := ogDescriptionRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		preview.Description = html.UnescapeString(m[1])
	}
	if m := ogImageRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		preview.ImageURL = html.UnescapeString(m[1])
	}
	if m := ogURLRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		preview.URL = html.UnescapeString(m[1])
	}
	if m := ogSiteNameRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		preview.SiteName = html.UnescapeString(m[1])
	}

	// Fallbacks for title and description
	if preview.Title == "" {
		if m := htmlTitleRe.FindStringSubmatch(rawHTML); len(m) > 1 {
			preview.Title = html.UnescapeString(strings.TrimSpace(m[1]))
		}
	}
	if preview.Description == "" {
		if m := metaDescRe.FindStringSubmatch(rawHTML); len(m) > 1 {
			preview.Description = html.UnescapeString(m[1])
		}
	}

	return preview
}

// buildOGAttachment creates a Mattermost message attachment from an OG preview.
func buildOGAttachment(preview *OGPreview, originalURL string) *model.SlackAttachment {
	// Determine footer text from site name or domain
	footer := "Link Preview"
	if preview.SiteName != "" {
		footer = preview.SiteName
	} else if parsed, err := url.Parse(originalURL); err == nil {
		footer = parsed.Hostname()
	}

	desc := preview.Description
	if len(desc) > 300 {
		desc = desc[:300] + "..."
	}

	attachment := &model.SlackAttachment{
		Fallback:  fmt.Sprintf("🔗 %s", preview.Title),
		Color:     "#808080",
		Title:     preview.Title,
		TitleLink: originalURL,
		Text:      desc,
		Footer:    fmt.Sprintf("🔗 %s", footer),
	}

	if preview.ImageURL != "" {
		attachment.ImageURL = preview.ImageURL
	}

	return attachment
}
