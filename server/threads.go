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

// ThreadsPost holds data extracted from OG meta tags on a Threads page.
type ThreadsPost struct {
	Title       string // og:title, e.g. "Username (@handle) on Threads"
	Description string // og:description
	ImageURL    string // og:image
	URL         string // og:url (canonical)
}

var (
	ogTitleRe       = regexp.MustCompile(`<meta\s+(?:property|name)="og:title"\s+content="([^"]*)"`)
	ogImageRe       = regexp.MustCompile(`<meta\s+(?:property|name)="og:image"\s+content="([^"]*)"`)
	ogURLRe         = regexp.MustCompile(`<meta\s+(?:property|name)="og:url"\s+content="([^"]*)"`)
	ogDescriptionRe = regexp.MustCompile(`<meta\s+(?:property|name)="og:description"\s+content="([^"]*)"`)
)

// fetchThreadsPost fetches a Threads URL and extracts OG meta tags.
func fetchThreadsPost(url string) (*ThreadsPost, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Threads requires a browser-like User-Agent to return OG tags
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MattermostPlugin/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Threads post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	post := parseThreadsHTML(string(body))
	if post.Title == "" && post.Description == "" {
		return nil, fmt.Errorf("no OG metadata found for Threads post")
	}

	// Fall back to the original URL if og:url wasn't found
	if post.URL == "" {
		post.URL = url
	}

	return post, nil
}

// parseThreadsHTML extracts OG meta tags from raw HTML.
func parseThreadsHTML(rawHTML string) *ThreadsPost {
	post := &ThreadsPost{}

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

	return post
}

// buildThreadsAttachment creates a Mattermost message attachment from a Threads post.
func buildThreadsAttachment(post *ThreadsPost, originalURL string) *model.SlackAttachment {
	// Parse author from og:title — format is "Username (@handle) on Threads"
	authorName := post.Title
	handle := ""
	if idx := strings.Index(post.Title, " on Threads"); idx > 0 {
		authorName = post.Title[:idx]
	}
	// Extract @handle from "Username (@handle)"
	if start := strings.Index(authorName, "(@"); start > 0 {
		if end := strings.Index(authorName[start:], ")"); end > 0 {
			handle = authorName[start+1 : start+end]
		}
	}

	displayURL := originalURL
	if post.URL != "" {
		displayURL = post.URL
	}

	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("Threads: %s", post.Description),
		Color:      "#000000",
		AuthorName: authorName,
		AuthorLink: displayURL,
		Text:       post.Description,
		Footer:     "Threads Preview",
		FooterIcon: "https://static.cdninstagram.com/rsrc.php/v3/yI/r/VsNE-OHk_8a.png",
	}

	if handle != "" {
		attachment.Title = handle
		attachment.TitleLink = displayURL
	}

	if post.ImageURL != "" {
		attachment.ImageURL = post.ImageURL
	}

	return attachment
}
