package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractThreadsURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "threads.net URL",
			text:     "Check this out https://www.threads.net/@zuck/post/ABC123",
			expected: []string{"https://www.threads.net/@zuck/post/ABC123"},
		},
		{
			name:     "threads.com URL",
			text:     "Look at https://threads.com/@user.name/post/xyz-789",
			expected: []string{"https://threads.com/@user.name/post/xyz-789"},
		},
		{
			name:     "no www prefix",
			text:     "https://threads.net/@someone/post/ABC123",
			expected: []string{"https://threads.net/@someone/post/ABC123"},
		},
		{
			name:     "multiple URLs",
			text:     "https://threads.net/@a/post/111 and https://threads.com/@b/post/222",
			expected: []string{"https://threads.net/@a/post/111", "https://threads.com/@b/post/222"},
		},
		{
			name:     "no Threads URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "deduplication",
			text:     "https://threads.net/@zuck/post/ABC https://threads.net/@zuck/post/ABC",
			expected: []string{"https://threads.net/@zuck/post/ABC"},
		},
		{
			name:     "mixed with Mastodon URL",
			text:     "https://mastodon.social/@user/12345 https://threads.net/@zuck/post/ABC",
			expected: []string{"https://threads.net/@zuck/post/ABC"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractThreadsURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseThreadsHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected *ThreadsPost
	}{
		{
			name: "full OG tags",
			html: `<html><head>
				<meta property="og:title" content="Mark Zuckerberg (@zuck) on Threads">
				<meta property="og:image" content="https://example.com/image.jpg">
				<meta property="og:url" content="https://www.threads.net/@zuck/post/ABC123">
				<meta property="og:description" content="This is a post about something cool">
			</head></html>`,
			expected: &ThreadsPost{
				Title:       "Mark Zuckerberg (@zuck) on Threads",
				ImageURL:    "https://example.com/image.jpg",
				URL:         "https://www.threads.net/@zuck/post/ABC123",
				Description: "This is a post about something cool",
			},
		},
		{
			name: "HTML entities decoded",
			html: `<html><head>
				<meta property="og:title" content="The Bulwark (&#064;bulwarkonline) on Threads">
				<meta property="og:image" content="https://example.com/img?a=1&amp;b=2">
				<meta property="og:url" content="https://www.threads.com/&#064;bulwarkonline/post/ABC">
				<meta property="og:description" content="Full post text &amp; more">
			</head></html>`,
			expected: &ThreadsPost{
				Title:       "The Bulwark (@bulwarkonline) on Threads",
				ImageURL:    "https://example.com/img?a=1&b=2",
				URL:         "https://www.threads.com/@bulwarkonline/post/ABC",
				Description: "Full post text & more",
			},
		},
		{
			name: "missing image",
			html: `<html><head>
				<meta property="og:title" content="User (@user) on Threads">
				<meta property="og:description" content="Text only post">
			</head></html>`,
			expected: &ThreadsPost{
				Title:       "User (@user) on Threads",
				Description: "Text only post",
			},
		},
		{
			name:     "no OG tags",
			html:     `<html><head><title>Page</title></head></html>`,
			expected: &ThreadsPost{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseThreadsHTML(tt.html)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFetchThreadsPost(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><head>
				<meta property="og:title" content="Test User (&#064;test) on Threads">
				<meta property="og:image" content="https://example.com/img.jpg?a=1&amp;b=2">
				<meta property="og:url" content="https://www.threads.net/@test/post/XYZ">
				<meta property="og:description" content="Hello world">
			</head></html>`))
		}))
		defer server.Close()

		post, err := fetchThreadsPost(server.URL)
		require.NoError(t, err)
		assert.Equal(t, "Test User (@test) on Threads", post.Title)
		assert.Equal(t, "Hello world", post.Description)
		assert.Equal(t, "https://example.com/img.jpg?a=1&b=2", post.ImageURL)
	})

	t.Run("404 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		_, err := fetchThreadsPost(server.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})

	t.Run("no OG tags", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><head><title>Empty</title></head></html>`))
		}))
		defer server.Close()

		_, err := fetchThreadsPost(server.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no OG metadata found")
	})
}

func TestBuildThreadsAttachment(t *testing.T) {
	t.Run("full post", func(t *testing.T) {
		post := &ThreadsPost{
			Title:       "Mark Zuckerberg (@zuck) on Threads",
			Description: "This is a cool post",
			ImageURL:    "https://example.com/image.jpg",
			URL:         "https://www.threads.net/@zuck/post/ABC123",
		}

		att := buildThreadsAttachment(post, "https://threads.net/@zuck/post/ABC123")

		assert.Equal(t, "#000000", att.Color)
		assert.Equal(t, "Mark Zuckerberg (@zuck)", att.AuthorName)
		assert.Equal(t, "https://www.threads.net/@zuck/post/ABC123", att.AuthorLink)
		assert.Equal(t, "@zuck", att.Title)
		assert.Equal(t, "https://www.threads.net/@zuck/post/ABC123", att.TitleLink)
		assert.Equal(t, "This is a cool post", att.Text)
		assert.Equal(t, "https://example.com/image.jpg", att.ImageURL)
		assert.Equal(t, "Threads Preview", att.Footer)
	})

	t.Run("post without image", func(t *testing.T) {
		post := &ThreadsPost{
			Title:       "Someone (@someone) on Threads",
			Description: "Text only",
			URL:         "https://www.threads.net/@someone/post/XYZ",
		}

		att := buildThreadsAttachment(post, "https://threads.net/@someone/post/XYZ")

		assert.Equal(t, "Someone (@someone)", att.AuthorName)
		assert.Equal(t, "@someone", att.Title)
		assert.Equal(t, "Text only", att.Text)
		assert.Empty(t, att.ImageURL)
	})

	t.Run("post without og:url uses original URL", func(t *testing.T) {
		post := &ThreadsPost{
			Title:       "User (@user) on Threads",
			Description: "Hello",
		}
		originalURL := "https://threads.net/@user/post/ABC"

		att := buildThreadsAttachment(post, originalURL)

		assert.Equal(t, originalURL, att.AuthorLink)
		assert.Equal(t, originalURL, att.TitleLink)
	})
}
