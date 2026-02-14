package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// TikTok URL patterns:
// Full: https://www.tiktok.com/@username/video/1234567890
// Short: https://vt.tiktok.com/ZSm6d8htK/
var tiktokPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://(?:www\.)?tiktok\.com/@[a-zA-Z0-9_.]+/video/\d+`),
	regexp.MustCompile(`https?://vt\.tiktok\.com/[a-zA-Z0-9]+/?`),
}

// extractTikTokURLs finds all TikTok URLs in the given text.
func extractTikTokURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	for _, pattern := range tiktokPatterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			if !seen[match] {
				urls = append(urls, match)
				seen[match] = true
			}
		}
	}

	return urls
}

// TikTokOEmbed holds data from the TikTok oEmbed API response.
type TikTokOEmbed struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	AuthorURL    string `json:"author_url"`
	AuthorID     string `json:"author_unique_id"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// tiktokOEmbedBase is the base URL for TikTok oEmbed API. Override in tests.
var tiktokOEmbedBase = "https://www.tiktok.com/oembed"

// fetchTikTokOEmbed fetches oEmbed data for a TikTok video URL.
func fetchTikTokOEmbed(videoURL string) (*TikTokOEmbed, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	oembedURL := tiktokOEmbedBase + "?url=" + url.QueryEscape(videoURL)

	resp, err := client.Get(oembedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TikTok oEmbed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var oembed TikTokOEmbed
	if err := json.Unmarshal(body, &oembed); err != nil {
		return nil, fmt.Errorf("failed to parse oEmbed response: %w", err)
	}

	return &oembed, nil
}

// buildTikTokAttachment creates a Mattermost message attachment from TikTok oEmbed data.
func buildTikTokAttachment(oembed *TikTokOEmbed, originalURL string) *model.SlackAttachment {
	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("TikTok: %s", oembed.Title),
		Color:      "#000000",
		AuthorName: oembed.AuthorName,
		AuthorLink: oembed.AuthorURL,
		Title:      fmt.Sprintf("@%s", oembed.AuthorID),
		TitleLink:  originalURL,
		Footer:     "TikTok Preview",
		FooterIcon: "https://www.tiktok.com/favicon.ico",
	}

	if oembed.Title != "" {
		attachment.Text = oembed.Title
	}

	if oembed.ThumbnailURL != "" {
		attachment.ImageURL = oembed.ThumbnailURL
	}

	return attachment
}
