package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractActivityPubURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "pleroma/akkoma/mangane object",
			text:     "look https://atomicpoet.org/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd wow",
			expected: []string{"https://atomicpoet.org/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd"},
		},
		{
			name:     "pleroma notice",
			text:     "https://atomicpoet.org/notice/BAvFGvu3d0efp1JabY",
			expected: []string{"https://atomicpoet.org/notice/BAvFGvu3d0efp1JabY"},
		},
		{
			name:     "gotosocial status",
			text:     "https://gts.example/@alice/statuses/01HXYZABCDEFGHJKMNPQRSTVWX",
			expected: []string{"https://gts.example/@alice/statuses/01HXYZABCDEFGHJKMNPQRSTVWX"},
		},
		{
			name:     "friendica display",
			text:     "https://friendica.example/display/a1b2c3d4-1234-5678",
			expected: []string{"https://friendica.example/display/a1b2c3d4-1234-5678"},
		},
		{
			name:     "lemmy post and comment",
			text:     "https://lemmy.world/post/123 and https://lemmy.world/comment/456",
			expected: []string{"https://lemmy.world/post/123", "https://lemmy.world/comment/456"},
		},
		{
			name:     "deduplication",
			text:     "https://a.example/notice/abc https://a.example/notice/abc",
			expected: []string{"https://a.example/notice/abc"},
		},
		{
			name:     "mastodon URL not matched",
			text:     "https://mastodon.social/@user/123456",
			expected: []string{},
		},
		{
			name:     "non-uuid objects path not matched",
			text:     "https://example.com/objects/foo",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractActivityPubURLs(tt.text))
		})
	}
}

func TestAPLink(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{"empty", ``, ""},
		{"string", `"https://a.example/x"`, "https://a.example/x"},
		{"link object", `{"type":"Link","href":"https://a.example/x"}`, "https://a.example/x"},
		{"image object", `{"type":"Image","url":"https://a.example/i.png"}`, "https://a.example/i.png"},
		{"array of strings", `["https://a.example/1","https://a.example/2"]`, "https://a.example/1"},
		{
			name:     "array prefers html link",
			raw:      `[{"type":"Link","mediaType":"video/mp4","href":"https://a.example/v.mp4"},{"type":"Link","mediaType":"text/html","href":"https://a.example/watch"}]`,
			expected: "https://a.example/watch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, apLink(json.RawMessage(tt.raw)))
		})
	}
}

func TestFetchActivityPubObjectAndActor(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Accept"), "application/activity+json")
		w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
		switch r.URL.Path {
		case "/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd":
			fmt.Fprintf(w, `{"type":"Note","id":"%[1]s/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd","attributedTo":"%[1]s/users/poet","content":"<p>Hello &amp; welcome</p>","attachment":[{"type":"Document","mediaType":"image/png","url":"%[1]s/media/a.png"}]}`, server.URL)
		case "/users/poet":
			fmt.Fprintf(w, `{"type":"Person","id":"%[1]s/users/poet","name":"Poet","preferredUsername":"poet","url":"%[1]s/users/poet","icon":{"type":"Image","url":"%[1]s/media/me.jpg"}}`, server.URL)
		case "/objects/html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html></html>")
		case "/objects/person":
			fmt.Fprint(w, `{"type":"Person","id":"x"}`)
		case "/objects/private":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Run("note with actor", func(t *testing.T) {
		obj, err := fetchActivityPubObject(server.URL + "/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd")
		require.NoError(t, err)
		assert.Equal(t, "Note", obj.Type)

		actor, err := fetchActivityPubActor(obj)
		require.NoError(t, err)
		assert.Equal(t, "Poet", actor.Name)
		assert.Equal(t, server.URL+"/media/me.jpg", apLink(actor.Icon))

		att := buildActivityPubAttachment(obj, actor, server.URL+"/objects/117c6d97-4fdd-4531-b1e2-daabf04b9fcd")
		assert.Equal(t, "Poet", att.AuthorName)
		assert.Contains(t, att.Title, "@poet@")
		assert.Equal(t, "Hello & welcome", att.Text)
		assert.Equal(t, server.URL+"/media/a.png", att.ImageURL)
	})

	t.Run("html response rejected", func(t *testing.T) {
		_, err := fetchActivityPubObject(server.URL + "/objects/html")
		assert.ErrorContains(t, err, "not an ActivityPub resource")
	})

	t.Run("unsupported type rejected", func(t *testing.T) {
		_, err := fetchActivityPubObject(server.URL + "/objects/person")
		assert.ErrorContains(t, err, "unsupported ActivityPub object type")
	})

	t.Run("unauthorized", func(t *testing.T) {
		_, err := fetchActivityPubObject(server.URL + "/objects/private")
		assert.ErrorContains(t, err, "signed requests")
	})

	t.Run("not found", func(t *testing.T) {
		_, err := fetchActivityPubObject(server.URL + "/objects/missing")
		assert.ErrorContains(t, err, "not found")
	})
}

