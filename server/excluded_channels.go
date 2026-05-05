package main

import (
	"encoding/json"
)

// kvKeyExcludedChannels stores channel IDs toggled off via the
// /social-previews slash command. Stored as a JSON array of strings.
const kvKeyExcludedChannels = "excluded_channels"

// loadKVExcludedChannels reads the slash-command-managed exclusion list from
// the plugin KV store. Returns an empty slice if the key is absent.
func (p *Plugin) loadKVExcludedChannels() ([]string, error) {
	raw, appErr := p.API.KVGet(kvKeyExcludedChannels)
	if appErr != nil {
		return nil, appErr
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// saveKVExcludedChannels writes the slash-command-managed exclusion list to
// the plugin KV store.
func (p *Plugin) saveKVExcludedChannels(ids []string) error {
	if ids == nil {
		ids = []string{}
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	if appErr := p.API.KVSet(kvKeyExcludedChannels, data); appErr != nil {
		return appErr
	}
	return nil
}

// isChannelExcluded reports whether channelID is excluded by either the
// System Console setting or the slash-command-managed KV list.
func (p *Plugin) isChannelExcluded(channelID string) bool {
	if channelID == "" {
		return false
	}
	for _, id := range p.getConfiguration().excludedChannelsParsed {
		if id == channelID {
			return true
		}
	}
	kv, err := p.loadKVExcludedChannels()
	if err != nil {
		p.API.LogWarn("SOCIAL PREVIEWS: Failed to load excluded channels from KV", "error", err.Error())
		return false
	}
	for _, id := range kv {
		if id == channelID {
			return true
		}
	}
	return false
}

// addKVExcludedChannel adds channelID to the slash-command-managed list.
// Returns true if it was newly added, false if it was already present.
func (p *Plugin) addKVExcludedChannel(channelID string) (bool, error) {
	ids, err := p.loadKVExcludedChannels()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == channelID {
			return false, nil
		}
	}
	ids = append(ids, channelID)
	if err := p.saveKVExcludedChannels(ids); err != nil {
		return false, err
	}
	return true, nil
}

// removeKVExcludedChannel removes channelID from the slash-command-managed
// list. Returns true if it was removed, false if it wasn't present.
func (p *Plugin) removeKVExcludedChannel(channelID string) (bool, error) {
	ids, err := p.loadKVExcludedChannels()
	if err != nil {
		return false, err
	}
	out := make([]string, 0, len(ids))
	removed := false
	for _, id := range ids {
		if id == channelID {
			removed = true
			continue
		}
		out = append(out, id)
	}
	if !removed {
		return false, nil
	}
	if err := p.saveKVExcludedChannels(out); err != nil {
		return false, err
	}
	return true, nil
}
