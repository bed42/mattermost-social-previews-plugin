package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractInstagramURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "post URL",
			text:     "Check this https://www.instagram.com/p/ABC123def/",
			expected: []string{"https://www.instagram.com/p/ABC123def/"},
		},
		{
			name:     "reel URL",
			text:     "https://www.instagram.com/reel/XYZ789/",
			expected: []string{"https://www.instagram.com/reel/XYZ789/"},
		},
		{
			name:     "reels URL (plural)",
			text:     "https://www.instagram.com/reels/DM8LEgzpv5K/",
			expected: []string{"https://www.instagram.com/reels/DM8LEgzpv5K/"},
		},
		{
			name:     "without www",
			text:     "https://instagram.com/p/ABC123/",
			expected: []string{"https://instagram.com/p/ABC123/"},
		},
		{
			name:     "without trailing slash",
			text:     "https://www.instagram.com/p/ABC123",
			expected: []string{"https://www.instagram.com/p/ABC123"},
		},
		{
			name:     "URL with query params stripped",
			text:     "https://www.instagram.com/p/ABC123/?img_index=2",
			expected: []string{"https://www.instagram.com/p/ABC123/"},
		},
		{
			name:     "multiple URLs",
			text:     "https://instagram.com/p/AAA/ and https://instagram.com/reel/BBB/",
			expected: []string{"https://instagram.com/p/AAA/", "https://instagram.com/reel/BBB/"},
		},
		{
			name:     "no Instagram URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "deduplication",
			text:     "https://instagram.com/p/ABC/ https://instagram.com/p/ABC/",
			expected: []string{"https://instagram.com/p/ABC/"},
		},
		{
			name:     "shortcode with hyphens and underscores",
			text:     "https://www.instagram.com/p/C_x-Y_z/",
			expected: []string{"https://www.instagram.com/p/C_x-Y_z/"},
		},
		{
			name:     "profile URL not matched",
			text:     "https://www.instagram.com/username/",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInstagramURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeInstagramURL(t *testing.T) {
	assert.Equal(t, "https://www.instagram.com/reel/DUrNI1KkTll/", normalizeInstagramURL("https://www.instagram.com/reels/DUrNI1KkTll/"))
	assert.Equal(t, "https://www.instagram.com/reel/ABC123/", normalizeInstagramURL("https://www.instagram.com/reel/ABC123/"))
	assert.Equal(t, "https://www.instagram.com/p/ABC123/", normalizeInstagramURL("https://www.instagram.com/p/ABC123/"))
}

func TestParseInstagramHTML(t *testing.T) {
	t.Run("photo post", func(t *testing.T) {
		html := `<html><head>
			<meta property="og:title" content="Alice on Instagram: &quot;Beautiful sunset&quot;">
			<meta property="og:description" content="Beautiful sunset over the ocean">
			<meta property="og:image" content="https://scontent.cdninstagram.com/v/photo.jpg">
			<meta property="og:url" content="https://www.instagram.com/p/ABC123/">
			<meta property="og:type" content="instapp:photo">
		</head></html>`

		post := parseInstagramHTML(html)
		assert.Equal(t, `Alice on Instagram: "Beautiful sunset"`, post.Title)
		assert.Equal(t, "Beautiful sunset over the ocean", post.Description)
		assert.Equal(t, "https://scontent.cdninstagram.com/v/photo.jpg", post.ImageURL)
		assert.Equal(t, "https://www.instagram.com/p/ABC123/", post.URL)
		assert.Equal(t, "instapp:photo", post.Type)
		assert.Empty(t, post.VideoURL)
	})

	t.Run("reel/video post", func(t *testing.T) {
		html := `<html><head>
			<meta property="og:title" content="Bob on Instagram: &quot;Dance moves&quot;">
			<meta property="og:description" content="Check out my moves">
			<meta property="og:image" content="https://scontent.cdninstagram.com/v/thumb.jpg">
			<meta property="og:video" content="https://scontent.cdninstagram.com/v/reel.mp4">
			<meta property="og:url" content="https://www.instagram.com/reel/XYZ789/">
			<meta property="og:type" content="video">
		</head></html>`

		post := parseInstagramHTML(html)
		assert.Equal(t, `Bob on Instagram: "Dance moves"`, post.Title)
		assert.Equal(t, "https://scontent.cdninstagram.com/v/reel.mp4", post.VideoURL)
		assert.Equal(t, "video", post.Type)
	})

	t.Run("empty HTML", func(t *testing.T) {
		post := parseInstagramHTML("<html><head></head></html>")
		assert.Empty(t, post.Title)
		assert.Empty(t, post.Description)
		assert.Empty(t, post.ImageURL)
	})
}

func TestFetchInstagramPost(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.Header.Get("User-Agent"), "MattermostPlugin")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><head>
				<meta property="og:title" content="Alice on Instagram: &quot;Hello!&quot;">
				<meta property="og:description" content="Hello world">
				<meta property="og:image" content="https://scontent.cdninstagram.com/photo.jpg">
				<meta property="og:url" content="https://www.instagram.com/p/ABC123/">
			</head></html>`))
		}))
		defer server.Close()

		oldClient := instagramHTTPClient
		instagramHTTPClient = server.Client()
		defer func() { instagramHTTPClient = oldClient }()

		post, err := fetchInstagramPost(server.URL + "/p/ABC123/")
		require.NoError(t, err)
		assert.Equal(t, `Alice on Instagram: "Hello!"`, post.Title)
		assert.Equal(t, "Hello world", post.Description)
		assert.Equal(t, "https://scontent.cdninstagram.com/photo.jpg", post.ImageURL)
	})

	t.Run("no OG metadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><head></head></html>`))
		}))
		defer server.Close()

		oldClient := instagramHTTPClient
		instagramHTTPClient = server.Client()
		defer func() { instagramHTTPClient = oldClient }()

		_, err := fetchInstagramPost(server.URL + "/p/ABC123/")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no OG metadata")
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldClient := instagramHTTPClient
		instagramHTTPClient = server.Client()
		defer func() { instagramHTTPClient = oldClient }()

		_, err := fetchInstagramPost(server.URL + "/p/NOTFOUND/")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})
}

