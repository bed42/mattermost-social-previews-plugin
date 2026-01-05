# Mastodon Preview Plugin for Mattermost

A Mattermost plugin that automatically displays rich previews for Mastodon status URLs. Works on all platforms including **web, desktop, iOS, and Android**.

## Features

✅ **Cross-platform** - Works on all Mattermost clients (web, desktop, mobile)
✅ **Automatic detection** - Detects Mastodon URLs from any federated instance
✅ **Rich previews** - Shows author, avatar, content, images, and engagement metrics
✅ **No configuration** - Works out of the box with default settings
✅ **Privacy-friendly** - Only fetches public posts, no authentication required

## How it Works

The plugin uses Mattermost's **message attachments** API (same approach as GitHub and Jira plugins):

1. When a user posts a message containing a Mastodon URL
2. The plugin's `MessageWillBePosted` hook detects the URL
3. The plugin fetches the status data from the Mastodon instance's public API
4. A rich preview attachment is added to the post
5. All clients (web, desktop, mobile) display the preview

## Supported URL Patterns

The plugin recognizes these Mastodon URL formats:

- `https://mastodon.social/@username/123456789`
- `https://fosstodon.org/users/username/statuses/123456789`
- `https://instance.tld/@username@other.instance/123456789` (federated)

Works with **any Mastodon instance** (Mastodon, Pixelfed, Pleroma, etc.)

## Preview Content

Each preview displays:

- **Author information** - Display name, username, and avatar
- **Post content** - Text content with HTML formatting converted to plain text
- **Media** - First image or video thumbnail (if present)
- **Engagement metrics** - Reply count, boost count, and favorites count
- **Poll information** - Poll vote count and status (if present)
- **Link** - Click-through to original post on Mastodon

## Installation

### Option 1: Upload Pre-built Plugin

1. Download the latest release: [dist/mastodon-preview-1.0.0.tar.gz](dist/mastodon-preview-1.0.0.tar.gz)
2. Go to **System Console → Plugins → Management**
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
cd mastodon-mattermost-plugin

# Build the plugin
make

# The plugin bundle will be created at:
# dist/mastodon-preview-1.0.0.tar.gz
```

Then upload via System Console as described in Option 1.

## Usage

Simply post a Mastodon URL in any channel or direct message:

```
Check out this post: https://mastodon.social/@someone/123456789
```

The plugin will automatically add a rich preview below your message.

## Configuration

No configuration required! The plugin works out of the box.

## Limitations

1. **Public posts only** - The plugin can only preview public Mastodon posts (no authentication)
2. **First media only** - Only the first image/video attachment is shown in the preview
3. **Polls not interactive** - Poll results are shown as text, not interactive voting
4. **Post creation latency** - Fetching Mastodon data adds 100-500ms delay when posting
5. **Rate limits** - Popular Mastodon instances may rate limit requests
6. **Layout constraints** - Preview layout is constrained by Mattermost's attachment format

## Architecture

### Server Component (Go)

- [server/plugin.go](server/plugin.go) - Main plugin with `MessageWillBePosted` hook
- [server/mastodon.go](server/mastodon.go) - Mastodon API client
- [server/types.go](server/types.go) - Data structures for Mastodon API responses
- [server/url_utils.go](server/url_utils.go) - URL pattern matching and parsing

### Message Attachments

The plugin uses Mattermost's standard message attachment format:

```go
attachment := map[string]interface{}{
    "fallback":    "Mastodon post by @username",
    "color":       "#6364FF", // Mastodon brand color
    "author_name": "Display Name",
    "author_link": "https://instance.tld/@username",
    "author_icon": "https://instance.tld/avatar.jpg",
    "title":       "@username",
    "title_link":  "https://instance.tld/@username/123",
    "text":        "Post content...",
    "image_url":   "https://instance.tld/media/image.jpg",
    "fields": [
        {"title": "Replies", "value": "10", "short": true},
        {"title": "Boosts", "value": "25", "short": true},
        {"title": "Favorites", "value": "50", "short": true},
    ],
}
```

## Development

### Running Tests

```bash
# Run Go tests
make test-server

# Run JavaScript tests
make test-webapp

# Run all tests
make test
```

### Linting

```bash
# Check code style
make check-style
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

1. Go to **System Console → Environment → Logging**
2. Set **File Log Level** to `DEBUG`
3. Check logs at `/var/log/mattermost/mattermost.log`

Look for log entries prefixed with `mastodon-preview`

## Troubleshooting

### Preview not showing

- Check that the URL matches a supported Mastodon pattern
- Verify the post is public (not followers-only or private)
- Check Mattermost logs for errors
- Try the URL directly in a browser to ensure it's accessible

### "Rate limited" errors

- The Mastodon instance is rate limiting requests
- This is a temporary issue - try again later
- Consider asking the instance admin to whitelist your Mattermost server

### "Status not found or private"

- The post may have been deleted
- The post may be private/followers-only
- The Mastodon instance may be down

## Future Enhancements

Potential improvements for future versions:

- **Web-only rich previews** - Add custom React components for richer web/desktop experience
- **Caching** - Cache fetched data to reduce API calls and improve performance
- **Configuration options** - Allow admins to disable plugin per-channel or per-team
- **Multiple media** - Display all media attachments, not just the first
- **Interactive polls** - Show poll results with visual bars (web/desktop only)
- **Instance verification** - Verify domains are actually Mastodon instances via NodeInfo

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Credits

Built using the [Mattermost Plugin Starter Template](https://github.com/mattermost/mattermost-plugin-starter-template)

Inspired by the official Mattermost GitHub and Jira plugins.
