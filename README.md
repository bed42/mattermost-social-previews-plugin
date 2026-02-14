# Social Previews Plugin for Mattermost

> **Disclaimer:** This project was built with the guidance of a human developer and implemented primarily by [Claude Code](https://claude.com/claude-code) (Anthropic's AI coding assistant). If LLM-assisted development is not your thing, consider this your fair warning.

A Mattermost plugin that automatically displays rich previews for social media URLs. Supports **Mastodon**, **Bluesky**, **Twitter/X**, **Threads**, **TikTok**, and **Instagram**. Works on all platforms including **web, desktop, iOS, and Android**.

## Features

- **Multi-platform** - Previews from Mastodon, Bluesky, Twitter/X, Threads, TikTok, and Instagram
- **Cross-platform** - Works on all Mattermost clients (web, desktop, mobile)
- **Automatic detection** - Detects social media URLs and generates previews inline
- **Rich previews** - Shows author, avatar, content, images, and engagement metrics
- **No configuration** - Works out of the box with default settings
- **Privacy-friendly** - Only fetches public posts, no authentication required

## How it Works

The plugin uses Mattermost's **message attachments** API (same approach as GitHub and Jira plugins):

1. When a user posts a message containing a supported social media URL
2. The plugin's `MessageWillBePosted` hook detects the URL
3. The plugin fetches the post data from the platform's public API or via oEmbed
4. A rich preview attachment is added to the post
5. All clients (web, desktop, mobile) display the preview

## Supported Platforms & URL Patterns

### Mastodon (and compatible: Pixelfed, Pleroma, etc.)

- `https://mastodon.social/@username/123456789`
- `https://fosstodon.org/users/username/statuses/123456789`
- `https://instance.tld/@username@other.instance/123456789` (federated)

### Bluesky

- `https://bsky.app/profile/username.bsky.social/post/abc123`

### Twitter / X

- `https://twitter.com/username/status/123456789`
- `https://x.com/username/status/123456789`

### Threads

- `https://www.threads.net/@username/post/abc123`

### TikTok

- `https://www.tiktok.com/@username/video/123456789`

### Instagram

- `https://www.instagram.com/p/abc123/`
- `https://www.instagram.com/reel/abc123/`

## Preview Content

Each preview displays (where available per platform):

- **Author information** - Display name, username, and avatar
- **Post content** - Text content with HTML formatting converted to plain text
- **Media** - Images or video thumbnails (if present)
- **Engagement metrics** - Replies, reposts/boosts, and likes/favorites (togglable in settings)
- **Poll information** - Poll vote count and status (Mastodon, Bluesky)
- **Link** - Click-through to the original post

## Installation

### Option 1: Upload Pre-built Plugin

1. Download the latest release from the [Releases page](https://github.com/bed/mattermost-social-previews-plugin/releases)
2. Go to **System Console > Plugins > Management**
3. Click **Upload Plugin**
4. Select the downloaded `.tar.gz` file
5. Click **Enable** on the plugin

### Option 2: Build from Source

Requirements:

- Go 1.21 or higher
- Node.js 16+ and npm 8+
- Make

```bash
# Clone the repository
git clone https://github.com/bed/mattermost-social-previews-plugin.git
cd mattermost-social-previews-plugin

# Build the plugin
make

# The plugin bundle will be created at:
# dist/social-previews-1.0.0.tar.gz
```

Then upload via System Console as described in Option 1.

## Usage

Simply post a social media URL in any channel or direct message:

```
Check out this post: https://mastodon.social/@someone/123456789
Look at this: https://bsky.app/profile/someone.bsky.social/post/abc123
```

The plugin will automatically add a rich preview below your message.

## Configuration

Go to **System Console > Plugins > Social Previews** to configure:

- **Show Engagement Metrics** - Toggle display of reply/boost/favorite counts (default: enabled)

## Limitations

1. **Public posts only** - The plugin can only preview public posts (no authentication)
2. **First media only** - Only the first image/video attachment is shown in the preview
3. **Post creation latency** - Fetching data adds 100-500ms delay when posting
4. **Rate limits** - Social media platforms may rate limit requests
5. **Layout constraints** - Preview layout is constrained by Mattermost's attachment format
6. **Platform API changes** - Third-party APIs may change without notice

## Architecture

### Server Component (Go)

| File | Role |
|------|------|
| [server/plugin.go](server/plugin.go) | Main plugin with `MessageWillBePosted` hook |
| [server/mastodon.go](server/mastodon.go) | Mastodon API client and attachment builder |
| [server/bluesky.go](server/bluesky.go) | Bluesky AT Protocol client and attachment builder |
| [server/twitter.go](server/twitter.go) | Twitter/X preview via FxTwitter API |
| [server/threads.go](server/threads.go) | Threads preview via oEmbed |
| [server/tiktok.go](server/tiktok.go) | TikTok preview via oEmbed |
| [server/instagram.go](server/instagram.go) | Instagram preview via oEmbed |
| [server/types.go](server/types.go) | Mastodon API data structures |
| [server/url_utils.go](server/url_utils.go) | URL pattern matching and parsing |
| [server/configuration.go](server/configuration.go) | Plugin configuration management |

## Development

### Running Tests

```bash
# Run all tests
make test

# Run Go server tests only
make test-server
```

### Deploying for Development

If your Mattermost server is running locally:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=your-admin-token
make deploy
```

### Debugging

Enable plugin debug logging in Mattermost:

1. Go to **System Console > Environment > Logging**
2. Set **File Log Level** to `DEBUG`
3. Check logs at `/var/log/mattermost/mattermost.log`

Look for log entries prefixed with `social-previews`

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Credits

Built using the [Mattermost Plugin Starter Template](https://github.com/mattermost/mattermost-plugin-starter-template).
