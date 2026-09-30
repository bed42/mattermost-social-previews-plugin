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

// ActivityPub URL patterns for fediverse software that doesn't expose a
// Mastodon-compatible status URL (or API). Posts are fetched directly as
// ActivityStreams JSON via content negotiation.
var activityPubPatterns = []*regexp.Regexp{
	// Pleroma / Akkoma / Mangane / Soapbox: /objects/{uuid} and /notice/{id}
	regexp.MustCompile(`https?://[^/\s]+/objects/[0-9a-fA-F-]{36}`),
	regexp.MustCompile(`https?://[^/\s]+/notice/[a-zA-Z0-9]+`),
	// GoToSocial: /@user/statuses/{ULID}
	regexp.MustCompile(`https?://[^/\s]+/@[a-zA-Z0-9_.]+/statuses/[0-9A-Z]{26}`),
	// Friendica: /display/{guid}
	regexp.MustCompile(`https?://[^/\s]+/display/[a-zA-Z0-9-]+`),
	// Lemmy / PieFed / Mbin: /post/{id}, /comment/{id}
	regexp.MustCompile(`https?://[^/\s]+/(?:post|comment)/\d+`),
}

// activityPubAccept is the Accept header required by ActivityPub servers to
// return JSON instead of their HTML frontend.
const activityPubAccept = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

// activityPubObjectTypes are the object types we know how to render.
var activityPubObjectTypes = map[string]bool{
	"Note": true, "Article": true, "Page": true, "Question": true,
	"Video": true, "Image": true, "Audio": true, "Event": true,
}

// APObject is the subset of an ActivityStreams object needed for a preview.
// Fields that can be a string, object, or array are kept raw and resolved
// with apLink.
type APObject struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	URL          json.RawMessage `json:"url"`
	AttributedTo json.RawMessage `json:"attributedTo"`
	Name         string          `json:"name"`
	Summary      string          `json:"summary"`
	Content      string          `json:"content"`
	Sensitive    bool            `json:"sensitive"`
	Published    string          `json:"published"`
	InReplyTo    json.RawMessage `json:"inReplyTo"`
	Attachment   []APAttachment  `json:"attachment"`
	Image        json.RawMessage `json:"image"`
	OneOf        []APPollOption  `json:"oneOf"`
	AnyOf        []APPollOption  `json:"anyOf"`
	Closed       json.RawMessage `json:"closed"`
	EndTime      string          `json:"endTime"`
}

// APAttachment is a media attachment (Document/Image/Video) or Link.
type APAttachment struct {
	Type      string          `json:"type"`
	MediaType string          `json:"mediaType"`
	URL       json.RawMessage `json:"url"`
	Href      string          `json:"href"`
	Name      string          `json:"name"`
}

// APPollOption is a Question option with its reply count.
type APPollOption struct {
	Name    string `json:"name"`
	Replies struct {
		TotalItems int `json:"totalItems"`
	} `json:"replies"`
}

// APActor is the subset of an ActivityPub actor needed for author display.
type APActor struct {
	ID                string          `json:"id"`
	Type              string          `json:"type"`
	Name              string          `json:"name"`
	PreferredUsername string          `json:"preferredUsername"`
	URL               json.RawMessage `json:"url"`
	Icon              json.RawMessage `json:"icon"`
}

// extractActivityPubURLs finds URLs matching known non-Mastodon fediverse post patterns.
func extractActivityPubURLs(text string) []string {
	urls := []string{}
	seen := make(map[string]bool)
	for _, pattern := range activityPubPatterns {
		for _, match := range pattern.FindAllString(text, -1) {
			if !seen[match] {
				urls = append(urls, match)
				seen[match] = true
			}
		}
	}
	return urls
}

