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
	// Iceshrimp.NET and similar: https://instance.domain/notes/alphanumericID
	regexp.MustCompile(`https?://([^/\s]+)/notes/([a-zA-Z0-9]+)`),
}

// Threads URL pattern: https://threads.net/@username/post/SHORTCODE or threads.com
var threadsPattern = regexp.MustCompile(`https?://(?:www\.)?threads\.(?:net|com)/@[a-zA-Z0-9_.]+/post/[a-zA-Z0-9_-]+`)

// extractThreadsURLs finds all Threads URLs in the given text
func extractThreadsURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	matches := threadsPattern.FindAllString(text, -1)
	for _, match := range matches {
		if !seen[match] {
			urls = append(urls, match)
			seen[match] = true
		}
	}

	return urls
}

// Patterns for extracting instance URL and status ID
var mastodonExtractPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(https?://[^/]+)/@[^/]+/(\d+)`),
	regexp.MustCompile(`^(https?://[^/]+)/users/[^/]+/statuses/(\d+)`),
	regexp.MustCompile(`^(https?://[^/]+)/notes/([a-zA-Z0-9]+)`),
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

// genericURLPattern matches http/https URLs in text
var genericURLPattern = regexp.MustCompile(`https?://[^\s<>"]+`)

// isExcludedURL checks if a URL should be excluded by checking if it matches or
// starts with any of the excluded URLs (handles query params/fragments on the same base URL).
func isExcludedURL(url string, excludeURLs []string) bool {
	for _, excluded := range excludeURLs {
		if url == excluded {
			return true
		}
		base := strings.TrimRight(excluded, "/")
		if strings.HasPrefix(url, base+"/") || strings.HasPrefix(url, base+"?") {
			return true
		}
	}
	return false
}

// extractGenericURLs finds all URLs in text that aren't in the excludeURLs list.
// URLs matching the siteURL prefix (the Mattermost server's own URL) are also excluded.
func extractGenericURLs(text string, excludeURLs []string, siteURL string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	matches := genericURLPattern.FindAllString(text, -1)
	for _, match := range matches {
		// Strip trailing punctuation that's likely not part of the URL
		match = strings.TrimRight(match, ".,;:!?)")
		if !seen[match] && !isExcludedURL(match, excludeURLs) && (siteURL == "" || !strings.HasPrefix(match, siteURL)) {
			urls = append(urls, match)
			seen[match] = true
		}
	}

	return urls
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
