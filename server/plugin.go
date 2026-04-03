package main

import (
	"strings"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	p.API.LogInfo("SOCIAL PREVIEWS: Activated successfully")
	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	p.API.LogInfo("SOCIAL PREVIEWS: Deactivated")
	return nil
}

// MessageWillBePosted is invoked when a message is posted by a user before it is committed to the database.
// This hook allows us to detect Mastodon URLs and add rich preview attachments.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	// Panic recovery to prevent plugin crashes
	defer func() {
		if r := recover(); r != nil {
			p.API.LogError("SOCIAL PREVIEWS: PANIC!", "panic", r)
		}
	}()

	p.API.LogInfo("SOCIAL PREVIEWS: MessageWillBePosted called", "message", post.Message)

	// Strip backtick-wrapped content so URLs in code formatting are ignored
	cleanMessage := stripBacktickContent(post.Message)

	// Strip tracking params from URLs (only from non-backtick portions)
	urlReplacements := cleanMessageURLs(cleanMessage)
	if len(urlReplacements) > 0 {
		for original, clean := range urlReplacements {
			post.Message = strings.ReplaceAll(post.Message, original, clean)
		}
		cleanMessage = stripBacktickContent(post.Message)
		p.API.LogInfo("SOCIAL PREVIEWS: Stripped tracking params", "count", len(urlReplacements))
	}

	// Extract Mastodon URLs from post
	mastodonURLs := extractMastodonURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted URLs", "count", len(mastodonURLs), "urls", mastodonURLs)

	// Extract Threads URLs from post
	threadsURLs := extractThreadsURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted Threads URLs", "count", len(threadsURLs), "urls", threadsURLs)

	// Extract TikTok URLs from post
	tiktokURLs := extractTikTokURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted TikTok URLs", "count", len(tiktokURLs), "urls", tiktokURLs)

	// Extract Bluesky URLs from post
	blueskyURLs := extractBlueskyURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted Bluesky URLs", "count", len(blueskyURLs), "urls", blueskyURLs)

	// Extract Twitter/X URLs from post
	twitterURLs := extractTwitterURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted Twitter/X URLs", "count", len(twitterURLs), "urls", twitterURLs)

	// Extract Instagram URLs from post
	instagramURLs := extractInstagramURLs(cleanMessage)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted Instagram URLs", "count", len(instagramURLs), "urls", instagramURLs)

	// Also extract generic URLs for fallback OG previews
	handledURLs := append(mastodonURLs, threadsURLs...)
	handledURLs = append(handledURLs, tiktokURLs...)
	handledURLs = append(handledURLs, blueskyURLs...)
	handledURLs = append(handledURLs, twitterURLs...)
	handledURLs = append(handledURLs, instagramURLs...)

	// Exclude internal Mattermost links (the server handles its own permalinks)
	siteURL := ""
	if cfg := p.API.GetConfig(); cfg != nil && cfg.ServiceSettings.SiteURL != nil {
		siteURL = *cfg.ServiceSettings.SiteURL
	}
	genericURLs := extractGenericURLs(cleanMessage, handledURLs, siteURL)
	p.API.LogInfo("SOCIAL PREVIEWS: Extracted generic URLs", "count", len(genericURLs), "urls", genericURLs)

	if len(mastodonURLs) == 0 && len(threadsURLs) == 0 && len(tiktokURLs) == 0 && len(blueskyURLs) == 0 && len(twitterURLs) == 0 && len(instagramURLs) == 0 && len(genericURLs) == 0 {
		p.API.LogInfo("SOCIAL PREVIEWS: No preview URLs found, skipping")
		return post, ""
	}

	// Fetch data for each Mastodon URL
	attachments := []*model.SlackAttachment{}
	for _, url := range mastodonURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching Mastodon post", "url", url)

		mastodonPost, err := p.fetchMastodonPost(url)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched", "url", url, "author", mastodonPost.Account.Username)

		// If this is a reply, fetch and show the parent post first
		replyToUsername := ""
		if mastodonPost.InReplyToID != nil {
			instanceURL, _, ok := parseMastodonURL(url)
			if ok {
				parentPost, err := p.fetchMastodonStatus(instanceURL, *mastodonPost.InReplyToID)
				if err != nil {
					p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch parent post", "parentID", *mastodonPost.InReplyToID, "error", err.Error())
				} else {
					parentAttachment := buildAttachment(parentPost, parentPost.URL, "")
					attachments = append(attachments, parentAttachment)
					replyToUsername = parentPost.Account.Acct
				}
			}
		}

		// Create message attachment first
		attachment := buildAttachment(mastodonPost, url, replyToUsername)
		attachments = append(attachments, attachment)

		// Then check for embedded Mastodon URLs in the post content (e.g. "RE:" quote links)
		embeddedURLs := extractMastodonURLs(mastodonPost.Content)
		for _, embeddedURL := range embeddedURLs {
			p.API.LogInfo("SOCIAL PREVIEWS: Fetching embedded Mastodon post", "url", embeddedURL)
			embeddedPost, err := p.fetchMastodonPost(embeddedURL)
			if err != nil {
				p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch embedded post", "url", embeddedURL, "error", err.Error())
				continue
			}
			embeddedAttachment := buildAttachment(embeddedPost, embeddedURL, "")
			attachments = append(attachments, embeddedAttachment)
		}
	}

	// Fetch data for each Threads URL
	for _, url := range threadsURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching Threads post", "url", url)

		threadsPost, err := fetchThreadsPost(url)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch Threads post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched Threads post", "url", url, "title", threadsPost.Title)

		attachment := buildThreadsAttachment(threadsPost, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each TikTok URL
	for _, url := range tiktokURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching TikTok video", "url", url)

		oembed, err := fetchTikTokOEmbed(url)
		if err != nil {
			// Direct oEmbed failed — try resolving redirects and normalizing /photo/ to /video/
			p.API.LogDebug("SOCIAL PREVIEWS: Direct TikTok oEmbed failed, trying resolve fallback", "url", url, "error", err.Error())
			resolvedURL, resolveErr := resolveTikTokForOEmbed(url)
			if resolveErr != nil {
				p.API.LogWarn("SOCIAL PREVIEWS: Failed to resolve TikTok URL", "url", url, "error", resolveErr.Error())
				continue
			}
			oembed, err = fetchTikTokOEmbed(resolvedURL)
			if err != nil {
				p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch TikTok video", "url", url, "error", err.Error())
				continue
			}
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched TikTok video", "url", url, "author", oembed.AuthorName)

		attachment := buildTikTokAttachment(oembed, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Bluesky URL
	for _, url := range blueskyURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching Bluesky post", "url", url)

		handle, rkey, ok := parseBlueskyURL(url)
		if !ok {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to parse Bluesky URL", "url", url)
			continue
		}

		bskyPost, err := fetchBlueskyPost(handle, rkey)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch Bluesky post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched Bluesky post", "url", url, "author", bskyPost.Author.Handle)

		attachment := buildBlueskyAttachment(bskyPost, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Twitter/X URL
	for _, url := range twitterURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching Twitter/X post", "url", url)

		username, tweetID, ok := parseTwitterURL(url)
		if !ok {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to parse Twitter/X URL", "url", url)
			continue
		}

		tweet, err := fetchTwitterPost(username, tweetID)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch Twitter/X post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched Twitter/X post", "url", url, "author", tweet.Author.ScreenName)

		attachment := buildTwitterAttachment(tweet, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Instagram URL
	for _, url := range instagramURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching Instagram post", "url", url)

		igPost, err := fetchInstagramPost(url)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to fetch Instagram post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched Instagram post", "url", url, "title", igPost.Title)

		attachment := buildInstagramAttachment(igPost, url)
		attachments = append(attachments, attachment)
	}

	// Fallback: generic OG preview for unhandled URLs
	for _, url := range genericURLs {
		p.API.LogInfo("SOCIAL PREVIEWS: Fetching generic OG preview", "url", url)

		preview, err := fetchOGPreview(url)
		if err != nil {
			p.API.LogDebug("SOCIAL PREVIEWS: Direct OG fetch failed, trying noembed fallback", "url", url, "error", err.Error())

			preview, err = fetchNoembedPreview(url)
			if err != nil {
				p.API.LogDebug("SOCIAL PREVIEWS: No preview available", "url", url, "error", err.Error())
				continue
			}
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Successfully fetched OG preview", "url", url, "title", preview.Title)

		attachment := buildOGAttachment(preview, url)
		attachments = append(attachments, attachment)
	}

	// Attach to post props
	if len(attachments) > 0 {
		p.API.LogInfo("SOCIAL PREVIEWS: Adding attachments to post", "count", len(attachments))

		if post.Props == nil {
			post.Props = make(map[string]interface{})
		}

		// Append to existing attachments if any
		existingAttachments, ok := post.Props["attachments"].([]*model.SlackAttachment)
		if ok {
			attachments = append(existingAttachments, attachments...)
		}

		post.Props["attachments"] = attachments
		p.API.LogInfo("SOCIAL PREVIEWS: ✅ Attachments added successfully!")
	}

	return post, ""
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
