package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRedditURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "www reddit with slug",
			text:     "Check this https://www.reddit.com/r/australian/comments/1tmz3zi/im_clare_oneil_australias_minister_for_housing/",
			expected: []string{"https://www.reddit.com/r/australian/comments/1tmz3zi/im_clare_oneil_australias_minister_for_housing/"},
		},
		{
			name:     "no www subdomain",
			text:     "https://reddit.com/r/aww/comments/abc123/",
			expected: []string{"https://reddit.com/r/aww/comments/abc123/"},
		},
		{
			name:     "old reddit",
			text:     "https://old.reddit.com/r/golang/comments/abc123/some_post/",
			expected: []string{"https://old.reddit.com/r/golang/comments/abc123/some_post/"},
		},
		{
			name:     "np reddit",
			text:     "https://np.reddit.com/r/golang/comments/abc123/",
			expected: []string{"https://np.reddit.com/r/golang/comments/abc123/"},
		},
		{
			name:     "redd.it shortlink",
			text:     "share via https://redd.it/1tmz3zi",
			expected: []string{"https://redd.it/1tmz3zi"},
		},
		{
			name:     "no slug, no trailing slash",
			text:     "https://www.reddit.com/r/golang/comments/abc123",
			expected: []string{"https://www.reddit.com/r/golang/comments/abc123"},
		},
		{
			name:     "deduplication",
			text:     "https://www.reddit.com/r/aww/comments/abc/ https://www.reddit.com/r/aww/comments/abc/",
			expected: []string{"https://www.reddit.com/r/aww/comments/abc/"},
		},
		{
			name:     "no reddit URLs",
			text:     "Just a normal message with https://example.com",
			expected: []string{},
		},
		{
			name:     "subreddit only URL is not matched",
			text:     "https://www.reddit.com/r/australian/",
			expected: []string{},
		},
		{
			name:     "share URL",
			text:     "Check https://www.reddit.com/r/australian/s/9Ww5Yb4iLG",
			expected: []string{"https://www.reddit.com/r/australian/s/9Ww5Yb4iLG"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRedditURLs(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRedditURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantSubreddit string
		wantPostID    string
		wantOK        bool
	}{
		{
			name:          "canonical with slug",
			url:           "https://www.reddit.com/r/australian/comments/1tmz3zi/im_clare_oneil_australias_minister_for_housing/",
			wantSubreddit: "australian",
			wantPostID:    "1tmz3zi",
			wantOK:        true,
		},
		{
			name:          "old reddit",
			url:           "https://old.reddit.com/r/golang/comments/abc123/some_post/",
			wantSubreddit: "golang",
			wantPostID:    "abc123",
			wantOK:        true,
		},
		{
			name:       "redd.it shortlink — no subreddit",
			url:        "https://redd.it/1tmz3zi",
			wantPostID: "1tmz3zi",
			wantOK:     true,
		},
		{
			name:   "not a reddit URL",
			url:    "https://example.com/foo",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, postID, ok := parseRedditURL(tt.url)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantSubreddit, sub)
				assert.Equal(t, tt.wantPostID, postID)
			}
		})
	}
}

