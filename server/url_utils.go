package main

import (
	"regexp"
	"strings"
)

// Mastodon URL patterns:
// Pattern 1: https://instance.domain/@username/123456
// Pattern 2: https://instance.domain/users/username/statuses/123456
// Pattern 3: https://instance.domain/@username@other.instance/123456 (federated)
var mastodonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://([^/\s]+)/@([a-zA-Z0-9_]+)/(\d+)`),
	regexp.MustCompile(`https?://([^/\s]+)/users/([a-zA-Z0-9_]+)/statuses/(\d+)`),
	regexp.MustCompile(`https?://([^/\s]+)/@([a-zA-Z0-9_]+)@[^/\s]+/(\d+)`),
}

// Patterns for extracting instance URL and status ID
var mastodonExtractPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(https?://[^/]+)/@[^/]+/(\d+)`),
	regexp.MustCompile(`^(https?://[^/]+)/users/[^/]+/statuses/(\d+)`),
}

// extractMastodonURLs finds all Mastodon URLs in the given text
func extractMastodonURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	for _, pattern := range mastodonPatterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			// Avoid duplicates
			if !seen[match] {
				urls = append(urls, match)
				seen[match] = true
			}
		}
	}

	return urls
}

// isMastodonURL checks if a URL matches Mastodon URL patterns
func isMastodonURL(url string) bool {
	for _, pattern := range mastodonPatterns {
		if pattern.MatchString(url) {
			return true
		}
	}
	return false
}

// parseMastodonURL extracts the instance URL and status ID from a Mastodon URL
// Returns: (instanceURL, statusID, ok)
func parseMastodonURL(url string) (string, string, bool) {
	// Remove trailing slash if present
	url = strings.TrimRight(url, "/")

	for _, pattern := range mastodonExtractPatterns {
		matches := pattern.FindStringSubmatch(url)
		if len(matches) == 3 {
			return matches[1], matches[2], true
		}
	}
	return "", "", false
}