// apLink resolves an ActivityStreams link-ish value (string, Link/Image object
// with href/url, or array of those) to the first usable URL.
func apLink(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Href      string          `json:"href"`
		URL       json.RawMessage `json:"url"`
		ID        string          `json:"id"`
		MediaType string          `json:"mediaType"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Href != "" {
			return obj.Href
		}
		if u := apLink(obj.URL); u != "" {
			return u
		}
		return obj.ID
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		// Prefer an HTML link when several representations are listed
		for _, item := range arr {
			if err := json.Unmarshal(item, &obj); err == nil && strings.Contains(obj.MediaType, "html") && obj.Href != "" {
				return obj.Href
			}
		}
		for _, item := range arr {
			if u := apLink(item); u != "" {
				return u
			}
		}
	}
	return ""
}

// fetchActivityPubJSON GETs rawURL requesting ActivityStreams JSON and decodes it into v.
func fetchActivityPubJSON(rawURL string, v interface{}) error {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", activityPubAccept)
	req.Header.Set("User-Agent", "Mattermost-Social-Previews/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("post not found or deleted")
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("instance requires signed requests or post is private")
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited by instance")
	default:
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "json") {
		return fmt.Errorf("not an ActivityPub resource: %s", contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}

// fetchActivityPubObject fetches a fediverse post as an ActivityStreams object.
func fetchActivityPubObject(rawURL string) (*APObject, error) {
	var obj APObject
	if err := fetchActivityPubJSON(rawURL, &obj); err != nil {
		return nil, err
	}
	if !activityPubObjectTypes[obj.Type] {
		return nil, fmt.Errorf("unsupported ActivityPub object type: %q", obj.Type)
	}
	return &obj, nil
}

// fetchActivityPubActor fetches the author of an object. attributedTo may be
// a string, an embedded actor, or an array (e.g. Lemmy: [actor, community]).
func fetchActivityPubActor(obj *APObject) (*APActor, error) {
	var embedded APActor
	if err := json.Unmarshal(obj.AttributedTo, &embedded); err == nil && embedded.PreferredUsername != "" {
		return &embedded, nil
	}

	actorURL := ""
	var arr []json.RawMessage
	if err := json.Unmarshal(obj.AttributedTo, &arr); err == nil {
		// Pick the Person rather than the Group (community) when both are listed
		for _, item := range arr {
			var a struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}
			if json.Unmarshal(item, &a) == nil && a.Type == "Person" && a.ID != "" {
				actorURL = a.ID
				break
			}
		}
	}
	if actorURL == "" {
		actorURL = apLink(obj.AttributedTo)
	}
	if actorURL == "" {
		return nil, fmt.Errorf("object has no author")
	}

	var actor APActor
	if err := fetchActivityPubJSON(actorURL, &actor); err != nil {
		return nil, err
	}
	if actor.ID == "" {
		actor.ID = actorURL
	}
	return &actor, nil
}

// apActorHandle builds a @user@host handle for an actor.
func apActorHandle(actor *APActor) string {
	if actor == nil || actor.PreferredUsername == "" {
		return ""
	}
	if u, err := url.Parse(actor.ID); err == nil && u.Host != "" {
		return fmt.Sprintf("@%s@%s", actor.PreferredUsername, u.Host)
	}
	return "@" + actor.PreferredUsername
}

// apMediaKind classifies an attachment as "image", "video", "audio" or "link".
func apMediaKind(a APAttachment) string {
	mt := strings.ToLower(a.MediaType)
	switch {
	case strings.HasPrefix(mt, "image/") || a.Type == "Image":
		return "image"
	case strings.HasPrefix(mt, "video/") || a.Type == "Video":
		return "video"
	case strings.HasPrefix(mt, "audio/") || a.Type == "Audio":
		return "audio"
	}
	return "link"
}

// buildActivityPubAttachment creates a Mattermost attachment from an ActivityPub object.
func buildActivityPubAttachment(obj *APObject, actor *APActor, originalURL string) *model.SlackAttachment {
	host := ""
	if u, err := url.Parse(originalURL); err == nil {
		host = u.Hostname()
	}

	var parts []string
	if obj.Name != "" && obj.Type != "Note" {
		parts = append(parts, "**"+obj.Name+"**")
	}
	if obj.Summary != "" && (obj.Sensitive || obj.Type == "Note") {
		parts = append(parts, "⚠️ CW: "+stripHTML(obj.Summary))
	}
	if content := stripHTML(obj.Content); content != "" {
		parts = append(parts, content)
	}
	content := strings.Join(parts, "\n\n")

	attachment := &model.SlackAttachment{
		Fallback:  fmt.Sprintf("🌐 Fediverse: %s", content),
		Color:     "#A730B8", // Fediverse pentagram purple
		TitleLink: originalURL,
		Text:      wrapText(content, previewWrapWidth),
		Footer:    fmt.Sprintf("🌐 Fediverse Preview · %s", host),
	}

	if actor != nil {
		attachment.AuthorName = actor.Name
		if attachment.AuthorName == "" {
			attachment.AuthorName = actor.PreferredUsername
		}
		attachment.AuthorLink = apLink(actor.URL)
		if attachment.AuthorLink == "" {
			attachment.AuthorLink = actor.ID
		}
		attachment.AuthorIcon = apLink(actor.Icon)
		attachment.Title = apActorHandle(actor)
	}
	if attachment.Title == "" {
		attachment.Title = host
	}

	fields := []*model.SlackAttachmentField{}

	var media []APAttachment
	for _, a := range obj.Attachment {
		if apMediaKind(a) != "link" {
			media = append(media, a)
		}
	}

	if len(media) > 0 {
		first := media[0]
		switch apMediaKind(first) {
		case "image":
			attachment.ImageURL = apLink(first.URL)
		case "video":
			fields = append(fields, &model.SlackAttachmentField{
				Title: "🎬 Video",
				Value: fmt.Sprintf("[Watch video](%s)", apLink(first.URL)),
			})
		}
	}
	if attachment.ImageURL == "" {
		attachment.ImageURL = apLink(obj.Image)
	}

	if len(media) > 1 {
		var links []string
		for i, m := range media {
			links = append(links, fmt.Sprintf("[%s %d/%d](%s)", apMediaKind(m), i+1, len(media), apLink(m.URL)))
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: fmt.Sprintf("📎 %d Attachments", len(media)),
			Value: strings.Join(links, " • "),
		})
	}

	// Link attachments (e.g. Lemmy link posts)
	for _, a := range obj.Attachment {
		if apMediaKind(a) != "link" {
			continue
		}
		href := a.Href
		if href == "" {
			href = apLink(a.URL)
		}
		if href == "" || a.Type == "PropertyValue" {
			continue
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: "🔗 Link",
			Value: href,
		})
		break
	}

	if obj.Type == "Question" {
		options := obj.OneOf
		if len(options) == 0 {
			options = obj.AnyOf
		}
		total := 0
		for _, o := range options {
			total += o.Replies.TotalItems
		}
		pollText := fmt.Sprintf("Poll with %d votes", total)
		if len(obj.Closed) > 0 && string(obj.Closed) != "null" && string(obj.Closed) != "false" {
			pollText += " (closed)"
		} else if end, err := time.Parse(time.RFC3339, obj.EndTime); err == nil && end.Before(time.Now()) {
			pollText += " (closed)"
		}
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Poll",
			Value: pollText,
		})
	}

	attachment.Fields = fields
	return attachment
}