func TestFetchRedditPost(t *testing.T) {
	t.Run("self post with selftext and subreddit icon", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/comments/1tmz3zi.json", r.URL.Path)
			assert.Contains(t, r.URL.RawQuery, "sr_detail=1")
			assert.Contains(t, r.URL.RawQuery, "raw_json=1")
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"data": {"children": [
					{"kind": "t3", "data": {
						"title": "I'm Clare O'Neil",
						"author": "clareoneilmp",
						"subreddit": "australian",
						"subreddit_name_prefixed": "r/australian",
						"selftext": "Hi Reddit. I'm Clare.",
						"permalink": "/r/australian/comments/1tmz3zi/post/",
						"score": 55,
						"num_comments": 174,
						"is_self": true,
						"link_flair_text": "AMA: Live",
						"sr_detail": {
							"community_icon": "https://styles.redditmedia.com/icon.png",
							"icon_img": "",
							"display_name_prefixed": "r/australian"
						},
						"media_metadata": {
							"xyz1": {"status": "valid", "s": {"u": "https://preview.redd.it/xyz1.jpg?width=3024"}}
						}
					}}
				]}},
				{"data": {"children": []}}
			]`))
		}))
		defer server.Close()

		oldBase := redditAPIBase
		redditAPIBase = server.URL
		defer func() { redditAPIBase = oldBase }()

		post, err := fetchRedditPost("1tmz3zi")
		require.NoError(t, err)
		assert.Equal(t, "I'm Clare O'Neil", post.Title)
		assert.Equal(t, "clareoneilmp", post.Author)
		assert.Equal(t, "r/australian", post.Subreddit)
		assert.Equal(t, "https://styles.redditmedia.com/icon.png", post.SubredditIcon)
		assert.Equal(t, "Hi Reddit. I'm Clare.", post.Selftext)
		assert.Equal(t, "https://www.reddit.com/r/australian/comments/1tmz3zi/post/", post.Permalink)
		assert.Equal(t, 55, post.Score)
		assert.Equal(t, 174, post.NumComments)
		assert.True(t, post.IsSelf)
		assert.Equal(t, "AMA: Live", post.LinkFlair)
		assert.Equal(t, "https://preview.redd.it/xyz1.jpg?width=3024", post.ImageURL)
		assert.Empty(t, post.LinkURL)
	})

	t.Run("link post uses url_overridden_by_dest and preview image", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"data": {"children": [
					{"kind": "t3", "data": {
						"title": "Cool article",
						"author": "linkposter",
						"subreddit": "news",
						"subreddit_name_prefixed": "r/news",
						"permalink": "/r/news/comments/abc/cool/",
						"url_overridden_by_dest": "https://example.com/article",
						"score": 1234,
						"num_comments": 56,
						"is_self": false,
						"preview": {"images": [{"source": {"url": "https://external-preview.redd.it/img.jpg"}}]}
					}}
				]}},
				{"data": {"children": []}}
			]`))
		}))
		defer server.Close()

		oldBase := redditAPIBase
		redditAPIBase = server.URL
		defer func() { redditAPIBase = oldBase }()

		post, err := fetchRedditPost("abc")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/article", post.LinkURL)
		assert.Equal(t, "https://external-preview.redd.it/img.jpg", post.ImageURL)
		assert.False(t, post.IsSelf)
	})

	t.Run("falls back to icon_img when community_icon empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"data": {"children": [
					{"kind": "t3", "data": {
						"title": "Post",
						"subreddit_name_prefixed": "r/sub",
						"is_self": true,
						"sr_detail": {"community_icon": "", "icon_img": "https://b.thumbs.redditmedia.com/icon.png"}
					}}
				]}},
				{"data": {"children": []}}
			]`))
		}))
		defer server.Close()

		oldBase := redditAPIBase
		redditAPIBase = server.URL
		defer func() { redditAPIBase = oldBase }()

		post, err := fetchRedditPost("xyz")
		require.NoError(t, err)
		assert.Equal(t, "https://b.thumbs.redditmedia.com/icon.png", post.SubredditIcon)
	})

	t.Run("404 returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldBase := redditAPIBase
		redditAPIBase = server.URL
		defer func() { redditAPIBase = oldBase }()

		_, err := fetchRedditPost("missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})
}

func TestBuildRedditAttachment(t *testing.T) {
	t.Run("self post renders title, body, subreddit and footer", func(t *testing.T) {
		post := &RedditPost{
			Title:         "I'm Clare O'Neil",
			Author:        "clareoneilmp",
			Subreddit:     "r/australian",
			SubredditIcon: "https://styles.redditmedia.com/icon.png",
			Selftext:      "Hi Reddit.",
			Permalink:     "https://www.reddit.com/r/australian/comments/1tmz3zi/post/",
			Score:         55,
			NumComments:   174,
			IsSelf:        true,
			LinkFlair:     "AMA: Live",
			ImageURL:      "https://preview.redd.it/img.jpg",
		}

		att := buildRedditAttachment(post, "https://www.reddit.com/r/australian/comments/1tmz3zi/")

		assert.Equal(t, "#FF4500", att.Color)
		assert.Equal(t, "r/australian", att.AuthorName)
		assert.Equal(t, "https://www.reddit.com/r/australian", att.AuthorLink)
		assert.Equal(t, "https://styles.redditmedia.com/icon.png", att.AuthorIcon)
		assert.Equal(t, "[AMA: Live] I'm Clare O'Neil", att.Title)
		assert.Equal(t, "https://www.reddit.com/r/australian/comments/1tmz3zi/post/", att.TitleLink)
		assert.Equal(t, "Hi Reddit.", att.Text)
		assert.Equal(t, "https://preview.redd.it/img.jpg", att.ImageURL)
		assert.Contains(t, att.Footer, "Reddit")
		assert.Contains(t, att.Footer, "⬆ 55")
		assert.Contains(t, att.Footer, "💬 174")
		assert.Contains(t, att.Footer, "u/clareoneilmp")
		assert.Empty(t, att.Fields)
	})

	t.Run("link post adds external link field", func(t *testing.T) {
		post := &RedditPost{
			Title:       "News article",
			Author:      "linker",
			Subreddit:   "r/news",
			Permalink:   "https://www.reddit.com/r/news/comments/abc/",
			LinkURL:     "https://example.com/article",
			Score:       1234,
			NumComments: 56,
		}

		att := buildRedditAttachment(post, "https://www.reddit.com/r/news/comments/abc/")

		require.Len(t, att.Fields, 1)
		assert.Equal(t, "🔗 Link", att.Fields[0].Title)
		assert.Equal(t, "https://example.com/article", att.Fields[0].Value)
	})

	t.Run("nsfw post hides image and prefixes title", func(t *testing.T) {
		post := &RedditPost{
			Title:     "Saucy",
			Subreddit: "r/nsfw",
			Permalink: "https://www.reddit.com/r/nsfw/comments/abc/",
			Over18:    true,
			ImageURL:  "https://preview.redd.it/spicy.jpg",
		}

		att := buildRedditAttachment(post, "https://www.reddit.com/r/nsfw/comments/abc/")

		assert.True(t, strings.HasPrefix(att.Title, "🔞 "))
		assert.Empty(t, att.ImageURL, "NSFW image should be suppressed")
	})

	t.Run("long selftext is truncated", func(t *testing.T) {
		long := strings.Repeat("word ", 200) // 1000 chars
		post := &RedditPost{
			Title:     "Long",
			Subreddit: "r/long",
			Permalink: "https://www.reddit.com/r/long/comments/abc/",
			Selftext:  long,
		}

		att := buildRedditAttachment(post, "https://www.reddit.com/r/long/comments/abc/")
		assert.True(t, strings.HasSuffix(att.Text, "…"), "long selftext should be truncated with an ellipsis")
	})
}

func TestStripRedditMarkdown(t *testing.T) {
	in := "Some intro text.\n\nhttps://preview.redd.it/img.jpg?width=3024&format=pjpg\n\nMore text."
	out := stripRedditMarkdown(in)
	assert.NotContains(t, out, "preview.redd.it")
	assert.Contains(t, out, "Some intro text.")
	assert.Contains(t, out, "More text.")
}

func TestBuildRedditAttachment_FooterWithoutMetrics(t *testing.T) {
	// oEmbed-derived posts have no score/comments; the footer should drop
	// those rather than display "⬆ 0 • 💬 0".
	post := &RedditPost{
		Title:     "Some title",
		Author:    "alice",
		Subreddit: "r/foo",
		Permalink: "https://www.reddit.com/r/foo/comments/abc/",
	}
	att := buildRedditAttachment(post, post.Permalink)
	assert.Equal(t, "Reddit • u/alice", att.Footer)
	assert.NotContains(t, att.Footer, "⬆")
	assert.NotContains(t, att.Footer, "💬")
}

func TestResolveRedditShareURL(t *testing.T) {
	t.Run("follows redirect to canonical post URL", func(t *testing.T) {
		// API mock that the share URL ultimately redirects to.
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/r/australian/comments/1tmz3zi/im_clare/", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()

		share := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/r/australian/s/abc", r.URL.Path)
			assert.Equal(t, "HEAD", r.Method)
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			http.Redirect(w, r, target.URL+"/r/australian/comments/1tmz3zi/im_clare/", http.StatusTemporaryRedirect)
		}))
		defer share.Close()

		final, err := resolveRedditShareURL(share.URL + "/r/australian/s/abc")
		require.NoError(t, err)
		assert.Equal(t, target.URL+"/r/australian/comments/1tmz3zi/im_clare/", final)
	})

	t.Run("returns error when no redirect happens", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		_, err := resolveRedditShareURL(server.URL + "/r/australian/s/abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did not redirect")
	})
}

func TestFetchRedditPostFromURL_DirectURL(t *testing.T) {
	// Verifies the happy path: a canonical /comments/ URL skips the share
	// resolver entirely and goes straight to the JSON fetch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/comments/abc.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"data": {"children": [
				{"kind": "t3", "data": {
					"title": "Direct post",
					"subreddit_name_prefixed": "r/golang",
					"is_self": true
				}}
			]}},
			{"data": {"children": []}}
		]`))
	}))
	defer server.Close()

	oldBase := redditAPIBase
	redditAPIBase = server.URL
	defer func() { redditAPIBase = oldBase }()

	post, err := fetchRedditPostFromURL("https://www.reddit.com/r/golang/comments/abc/some_post/")
	require.NoError(t, err)
	assert.Equal(t, "Direct post", post.Title)
}

