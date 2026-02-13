# Mastodon Preview Plugin for Mattermost

## What It Does
A Mattermost plugin that automatically generates rich previews when users post Mastodon URLs. It intercepts messages via the `MessageWillBePosted` hook, detects Mastodon-style URLs, fetches post data from the Mastodon public API (`/api/v1/statuses/{id}`), and renders a formatted SlackAttachment with the post content.

## Features
- **Rich previews**: Author info (name, avatar, profile link), post content (HTML stripped to plain text)
- **Media support**: First image shown inline; video thumbnails; links to all attachments when multiple exist
- **Engagement metrics**: Reply, boost, and favorite counts (togglable via `show_engagement_metrics` setting)
- **Poll display**: Vote counts and expiration status
- **Link preview cards**: Open Graph metadata (title, description, image) for URLs shared in posts
- **Cross-instance**: Works with any Mastodon-compatible instance (Mastodon, Pixelfed, Pleroma, etc.)
- **Public-only**: No authentication required; only previews public posts

## Key Files
| File | Role |
|------|------|
| `server/plugin.go` | Entry point; `MessageWillBePosted` hook, `OnActivate`/`OnDeactivate` lifecycle |
| `server/mastodon.go` | API client (`fetchMastodonStatus`), HTML stripping (`stripHTML`), attachment builder (`buildAttachment`) |
| `server/types.go` | Mastodon API data structures (`MastodonStatus`, `MastodonAccount`, `MastodonMedia`, `MastodonCard`, `MastodonPoll`, etc.) |
| `server/url_utils.go` | Regex-based URL detection and parsing (extracts instance URL + status ID) |
| `server/configuration.go` | Thread-safe config management for `show_engagement_metrics` setting |
| `server/api.go` | HTTP API routes (minimal — `/api/v1/hello`) |
| `plugin.json` | Plugin manifest (ID: `mastodon-preview`, min Mattermost 7.0.0) |

## URL Patterns Matched
- `https://instance.domain/@username/123456`
- `https://instance.domain/users/username/statuses/123456`
- `https://instance.domain/@username@other.instance/123456` (federated)

## Build
- `make` — lint + test + dist
- `make server` — compile Go
- `make test` — run all tests
- `make dist` — bundle plugin
- `make deploy` — deploy to Mattermost instance

# Agents: Mandatory: use td usage --new-session to see open work

# Agents: Before context ends, ALWAYS run

td handoff <issue-id> --done "..." --remaining "..." --decision "..." --uncertain "..."
