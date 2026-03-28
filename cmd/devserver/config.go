package main

import (
	"encoding/json"
	"fmt"
)

// DevConfig holds the development environment configuration.
type DevConfig struct {
	Assets map[int64]DevAsset `json:"-"`
	Policy *DevPolicyConfig   `json:"policy,omitempty"`
	raw    json.RawMessage
}

// DevAsset holds per-asset test configuration.
type DevAsset struct {
	Name       string            `json:"name,omitempty"`
	Config     json.RawMessage   `json:"config"`
	Credential map[string]string `json:"credential,omitempty"`
}

// DevPolicyConfig holds policy configuration for dev testing.
type DevPolicyConfig struct {
	AllowList []string `json:"allow_list,omitempty"`
	DenyList  []string `json:"deny_list,omitempty"`
}

// Load parses devconfig.json data. Asset IDs are string keys in JSON.
func (c *DevConfig) Load(data []byte) error {
	c.raw = data
	var raw struct {
		Assets map[string]DevAsset `json:"assets"`
		Policy *DevPolicyConfig    `json:"policy,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Assets = make(map[int64]DevAsset)
	for k, v := range raw.Assets {
		var id int64
		if _, err := fmt.Sscanf(k, "%d", &id); err != nil {
			return fmt.Errorf("invalid asset ID %q: %v", k, err)
		}
		c.Assets[id] = v
	}
	c.Policy = raw.Policy
	return nil
}

// JSON returns the raw JSON for API responses.
func (c *DevConfig) JSON() json.RawMessage {
	if c.raw != nil {
		return c.raw
	}
	return json.RawMessage("{}")
}
