// Package main implements the Crates plugin for Relicta.
package main

import (
	"context"
	"fmt"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// CratesPlugin implements the Publish crates to crates.io (Rust) plugin.
type CratesPlugin struct{}

// GetInfo returns plugin metadata.
func (p *CratesPlugin) GetInfo() plugin.Info {
	return plugin.Info{
		Name:        "crates",
		Version:     "2.0.0",
		Description: "Publish crates to crates.io (Rust)",
		Author:      "Relicta Team",
		Hooks: []plugin.Hook{
			plugin.HookPostPublish,
		},
		ConfigSchema: `{
			"type": "object",
			"properties": {}
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *CratesPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	switch req.Hook {
	case plugin.HookPostPublish:
		if req.DryRun {
			return &plugin.ExecuteResponse{
				Success: true,
				Message: "Would execute crates plugin",
			}, nil
		}
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "Crates plugin executed successfully",
		}, nil
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// Validate validates the plugin configuration.
func (p *CratesPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	return vb.Build(), nil
}
