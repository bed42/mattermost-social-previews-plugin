package main

import "time"

// MastodonStatus represents a status (post/toot) from the Mastodon API
type MastodonStatus struct {
	ID               string              `json:"id"`
	CreatedAt        time.Time           `json:"created_at"`
	Content          string              `json:"content"`
	SpoilerText      string              `json:"spoiler_text"`
	Visibility       string              `json:"visibility"`
	Sensitive        bool                `json:"sensitive"`
	URL              string              `json:"url"`
	RepliesCount     int                 `json:"replies_count"`
	ReblogsCount     int                 `json:"reblogs_count"`
	FavouritesCount  int                 `json:"favourites_count"`
	Account          MastodonAccount     `json:"account"`
	MediaAttachments []MastodonMedia     `json:"media_attachments"`
	Mentions         []MastodonMention   `json:"mentions"`
	Tags             []MastodonTag       `json:"tags"`
	Card             *MastodonCard       `json:"card"`
	Poll             *MastodonPoll       `json:"poll"`
	Reblog           *MastodonStatus     `json:"reblog"`
	Language         string              `json:"language"`
}

// MastodonAccount represents a Mastodon user account
type MastodonAccount struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Acct         string `json:"acct"`
	DisplayName  string `json:"display_name"`
	Avatar       string `json:"avatar"`
	AvatarStatic string `json:"avatar_static"`
	URL          string `json:"url"`
	Bot          bool   `json:"bot"`
}

// MastodonMedia represents a media attachment (image, video, audio, etc.)
type MastodonMedia struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // image, video, gifv, audio
	URL         string    `json:"url"`
	PreviewURL  string    `json:"preview_url"`
	Description string    `json:"description"`
	Blurhash    string    `json:"blurhash"`
	Meta        MediaMeta `json:"meta"`
}

// MediaMeta contains metadata about media files
type MediaMeta struct {
	Original MediaDetails `json:"original"`
	Small    MediaDetails `json:"small"`
}

// MediaDetails contains dimensions and aspect ratio
type MediaDetails struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Size   string  `json:"size"`
	Aspect float64 `json:"aspect"`
}

// MastodonCard represents a link preview card
type MastodonCard struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"` // link, photo, video, rich
	Image       string `json:"image"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

// MastodonPoll represents a poll in a status
type MastodonPoll struct {
	ID         string       `json:"id"`
	ExpiresAt  *time.Time   `json:"expires_at"`
	Expired    bool         `json:"expired"`
	Multiple   bool         `json:"multiple"`
	VotesCount int          `json:"votes_count"`
	Options    []PollOption `json:"options"`
}

// PollOption represents a single option in a poll
type PollOption struct {
	Title      string `json:"title"`
	VotesCount int    `json:"votes_count"`
}

// MastodonMention represents a mentioned user
type MastodonMention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URL      string `json:"url"`
}

// MastodonTag represents a hashtag
type MastodonTag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
