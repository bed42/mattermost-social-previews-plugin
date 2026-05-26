package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// Reddit URL patterns. Supports:
//   - https://www.reddit.com/r/<sub>/comments/<id>/<slug>/
//   - https://www.reddit.com/r/<sub>/comments/<id>/
//   - https://reddit.com/r/<sub>/comments/<id>/
//   - https://old.reddit.com/, np.reddit.com/, new.reddit.com/
//   - https://redd.it/<id>
//   - https://www.reddit.com/r/<sub>/s/<shortid> (share links — resolved via redirect)
var redditPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://(?:www\.|old\.|np\.|new\.)?reddit\.com/r/([a-zA-Z0-9_]+)/comments/([a-z0-9]+)(?:/[^\s]*)?`),
	regexp.MustCompile(`https?://redd\.it/([a-z0-9]+)`),
	regexp.MustCompile(`https?://(?:www\.)?reddit\.com/r/[a-zA-Z0-9_]+/s/[a-zA-Z0-9]+`),
}

// redditAPIBase is the base URL for Reddit's public JSON API. Override in tests.
var redditAPIBase = "https://www.reddit.com"

// redditAPIFallback is tried when redditAPIBase returns 403/429. Reddit's
// edge filtering between www and old subdomains is sometimes inconsistent —
// servers blocked from one can occasionally reach the other.
var redditAPIFallback = "https://old.reddit.com"

// redditUserAgent identifies the request as a real Chrome browser. Reddit
// aggressively 403s requests from cloud/Docker IPs that announce themselves
// as bots (including Slackbot, Discordbot, our own previous UA, etc.); a
// browser-style UA + Accept headers is the only thing that reliably gets a
// 200 from many hosting providers.
const redditUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// extractRedditURLs finds all Reddit post URLs in the given text.
func extractRedditURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)

	for _, pattern := range redditPatterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			if !seen[match] {
				urls = append(urls, match)
				seen[match] = true
			}
		}
	}

	return urls
}

// parseRedditURL extracts the post ID from a Reddit URL. The subreddit is
// returned when known (the canonical /r/<sub>/comments/<id>/ form) and empty
// for redd.it short URLs. Share URLs (/r/<sub>/s/<id>) cannot be parsed
// directly — they must be resolved via resolveRedditShareURL first.
func parseRedditURL(rawURL string) (subreddit string, postID string, ok bool) {
	if m := redditPatterns[0].FindStringSubmatch(rawURL); len(m) == 3 {
		return m[1], m[2], true
	}
	if m := redditPatterns[1].FindStringSubmatch(rawURL); len(m) == 2 {
		return "", m[1], true
	}
	return "", "", false
}

// resolveRedditShareURL follows redirects on a /r/<sub>/s/<id> share link to
// recover the canonical post URL. Reddit's share endpoint returns a 307
// pointing at the real /comments/<id>/ permalink — invalid tokens redirect to
// the subreddit root instead, which parseRedditURL will then reject.
func resolveRedditShareURL(rawURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", redditUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to resolve Reddit share URL: %w", err)
	}
	resp.Body.Close()

	final := resp.Request.URL.String()
	if final == rawURL {
		return "", fmt.Errorf("share URL did not redirect")
	}
	return final, nil
}

