package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// fetchMastodonStatus fetches a Mastodon status from the given instance
func (p *Plugin) fetchMastodonStatus(instanceURL, statusID string) (*MastodonStatus, error) {
	// Construct Mastodon API URL
	apiURL := fmt.Sprintf("%s/api/v1/statuses/%s", instanceURL, statusID)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch status: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("status not found or private")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by Mastodon instance")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var status MastodonStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &status, nil
}

// fetchMastodonPost is a convenience wrapper that parses the URL and fetches the status
func (p *Plugin) fetchMastodonPost(url string) (*MastodonStatus, error) {
	instanceURL, statusID, ok := parseMastodonURL(url)
	if !ok {
		return nil, fmt.Errorf("invalid Mastodon URL: %s", url)
	}

	return p.fetchMastodonStatus(instanceURL, statusID)
}

// stripHTML removes HTML tags from content and converts to plain text
func stripHTML(html string) string {
	// Replace <br> and <p> tags with newlines
	html = regexp.MustCompile(`<br\s*/?>|</p>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<p[^>]*>`).ReplaceAllString(html, "\n")

	// Remove all other HTML tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")

	// Decode HTML entities
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Clean up extra whitespace
	html = strings.TrimSpace(html)
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")

	return html
}

// buildAttachment creates a Mattermost message attachment from a Mastodon status
func buildAttachment(status *MastodonStatus, url string) map[string]interface{} {
	attachment := map[string]interface{}{
		"fallback":    fmt.Sprintf("Mastodon post by @%s", status.Account.Username),
		"color":       "#6364FF", // Mastodon brand color
		"author_name": status.Account.DisplayName,
		"author_link": status.Account.URL,
		"author_icon": status.Account.Avatar,
		"title":       fmt.Sprintf("@%s", status.Account.Acct),
		"title_link":  url,
		"text":        stripHTML(status.Content),
		"footer":      "Mastodon",
	}

	// Add first media attachment as image or thumbnail
	if len(status.MediaAttachments) > 0 {
		media := status.MediaAttachments[0]
		if media.Type == "image" || media.Type == "gifv" {
			attachment["image_url"] = media.URL
		} else if media.Type == "video" && media.PreviewURL != "" {
			attachment["thumb_url"] = media.PreviewURL
		}
	}

	// Add engagement metrics as fields
	fields := []map[string]interface{}{
		{"title": "Replies", "value": fmt.Sprintf("%d", status.RepliesCount), "short": true},
		{"title": "Boosts", "value": fmt.Sprintf("%d", status.ReblogsCount), "short": true},
		{"title": "Favorites", "value": fmt.Sprintf("%d", status.FavouritesCount), "short": true},
	}

	// Add poll information if present
	if status.Poll != nil {
		pollText := fmt.Sprintf("Poll with %d votes", status.Poll.VotesCount)
		if status.Poll.Expired {
			pollText += " (closed)"
		}
		fields = append(fields, map[string]interface{}{
			"title": "Poll",
			"value": pollText,
			"short": false,
		})
	}

	attachment["fields"] = fields

	return attachment
}
