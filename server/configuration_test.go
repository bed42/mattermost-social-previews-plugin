package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDisabledDomains(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   \n\n  \t  ",
			expected: nil,
		},
		{
			name:     "single domain",
			input:    "example.com",
			expected: []string{"example.com"},
		},
		{
			name:     "newline separated",
			input:    "example.com\nbsky.app\nthreads.net",
			expected: []string{"example.com", "bsky.app", "threads.net"},
		},
		{
			name:     "comma separated",
			input:    "example.com, bsky.app, threads.net",
			expected: []string{"example.com", "bsky.app", "threads.net"},
		},
		{
			name:     "mixed separators",
			input:    "example.com,bsky.app\nthreads.net",
			expected: []string{"example.com", "bsky.app", "threads.net"},
		},
		{
			name:     "trims whitespace",
			input:    "  example.com  \n   bsky.app  ",
			expected: []string{"example.com", "bsky.app"},
		},
		{
			name:     "drops blank lines",
			input:    "example.com\n\n\nbsky.app",
			expected: []string{"example.com", "bsky.app"},
		},
		{
			name:     "lowercases entries",
			input:    "Example.COM\nBsky.App",
			expected: []string{"example.com", "bsky.app"},
		},
		{
			name:     "handles CRLF line endings",
			input:    "example.com\r\nbsky.app\r\n",
			expected: []string{"example.com", "bsky.app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDisabledDomains(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseExcludedChannels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty", input: "", expected: nil},
		{name: "single id", input: "abc123", expected: []string{"abc123"}},
		{
			name:     "newline separated",
			input:    "channel-one\nchannel-two\nchannel-three",
			expected: []string{"channel-one", "channel-two", "channel-three"},
		},
		{
			name:     "comma separated with whitespace",
			input:    "channel-one, channel-two , channel-three",
			expected: []string{"channel-one", "channel-two", "channel-three"},
		},
		{
			name:     "case is preserved (channel IDs are case-sensitive)",
			input:    "AbCdEf\nGHIJKL",
			expected: []string{"AbCdEf", "GHIJKL"},
		},
		{
			name:     "drops blanks and CRLF",
			input:    "ch1\r\n\r\nch2\r\n",
			expected: []string{"ch1", "ch2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseExcludedChannels(tt.input))
		})
	}
}

func TestConfigurationClone(t *testing.T) {
	original := &configuration{
		DisabledDomains:        "example.com\nbsky.app",
		ExcludedChannels:       "ch1\nch2",
		disabledDomainsParsed:  []string{"example.com", "bsky.app"},
		excludedChannelsParsed: []string{"ch1", "ch2"},
	}

	clone := original.Clone()

	assert.Equal(t, original.DisabledDomains, clone.DisabledDomains)
	assert.Equal(t, original.ExcludedChannels, clone.ExcludedChannels)
	assert.Equal(t, original.disabledDomainsParsed, clone.disabledDomainsParsed)
	assert.Equal(t, original.excludedChannelsParsed, clone.excludedChannelsParsed)

	// Mutating clone slices must not affect the original
	clone.disabledDomainsParsed[0] = "mutated.com"
	clone.excludedChannelsParsed[0] = "mutated"
	assert.Equal(t, "example.com", original.disabledDomainsParsed[0])
	assert.Equal(t, "ch1", original.excludedChannelsParsed[0])
}
