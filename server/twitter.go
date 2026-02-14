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

// Twitter/X URL patterns:
// https://twitter.com/username/status/1234567890
// https://x.com/username/status/1234567890
// https://mobile.twitter.com/username/status/1234567890
var twitterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://(?:(?:www|mobile)\.)?twitter\.com/([a-zA-Z0-9_]+)/status/(\d+)`),
	regexp.MustCompile(`https?://(?:www\.)?x\.com/([a-zA-Z0-9_]+)/status/(\d+)`),
}

// twitterAPIBase is the base URL for the fxtwitter API. Override in tests.
var twitterAPIBase = "https://api.fxtwitter.com"

// extractTwitterURLs finds all Twitter/X URLs in the given text.
func extractTwitterURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	for _, pattern := range twitterPatterns {
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

// parseTwitterURL extracts the username and tweet ID from a Twitter/X URL.
func parseTwitterURL(rawURL string) (username string, tweetID string, ok bool) {
	for _, pattern := range twitterPatterns {
		m := pattern.FindStringSubmatch(rawURL)
		if len(m) == 3 {
			return m[1], m[2], true
		}
	}
	return "", "", false
}

// TwitterPost holds data from the fxtwitter API.
type TwitterPost struct {
	Text        string
	Author      TwitterAuthor
	Images      []TwitterMedia
	Video       *TwitterVideo
	LikeCount   int
	ReplyCount  int
	RetweetCount int
	QuoteCount  int
}

// TwitterAuthor represents the tweet author.
type TwitterAuthor struct {
	Name       string
	ScreenName string
	AvatarURL  string
}

// TwitterMedia represents an image in a tweet.
type TwitterMedia struct {
	URL string
	Alt string
}

// TwitterVideo represents a video in a tweet.
type TwitterVideo struct {
	ThumbnailURL string
	URL          string
}

// fetchTwitterPost fetches a tweet via the fxtwitter API.
func fetchTwitterPost(username, tweetID string) (*TwitterPost, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	apiURL := fmt.Sprintf("%s/%s/status/%s", twitterAPIBase, username, tweetID)

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tweet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseTwitterResponse(body)
}

// parseTwitterResponse parses the fxtwitter API JSON into a TwitterPost.
func parseTwitterResponse(data []byte) (*TwitterPost, error) {
	var raw struct {
		Code  int `json:"code"`
		Tweet struct {
			Text   string `json:"text"`
			Author struct {
				Name       string `json:"name"`
				ScreenName string `json:"screen_name"`
				AvatarURL  string `json:"avatar_url"`
			} `json:"author"`
			Media *struct {
				Photos []struct {
					URL    string `json:"url"`
					AltText string `json:"altText"`
				} `json:"photos"`
				Videos []struct {
					ThumbnailURL string `json:"thumbnail_url"`
					URL          string `json:"url"`
				} `json:"videos"`
			} `json:"media"`
			Likes    int `json:"likes"`
			Replies  int `json:"replies"`
			Retweets int `json:"retweets"`
			Quotes   int `json:"quotes"`
		} `json:"tweet"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	t := raw.Tweet
	post := &TwitterPost{
		Text:         t.Text,
		LikeCount:    t.Likes,
		ReplyCount:   t.Replies,
		RetweetCount: t.Retweets,
		QuoteCount:   t.Quotes,
		Author: TwitterAuthor{
			Name:       t.Author.Name,
			ScreenName: t.Author.ScreenName,
			AvatarURL:  t.Author.AvatarURL,
		},
	}

	if t.Media != nil {
		for _, photo := range t.Media.Photos {
			post.Images = append(post.Images, TwitterMedia{
				URL: photo.URL,
				Alt: photo.AltText,
			})
		}
		if len(t.Media.Videos) > 0 {
			v := t.Media.Videos[0]
			post.Video = &TwitterVideo{
				ThumbnailURL: v.ThumbnailURL,
				URL:          v.URL,
			}
		}
	}

	return post, nil
}

// buildTwitterAttachment creates a Mattermost message attachment from a tweet.
func buildTwitterAttachment(post *TwitterPost, originalURL string, config *configuration) *model.SlackAttachment {
	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("X: %s", post.Text),
		Color:      "#000000", // X brand color
		AuthorName: post.Author.Name,
		AuthorLink: fmt.Sprintf("https://x.com/%s", post.Author.ScreenName),
		AuthorIcon: post.Author.AvatarURL,
		Title:      fmt.Sprintf("@%s", post.Author.ScreenName),
		TitleLink:  originalURL,
		Text:       post.Text,
		Footer:     "X Preview",
		FooterIcon: "https://abs.twimg.com/favicons/twitter.3.ico",
	}

	// Add first image inline
	if len(post.Images) > 0 {
		attachment.ImageURL = post.Images[0].URL
	}

	// Add video thumbnail
	if post.Video != nil && post.Video.ThumbnailURL != "" {
		if attachment.ImageURL == "" {
			attachment.ImageURL = post.Video.ThumbnailURL
		} else {
			attachment.ThumbURL = post.Video.ThumbnailURL
		}
	}

	fields := []*model.SlackAttachmentField{}

	// Video link
	if post.Video != nil && post.Video.URL != "" {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🎬 Video",
			Value: fmt.Sprintf("[Watch video](%s)", post.Video.URL),
			Short: false,
		})
	}

	// Engagement metrics
	showEngagement := true
	if config != nil && config.ShowEngagementMetrics != nil {
		showEngagement = *config.ShowEngagementMetrics
	}

	if showEngagement {
		fields = append(fields,
			&model.SlackAttachmentField{Title: "Replies", Value: fmt.Sprintf("%d", post.ReplyCount), Short: true},
			&model.SlackAttachmentField{Title: "Retweets", Value: fmt.Sprintf("%d", post.RetweetCount), Short: true},
			&model.SlackAttachmentField{Title: "Likes", Value: fmt.Sprintf("%d", post.LikeCount), Short: true},
		)
	}

	// Multiple images
	if len(post.Images) > 1 {
		var imgLinks []string
		for i, img := range post.Images {
			imgLinks = append(imgLinks, fmt.Sprintf("[image %d/%d](%s)", i+1, len(post.Images), img.URL))
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: fmt.Sprintf("📎 %d Images", len(post.Images)),
			Value: strings.Join(imgLinks, " • "),
			Short: false,
		})
	}

	attachment.Fields = fields

	return attachment
}
