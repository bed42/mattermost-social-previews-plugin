package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	commandTrigger = "social-previews"
	commandHint    = "[disable|enable|status]"
	commandDesc    = "Disable, enable, or check social-preview generation in the current channel"
	commandHelp    = "Use `/social-previews disable` to turn off previews in this channel, `/social-previews enable` to turn them back on, or `/social-previews status` to check."
)

// registerCommand registers the /social-previews slash command. Called from OnActivate.
func (p *Plugin) registerCommand() error {
	auto := model.NewAutocompleteData(commandTrigger, commandHint, commandDesc)
	auto.AddCommand(model.NewAutocompleteData("disable", "", "Disable previews in this channel"))
	auto.AddCommand(model.NewAutocompleteData("enable", "", "Re-enable previews in this channel"))
	auto.AddCommand(model.NewAutocompleteData("status", "", "Show whether previews are enabled in this channel"))

	return p.API.RegisterCommand(&model.Command{
		Trigger:          commandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: commandDesc,
		AutoCompleteHint: commandHint,
		AutocompleteData: auto,
	})
}

// ExecuteCommand handles the /social-previews slash command.
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	fields := strings.Fields(args.Command)
	if len(fields) == 0 || strings.TrimPrefix(fields[0], "/") != commandTrigger {
		return ephemeral(commandHelp), nil
	}

	sub := ""
	if len(fields) > 1 {
		sub = strings.ToLower(fields[1])
	}

	switch sub {
	case "disable":
		return p.handleDisable(args), nil
	case "enable":
		return p.handleEnable(args), nil
	case "status", "":
		return p.handleStatus(args), nil
	default:
		return ephemeral(fmt.Sprintf("Unknown subcommand `%s`.\n\n%s", sub, commandHelp)), nil
	}
}

func (p *Plugin) handleDisable(args *model.CommandArgs) *model.CommandResponse {
	added, err := p.addKVExcludedChannel(args.ChannelId)
	if err != nil {
		p.API.LogError("SOCIAL PREVIEWS: Failed to disable channel", "channel_id", args.ChannelId, "error", err.Error())
		return ephemeral("Failed to update channel exclusion list. Check the server logs.")
	}
	if !added {
		return ephemeral("Social previews are already disabled in this channel.")
	}
	return ephemeral("Social previews are now disabled in this channel.")
}

func (p *Plugin) handleEnable(args *model.CommandArgs) *model.CommandResponse {
	removed, err := p.removeKVExcludedChannel(args.ChannelId)
	if err != nil {
		p.API.LogError("SOCIAL PREVIEWS: Failed to enable channel", "channel_id", args.ChannelId, "error", err.Error())
		return ephemeral("Failed to update channel exclusion list. Check the server logs.")
	}
	// Note: a channel may still be excluded via the System Console setting.
	if p.isChannelInConfigList(args.ChannelId) {
		msg := "This channel is excluded by the System Console `Excluded Channels` setting; an admin must remove it there to re-enable previews."
		if removed {
			msg = "Removed channel from the slash-command list, but " + msg
		}
		return ephemeral(msg)
	}
	if !removed {
		return ephemeral("Social previews are already enabled in this channel.")
	}
	return ephemeral("Social previews are now enabled in this channel.")
}

func (p *Plugin) handleStatus(args *model.CommandArgs) *model.CommandResponse {
	if p.isChannelExcluded(args.ChannelId) {
		source := "via /social-previews disable"
		if p.isChannelInConfigList(args.ChannelId) {
			source = "via the System Console `Excluded Channels` setting"
		}
		return ephemeral(fmt.Sprintf("Social previews are **disabled** in this channel (%s).", source))
	}
	return ephemeral("Social previews are **enabled** in this channel.")
}

// isChannelInConfigList reports whether channelID is excluded by the System
// Console setting (as opposed to the slash-command-managed KV list).
func (p *Plugin) isChannelInConfigList(channelID string) bool {
	for _, id := range p.getConfiguration().excludedChannelsParsed {
		if id == channelID {
			return true
		}
	}
	return false
}

func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}
