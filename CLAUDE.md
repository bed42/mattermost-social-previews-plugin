# Social Previews Plugin for Mattermost

## What It Does

A Mattermost plugin that automatically generates rich previews when users post social media URLs. It intercepts messages via the `MessageWillBePosted` hook, detects URLs from supported platforms, fetches post data from public APIs or oEmbed endpoints, and renders formatted SlackAttachments with the post content.

## Supported Platforms

- **Mastodon** (and compatible: Pixelfed, Pleroma, etc.) - via Mastodon public API (`/api/v1/statuses/{id}`)
- **Bluesky** - via AT Protocol public API
- **Twitter/X** - via FxTwitter API
- **Threads** - via oEmbed
- **TikTok** - via oEmbed
- **Instagram** - via oEmbed

## Features

- **Rich previews**: Author info (name, avatar, profile link), post content (HTML stripped to plain text)
- **Media support**: First image shown inline; video thumbnails; links to all attachments when multiple exist
- **Poll display**: Vote counts and expiration status (Mastodon, Bluesky)
- **Link preview cards**: Open Graph metadata (title, description, image) for URLs shared in posts
- **Cross-instance**: Works with any Mastodon-compatible instance
- **Public-only**: No authentication required; only previews public posts

## Key Files

| File | Role |
|------|------|
| `server/plugin.go` | Entry point; `MessageWillBePosted` hook, `OnActivate`/`OnDeactivate` lifecycle |
| `server/mastodon.go` | Mastodon API client, HTML stripping (`stripHTML`), attachment builder |
| `server/bluesky.go` | Bluesky AT Protocol client and attachment builder |
| `server/twitter.go` | Twitter/X preview via FxTwitter API |
| `server/threads.go` | Threads preview via oEmbed |
| `server/tiktok.go` | TikTok preview via oEmbed |
| `server/instagram.go` | Instagram preview via oEmbed |
| `server/types.go` | Mastodon API data structures (`MastodonStatus`, `MastodonAccount`, `MastodonMedia`, etc.) |
| `server/url_utils.go` | Regex-based URL detection and parsing for Mastodon URLs |
| `server/api.go` | HTTP API routes (minimal - `/api/v1/hello`) |
| `plugin.json` | Plugin manifest (ID: `social-previews`, min Mattermost 7.0.0) |

## URL Patterns Matched

- Mastodon: `https://instance.domain/@username/123456`, `/users/username/statuses/123456`, federated
- Bluesky: `https://bsky.app/profile/handle/post/rkey`
- Twitter/X: `https://twitter.com/user/status/id`, `https://x.com/user/status/id`
- Threads: `https://www.threads.net/@user/post/id`
- TikTok: `https://www.tiktok.com/@user/video/id`
- Instagram: `https://www.instagram.com/p/id/`, `/reel/id/`

## Build

- `make` — lint + test + dist
- `make server` — compile Go
- `make test` — run all tests
- `make dist` — bundle plugin
- `make deploy` — deploy to Mattermost instance

# Agents: Mandatory: use td usage --new-session to see open work

# Agents: Before context ends, ALWAYS run

td handoff <issue-id> --done "..." --remaining "..." --decision "..." --uncertain "..."
