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
// Photo: https://www.tiktok.com/@username/photo/1234567890
// Short: https://vt.tiktok.com/ZSm6d8htK/
var tiktokPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://(?:www\.)?tiktok\.com/@[a-zA-Z0-9_.]+/(?:video|photo)/\d+`),
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

// tiktokPhotoPattern matches /photo/ paths that need to be rewritten to /video/ for oEmbed.
var tiktokPhotoPattern = regexp.MustCompile(`(/photo/)(\d+)`)

// resolveTikTokForOEmbed follows redirects on a TikTok URL and normalizes
// /photo/ paths to /video/ so the oEmbed API accepts them.
// Only call this as a fallback when the direct oEmbed call fails.
func resolveTikTokForOEmbed(rawURL string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to resolve TikTok URL: %w", err)
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no redirect location for TikTok URL")
	}

	// Strip query params and normalize /photo/ to /video/
	if u, err := url.Parse(loc); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		loc = u.String()
	}
	loc = tiktokPhotoPattern.ReplaceAllString(loc, "/video/$2")

	return loc, nil
}

// tiktokOEmbedAttempts and tiktokRetryDelay control retries of the oEmbed call.
// TikTok's oEmbed endpoint randomly sheds load with 503 "overload-protect
// triggered" (and occasionally 429) on roughly half of requests regardless of
// client, so a few quick retries make previews reliable. Override in tests.
var (
	tiktokOEmbedAttempts = 4
	tiktokRetryDelay     = 300 * time.Millisecond
)

// fetchTikTok fetches preview data for a TikTok URL. TikTok's oEmbed endpoint
// now returns only an iframe (no title, author or thumbnail), so the video page's
// embedded rehydration JSON is the primary source, with oEmbed as a fallback.
func fetchTikTok(rawURL string) (*TikTokOEmbed, error) {
	data, pageErr := fetchTikTokPage(rawURL)
	if pageErr == nil {
		return data, nil
	}

	data, err := fetchTikTokOEmbed(rawURL)
	if err != nil {
		// Direct oEmbed failed — try resolving redirects and normalizing /photo/ to /video/
		resolvedURL, resolveErr := resolveTikTokForOEmbed(rawURL)
		if resolveErr != nil {
			return nil, fmt.Errorf("page: %v; oEmbed: %w", pageErr, resolveErr)
		}
		data, err = fetchTikTokOEmbed(resolvedURL)
		if err != nil {
			return nil, fmt.Errorf("page: %v; oEmbed: %w", pageErr, err)
		}
	}
	return data, nil
}

// tiktokRehydrationPattern extracts the JSON state TikTok embeds in video pages.
var tiktokRehydrationPattern = regexp.MustCompile(`(?s)<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)

// tiktokRehydration holds the parts of a video page's rehydration JSON we use.
type tiktokRehydration struct {
	DefaultScope struct {
		VideoDetail struct {
			StatusCode int `json:"statusCode"`
			ItemInfo   struct {
				ItemStruct struct {
					Desc   string `json:"desc"`
					Author struct {
						UniqueID string `json:"uniqueId"`
						Nickname string `json:"nickname"`
					} `json:"author"`
					Video struct {
						Cover string `json:"cover"`
					} `json:"video"`
					ImagePost struct {
						Images []struct {
							ImageURL struct {
								URLList []string `json:"urlList"`
							} `json:"imageURL"`
						} `json:"images"`
					} `json:"imagePost"`
				} `json:"itemStruct"`
			} `json:"itemInfo"`
		} `json:"webapp.video-detail"`
	} `json:"__DEFAULT_SCOPE__"`
}

// tiktokPhotoToVideoURL rewrites a /photo/ URL to its /video/ form without
// query or fragment. ok is false if the URL is not a photo URL.
func tiktokPhotoToVideoURL(u *url.URL) (string, bool) {
	if !tiktokPhotoPattern.MatchString(u.Path) {
		return "", false
	}
	v := *u
	v.Path = tiktokPhotoPattern.ReplaceAllString(u.Path, "/video/$2")
	v.RawQuery = ""
	v.Fragment = ""
	return v.String(), true
}

// fetchTikTokPage fetches a TikTok video or photo page (following short-link
// redirects) and extracts preview data from its embedded rehydration JSON.
// Photo pages don't include webapp.video-detail, but the same ID under /video/
// does (with imagePost populated), so photo URLs — direct or reached via a
// short-link redirect — are fetched in their /video/ form.
func fetchTikTokPage(rawURL string) (*TikTokOEmbed, error) {
	if u, err := url.Parse(rawURL); err == nil {
		if videoURL, ok := tiktokPhotoToVideoURL(u); ok {
			rawURL = videoURL
		}
	}

	body, finalURL, err := getTikTokPage(rawURL)
	if err != nil {
		return nil, err
	}

	data, err := parseTikTokPage(body)
	if err != nil {
		if videoURL, ok := tiktokPhotoToVideoURL(finalURL); ok {
			if body, _, err2 := getTikTokPage(videoURL); err2 == nil {
				return parseTikTokPage(body)
			}
		}
		return nil, err
	}
	return data, nil
}

// getTikTokPage fetches a TikTok page and returns its body and the URL it
// ended up at after redirects.
func getTikTokPage(rawURL string) ([]byte, *url.URL, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch TikTok page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}
	return body, resp.Request.URL, nil
}

// parseTikTokPage extracts preview data from a TikTok page's rehydration JSON.
func parseTikTokPage(body []byte) (*TikTokOEmbed, error) {

	m := tiktokRehydrationPattern.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("no rehydration data in TikTok page")
	}

	var state tiktokRehydration
	if err := json.Unmarshal(m[1], &state); err != nil {
		return nil, fmt.Errorf("failed to parse rehydration data: %w", err)
	}

	detail := state.DefaultScope.VideoDetail
	item := detail.ItemInfo.ItemStruct
	if detail.StatusCode != 0 || item.Author.UniqueID == "" {
		return nil, fmt.Errorf("TikTok page has no video data (status %d)", detail.StatusCode)
	}

	data := &TikTokOEmbed{
		Title:        item.Desc,
		AuthorName:   item.Author.Nickname,
		AuthorURL:    "https://www.tiktok.com/@" + item.Author.UniqueID,
		AuthorID:     item.Author.UniqueID,
		ThumbnailURL: item.Video.Cover,
	}
	if images := item.ImagePost.Images; len(images) > 0 && len(images[0].ImageURL.URLList) > 0 {
		data.ThumbnailURL = images[0].ImageURL.URLList[0]
	}

	return data, nil
}

// fetchTikTokOEmbed fetches oEmbed data for a TikTok video URL.
func fetchTikTokOEmbed(videoURL string) (*TikTokOEmbed, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	oembedURL := tiktokOEmbedBase + "?url=" + url.QueryEscape(videoURL)

	var resp *http.Response
	for attempt := 1; ; attempt++ {
		var err error
		resp, err = client.Get(oembedURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch TikTok oEmbed: %w", err)
		}
		retryable := resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests
		if !retryable || attempt >= tiktokOEmbedAttempts {
			break
		}
		resp.Body.Close()
		time.Sleep(time.Duration(attempt) * tiktokRetryDelay)
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

	if oembed.AuthorID == "" {
		return nil, fmt.Errorf("oEmbed response has no author data")
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
		attachment.Text = wrapText(oembed.Title, previewWrapWidth)
	}

	if oembed.ThumbnailURL != "" {
		attachment.ImageURL = oembed.ThumbnailURL
	}

	return attachment
}
