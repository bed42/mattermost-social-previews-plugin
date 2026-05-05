package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCommand_Disable(t *testing.T) {
	p, _, _ := newTestPlugin(t, nil)

	resp, appErr := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews disable",
		ChannelId: "ch-1",
	})
	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeEphemeral, resp.ResponseType)
	assert.Contains(t, resp.Text, "now disabled")

	// True after the toggle
	assert.True(t, p.isChannelExcluded("ch-1"))

	// Repeat disable: idempotent message
	resp, _ = p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews disable",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "already disabled")
}

func TestExecuteCommand_Enable(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-1"})

	resp, _ := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews enable",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "now enabled")
	assert.False(t, p.isChannelExcluded("ch-1"))

	// Already enabled
	resp, _ = p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews enable",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "already enabled")
}

func TestExecuteCommand_EnableWhileConfigExcluded(t *testing.T) {
	// Channel is excluded by both the System Console list and the KV list.
	// Enable should remove it from KV but warn that the config still excludes it.
	p, _, store := newTestPlugin(t, []string{"ch-1"})
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-1"})

	resp, _ := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews enable",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "System Console")
	// Still excluded by config
	assert.True(t, p.isChannelExcluded("ch-1"))
}

func TestExecuteCommand_Status(t *testing.T) {
	p, _, store := newTestPlugin(t, nil)

	// Default: enabled
	resp, _ := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews status",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "**enabled**")

	// After KV disable
	store[kvKeyExcludedChannels] = mustJSON(t, []string{"ch-1"})
	resp, _ = p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews status",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "**disabled**")
	assert.Contains(t, resp.Text, "/social-previews disable")

	// Bare command with no subcommand defaults to status
	resp, _ = p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "**disabled**")
}

func TestExecuteCommand_StatusFromConfig(t *testing.T) {
	p, _, _ := newTestPlugin(t, []string{"ch-1"})
	resp, _ := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews status",
		ChannelId: "ch-1",
	})
	assert.Contains(t, resp.Text, "**disabled**")
	assert.Contains(t, resp.Text, "System Console")
}

func TestExecuteCommand_UnknownSubcommand(t *testing.T) {
	p, _, _ := newTestPlugin(t, nil)
	resp, _ := p.ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/social-previews wibble",
		ChannelId: "ch-1",
	})
	assert.True(t, strings.Contains(resp.Text, "Unknown subcommand"))
}
