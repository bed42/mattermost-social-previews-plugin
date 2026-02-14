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

// Bluesky URL pattern: https://bsky.app/profile/handle-or-did/post/rkey
var blueskyPattern = regexp.MustCompile(`https?://bsky\.app/profile/([^/\s]+)/post/([a-zA-Z0-9]+)`)

// blueskyAPIBase is the public Bluesky API base URL. Override in tests.
var blueskyAPIBase = "https://public.api.bsky.app"

// extractBlueskyURLs finds all Bluesky URLs in the given text.
func extractBlueskyURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	matches := blueskyPattern.FindAllString(text, -1)
	for _, match := range matches {
		if !seen[match] {
			urls = append(urls, match)
			seen[match] = true
		}
	}

	return urls
}

// parseBlueskyURL extracts the handle/DID and post rkey from a Bluesky URL.
func parseBlueskyURL(rawURL string) (handle string, rkey string, ok bool) {
	m := blueskyPattern.FindStringSubmatch(rawURL)
	if len(m) != 3 {
		return "", "", false
	}
	return m[1], m[2], true
}

// BlueskyPost represents the relevant data from a Bluesky post.
type BlueskyPost struct {
	URI       string
	CID       string
	Author    BlueskyAuthor
	Text      string
	CreatedAt string
	Images    []BlueskyImage
	Video     *BlueskyVideo
	External  *BlueskyExternal
	LikeCount int
	ReplyCount int
	RepostCount int
	QuoteCount int
}

// BlueskyAuthor represents a Bluesky post author.
type BlueskyAuthor struct {
	DID         string
	Handle      string
	DisplayName string
	Avatar      string
}

// BlueskyImage represents an image embed.
type BlueskyImage struct {
	Thumb    string
	Fullsize string
	Alt      string
}

// BlueskyVideo represents a video embed.
type BlueskyVideo struct {
	Thumbnail string
	Playlist  string // HLS playlist URL
}

// BlueskyExternal represents an external link embed (like a card).
type BlueskyExternal struct {
	URI         string
	Title       string
	Description string
	Thumb       string
}

// resolveBlueskyHandle resolves a Bluesky handle to a DID.
func resolveBlueskyHandle(handle string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	apiURL := fmt.Sprintf("%s/xrpc/com.atproto.identity.resolveHandle?handle=%s",
		blueskyAPIBase, url.QueryEscape(handle))

	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to resolve handle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to resolve handle: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.DID, nil
}

// fetchBlueskyPost fetches a Bluesky post by handle/DID and rkey.
func fetchBlueskyPost(handle, rkey string) (*BlueskyPost, error) {
	did := handle
	if !strings.HasPrefix(handle, "did:") {
		resolved, err := resolveBlueskyHandle(handle)
		if err != nil {
			return nil, err
		}
		did = resolved
	}

	atURI := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey)

	client := &http.Client{Timeout: 10 * time.Second}
	apiURL := fmt.Sprintf("%s/xrpc/app.bsky.feed.getPostThread?uri=%s&depth=0",
		blueskyAPIBase, url.QueryEscape(atURI))

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseBlueskyResponse(body)
}

