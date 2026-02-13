package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
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
func buildAttachment(status *MastodonStatus, url string, config *configuration) *model.SlackAttachment {
	// Strip HTML from content
	content := stripHTML(status.Content)

	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("🦣 Mastodon: %s", content),
		Color:      "#6364FF", // Mastodon brand color - distinctive purple left border
		AuthorName: status.Account.DisplayName,
		AuthorLink: status.Account.URL,
		AuthorIcon: status.Account.Avatar,
		Title:      fmt.Sprintf("@%s", status.Account.Acct),
		TitleLink:  url,
		Text:       content,
		Footer:     "🦣 Mastodon Preview",
		FooterIcon: "https://joinmastodon.org/favicon-32x32.png",
	}

	// Add first media attachment as image or thumbnail
	var firstMediaType string
	if len(status.MediaAttachments) > 0 {
		media := status.MediaAttachments[0]
		firstMediaType = media.Type
		if media.Type == "image" || media.Type == "gifv" {
			attachment.ImageURL = media.URL
		} else if media.Type == "video" && media.PreviewURL != "" {
			attachment.ThumbURL = media.PreviewURL
		}
	}

	// Add engagement metrics as fields (if enabled)
	fields := []*model.SlackAttachmentField{}

	// Add video link field if first attachment is a video
	if firstMediaType == "video" && len(status.MediaAttachments) > 0 {
		media := status.MediaAttachments[0]
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🎬 Video",
			Value: fmt.Sprintf("[Watch video](%s)", media.URL),
			Short: false,
		})
	}

	// Check if engagement metrics should be shown (default to true if not configured)
	showEngagement := true
	if config != nil && config.ShowEngagementMetrics != nil {
		showEngagement = *config.ShowEngagementMetrics
	}

	if showEngagement {
		fields = append(fields,
			&model.SlackAttachmentField{Title: "Replies", Value: fmt.Sprintf("%d", status.RepliesCount), Short: true},
			&model.SlackAttachmentField{Title: "Boosts", Value: fmt.Sprintf("%d", status.ReblogsCount), Short: true},
			&model.SlackAttachmentField{Title: "Favorites", Value: fmt.Sprintf("%d", status.FavouritesCount), Short: true},
		)
	}

	// If there are multiple media attachments, add links to all of them
	if len(status.MediaAttachments) > 1 {
		var mediaLinks []string
		for i, media := range status.MediaAttachments {
			mediaType := media.Type
			if mediaType == "" {
				mediaType = "media"
			}
			mediaLinks = append(mediaLinks, fmt.Sprintf("[%s %d/%d](%s)", mediaType, i+1, len(status.MediaAttachments), media.URL))
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: fmt.Sprintf("📎 %d Attachments", len(status.MediaAttachments)),
			Value: strings.Join(mediaLinks, " • "),
			Short: false,
		})
	}

	// Add poll information if present
	if status.Poll != nil {
		pollText := fmt.Sprintf("Poll with %d votes", status.Poll.VotesCount)
		if status.Poll.Expired {
			pollText += " (closed)"
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Poll",
			Value: pollText,
			Short: false,
		})
	}

	// Add link preview card if present
	if status.Card != nil && status.Card.URL != "" {
		cardValue := fmt.Sprintf("**[%s](%s)**", status.Card.Title, status.Card.URL)
		if status.Card.Description != "" {
			desc := status.Card.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			cardValue += "\n" + desc
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🔗 Link Preview",
			Value: cardValue,
			Short: false,
		})

		// Use card image if no media attachment already set the image
		if status.Card.Image != "" && attachment.ImageURL == "" {
			attachment.ImageURL = status.Card.Image
		}
	}

	attachment.Fields = fields

	return attachment
}
