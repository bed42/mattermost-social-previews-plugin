package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTikTokURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "standard URL",
			text:     "Check this https://www.tiktok.com/@zachking/video/7436498789814498590",
			expected: []string{"https://www.tiktok.com/@zachking/video/7436498789814498590"},
		},
		{
			name:     "no www",
			text:     "https://tiktok.com/@user/video/1234567890",
			expected: []string{"https://tiktok.com/@user/video/1234567890"},
		},
		{
			name:     "URL with query params stripped",
			text:     "https://www.tiktok.com/@user5367745512187/video/7583617257847196942?is_from_webapp=1&sender_device=pc",
			expected: []string{"https://www.tiktok.com/@user5367745512187/video/7583617257847196942"},
		},
		{
			name:     "multiple URLs",
			text:     "https://tiktok.com/@a/video/111 and https://tiktok.com/@b/video/222",
			expected: []string{"https://tiktok.com/@a/video/111", "https://tiktok.com/@b/video/222"},
		},
		{
			name:     "short link with trailing slash",
			text:     "https://vt.tiktok.com/ZSm6d8htK/",
			expected: []string{"https://vt.tiktok.com/ZSm6d8htK/"},
		},
		{
			name:     "short link without trailing slash",
			text:     "check this https://vt.tiktok.com/ZSm6d8htK",
			expected: []string{"https://vt.tiktok.com/ZSm6d8htK"},
		},
		{
			name:     "no TikTok URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "deduplication",
			text:     "https://tiktok.com/@z/video/123 https://tiktok.com/@z/video/123",
			expected: []string{"https://tiktok.com/@z/video/123"},
		},
		{
			name:     "username with dots and underscores",
			text:     "https://www.tiktok.com/@user.name_123/video/9876543210",
			expected: []string{"https://www.tiktok.com/@user.name_123/video/9876543210"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTikTokURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFetchTikTokOEmbed(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"title": "Cool video #fyp",
				"author_name": "Lucky Bii",
				"author_url": "https://www.tiktok.com/@user123",
				"author_unique_id": "user123",
				"thumbnail_url": "https://p16-sign.tiktokcdn-us.com/thumb.jpg"
			}`))
		}))
		defer server.Close()

		oldBase := tiktokOEmbedBase
		tiktokOEmbedBase = server.URL
		defer func() { tiktokOEmbedBase = oldBase }()

		oembed, err := fetchTikTokOEmbed("https://www.tiktok.com/@user123/video/123")
		require.NoError(t, err)
		assert.Equal(t, "Cool video #fyp", oembed.Title)
		assert.Equal(t, "Lucky Bii", oembed.AuthorName)
		assert.Equal(t, "user123", oembed.AuthorID)
		assert.Equal(t, "https://p16-sign.tiktokcdn-us.com/thumb.jpg", oembed.ThumbnailURL)
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldBase := tiktokOEmbedBase
		tiktokOEmbedBase = server.URL
		defer func() { tiktokOEmbedBase = oldBase }()

		_, err := fetchTikTokOEmbed("https://www.tiktok.com/@user/video/999")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})
}

func TestBuildTikTokAttachment(t *testing.T) {
	t.Run("full video", func(t *testing.T) {
		oembed := &TikTokOEmbed{
			Title:        "Amazing trick #magic",
			AuthorName:   "Zach King",
			AuthorURL:    "https://www.tiktok.com/@zachking",
			AuthorID:     "zachking",
			ThumbnailURL: "https://p16-sign.tiktokcdn-us.com/thumb.jpg",
		}

		att := buildTikTokAttachment(oembed, "https://www.tiktok.com/@zachking/video/123")

		assert.Equal(t, "#000000", att.Color)
		assert.Equal(t, "Zach King", att.AuthorName)
		assert.Equal(t, "https://www.tiktok.com/@zachking", att.AuthorLink)
		assert.Equal(t, "@zachking", att.Title)
		assert.Equal(t, "https://www.tiktok.com/@zachking/video/123", att.TitleLink)
		assert.Equal(t, "Amazing trick #magic", att.Text)
		assert.Equal(t, "https://p16-sign.tiktokcdn-us.com/thumb.jpg", att.ImageURL)
		assert.Equal(t, "TikTok Preview", att.Footer)
	})

	t.Run("video without title", func(t *testing.T) {
		oembed := &TikTokOEmbed{
			AuthorName:   "Someone",
			AuthorURL:    "https://www.tiktok.com/@someone",
			AuthorID:     "someone",
			ThumbnailURL: "https://example.com/thumb.jpg",
		}

		att := buildTikTokAttachment(oembed, "https://www.tiktok.com/@someone/video/456")

		assert.Equal(t, "Someone", att.AuthorName)
		assert.Equal(t, "@someone", att.Title)
		assert.Empty(t, att.Text)
	})
}