// fetchRedditPostFromURL is the single entry point for the plugin hook. It
// resolves share links, parses the post ID, tries the rich JSON API, then
// falls back to the oEmbed endpoint when Reddit's main API is blocked (some
// cloud/Docker IP ranges are 403'd at the CDN; /oembed lives on different
// infrastructure and is often still reachable).
func fetchRedditPostFromURL(rawURL string) (*RedditPost, error) {
	subreddit, postID, ok := parseRedditURL(rawURL)
	canonicalURL := rawURL
	if !ok {
		resolved, err := resolveRedditShareURL(rawURL)
		if err != nil {
			return nil, err
		}
		canonicalURL = resolved
		subreddit, postID, ok = parseRedditURL(resolved)
		if !ok {
			return nil, fmt.Errorf("invalid Reddit URL after redirect: %s", resolved)
		}
	}

	// Primary path: rich JSON listing.
	post, err := fetchRedditPost(postID)
	if err == nil {
		return post, nil
	}

	// Fallback path: oEmbed. Only helps for the IP-block case; for genuine
	// 404s / private posts we let the original error propagate.
	if !isRedditBlocked(err) {
		return nil, err
	}

	// oEmbed needs a /r/<sub>/comments/<id>/ form. Synthesize one if the
	// input was a redd.it shortlink (sub is unknown — Reddit routes by ID
	// anyway, so a placeholder works).
	oembedTargetURL := canonicalURL
	if subreddit == "" || !strings.Contains(oembedTargetURL, "/comments/") {
		oembedTargetURL = fmt.Sprintf("https://www.reddit.com/r/_/comments/%s/", postID)
	}

	oePost, oeErr := fetchRedditOEmbed(oembedTargetURL)
	if oeErr != nil {
		// Surface the original (more informative) JSON-API error.
		return nil, err
	}
	return oePost, nil
}

// fetchRedditOEmbed calls https://www.reddit.com/oembed for a canonical post
// URL and returns a partially-populated RedditPost. The oEmbed response only
// gives us title, author, and (parsed out of the embed HTML) the subreddit —
// no body text, no image, no score. It's the no-rich-data fallback for when
// Reddit's main API is blocked.
func fetchRedditOEmbed(canonicalURL string) (*RedditPost, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	apiURL := "https://www.reddit.com/oembed?url=" + url.QueryEscape(canonicalURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create oembed request: %w", err)
	}
	req.Header.Set("User-Agent", redditUserAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oembed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oembed unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read oembed response: %w", err)
	}

	var oe struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		HTML       string `json:"html"`
	}
	if err := json.Unmarshal(body, &oe); err != nil {
		return nil, fmt.Errorf("failed to parse oembed response: %w", err)
	}
	if oe.Title == "" {
		return nil, fmt.Errorf("oembed response missing title")
	}

	post := &RedditPost{
		Title:     oe.Title,
		Author:    oe.AuthorName,
		Permalink: canonicalURL,
	}

	// The embed HTML contains an anchor to the real /r/<sub>/ path; pull
	// the subreddit name out of it so the author row in the preview lines
	// up with what you'd get from the JSON API.
	if m := oembedSubredditRe.FindStringSubmatch(oe.HTML); len(m) == 2 {
		post.Subreddit = "r/" + m[1]
	}

	return post, nil
}

// oembedSubredditRe extracts the subreddit from the anchor Reddit injects
// into its oembed `html` field (e.g. <a href="https://www.reddit.com/r/x/">).
var oembedSubredditRe = regexp.MustCompile(`reddit\.com/r/([a-zA-Z0-9_]+)/?"`)

// RedditPost holds the fields we render in a preview.
type RedditPost struct {
	Title         string
	Author        string
	Subreddit     string // e.g. "r/australian"
	SubredditIcon string
	Selftext      string
	Permalink     string // full https URL to the post
	LinkURL       string // for link posts, the external URL the post points at
	ImageURL      string // best image for the preview, if any
	Score         int
	NumComments   int
	IsSelf        bool
	IsVideo       bool
	Over18        bool
	Spoiler       bool
	LinkFlair     string
}

// fetchRedditPost fetches a Reddit post by ID via the public JSON API. If
// the primary host returns 403/429, it retries against old.reddit.com.
func fetchRedditPost(postID string) (*RedditPost, error) {
	body, err := redditGetJSON(redditAPIBase, postID)
	if err != nil {
		if isRedditBlocked(err) && redditAPIFallback != "" && redditAPIFallback != redditAPIBase {
			body, err = redditGetJSON(redditAPIFallback, postID)
		}
		if err != nil {
			return nil, err
		}
	}
	return parseRedditResponse(body)
}

