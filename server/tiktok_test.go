package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
			_, _ = w.Write([]byte(`{
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

	t.Run("retries overload 503 then succeeds", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("overload-protect triggered"))
				return
			}
			_, _ = w.Write([]byte(`{"title": "ok", "author_unique_id": "user123"}`))
		}))
		defer server.Close()

		oldBase, oldDelay := tiktokOEmbedBase, tiktokRetryDelay
		tiktokOEmbedBase, tiktokRetryDelay = server.URL, 0
		defer func() { tiktokOEmbedBase, tiktokRetryDelay = oldBase, oldDelay }()

		oembed, err := fetchTikTokOEmbed("https://www.tiktok.com/@user123/video/123")
		require.NoError(t, err)
		assert.Equal(t, "ok", oembed.Title)
		assert.Equal(t, 3, calls)
	})

	t.Run("gives up after max attempts", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		oldBase, oldDelay := tiktokOEmbedBase, tiktokRetryDelay
		tiktokOEmbedBase, tiktokRetryDelay = server.URL, 0
		defer func() { tiktokOEmbedBase, tiktokRetryDelay = oldBase, oldDelay }()

		_, err := fetchTikTokOEmbed("https://www.tiktok.com/@user123/video/123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 503")
		assert.Equal(t, tiktokOEmbedAttempts, calls)
	})

	t.Run("iframe-only response is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"version":"1.0","type":"video","html":"<iframe></iframe>","provider_name":"TikTok"}`))
		}))
		defer server.Close()

		oldBase := tiktokOEmbedBase
		tiktokOEmbedBase = server.URL
		defer func() { tiktokOEmbedBase = oldBase }()

		_, err := fetchTikTokOEmbed("https://www.tiktok.com/@user123/video/123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no author data")
	})
}

func tiktokPageServer(t *testing.T, state string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head></head><body><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">` + state + `</script></body></html>`))
	}))
}

func TestFetchTikTokPage(t *testing.T) {
	t.Run("video", func(t *testing.T) {
		server := tiktokPageServer(t, `{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":0,"itemInfo":{"itemStruct":{
			"desc":"Cool video #fyp",
			"author":{"uniqueId":"user123","nickname":"Lucky Bii"},
			"video":{"cover":"https://p16-sign.tiktokcdn.com/cover.jpg"}}}}}}`)
		defer server.Close()

		data, err := fetchTikTokPage(server.URL)
		require.NoError(t, err)
		assert.Equal(t, "Cool video #fyp", data.Title)
		assert.Equal(t, "Lucky Bii", data.AuthorName)
		assert.Equal(t, "user123", data.AuthorID)
		assert.Equal(t, "https://www.tiktok.com/@user123", data.AuthorURL)
		assert.Equal(t, "https://p16-sign.tiktokcdn.com/cover.jpg", data.ThumbnailURL)
	})

	t.Run("photo post uses first image", func(t *testing.T) {
		server := tiktokPageServer(t, `{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":0,"itemInfo":{"itemStruct":{
			"desc":"slides",
			"author":{"uniqueId":"user123","nickname":"Lucky Bii"},
			"video":{"cover":""},
			"imagePost":{"images":[{"imageURL":{"urlList":["https://example.com/1.jpg","https://example.com/1b.jpg"]}}]}}}}}}`)
		defer server.Close()

		data, err := fetchTikTokPage(server.URL)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/1.jpg", data.ThumbnailURL)
	})

	// Photo pages lack webapp.video-detail; the same ID under /video/ has it.
	photoMux := func(hits *[]string) *httptest.Server {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*hits = append(*hits, r.URL.String())
			switch {
			case r.URL.Path == "/ZSshort/":
				http.Redirect(w, r, server.URL+"/@user123/photo/7712206707547507989?_r=1&_t=abc", http.StatusFound)
			case strings.Contains(r.URL.Path, "/photo/"):
				_, _ = w.Write([]byte(`<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.app-context":{}}}</script>`))
			case r.URL.Path == "/@user123/video/7712206707547507989" && r.URL.RawQuery == "":
				_, _ = w.Write([]byte(`<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":0,"itemInfo":{"itemStruct":{
					"desc":"photo desc","author":{"uniqueId":"user123","nickname":"Lucky Bii"},"video":{"cover":"https://example.com/cover.jpg"},
					"imagePost":{"images":[{"imageURL":{"urlList":["https://example.com/1.jpg"]}}]}}}}}}</script>`))
			default:
				http.NotFound(w, r)
			}
		}))
		return server
	}

	t.Run("direct photo URL fetched as video", func(t *testing.T) {
		var hits []string
		server := photoMux(&hits)
		defer server.Close()

		data, err := fetchTikTokPage(server.URL + "/@user123/photo/7712206707547507989?_r=1")
		require.NoError(t, err)
		assert.Equal(t, "photo desc", data.Title)
		assert.Equal(t, "https://example.com/1.jpg", data.ThumbnailURL)
		assert.Equal(t, []string{"/@user123/video/7712206707547507989"}, hits)
	})

	t.Run("short link redirecting to photo refetched as video", func(t *testing.T) {
		var hits []string
		server := photoMux(&hits)
		defer server.Close()

		data, err := fetchTikTokPage(server.URL + "/ZSshort/")
		require.NoError(t, err)
		assert.Equal(t, "user123", data.AuthorID)
		assert.Equal(t, "https://example.com/1.jpg", data.ThumbnailURL)
		assert.Len(t, hits, 3)
	})

	t.Run("unavailable video", func(t *testing.T) {
		server := tiktokPageServer(t, `{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"statusCode":10204,"itemInfo":{}}}}`)
		defer server.Close()

		_, err := fetchTikTokPage(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 10204")
	})

	t.Run("no rehydration script", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<html>captcha</html>`))
		}))
		defer server.Close()

		_, err := fetchTikTokPage(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rehydration data")
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
