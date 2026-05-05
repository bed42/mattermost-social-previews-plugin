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

func TestStripTrackingParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "utm params stripped",
			input:    "https://example.com/post/123?utm_source=twitter&utm_medium=social",
			expected: "https://example.com/post/123",
		},
		{
			name:     "tracking stripped, legitimate kept",
			input:    "https://example.com/watch?v=abc123&utm_source=fb",
			expected: "https://example.com/watch?v=abc123",
		},
		{
			name:     "no query params unchanged",
			input:    "https://bsky.app/profile/alice/post/3abc",
			expected: "https://bsky.app/profile/alice/post/3abc",
		},
		{
			name:     "no tracking params unchanged",
			input:    "https://example.com/search?q=hello&page=2",
			expected: "https://example.com/search?q=hello&page=2",
		},
		{
			name:     "all tracking params removed clears query string",
			input:    "https://example.com/page?fbclid=abc&gclid=xyz",
			expected: "https://example.com/page",
		},
		{
			name:     "mixed case param names stripped",
			input:    "https://example.com/?UTM_Source=test&UTM_MEDIUM=email",
			expected: "https://example.com/",
		},
		{
			name:     "twitter s and t params stripped",
			input:    "https://x.com/user/status/123?s=20&t=abcdef",
			expected: "https://x.com/user/status/123",
		},
		{
			name:     "instagram igsh stripped",
			input:    "https://www.instagram.com/p/abc123/?igsh=xyz&igshid=456",
			expected: "https://www.instagram.com/p/abc123/",
		},
		{
			name:     "fragment preserved",
			input:    "https://example.com/page?utm_source=tw#section",
			expected: "https://example.com/page#section",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTrackingParams(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCleanMessageURLs(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedReplaces int
	}{
		{
			name:             "message with dirty URL",
			input:            "check this out https://example.com/post?utm_source=twitter",
			expectedReplaces: 1,
		},
		{
			name:             "message with multiple dirty URLs",
			input:            "link1 https://a.com/?fbclid=x and link2 https://b.com/?gclid=y",
			expectedReplaces: 2,
		},
		{
			name:             "message with clean URL",
			input:            "check https://example.com/post out",
			expectedReplaces: 0,
		},
		{
			name:             "message with no URLs",
			input:            "just a normal message",
			expectedReplaces: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replacements := cleanMessageURLs(tt.input)
			assert.Equal(t, tt.expectedReplaces, len(replacements))
			// Verify all replacements actually differ from original
			for original, clean := range replacements {
				assert.NotEqual(t, original, clean)
			}
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

func TestIsDomainDisabled(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		disabled []string
		expected bool
	}{
		{
			name:     "exact host match",
			url:      "https://example.com/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "subdomain match",
			url:      "https://news.example.com/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "deep subdomain match",
			url:      "https://a.b.c.example.com/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "non-match",
			url:      "https://other.com/foo",
			disabled: []string{"example.com"},
			expected: false,
		},
		{
			name:     "lookalike suffix does not match",
			url:      "https://notexample.com/foo",
			disabled: []string{"example.com"},
			expected: false,
		},
		{
			name:     "case insensitive host",
			url:      "https://Example.COM/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "http scheme",
			url:      "http://example.com/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "host with port",
			url:      "https://example.com:8080/foo",
			disabled: []string{"example.com"},
			expected: true,
		},
		{
			name:     "empty disabled list",
			url:      "https://example.com/foo",
			disabled: nil,
			expected: false,
		},
		{
			name:     "malformed url",
			url:      "not a url",
			disabled: []string{"example.com"},
			expected: false,
		},
		{
			name:     "matches second entry",
			url:      "https://bsky.app/profile/x/post/y",
			disabled: []string{"example.com", "bsky.app"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDomainDisabled(tt.url, tt.disabled)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterDisabledDomains(t *testing.T) {
	tests := []struct {
		name     string
		urls     []string
		disabled []string
		expected []string
	}{
		{
			name:     "empty disabled returns input unchanged",
			urls:     []string{"https://example.com/a", "https://other.com/b"},
			disabled: nil,
			expected: []string{"https://example.com/a", "https://other.com/b"},
		},
		{
			name:     "filters matching urls",
			urls:     []string{"https://example.com/a", "https://other.com/b", "https://news.example.com/c"},
			disabled: []string{"example.com"},
			expected: []string{"https://other.com/b"},
		},
		{
			name:     "all filtered",
			urls:     []string{"https://example.com/a", "https://example.com/b"},
			disabled: []string{"example.com"},
			expected: []string{},
		},
		{
			name:     "none filtered",
			urls:     []string{"https://other.com/a"},
			disabled: []string{"example.com"},
			expected: []string{"https://other.com/a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDisabledDomains(tt.urls, tt.disabled)
			assert.Equal(t, tt.expected, result)
		})
	}
}