func TestFetchRedditOEmbed_ParsesResponse(t *testing.T) {
	// Exercises the parsing logic without going through the network — we just
	// hit a local mock that pretends to be Reddit's oembed.
	mux := http.NewServeMux()
	mux.HandleFunc("/oembed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"title": "Resolved title",
			"author_name": "alice",
			"html": "<blockquote><a href=\"https://www.reddit.com/r/golang/comments/abc/post/\">Resolved title</a> by <a href=\"https://www.reddit.com/user/alice/\">u/alice</a> in <a href=\"https://www.reddit.com/r/golang/\">golang</a></blockquote>"
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Direct HTTP call against the mock + reuse the parsing path.
	resp, err := http.Get(srv.URL + "/oembed?url=" + url.QueryEscape("https://www.reddit.com/r/golang/comments/abc/post/"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var oe struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		HTML       string `json:"html"`
	}
	require.NoError(t, json.Unmarshal(body, &oe))

	assert.Equal(t, "Resolved title", oe.Title)
	assert.Equal(t, "alice", oe.AuthorName)

	m := oembedSubredditRe.FindStringSubmatch(oe.HTML)
	require.Len(t, m, 2)
	assert.Equal(t, "golang", m[1])
}

func TestFormatRedditCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{12300, "12.3k"},
		{1_234_000, "1.2m"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatRedditCount(tt.n))
	}
}
