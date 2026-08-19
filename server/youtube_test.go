package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractYouTubeURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "standard watch URL",
			text:     "Check this https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: []string{"https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		},
		{
			name:     "no www",
			text:     "https://youtube.com/watch?v=dQw4w9WgXcQ",
			expected: []string{"https://youtube.com/watch?v=dQw4w9WgXcQ"},
		},
		{
			name:     "mobile URL",
			text:     "https://m.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: []string{"https://m.youtube.com/watch?v=dQw4w9WgXcQ"},
		},
		{
			name:     "watch URL with extra params",
			text:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s",
			expected: []string{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s"},
		},
		{
			name:     "short link",
			text:     "Looks cool https://youtu.be/dtr5JL1zkiM",
			expected: []string{"https://youtu.be/dtr5JL1zkiM"},
		},
		{
			name:     "short link with share params",
			text:     "https://youtu.be/dtr5JL1zkiM?si=wt4pbFh_VdbCvArK",
			expected: []string{"https://youtu.be/dtr5JL1zkiM?si=wt4pbFh_VdbCvArK"},
		},
		{
			name:     "shorts URL",
			text:     "https://www.youtube.com/shorts/abc123def45",
			expected: []string{"https://www.youtube.com/shorts/abc123def45"},
		},
		{
			name:     "live URL",
			text:     "https://www.youtube.com/live/abc123def45",
			expected: []string{"https://www.youtube.com/live/abc123def45"},
		},
		{
			name:     "trailing punctuation stripped",
			text:     "Have you seen this? https://youtu.be/dtr5JL1zkiM, it's great",
			expected: []string{"https://youtu.be/dtr5JL1zkiM"},
		},
		{
			name:     "multiple URLs",
			text:     "https://youtu.be/aaaaaaaaaaa and https://youtu.be/bbbbbbbbbbb",
			expected: []string{"https://youtu.be/aaaaaaaaaaa", "https://youtu.be/bbbbbbbbbbb"},
		},
		{
			name:     "deduplication",
			text:     "https://youtu.be/dtr5JL1zkiM https://youtu.be/dtr5JL1zkiM",
			expected: []string{"https://youtu.be/dtr5JL1zkiM"},
		},
		{
			name:     "channel URL not matched",
			text:     "https://www.youtube.com/@MrBeast",
			expected: []string{},
		},
		{
			name:     "playlist URL not matched",
			text:     "https://www.youtube.com/playlist?list=PLabc123",
			expected: []string{},
		},
		{
			name:     "no YouTube URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractYouTubeURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFetchYouTubeOEmbed(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"title": "Never Gonna Give You Up",
				"author_name": "Rick Astley",
				"author_url": "https://www.youtube.com/@RickAstleyYT",
				"thumbnail_url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"
			}`))
		}))
		defer server.Close()

		oldBase := youtubeOEmbedBase
		youtubeOEmbedBase = server.URL
		defer func() { youtubeOEmbedBase = oldBase }()

		oembed, err := fetchYouTubeOEmbed("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
		require.NoError(t, err)
		assert.Equal(t, "Never Gonna Give You Up", oembed.Title)
		assert.Equal(t, "Rick Astley", oembed.AuthorName)
		assert.Equal(t, "https://www.youtube.com/@RickAstleyYT", oembed.AuthorURL)
		assert.Equal(t, "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", oembed.ThumbnailURL)
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldBase := youtubeOEmbedBase
		youtubeOEmbedBase = server.URL
		defer func() { youtubeOEmbedBase = oldBase }()

		_, err := fetchYouTubeOEmbed("https://www.youtube.com/watch?v=gone")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})
}

func TestBuildYouTubeAttachment(t *testing.T) {
	t.Run("full video", func(t *testing.T) {
		oembed := &YouTubeOEmbed{
			Title:        "Never Gonna Give You Up",
			AuthorName:   "Rick Astley",
			AuthorURL:    "https://www.youtube.com/@RickAstleyYT",
			ThumbnailURL: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg",
		}

		att := buildYouTubeAttachment(oembed, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")

		assert.Equal(t, "#FF0000", att.Color)
		assert.Equal(t, "Rick Astley", att.AuthorName)
		assert.Equal(t, "https://www.youtube.com/@RickAstleyYT", att.AuthorLink)
		assert.Equal(t, "Never Gonna Give You Up", att.Title)
		assert.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", att.TitleLink)
		assert.Equal(t, "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", att.ImageURL)
		assert.Equal(t, "YouTube Preview", att.Footer)
	})

	t.Run("video without thumbnail", func(t *testing.T) {
		oembed := &YouTubeOEmbed{
			Title:      "Some Video",
			AuthorName: "Someone",
			AuthorURL:  "https://www.youtube.com/@someone",
		}

		att := buildYouTubeAttachment(oembed, "https://youtu.be/abc123def45")

		assert.Equal(t, "Some Video", att.Title)
		assert.Empty(t, att.ImageURL)
	})
}