// parseBlueskyResponse parses the raw JSON from getPostThread into a BlueskyPost.
func parseBlueskyResponse(data []byte) (*BlueskyPost, error) {
	var raw struct {
		Thread struct {
			Post struct {
				URI    string `json:"uri"`
				CID    string `json:"cid"`
				Author struct {
					DID         string `json:"did"`
					Handle      string `json:"handle"`
					DisplayName string `json:"displayName"`
					Avatar      string `json:"avatar"`
				} `json:"author"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
					Embed     *struct {
						Type     string `json:"$type"`
						Images   []struct {
							Alt   string `json:"alt"`
							Image struct {
								Type string `json:"$type"`
								Ref  struct {
									Link string `json:"$link"`
								} `json:"ref"`
								MimeType string `json:"mimeType"`
							} `json:"image"`
						} `json:"images"`
						External *struct {
							URI         string `json:"uri"`
							Title       string `json:"title"`
							Description string `json:"description"`
						} `json:"external"`
					} `json:"embed"`
				} `json:"record"`
				Embed *struct {
					Type   string `json:"$type"`
					Images []struct {
						Thumb    string `json:"thumb"`
						Fullsize string `json:"fullsize"`
						Alt      string `json:"alt"`
					} `json:"images"`
					Video *struct {
						Thumbnail string `json:"thumbnail"`
						Playlist  string `json:"playlist"`
					} `json:"video"`
					External *struct {
						URI         string `json:"uri"`
						Title       string `json:"title"`
						Description string `json:"description"`
						Thumb       string `json:"thumb"`
					} `json:"external"`
				} `json:"embed"`
				LikeCount   int `json:"likeCount"`
				ReplyCount  int `json:"replyCount"`
				RepostCount int `json:"repostCount"`
				QuoteCount  int `json:"quoteCount"`
			} `json:"post"`
		} `json:"thread"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	p := raw.Thread.Post
	post := &BlueskyPost{
		URI:         p.URI,
		CID:         p.CID,
		Text:        p.Record.Text,
		CreatedAt:   p.Record.CreatedAt,
		LikeCount:   p.LikeCount,
		ReplyCount:  p.ReplyCount,
		RepostCount: p.RepostCount,
		QuoteCount:  p.QuoteCount,
		Author: BlueskyAuthor{
			DID:         p.Author.DID,
			Handle:      p.Author.Handle,
			DisplayName: p.Author.DisplayName,
			Avatar:      p.Author.Avatar,
		},
	}

	// Extract embeds from the resolved embed (not the record embed)
	if p.Embed != nil {
		embedType := p.Embed.Type

		// Images
		if strings.Contains(embedType, "images") {
			for _, img := range p.Embed.Images {
				post.Images = append(post.Images, BlueskyImage{
					Thumb:    img.Thumb,
					Fullsize: img.Fullsize,
					Alt:      img.Alt,
				})
			}
		}

		// Video
		if strings.Contains(embedType, "video") && p.Embed.Video != nil {
			post.Video = &BlueskyVideo{
				Thumbnail: p.Embed.Video.Thumbnail,
				Playlist:  p.Embed.Video.Playlist,
			}
		}

		// External link
		if strings.Contains(embedType, "external") && p.Embed.External != nil {
			post.External = &BlueskyExternal{
				URI:         p.Embed.External.URI,
				Title:       p.Embed.External.Title,
				Description: p.Embed.External.Description,
				Thumb:       p.Embed.External.Thumb,
			}
		}
	}

	return post, nil
}

// buildBlueskyAttachment creates a Mattermost message attachment from a Bluesky post.
func buildBlueskyAttachment(post *BlueskyPost, originalURL string, config *configuration) *model.SlackAttachment {
	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("Bluesky: %s", post.Text),
		Color:      "#0085FF", // Bluesky brand color
		AuthorName: post.Author.DisplayName,
		AuthorLink: fmt.Sprintf("https://bsky.app/profile/%s", post.Author.Handle),
		AuthorIcon: post.Author.Avatar,
		Title:      fmt.Sprintf("@%s", post.Author.Handle),
		TitleLink:  originalURL,
		Text:       post.Text,
		Footer:     "Bluesky Preview",
		FooterIcon: "https://bsky.app/static/favicon-32x32.png",
	}

	// Add first image inline
	if len(post.Images) > 0 {
		attachment.ImageURL = post.Images[0].Fullsize
	}

	// Add video thumbnail
	if post.Video != nil && post.Video.Thumbnail != "" {
		attachment.ThumbURL = post.Video.Thumbnail
	}

	fields := []*model.SlackAttachmentField{}

	// Video link
	if post.Video != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🎬 Video",
			Value: fmt.Sprintf("[Watch video](%s)", originalURL),
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
			&model.SlackAttachmentField{Title: "Reposts", Value: fmt.Sprintf("%d", post.RepostCount), Short: true},
			&model.SlackAttachmentField{Title: "Likes", Value: fmt.Sprintf("%d", post.LikeCount), Short: true},
		)
	}

	// Multiple images
	if len(post.Images) > 1 {
		var imgLinks []string
		for i, img := range post.Images {
			imgLinks = append(imgLinks, fmt.Sprintf("[image %d/%d](%s)", i+1, len(post.Images), img.Fullsize))
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: fmt.Sprintf("📎 %d Images", len(post.Images)),
			Value: strings.Join(imgLinks, " • "),
			Short: false,
		})
	}

	// External link card
	if post.External != nil && post.External.URI != "" {
		cardValue := fmt.Sprintf("**[%s](%s)**", post.External.Title, post.External.URI)
		if post.External.Description != "" {
			desc := post.External.Description
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

		// Use external thumb if no image already set
		if post.External.Thumb != "" && attachment.ImageURL == "" {
			attachment.ImageURL = post.External.Thumb
		}
	}

	attachment.Fields = fields

	return attachment
}
