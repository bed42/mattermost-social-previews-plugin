package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractBlueskyURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "standard URL with handle",
			text:     "Check this https://bsky.app/profile/alice.bsky.social/post/3abc123",
			expected: []string{"https://bsky.app/profile/alice.bsky.social/post/3abc123"},
		},
		{
			name:     "URL with DID",
			text:     "https://bsky.app/profile/did:plc:abc123def/post/3xyz789",
			expected: []string{"https://bsky.app/profile/did:plc:abc123def/post/3xyz789"},
		},
		{
			name:     "custom domain handle",
			text:     "https://bsky.app/profile/jay.bsky.team/post/3abc",
			expected: []string{"https://bsky.app/profile/jay.bsky.team/post/3abc"},
		},
		{
			name:     "multiple URLs",
			text:     "https://bsky.app/profile/a.bsky.social/post/111 and https://bsky.app/profile/b.bsky.social/post/222",
			expected: []string{"https://bsky.app/profile/a.bsky.social/post/111", "https://bsky.app/profile/b.bsky.social/post/222"},
		},
		{
			name:     "no Bluesky URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "deduplication",
			text:     "https://bsky.app/profile/a.bsky.social/post/111 https://bsky.app/profile/a.bsky.social/post/111",
			expected: []string{"https://bsky.app/profile/a.bsky.social/post/111"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBlueskyURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseBlueskyURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantHandle string
		wantRkey   string
		wantOK     bool
	}{
		{
			name:       "standard handle",
			url:        "https://bsky.app/profile/alice.bsky.social/post/3abc123",
			wantHandle: "alice.bsky.social",
			wantRkey:   "3abc123",
			wantOK:     true,
		},
		{
			name:       "DID",
			url:        "https://bsky.app/profile/did:plc:abc123/post/3xyz",
			wantHandle: "did:plc:abc123",
			wantRkey:   "3xyz",
			wantOK:     true,
		},
		{
			name:   "invalid URL",
			url:    "https://example.com/not-bluesky",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle, rkey, ok := parseBlueskyURL(tt.url)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantHandle, handle)
				assert.Equal(t, tt.wantRkey, rkey)
			}
		})
	}
}

