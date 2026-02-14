package main

import (
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
	p.API.LogInfo("🦣 MASTODON PLUGIN: Activated successfully")
	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	p.API.LogInfo("🦣 MASTODON PLUGIN: Deactivated")
	return nil
}

// MessageWillBePosted is invoked when a message is posted by a user before it is committed to the database.
// This hook allows us to detect Mastodon URLs and add rich preview attachments.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	// Panic recovery to prevent plugin crashes
	defer func() {
		if r := recover(); r != nil {
			p.API.LogError("🦣 MASTODON PLUGIN: PANIC!", "panic", r)
		}
	}()

	p.API.LogInfo("🦣 MASTODON PLUGIN: MessageWillBePosted called", "message", post.Message)

	// Extract Mastodon URLs from post
	mastodonURLs := extractMastodonURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted URLs", "count", len(mastodonURLs), "urls", mastodonURLs)

	// Extract Threads URLs from post
	threadsURLs := extractThreadsURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted Threads URLs", "count", len(threadsURLs), "urls", threadsURLs)

	// Extract TikTok URLs from post
	tiktokURLs := extractTikTokURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted TikTok URLs", "count", len(tiktokURLs), "urls", tiktokURLs)

	// Extract Bluesky URLs from post
	blueskyURLs := extractBlueskyURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted Bluesky URLs", "count", len(blueskyURLs), "urls", blueskyURLs)

	// Extract Twitter/X URLs from post
	twitterURLs := extractTwitterURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted Twitter/X URLs", "count", len(twitterURLs), "urls", twitterURLs)

	// Extract Instagram URLs from post
	instagramURLs := extractInstagramURLs(post.Message)
	p.API.LogInfo("🦣 MASTODON PLUGIN: Extracted Instagram URLs", "count", len(instagramURLs), "urls", instagramURLs)

	if len(mastodonURLs) == 0 && len(threadsURLs) == 0 && len(tiktokURLs) == 0 && len(blueskyURLs) == 0 && len(twitterURLs) == 0 && len(instagramURLs) == 0 {
		p.API.LogInfo("🦣 MASTODON PLUGIN: No preview URLs found, skipping")
		return post, ""
	}

	// Fetch data for each Mastodon URL
	attachments := []*model.SlackAttachment{}
	for _, url := range mastodonURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching Mastodon post", "url", url)

		mastodonPost, err := p.fetchMastodonPost(url)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched", "url", url, "author", mastodonPost.Account.Username)

		// Create message attachment
		config := p.getConfiguration()
		attachment := buildAttachment(mastodonPost, url, config)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Threads URL
	for _, url := range threadsURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching Threads post", "url", url)

		threadsPost, err := fetchThreadsPost(url)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch Threads post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched Threads post", "url", url, "title", threadsPost.Title)

		attachment := buildThreadsAttachment(threadsPost, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each TikTok URL
	for _, url := range tiktokURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching TikTok video", "url", url)

		oembed, err := fetchTikTokOEmbed(url)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch TikTok video", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched TikTok video", "url", url, "author", oembed.AuthorName)

		attachment := buildTikTokAttachment(oembed, url)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Bluesky URL
	for _, url := range blueskyURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching Bluesky post", "url", url)

		handle, rkey, ok := parseBlueskyURL(url)
		if !ok {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to parse Bluesky URL", "url", url)
			continue
		}

		bskyPost, err := fetchBlueskyPost(handle, rkey)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch Bluesky post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched Bluesky post", "url", url, "author", bskyPost.Author.Handle)

		config := p.getConfiguration()
		attachment := buildBlueskyAttachment(bskyPost, url, config)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Twitter/X URL
	for _, url := range twitterURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching Twitter/X post", "url", url)

		username, tweetID, ok := parseTwitterURL(url)
		if !ok {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to parse Twitter/X URL", "url", url)
			continue
		}

		tweet, err := fetchTwitterPost(username, tweetID)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch Twitter/X post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched Twitter/X post", "url", url, "author", tweet.Author.ScreenName)

		config := p.getConfiguration()
		attachment := buildTwitterAttachment(tweet, url, config)
		attachments = append(attachments, attachment)
	}

	// Fetch data for each Instagram URL
	for _, url := range instagramURLs {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Fetching Instagram post", "url", url)

		igPost, err := fetchInstagramPost(url)
		if err != nil {
			p.API.LogWarn("🦣 MASTODON PLUGIN: Failed to fetch Instagram post", "url", url, "error", err.Error())
			continue
		}

		p.API.LogInfo("🦣 MASTODON PLUGIN: Successfully fetched Instagram post", "url", url, "title", igPost.Title)

		attachment := buildInstagramAttachment(igPost, url)
		attachments = append(attachments, attachment)
	}

	// Attach to post props
	if len(attachments) > 0 {
		p.API.LogInfo("🦣 MASTODON PLUGIN: Adding attachments to post", "count", len(attachments))

		if post.Props == nil {
			post.Props = make(map[string]interface{})
		}

		// Append to existing attachments if any
		existingAttachments, ok := post.Props["attachments"].([]*model.SlackAttachment)
		if ok {
			attachments = append(existingAttachments, attachments...)
		}

		post.Props["attachments"] = attachments
		p.API.LogInfo("🦣 MASTODON PLUGIN: ✅ Attachments added successfully!")
	}

	return post, ""
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