func TestFetchActivityPubActorFromArray(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		fmt.Fprintf(w, `{"type":"Person","id":"%s/u/ruud","preferredUsername":"ruud"}`, server.URL)
	}))
	defer server.Close()

	obj := &APObject{AttributedTo: json.RawMessage(fmt.Sprintf(`[{"type":"Group","id":"%[1]s/c/x"},{"type":"Person","id":"%[1]s/u/ruud"}]`, server.URL))}
	actor, err := fetchActivityPubActor(obj)
	require.NoError(t, err)
	assert.Equal(t, "ruud", actor.PreferredUsername)
}

func TestBuildActivityPubAttachment(t *testing.T) {
	t.Run("no actor falls back to host title", func(t *testing.T) {
		obj := &APObject{Type: "Note", Content: "<p>hi</p>"}
		att := buildActivityPubAttachment(obj, nil, "https://pleroma.example/notice/abc")
		assert.Equal(t, "pleroma.example", att.Title)
		assert.Equal(t, "https://pleroma.example/notice/abc", att.TitleLink)
		assert.Contains(t, att.Footer, "pleroma.example")
	})

	t.Run("content warning and article title", func(t *testing.T) {
		note := buildActivityPubAttachment(&APObject{Type: "Note", Summary: "spoilers", Content: "x"}, nil, "https://a.example/notice/1")
		assert.Equal(t, "⚠️ CW: spoilers\n\nx", note.Text)

		article := buildActivityPubAttachment(&APObject{Type: "Article", Name: "Headline", Summary: "a summary", Content: "body"}, nil, "https://a.example/post/1")
		assert.Equal(t, "**Headline**\n\nbody", article.Text)
	})

	t.Run("video and multiple attachments", func(t *testing.T) {
		obj := &APObject{Type: "Note", Attachment: []APAttachment{
			{Type: "Document", MediaType: "video/mp4", URL: json.RawMessage(`"https://a.example/v.mp4"`)},
			{Type: "Document", MediaType: "image/jpeg", URL: json.RawMessage(`"https://a.example/i.jpg"`)},
		}}
		att := buildActivityPubAttachment(obj, nil, "https://a.example/notice/1")
		require.Len(t, att.Fields, 2)
		assert.Equal(t, "🎬 Video", att.Fields[0].Title)
		assert.Equal(t, "📎 2 Attachments", att.Fields[1].Title)
		assert.Empty(t, att.ImageURL)
	})

	t.Run("poll", func(t *testing.T) {
		obj := &APObject{Type: "Question", Closed: json.RawMessage(`"2020-01-01T00:00:00Z"`)}
		obj.OneOf = make([]APPollOption, 2)
		obj.OneOf[0].Replies.TotalItems = 3
		obj.OneOf[1].Replies.TotalItems = 4
		att := buildActivityPubAttachment(obj, nil, "https://a.example/notice/1")
		require.Len(t, att.Fields, 1)
		assert.Equal(t, "Poll with 7 votes (closed)", att.Fields[0].Value)
	})

	t.Run("link attachment", func(t *testing.T) {
		obj := &APObject{Type: "Page", Name: "Link post", Attachment: []APAttachment{{Type: "Link", Href: "https://news.example/story"}}}
		att := buildActivityPubAttachment(obj, nil, "https://lemmy.example/post/1")
		require.Len(t, att.Fields, 1)
		assert.Equal(t, "https://news.example/story", att.Fields[0].Value)
	})
}
