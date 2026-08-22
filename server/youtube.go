package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// YouTube URL patterns:
// Watch: https://www.youtube.com/watch?v=VIDEOID (with optional extra params)
// Short: https://youtu.be/VIDEOID
// Shorts/Live: https://www.youtube.com/shorts/VIDEOID, https://www.youtube.com/live/VIDEOID
// The full URL including query params is captured so the oEmbed API receives it verbatim
// and so the generic OG fallback excludes the exact posted URL.
var youtubePatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://(?:www\.|m\.)?youtube\.com/watch\?[^\s<>"]*v=[a-zA-Z0-9_-]{6,}[^\s<>"]*`),
	regexp.MustCompile(`https?://youtu\.be/[a-zA-Z0-9_-]{6,}[^\s<>"]*`),
	regexp.MustCompile(`https?://(?:www\.|m\.)?youtube\.com/(?:shorts|live)/[a-zA-Z0-9_-]{6,}[^\s<>"]*`),
}

// extractYouTubeURLs finds all YouTube video URLs in the given text.
func extractYouTubeURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	for _, pattern := range youtubePatterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			// Strip trailing punctuation that's likely not part of the URL
			// (same set as extractGenericURLs so exclusion matching stays exact)
			match = strings.TrimRight(match, ".,;:!?)")
			if !seen[match] {
				urls = append(urls, match)
				seen[match] = true
			}
		}
	}

	return urls
}

// YouTubeOEmbed holds data from the YouTube oEmbed API response.
type YouTubeOEmbed struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	AuthorURL    string `json:"author_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// youtubeOEmbedBase is the base URL for the YouTube oEmbed API. Override in tests.
// Direct page scraping is deliberately avoided: YouTube serves bots a metadata-free
// shell page localized to the server IP's country, while oEmbed always returns the
// video's real title regardless of where the request comes from.
var youtubeOEmbedBase = "https://www.youtube.com/oembed"

// fetchYouTubeOEmbed fetches oEmbed data for a YouTube video URL.
func fetchYouTubeOEmbed(videoURL string) (*YouTubeOEmbed, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	oembedURL := youtubeOEmbedBase + "?url=" + url.QueryEscape(videoURL)

	resp, err := client.Get(oembedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch YouTube oEmbed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var oembed YouTubeOEmbed
	if err := json.Unmarshal(body, &oembed); err != nil {
		return nil, fmt.Errorf("failed to parse oEmbed response: %w", err)
	}

	return &oembed, nil
}

// buildYouTubeAttachment creates a Mattermost message attachment from YouTube oEmbed data.
func buildYouTubeAttachment(oembed *YouTubeOEmbed, originalURL string) *model.SlackAttachment {
	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("YouTube: %s", oembed.Title),
		Color:      "#FF0000",
		AuthorName: oembed.AuthorName,
		AuthorLink: oembed.AuthorURL,
		Title:      oembed.Title,
		TitleLink:  originalURL,
		Footer:     "YouTube Preview",
		FooterIcon: "https://www.youtube.com/favicon.ico",
	}

	if oembed.ThumbnailURL != "" {
		attachment.ImageURL = oembed.ThumbnailURL
	}

	return attachment
}
