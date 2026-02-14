package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTwitterURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "x.com URL",
			text:     "Check this https://x.com/elonmusk/status/1234567890",
			expected: []string{"https://x.com/elonmusk/status/1234567890"},
		},
		{
			name:     "twitter.com URL",
			text:     "https://twitter.com/jack/status/9876543210",
			expected: []string{"https://twitter.com/jack/status/9876543210"},
		},
		{
			name:     "www.twitter.com URL",
			text:     "https://www.twitter.com/user/status/111",
			expected: []string{"https://www.twitter.com/user/status/111"},
		},
		{
			name:     "mobile.twitter.com URL",
			text:     "https://mobile.twitter.com/user/status/222",
			expected: []string{"https://mobile.twitter.com/user/status/222"},
		},
		{
			name:     "www.x.com URL",
			text:     "https://www.x.com/user/status/333",
			expected: []string{"https://www.x.com/user/status/333"},
		},
		{
			name:     "multiple URLs mixed platforms",
			text:     "https://x.com/a/status/111 and https://twitter.com/b/status/222",
			expected: []string{"https://twitter.com/b/status/222", "https://x.com/a/status/111"},
		},
		{
			name:     "URL with query params stripped",
			text:     "https://x.com/user/status/123?s=20&t=abc",
			expected: []string{"https://x.com/user/status/123"},
		},
		{
			name:     "no Twitter URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "deduplication",
			text:     "https://x.com/a/status/111 https://x.com/a/status/111",
			expected: []string{"https://x.com/a/status/111"},
		},
		{
			name:     "username with underscores",
			text:     "https://x.com/user_name_123/status/456",
			expected: []string{"https://x.com/user_name_123/status/456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTwitterURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTwitterURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantUsername  string
		wantTweetID  string
		wantOK       bool
	}{
		{
			name:        "x.com",
			url:         "https://x.com/elonmusk/status/1234567890",
			wantUsername: "elonmusk",
			wantTweetID: "1234567890",
			wantOK:      true,
		},
		{
			name:        "twitter.com",
			url:         "https://twitter.com/jack/status/9876543210",
			wantUsername: "jack",
			wantTweetID: "9876543210",
			wantOK:      true,
		},
		{
			name:   "invalid URL",
			url:    "https://example.com/not-twitter",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, tweetID, ok := parseTwitterURL(tt.url)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantUsername, username)
				assert.Equal(t, tt.wantTweetID, tweetID)
			}
		})
	}
}

