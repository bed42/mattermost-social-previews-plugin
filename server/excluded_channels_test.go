package main

import (
	"encoding/json"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestPlugin builds a Plugin with an injected mock API. KVGet/KVSet are
// backed by an in-memory map so tests can drive the real exclusion methods
// without a running Mattermost server.
func newTestPlugin(t *testing.T, configList []string) (*Plugin, *plugintest.API, map[string][]byte) {
	t.Helper()
	api := &plugintest.API{}
	store := map[string][]byte{}

	api.On("KVGet", mock.AnythingOfType("string")).Return(
		func(key string) []byte { return store[key] },
		func(key string) *model.AppError { return nil },
	)
	api.On("KVSet", mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(
		func(key string, value []byte) *model.AppError {
			store[key] = value
			return nil
		},
	)
	// Log* are variadic — register catch-alls covering 1..8 trailing key/value pairs
	// so any Log call signature in the codebase is accepted.
	for _, name := range []string{"LogInfo", "LogWarn", "LogError", "LogDebug"} {
		for n := 0; n <= 8; n++ {
			args := make([]interface{}, n+1)
			for i := range args {
				args[i] = mock.Anything
			}
			api.On(name, args...).Maybe().Return()
		}
	}

	p := &Plugin{}
	p.SetAPI(api)
	p.setConfiguration(&configuration{excludedChannelsParsed: configList})
	return p, api, store
}

func TestIsChannelExcluded_FromConfig(t *testing.T) {
	p, _, _ := newTestPlugin(t, []string{"channel-from-config"})
	assert.True(t, p.isChannelExcluded("channel-from-config"))
	assert.False(t, p.isChannelExcluded("other-channel"))
	assert.False(t, p.isChannelExcluded(""))
}

func TestIsChannelExcluded_FromKV(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"channel-from-kv"})
	assert.True(t, p.isChannelExcluded("channel-from-kv"))
	assert.False(t, p.isChannelExcluded("other-channel"))
}

func TestMessageWillBePosted_ExcludedChannelSuppressesBuiltInUnfurl(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-x"})

	// Message contains a URL → expect attachments prop to be set so Mattermost's
	// built-in unfurler is bypassed.
	post := &model.Post{ChannelId: "ch-x", Message: "look https://example.com/foo"}
	out, reason := p.MessageWillBePosted(nil, post)
	assert.Empty(t, reason)
	require.NotNil(t, out)
	require.NotNil(t, out.Props)
	atts, ok := out.Props["attachments"].([]*model.SlackAttachment)
	require.True(t, ok, "attachments prop must be present and of slice type")
	assert.Empty(t, atts, "attachments slice should be empty (no rendered card, just suppression)")
}

func TestMessageWillBePosted_ExcludedChannelNoURLLeavesPostAlone(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-x"})

	post := &model.Post{ChannelId: "ch-x", Message: "no links here, just words"}
	out, reason := p.MessageWillBePosted(nil, post)
	assert.Empty(t, reason)
	require.NotNil(t, out)
	// No URL → don't touch props at all
	if out.Props != nil {
		_, has := out.Props["attachments"]
		assert.False(t, has, "should not set attachments prop when message has no URL")
	}
}

func TestMessageWillBePosted_ExcludedChannelIgnoresURLInBackticks(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-x"})

	post := &model.Post{ChannelId: "ch-x", Message: "code: `https://example.com/foo`"}
	out, _ := p.MessageWillBePosted(nil, post)
	require.NotNil(t, out)
	if out.Props != nil {
		_, has := out.Props["attachments"]
		assert.False(t, has, "URLs inside backticks should not trigger unfurl suppression")
	}
}

func TestAddRemoveKVExcludedChannel(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)

	added, err := p.addKVExcludedChannel("ch1")
	require.NoError(t, err)
	assert.True(t, added)

	// Adding the same channel again is a no-op
	added, err = p.addKVExcludedChannel("ch1")
	require.NoError(t, err)
	assert.False(t, added)

	var ids []string
	require.NoError(t, json.Unmarshal(store[kvKeyExcludedChannels], &ids))
	assert.Equal(t, []string{"ch1"}, ids)

	// Remove: returns true the first time, false the second
	removed, err := p.removeKVExcludedChannel("ch1")
	require.NoError(t, err)
	assert.True(t, removed)

	removed, err = p.removeKVExcludedChannel("ch1")
	require.NoError(t, err)
	assert.False(t, removed)

	require.NoError(t, json.Unmarshal(store[kvKeyExcludedChannels], &ids))
	assert.Empty(t, ids)
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