// redditGetJSON performs a single GET against /comments/<id>.json on the
// given Reddit host and returns the response body. Errors include the host
// so the caller can tell which attempt failed.
func redditGetJSON(host, postID string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	apiURL := fmt.Sprintf("%s/comments/%s.json?sr_detail=1&raw_json=1&limit=1", host, postID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", redditUserAgent)
	// Browsers always send these — omitting them is a common bot tell.
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Reddit post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d (host=%s)", resp.StatusCode, host)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
}

// isRedditBlocked reports whether err looks like Reddit's anti-bot block
// (403/429) — the only cases worth retrying against the fallback host.
func isRedditBlocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status code: 403") || strings.Contains(msg, "status code: 429")
}

// parseRedditResponse parses the Reddit JSON listing returned by /comments/<id>.json
// into a RedditPost. The top-level shape is an array of two listings — the first
// contains the post itself, the second contains its comments.
func parseRedditResponse(data []byte) (*RedditPost, error) {
	var raw []struct {
		Data struct {
			Children []struct {
				Kind string          `json:"kind"`
				Data json.RawMessage `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(raw) == 0 || len(raw[0].Data.Children) == 0 {
		return nil, fmt.Errorf("no post in response")
	}

	var p struct {
		Title             string `json:"title"`
		Author            string `json:"author"`
		Subreddit         string `json:"subreddit"`
		SubredditPrefixed string `json:"subreddit_name_prefixed"`
		Selftext          string `json:"selftext"`
		Permalink         string `json:"permalink"`
		URL               string `json:"url_overridden_by_dest"`
		Score             int    `json:"score"`
		NumComments       int    `json:"num_comments"`
		IsSelf            bool   `json:"is_self"`
		IsVideo           bool   `json:"is_video"`
		Over18            bool   `json:"over_18"`
		Spoiler           bool   `json:"spoiler"`
		Thumbnail         string `json:"thumbnail"`
		LinkFlairText     string `json:"link_flair_text"`
		Preview           *struct {
			Images []struct {
				Source struct {
					URL string `json:"url"`
				} `json:"source"`
			} `json:"images"`
		} `json:"preview"`
		MediaMetadata map[string]struct {
			Status string `json:"status"`
			S      struct {
				U string `json:"u"`
			} `json:"s"`
		} `json:"media_metadata"`
		SrDetail *struct {
			CommunityIcon     string `json:"community_icon"`
			IconImg           string `json:"icon_img"`
			DisplayNamePrefix string `json:"display_name_prefixed"`
		} `json:"sr_detail"`
	}

	if err := json.Unmarshal(raw[0].Data.Children[0].Data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse post data: %w", err)
	}

	subreddit := p.SubredditPrefixed
	if subreddit == "" && p.Subreddit != "" {
		subreddit = "r/" + p.Subreddit
	}

	permalink := p.Permalink
	if permalink != "" && !strings.HasPrefix(permalink, "http") {
		permalink = "https://www.reddit.com" + permalink
	}

	post := &RedditPost{
		Title:       p.Title,
		Author:      p.Author,
		Subreddit:   subreddit,
		Selftext:    p.Selftext,
		Permalink:   permalink,
		Score:       p.Score,
		NumComments: p.NumComments,
		IsSelf:      p.IsSelf,
		IsVideo:     p.IsVideo,
		Over18:      p.Over18,
		Spoiler:     p.Spoiler,
		LinkFlair:   p.LinkFlairText,
	}

	if !p.IsSelf && p.URL != "" {
		post.LinkURL = p.URL
	}

	if p.SrDetail != nil {
		if p.SrDetail.CommunityIcon != "" {
			post.SubredditIcon = p.SrDetail.CommunityIcon
		} else if p.SrDetail.IconImg != "" {
			post.SubredditIcon = p.SrDetail.IconImg
		}
	}

	post.ImageURL = pickRedditImage(p.Preview, p.MediaMetadata, p.Thumbnail, p.Over18, p.Spoiler)

	return post, nil
}

// pickRedditImage chooses the best available image URL for the preview.
// Reddit exposes the same image in several places — prefer the high-res
// preview when present, fall back to media_metadata, then the thumbnail.
func pickRedditImage(preview *struct {
	Images []struct {
		Source struct {
			URL string `json:"url"`
		} `json:"source"`
	} `json:"images"`
}, mediaMetadata map[string]struct {
	Status string `json:"status"`
	S      struct {
		U string `json:"u"`
	} `json:"s"`
}, thumbnail string, over18, spoiler bool) string {
	if preview != nil && len(preview.Images) > 0 {
		if u := preview.Images[0].Source.URL; u != "" {
			return u
		}
	}
	for _, m := range mediaMetadata {
		if m.Status == "valid" && m.S.U != "" {
			return m.S.U
		}
	}
	// Thumbnail is sometimes a sentinel like "self", "default", "nsfw", "spoiler".
	if strings.HasPrefix(thumbnail, "http") && !over18 && !spoiler {
		return thumbnail
	}
	return ""
}

// truncate returns s shortened to maxRunes runes with an ellipsis suffix.
func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// buildRedditAttachment creates a Mattermost message attachment from a Reddit post.
func buildRedditAttachment(post *RedditPost, originalURL string) *model.SlackAttachment {
	subredditLink := ""
	if post.Subreddit != "" {
		subredditLink = "https://www.reddit.com/" + post.Subreddit
	}

	permalink := post.Permalink
	if permalink == "" {
		permalink = originalURL
	}

	title := post.Title
	if post.LinkFlair != "" {
		title = fmt.Sprintf("[%s] %s", post.LinkFlair, title)
	}
	if post.Over18 {
		title = "🔞 " + title
	}
	if post.Spoiler {
		title = "⚠️ Spoiler: " + title
	}

	bodyText := stripRedditMarkdown(post.Selftext)
	bodyText = truncate(strings.TrimSpace(bodyText), 600)

	// When we only have oEmbed data (Reddit's main API blocked the request),
	// score and comments aren't available — drop them from the footer rather
	// than misrepresent the post as having zero engagement.
	footer := "Reddit"
	if post.Score > 0 || post.NumComments > 0 {
		footer += fmt.Sprintf(" • ⬆ %s • 💬 %s", formatRedditCount(post.Score), formatRedditCount(post.NumComments))
	}
	if post.Author != "" {
		footer += " • u/" + post.Author
	}

	attachment := &model.SlackAttachment{
		Fallback:   fmt.Sprintf("Reddit: %s", post.Title),
		Color:      "#FF4500", // Reddit orange
		AuthorName: post.Subreddit,
		AuthorLink: subredditLink,
		AuthorIcon: post.SubredditIcon,
		Title:      title,
		TitleLink:  permalink,
		Text:       wrapText(bodyText, previewWrapWidth),
		Footer:     footer,
		FooterIcon: "https://www.redditstatic.com/icon.png",
	}

	if post.ImageURL != "" && !post.Over18 && !post.Spoiler {
		attachment.ImageURL = post.ImageURL
	}

	fields := []*model.SlackAttachmentField{}
	if post.LinkURL != "" {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🔗 Link",
			Value: post.LinkURL,
			Short: false,
		})
	}
	attachment.Fields = fields

	return attachment
}

// stripRedditMarkdown does a light pass to clean up the post selftext so it
// reads well in the limited preview area. Reddit selftext is markdown, which
// Mattermost will largely render, but we still want to drop image embeds that
// only make sense on the Reddit page itself.
func stripRedditMarkdown(text string) string {
	// Inline preview.redd.it links that Reddit injects for media_metadata entries.
	previewImg := regexp.MustCompile(`https?://preview\.redd\.it/\S+`)
	text = previewImg.ReplaceAllString(text, "")
	// Collapse runs of blank lines that result from removing those embeds.
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// formatRedditCount renders a vote/comment count the way Reddit does:
// 1.2k for 1234, 12.3k for 12300, 1.2m for 1234000.
func formatRedditCount(n int) string {
	if n < 0 {
		return "0"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
}
