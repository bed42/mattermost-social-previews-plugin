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

	if len(mastodonURLs) == 0 {
		p.API.LogInfo("🦣 MASTODON PLUGIN: No Mastodon URLs found, skipping")
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