func TestBuildInstagramAttachment(t *testing.T) {
	t.Run("photo post", func(t *testing.T) {
		post := &InstagramPost{
			Title:       "Alice on Instagram: \"Beautiful sunset\"",
			Description: "Beautiful sunset over the ocean",
			ImageURL:    "https://scontent.cdninstagram.com/photo.jpg",
			URL:         "https://www.instagram.com/p/ABC123/",
			Type:        "instapp:photo",
		}

		att := buildInstagramAttachment(post, "https://www.instagram.com/p/ABC123/")

		assert.Equal(t, "#E1306C", att.Color)
		assert.Equal(t, "Alice", att.AuthorName)
		assert.Equal(t, "Beautiful sunset over the ocean", att.Text)
		assert.Equal(t, "https://scontent.cdninstagram.com/photo.jpg", att.ImageURL)
		assert.Equal(t, "Instagram Preview", att.Footer)
		// No fields for a photo post
		assert.Empty(t, att.Fields)
	})

	t.Run("reel post", func(t *testing.T) {
		post := &InstagramPost{
			Title:       "Bob on Instagram: \"Dance moves\"",
			Description: "Check out my moves",
			ImageURL:    "https://scontent.cdninstagram.com/thumb.jpg",
			VideoURL:    "https://scontent.cdninstagram.com/reel.mp4",
			URL:         "https://www.instagram.com/reel/XYZ789/",
			Type:        "video",
		}

		att := buildInstagramAttachment(post, "https://www.instagram.com/reel/XYZ789/")

		assert.Equal(t, "Bob", att.AuthorName)
		assert.Equal(t, "Instagram Reel Preview", att.Footer)
		require.Len(t, att.Fields, 1)
		assert.Equal(t, "🎬 Reel", att.Fields[0].Title)
	})

	t.Run("fallback URL when og:url missing", func(t *testing.T) {
		post := &InstagramPost{
			Title:       "User on Instagram",
			Description: "Some post",
		}

		att := buildInstagramAttachment(post, "https://www.instagram.com/p/FALLBACK/")

		assert.Equal(t, "https://www.instagram.com/p/FALLBACK/", att.AuthorLink)
		assert.Equal(t, "https://www.instagram.com/p/FALLBACK/", att.TitleLink)
	})
}
