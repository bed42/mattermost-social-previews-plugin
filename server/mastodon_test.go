package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildAttachment_CardWithAll(t *testing.T) {
	status := &MastodonStatus{
		Content: "<p>Check this out</p>",
		Account: MastodonAccount{
			Acct:        "user@example.com",
			DisplayName: "Test User",
			URL:         "https://example.com/@user",
		},
		Card: &MastodonCard{
			URL:         "https://example.com/article",
			Title:       "Article Title",
			Description: "A short description of the article.",
			Image:       "https://example.com/image.png",
		},
	}

	att := buildAttachment(status, "https://example.com/@user/123", nil)

	// Should have a link preview field
	var linkField *struct{ Title, Value string }
	for _, f := range att.Fields {
		if f.Title == "🔗 Link Preview" {
			linkField = &struct{ Title, Value string }{f.Title, f.Value.(string)}
			break
		}
	}
	assert.NotNil(t, linkField, "expected a Link Preview field")
	assert.Contains(t, linkField.Value, "[Article Title](https://example.com/article)")
	assert.Contains(t, linkField.Value, "A short description of the article.")

	// Card image should be set since there are no media attachments
	assert.Equal(t, "https://example.com/image.png", att.ImageURL)
}

func TestBuildAttachment_CardNoImage(t *testing.T) {
	status := &MastodonStatus{
		Content: "<p>Check this out</p>",
		Account: MastodonAccount{Acct: "user"},
		Card: &MastodonCard{
			URL:         "https://example.com/article",
			Title:       "Article Title",
			Description: "Description here.",
		},
	}

	att := buildAttachment(status, "https://example.com/@user/123", nil)

	var found bool
	for _, f := range att.Fields {
		if f.Title == "🔗 Link Preview" {
			found = true
		}
	}
	assert.True(t, found, "expected a Link Preview field")
	assert.Empty(t, att.ImageURL, "ImageURL should not be set when card has no image")
}

func TestBuildAttachment_MediaTakesPriorityOverCard(t *testing.T) {
	status := &MastodonStatus{
		Content: "<p>Photo post</p>",
		Account: MastodonAccount{Acct: "user"},
		MediaAttachments: []MastodonMedia{
			{Type: "image", URL: "https://example.com/photo.jpg"},
		},
		Card: &MastodonCard{
			URL:   "https://example.com/article",
			Title: "Article",
			Image: "https://example.com/card-image.png",
		},
	}

	att := buildAttachment(status, "https://example.com/@user/123", nil)

	// Media attachment image should win
	assert.Equal(t, "https://example.com/photo.jpg", att.ImageURL)
}

func TestBuildAttachment_NoCard(t *testing.T) {
	status := &MastodonStatus{
		Content: "<p>Just a regular post</p>",
		Account: MastodonAccount{Acct: "user"},
	}

	att := buildAttachment(status, "https://example.com/@user/123", nil)

	for _, f := range att.Fields {
		assert.NotEqual(t, "🔗 Link Preview", f.Title, "should not have a Link Preview field")
	}
}

func TestBuildAttachment_CardDescriptionTruncated(t *testing.T) {
	longDesc := ""
	for i := 0; i < 250; i++ {
		longDesc += "x"
	}
	status := &MastodonStatus{
		Content: "<p>Post</p>",
		Account: MastodonAccount{Acct: "user"},
		Card: &MastodonCard{
			URL:         "https://example.com/article",
			Title:       "Long Article",
			Description: longDesc,
		},
	}

	att := buildAttachment(status, "https://example.com/@user/123", nil)

	for _, f := range att.Fields {
		if f.Title == "🔗 Link Preview" {
			val := f.Value.(string)
			// 200 chars of description + "..." = truncated
			assert.Contains(t, val, "...")
			// The description portion should not contain all 250 chars
			assert.NotContains(t, val, longDesc)
		}
	}
}