func TestFetchBlueskyPost(t *testing.T) {
	t.Run("successful fetch with DID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"thread": {
					"$type": "app.bsky.feed.defs#threadViewPost",
					"post": {
						"uri": "at://did:plc:test123/app.bsky.feed.post/3abc",
						"cid": "bafytest",
						"author": {
							"did": "did:plc:test123",
							"handle": "alice.bsky.social",
							"displayName": "Alice",
							"avatar": "https://cdn.bsky.app/avatar.jpg"
						},
						"record": {
							"$type": "app.bsky.feed.post",
							"text": "Hello from Bluesky!",
							"createdAt": "2024-01-01T00:00:00.000Z"
						},
						"likeCount": 42,
						"replyCount": 5,
						"repostCount": 10,
						"quoteCount": 3
					}
				}
			}`))
		}))
		defer server.Close()

		oldBase := blueskyAPIBase
		blueskyAPIBase = server.URL
		defer func() { blueskyAPIBase = oldBase }()

		post, err := fetchBlueskyPost("did:plc:test123", "3abc")
		require.NoError(t, err)
		assert.Equal(t, "Hello from Bluesky!", post.Text)
		assert.Equal(t, "Alice", post.Author.DisplayName)
		assert.Equal(t, "alice.bsky.social", post.Author.Handle)
		assert.Equal(t, 42, post.LikeCount)
		assert.Equal(t, 5, post.ReplyCount)
		assert.Equal(t, 10, post.RepostCount)
	})

	t.Run("successful fetch with handle resolution", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.URL.Path == "/xrpc/com.atproto.identity.resolveHandle" {
				w.Write([]byte(`{"did": "did:plc:resolved123"}`))
				return
			}

			w.Write([]byte(`{
				"thread": {
					"post": {
						"uri": "at://did:plc:resolved123/app.bsky.feed.post/3abc",
						"cid": "bafytest",
						"author": {
							"did": "did:plc:resolved123",
							"handle": "bob.bsky.social",
							"displayName": "Bob",
							"avatar": ""
						},
						"record": {
							"text": "Test post",
							"createdAt": "2024-01-01T00:00:00.000Z"
						},
						"likeCount": 0,
						"replyCount": 0,
						"repostCount": 0,
						"quoteCount": 0
					}
				}
			}`))
		}))
		defer server.Close()

		oldBase := blueskyAPIBase
		blueskyAPIBase = server.URL
		defer func() { blueskyAPIBase = oldBase }()

		post, err := fetchBlueskyPost("bob.bsky.social", "3abc")
		require.NoError(t, err)
		assert.Equal(t, "Test post", post.Text)
		assert.Equal(t, "Bob", post.Author.DisplayName)
	})

	t.Run("with image embeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"thread": {
					"post": {
						"uri": "at://did:plc:test/app.bsky.feed.post/3abc",
						"cid": "bafytest",
						"author": {
							"did": "did:plc:test",
							"handle": "photo.bsky.social",
							"displayName": "Photographer",
							"avatar": ""
						},
						"record": {
							"text": "Nice photo!",
							"createdAt": "2024-01-01T00:00:00.000Z"
						},
						"embed": {
							"$type": "app.bsky.embed.images#view",
							"images": [
								{"thumb": "https://cdn.bsky.app/thumb1.jpg", "fullsize": "https://cdn.bsky.app/full1.jpg", "alt": "A sunset"},
								{"thumb": "https://cdn.bsky.app/thumb2.jpg", "fullsize": "https://cdn.bsky.app/full2.jpg", "alt": "A sunrise"}
							]
						},
						"likeCount": 100,
						"replyCount": 5,
						"repostCount": 20,
						"quoteCount": 2
					}
				}
			}`))
		}))
		defer server.Close()

		oldBase := blueskyAPIBase
		blueskyAPIBase = server.URL
		defer func() { blueskyAPIBase = oldBase }()

		post, err := fetchBlueskyPost("did:plc:test", "3abc")
		require.NoError(t, err)
		assert.Equal(t, "Nice photo!", post.Text)
		require.Len(t, post.Images, 2)
		assert.Equal(t, "https://cdn.bsky.app/full1.jpg", post.Images[0].Fullsize)
		assert.Equal(t, "A sunset", post.Images[0].Alt)
		assert.Equal(t, "https://cdn.bsky.app/full2.jpg", post.Images[1].Fullsize)
	})

	t.Run("with external embed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"thread": {
					"post": {
						"uri": "at://did:plc:test/app.bsky.feed.post/3abc",
						"cid": "bafytest",
						"author": {
							"did": "did:plc:test",
							"handle": "news.bsky.social",
							"displayName": "News Bot",
							"avatar": ""
						},
						"record": {
							"text": "Interesting article",
							"createdAt": "2024-01-01T00:00:00.000Z"
						},
						"embed": {
							"$type": "app.bsky.embed.external#view",
							"external": {
								"uri": "https://example.com/article",
								"title": "Big News",
								"description": "Something happened",
								"thumb": "https://example.com/thumb.jpg"
							}
						},
						"likeCount": 0,
						"replyCount": 0,
						"repostCount": 0,
						"quoteCount": 0
					}
				}
			}`))
		}))
		defer server.Close()

		oldBase := blueskyAPIBase
		blueskyAPIBase = server.URL
		defer func() { blueskyAPIBase = oldBase }()

		post, err := fetchBlueskyPost("did:plc:test", "3abc")
		require.NoError(t, err)
		require.NotNil(t, post.External)
		assert.Equal(t, "https://example.com/article", post.External.URI)
		assert.Equal(t, "Big News", post.External.Title)
		assert.Equal(t, "https://example.com/thumb.jpg", post.External.Thumb)
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldBase := blueskyAPIBase
		blueskyAPIBase = server.URL
		defer func() { blueskyAPIBase = oldBase }()

		_, err := fetchBlueskyPost("did:plc:nonexistent", "3abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})
}

func TestBuildBlueskyAttachment(t *testing.T) {
	t.Run("text-only post", func(t *testing.T) {
		post := &BlueskyPost{
			Text:        "Hello world!",
			ReplyCount:  5,
			RepostCount: 10,
			LikeCount:   42,
			Author: BlueskyAuthor{
				Handle:      "alice.bsky.social",
				DisplayName: "Alice",
				Avatar:      "https://cdn.bsky.app/avatar.jpg",
			},
		}

		att := buildBlueskyAttachment(post, "https://bsky.app/profile/alice.bsky.social/post/3abc", nil)

		assert.Equal(t, "#0085FF", att.Color)
		assert.Equal(t, "Alice", att.AuthorName)
		assert.Equal(t, "https://bsky.app/profile/alice.bsky.social", att.AuthorLink)
		assert.Equal(t, "https://cdn.bsky.app/avatar.jpg", att.AuthorIcon)
		assert.Equal(t, "@alice.bsky.social", att.Title)
		assert.Equal(t, "Hello world!", att.Text)
		assert.Equal(t, "Bluesky Preview", att.Footer)

		// Engagement metrics shown by default
		require.Len(t, att.Fields, 3)
		assert.Equal(t, "Replies", att.Fields[0].Title)
		assert.Equal(t, "5", att.Fields[0].Value)
		assert.Equal(t, "Reposts", att.Fields[1].Title)
		assert.Equal(t, "10", att.Fields[1].Value)
		assert.Equal(t, "Likes", att.Fields[2].Title)
		assert.Equal(t, "42", att.Fields[2].Value)
	})

	t.Run("post with images", func(t *testing.T) {
		post := &BlueskyPost{
			Text: "Photos!",
			Images: []BlueskyImage{
				{Fullsize: "https://cdn.bsky.app/full1.jpg", Thumb: "https://cdn.bsky.app/thumb1.jpg"},
				{Fullsize: "https://cdn.bsky.app/full2.jpg", Thumb: "https://cdn.bsky.app/thumb2.jpg"},
			},
			Author: BlueskyAuthor{Handle: "photo.bsky.social", DisplayName: "Photo"},
		}

		att := buildBlueskyAttachment(post, "https://bsky.app/profile/photo.bsky.social/post/3abc", nil)

		assert.Equal(t, "https://cdn.bsky.app/full1.jpg", att.ImageURL)
		// Should have engagement (3) + images field (1)
		require.Len(t, att.Fields, 4)
		assert.Equal(t, "📎 2 Images", att.Fields[3].Title)
	})

	t.Run("post with video", func(t *testing.T) {
		post := &BlueskyPost{
			Text: "Video post",
			Video: &BlueskyVideo{
				Thumbnail: "https://cdn.bsky.app/video-thumb.jpg",
				Playlist:  "https://video.bsky.app/watch/playlist.m3u8",
			},
			Author: BlueskyAuthor{Handle: "vid.bsky.social", DisplayName: "Vid"},
		}

		att := buildBlueskyAttachment(post, "https://bsky.app/profile/vid.bsky.social/post/3abc", nil)

		assert.Equal(t, "https://cdn.bsky.app/video-thumb.jpg", att.ThumbURL)
		// Should have video link (1) + engagement (3)
		require.Len(t, att.Fields, 4)
		assert.Equal(t, "🎬 Video", att.Fields[0].Title)
	})

	t.Run("post with external link", func(t *testing.T) {
		post := &BlueskyPost{
			Text: "Check this out",
			External: &BlueskyExternal{
				URI:         "https://example.com/article",
				Title:       "Cool Article",
				Description: "An interesting read",
				Thumb:       "https://example.com/thumb.jpg",
			},
			Author: BlueskyAuthor{Handle: "news.bsky.social", DisplayName: "News"},
		}

		att := buildBlueskyAttachment(post, "https://bsky.app/profile/news.bsky.social/post/3abc", nil)

		// External thumb used as image since no other image
		assert.Equal(t, "https://example.com/thumb.jpg", att.ImageURL)
		// Should have engagement (3) + link preview (1)
		require.Len(t, att.Fields, 4)
		assert.Equal(t, "🔗 Link Preview", att.Fields[3].Title)
	})

	t.Run("engagement metrics disabled", func(t *testing.T) {
		showMetrics := false
		config := &configuration{ShowEngagementMetrics: &showMetrics}
		post := &BlueskyPost{
			Text:       "No metrics",
			LikeCount:  100,
			ReplyCount: 50,
			Author:     BlueskyAuthor{Handle: "test.bsky.social", DisplayName: "Test"},
		}

		att := buildBlueskyAttachment(post, "https://bsky.app/profile/test.bsky.social/post/3abc", config)

		assert.Empty(t, att.Fields)
	})
}
