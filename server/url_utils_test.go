package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripBacktickContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline code with URL removed",
			input:    "check `https://example.com` out",
			expected: "check  out",
		},
		{
			name:     "triple backtick code block removed",
			input:    "before\n```\nhttps://example.com\n```\nafter",
			expected: "before\n\nafter",
		},
		{
			name:     "triple backtick with language tag removed",
			input:    "before\n```go\nfmt.Println(\"https://example.com\")\n```\nafter",
			expected: "before\n\nafter",
		},
		{
			name:     "unwrapped URL preserved",
			input:    "check https://example.com out",
			expected: "check https://example.com out",
		},
		{
			name:     "mixed: backtick URL removed, bare URL preserved",
			input:    "`https://ignore.com` and https://keep.com",
			expected: " and https://keep.com",
		},
		{
			name:     "no backticks unchanged",
			input:    "just a normal message",
			expected: "just a normal message",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripBacktickContent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractURLsIgnoresBackticks(t *testing.T) {
	// Verify that URL extraction with stripBacktickContent works end-to-end
	text := "`https://bsky.app/profile/alice.bsky.social/post/3abc123` and https://bsky.app/profile/bob.bsky.social/post/3def456"
	cleaned := stripBacktickContent(text)

	blueskyURLs := extractBlueskyURLs(cleaned)
	assert.Equal(t, []string{"https://bsky.app/profile/bob.bsky.social/post/3def456"}, blueskyURLs)
}