func TestFetchTwitterPost(t *testing.T) {
	t.Run("successful fetch with text only", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/testuser/status/123", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"code": 200,
				"tweet": {
					"text": "Hello Twitter!",
					"author": {
						"name": "Test User",
						"screen_name": "testuser",
						"avatar_url": "https://pbs.twimg.com/avatar.jpg"
					},
					"likes": 42,
					"replies": 5,
					"retweets": 10,
					"quotes": 3
				}
			}`))
		}))
		defer server.Close()

		oldBase := twitterAPIBase
		twitterAPIBase = server.URL
		defer func() { twitterAPIBase = oldBase }()

		post, err := fetchTwitterPost("testuser", "123")
		require.NoError(t, err)
		assert.Equal(t, "Hello Twitter!", post.Text)
		assert.Equal(t, "Test User", post.Author.Name)
		assert.Equal(t, "testuser", post.Author.ScreenName)
		assert.Equal(t, "https://pbs.twimg.com/avatar.jpg", post.Author.AvatarURL)
		assert.Equal(t, 42, post.LikeCount)
		assert.Equal(t, 5, post.ReplyCount)
		assert.Equal(t, 10, post.RetweetCount)
		assert.Equal(t, 3, post.QuoteCount)
	})

	t.Run("with photos", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"code": 200,
				"tweet": {
					"text": "Check these photos!",
					"author": {
						"name": "Photographer",
						"screen_name": "photog",
						"avatar_url": ""
					},
					"media": {
						"photos": [
							{"url": "https://pbs.twimg.com/media/photo1.jpg", "altText": "First photo"},
							{"url": "https://pbs.twimg.com/media/photo2.jpg", "altText": "Second photo"}
						]
					},
					"likes": 100,
					"replies": 10,
					"retweets": 20,
					"quotes": 0
				}
			}`))
		}))
		defer server.Close()

		oldBase := twitterAPIBase
		twitterAPIBase = server.URL
		defer func() { twitterAPIBase = oldBase }()

		post, err := fetchTwitterPost("photog", "456")
		require.NoError(t, err)
		require.Len(t, post.Images, 2)
		assert.Equal(t, "https://pbs.twimg.com/media/photo1.jpg", post.Images[0].URL)
		assert.Equal(t, "First photo", post.Images[0].Alt)
		assert.Equal(t, "https://pbs.twimg.com/media/photo2.jpg", post.Images[1].URL)
		assert.Nil(t, post.Video)
	})

	t.Run("with video", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"code": 200,
				"tweet": {
					"text": "Watch this!",
					"author": {
						"name": "Video Creator",
						"screen_name": "vidcreator",
						"avatar_url": ""
					},
					"media": {
						"videos": [
							{"thumbnail_url": "https://pbs.twimg.com/thumb.jpg", "url": "https://video.twimg.com/vid.mp4"}
						]
					},
					"likes": 0,
					"replies": 0,
					"retweets": 0,
					"quotes": 0
				}
			}`))
		}))
		defer server.Close()

		oldBase := twitterAPIBase
		twitterAPIBase = server.URL
		defer func() { twitterAPIBase = oldBase }()

		post, err := fetchTwitterPost("vidcreator", "789")
		require.NoError(t, err)
		require.NotNil(t, post.Video)
		assert.Equal(t, "https://pbs.twimg.com/thumb.jpg", post.Video.ThumbnailURL)
		assert.Equal(t, "https://video.twimg.com/vid.mp4", post.Video.URL)
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldBase := twitterAPIBase
		twitterAPIBase = server.URL
		defer func() { twitterAPIBase = oldBase }()

		_, err := fetchTwitterPost("nobody", "999")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})
}

func TestBuildTwitterAttachment(t *testing.T) {
	t.Run("text-only tweet", func(t *testing.T) {
		post := &TwitterPost{
			Text:         "Hello world!",
			ReplyCount:   5,
			RetweetCount: 10,
			LikeCount:    42,
			Author: TwitterAuthor{
				Name:       "Test User",
				ScreenName: "testuser",
				AvatarURL:  "https://pbs.twimg.com/avatar.jpg",
			},
		}

		att := buildTwitterAttachment(post, "https://x.com/testuser/status/123", nil)

		assert.Equal(t, "#000000", att.Color)
		assert.Equal(t, "Test User", att.AuthorName)
		assert.Equal(t, "https://x.com/testuser", att.AuthorLink)
		assert.Equal(t, "https://pbs.twimg.com/avatar.jpg", att.AuthorIcon)
		assert.Equal(t, "@testuser", att.Title)
		assert.Equal(t, "https://x.com/testuser/status/123", att.TitleLink)
		assert.Equal(t, "Hello world!", att.Text)
		assert.Equal(t, "X Preview", att.Footer)

		// Engagement metrics shown by default
		require.Len(t, att.Fields, 3)
		assert.Equal(t, "Replies", att.Fields[0].Title)
		assert.Equal(t, "5", att.Fields[0].Value)
		assert.Equal(t, "Retweets", att.Fields[1].Title)
		assert.Equal(t, "10", att.Fields[1].Value)
		assert.Equal(t, "Likes", att.Fields[2].Title)
		assert.Equal(t, "42", att.Fields[2].Value)
	})

	t.Run("tweet with images", func(t *testing.T) {
		post := &TwitterPost{
			Text: "Photos!",
			Images: []TwitterMedia{
				{URL: "https://pbs.twimg.com/photo1.jpg"},
				{URL: "https://pbs.twimg.com/photo2.jpg"},
			},
			Author: TwitterAuthor{Name: "Photo", ScreenName: "photo"},
		}

		att := buildTwitterAttachment(post, "https://x.com/photo/status/456", nil)

		assert.Equal(t, "https://pbs.twimg.com/photo1.jpg", att.ImageURL)
		// Should have engagement (3) + images field (1)
		require.Len(t, att.Fields, 4)
		assert.Equal(t, "📎 2 Images", att.Fields[3].Title)
	})

	t.Run("tweet with video", func(t *testing.T) {
		post := &TwitterPost{
			Text: "Video tweet",
			Video: &TwitterVideo{
				ThumbnailURL: "https://pbs.twimg.com/thumb.jpg",
				URL:          "https://video.twimg.com/vid.mp4",
			},
			Author: TwitterAuthor{Name: "Vid", ScreenName: "vid"},
		}

		att := buildTwitterAttachment(post, "https://x.com/vid/status/789", nil)

		assert.Equal(t, "https://pbs.twimg.com/thumb.jpg", att.ImageURL)
		// Should have video link (1) + engagement (3)
		require.Len(t, att.Fields, 4)
		assert.Equal(t, "🎬 Video", att.Fields[0].Title)
	})

	t.Run("engagement metrics disabled", func(t *testing.T) {
		showMetrics := false
		config := &configuration{ShowEngagementMetrics: &showMetrics}
		post := &TwitterPost{
			Text:      "No metrics",
			LikeCount: 100,
			Author:    TwitterAuthor{Name: "Test", ScreenName: "test"},
		}

		att := buildTwitterAttachment(post, "https://x.com/test/status/123", config)

		assert.Empty(t, att.Fields)
	})
}
