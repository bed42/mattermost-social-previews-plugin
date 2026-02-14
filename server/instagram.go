package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// Instagram URL patterns:
// https://www.instagram.com/p/SHORTCODE/
// https://www.instagram.com/reel/SHORTCODE/
// https://instagram.com/p/SHORTCODE/
// https://www.instagram.com/p/SHORTCODE/?img_index=2
var instagramPattern = regexp.MustCompile(`https?://(?:www\.)?instagram\.com/(?:p|reels?)/[a-zA-Z0-9_-]+/?`)

// extractInstagramURLs finds all Instagram URLs in the given text.
func extractInstagramURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	matches := instagramPattern.FindAllString(text, -1)
	for _, match := range matches {
		if !seen[match] {
			urls = append(urls, match)
			seen[match] = true
		}
	}

	return urls
}

// InstagramPost holds data extracted from OG meta tags on an Instagram page.
type InstagramPost struct {
	Title       string // og:title
	Description string // og:description
	ImageURL    string // og:image
	VideoURL    string // og:video
	Type        string // og:type (e.g. "video" for reels)
	URL         string // og:url (canonical)
}

// instagramHTTPClient allows overriding the HTTP client in tests.
var instagramHTTPClient *http.Client

func getInstagramHTTPClient() *http.Client {
	if instagramHTTPClient != nil {
		return instagramHTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// normalizeInstagramURL rewrites /reels/ URLs to /reel/ for the API request.
func normalizeInstagramURL(url string) string {
	return strings.Replace(url, "/reels/", "/reel/", 1)
}

// fetchInstagramPost fetches an Instagram URL and extracts OG meta tags.
func fetchInstagramPost(url string) (*InstagramPost, error) {
	url = normalizeInstagramURL(url)
	client := getInstagramHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Instagram requires a browser-like User-Agent to return OG tags
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MattermostPlugin/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Instagram post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	post := parseInstagramHTML(string(body))
	if post.Title == "" && post.Description == "" {
		return nil, fmt.Errorf("no OG metadata found for Instagram post")
	}

	// Fall back to the original URL if og:url wasn't found
	if post.URL == "" {
		post.URL = url
	}

	return post, nil
}

// OG meta tag regexes (reuse the ones from threads.go)
var (
	ogVideoRe = regexp.MustCompile(`<meta\s+(?:property|name)="og:video"\s+content="([^"]*)"`)
	ogTypeRe  = regexp.MustCompile(`<meta\s+(?:property|name)="og:type"\s+content="([^"]*)"`)

	// Matches Instagram og:description prefix like:
	// "90K likes, 522 comments - aew on February 13, 2026: "
	// "1,234 likes, 56 comments - user on January 1, 2025: "
	instagramDescPrefixRe = regexp.MustCompile(`^[\d,.KMBkmb]+ likes, [\d,.KMBkmb]+ comments - .+ on \w+ \d{1,2}, \d{4}: `)
)

// parseInstagramHTML extracts OG meta tags from raw HTML.
func parseInstagramHTML(rawHTML string) *InstagramPost {
	post := &InstagramPost{}

	if m := ogTitleRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.Title = html.UnescapeString(m[1])
	}
	if m := ogImageRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.ImageURL = html.UnescapeString(m[1])
	}
	if m := ogURLRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.URL = html.UnescapeString(m[1])
	}
	if m := ogDescriptionRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.Description = html.UnescapeString(m[1])
	}
	if m := ogVideoRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.VideoURL = html.UnescapeString(m[1])
	}
	if m := ogTypeRe.FindStringSubmatch(rawHTML); len(m) > 1 {
		post.Type = html.UnescapeString(m[1])
	}

	return post
}

// cleanInstagramDescription strips the engagement metrics and date prefix from
// Instagram's og:description (e.g. "90K likes, 522 comments - user on Feb 13, 2026: ").
func cleanInstagramDescription(desc string) string {
	cleaned := instagramDescPrefixRe.ReplaceAllString(desc, "")
	// The caption text is often wrapped in quotes after the prefix
	cleaned = strings.TrimPrefix(cleaned, "\"")
	cleaned = strings.TrimSuffix(cleaned, "\"")
	return strings.TrimSpace(cleaned)
}

// buildInstagramAttachment creates a Mattermost message attachment from an Instagram post.
func buildInstagramAttachment(post *InstagramPost, originalURL string) *model.SlackAttachment {
	// Parse author from og:title — format varies:
	// Photos: "Username on Instagram: \"caption text\""
	// Reels: "Username on Instagram: \"caption text\""
	authorName := post.Title
	caption := cleanInstagramDescription(post.Description)

	if idx := strings.Index(post.Title, " on Instagram"); idx > 0 {
		authorName = post.Title[:idx]
	}

	displayURL := originalURL
	if post.URL != "" {
		displayURL = post.URL
	}

	isReel := strings.Contains(originalURL, "/reel/") ||
		strings.Contains(originalURL, "/reels/") ||
		strings.Contains(post.Type, "video")

	footerText := "Instagram Preview"
	if isReel {
		footerText = "Instagram Reel Preview"
	}

	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("Instagram: %s", caption),
		Color:      "#E1306C", // Instagram gradient pink
		AuthorName: authorName,
		AuthorLink: displayURL,
		Title:      authorName,
		TitleLink:  displayURL,
		Text:       caption,
		Footer:     footerText,
		FooterIcon: "https://www.instagram.com/static/images/ico/favicon-192.png/68d99ba29cc8.png",
	}

	if post.ImageURL != "" {
		attachment.ImageURL = post.ImageURL
	}

	fields := []*model.SlackAttachmentField{}

	// Video link for reels
	if isReel {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🎬 Reel",
			Value: fmt.Sprintf("[Watch reel](%s)", displayURL),
			Short: false,
		})
	}

	if len(fields) > 0 {
		attachment.Fields = fields
	}

	return attachment
}
