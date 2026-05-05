package main

import (
	"reflect"
	"strings"
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type configuration struct {
	// DisabledDomains is the raw textarea input from the System Console.
	// Domains may be separated by newlines or commas. URLs whose host matches
	// (or is a subdomain of) any entry will not generate previews.
	DisabledDomains string

	// ExcludedChannels is the raw textarea input from the System Console
	// holding channel IDs (newline or comma separated). Posts in these
	// channels will not generate previews. Admins can also toggle a channel
	// via the /social-previews slash command, which is stored separately.
	ExcludedChannels string

	// disabledDomainsParsed is the lowercased, trimmed list derived from
	// DisabledDomains. Populated in OnConfigurationChange so we don't re-parse
	// on every message.
	disabledDomainsParsed []string

	// excludedChannelsParsed is the trimmed list of channel IDs derived from
	// ExcludedChannels. Populated in OnConfigurationChange.
	excludedChannelsParsed []string
}

// Clone deep-copies the configuration, including the parsed slices.
func (c *configuration) Clone() *configuration {
	clone := *c
	if c.disabledDomainsParsed != nil {
		clone.disabledDomainsParsed = append([]string(nil), c.disabledDomainsParsed...)
	}
	if c.excludedChannelsParsed != nil {
		clone.excludedChannelsParsed = append([]string(nil), c.excludedChannelsParsed...)
	}
	return &clone
}

// parseDisabledDomains splits raw input on newlines and commas, trims
// whitespace, lowercases, and drops empty entries.
func parseDisabledDomains(raw string) []string {
	return splitAndClean(raw, true)
}

// parseExcludedChannels splits raw input on newlines and commas, trims
// whitespace, and drops empty entries. Channel IDs are case-sensitive,
// so case is preserved.
func parseExcludedChannels(raw string) []string {
	return splitAndClean(raw, false)
}

func splitAndClean(raw string, lower bool) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if lower {
			f = strings.ToLower(f)
		}
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return err
	}

	configuration.disabledDomainsParsed = parseDisabledDomains(configuration.DisabledDomains)
	configuration.excludedChannelsParsed = parseExcludedChannels(configuration.ExcludedChannels)

	p.setConfiguration(configuration)

	return nil
}
