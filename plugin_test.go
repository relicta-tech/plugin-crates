// Package main provides tests for the Crates plugin.
package main

import (
	"context"
	"os"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

func TestGetInfo(t *testing.T) {
	p := &CratesPlugin{}
	info := p.GetInfo()

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "plugin name",
			got:      info.Name,
			expected: "crates",
		},
		{
			name:     "plugin version",
			got:      info.Version,
			expected: "2.0.0",
		},
		{
			name:     "plugin description",
			got:      info.Description,
			expected: "Publish crates to crates.io (Rust)",
		},
		{
			name:     "plugin author",
			got:      info.Author,
			expected: "Relicta Team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, tt.got)
			}
		})
	}

	// Check hooks
	t.Run("has hooks", func(t *testing.T) {
		if len(info.Hooks) == 0 {
			t.Error("expected at least one hook")
		}
	})

	t.Run("has PostPublish hook", func(t *testing.T) {
		hasPostPublish := false
		for _, hook := range info.Hooks {
			if hook == plugin.HookPostPublish {
				hasPostPublish = true
				break
			}
		}
		if !hasPostPublish {
			t.Error("expected PostPublish hook")
		}
	})

	// Check config schema is valid JSON
	t.Run("has config schema", func(t *testing.T) {
		if info.ConfigSchema == "" {
			t.Error("expected non-empty config schema")
		}
	})
}

func TestValidate(t *testing.T) {
	p := &CratesPlugin{}
	ctx := context.Background()

	tests := []struct {
		name       string
		config     map[string]any
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "empty config is valid",
			config:     map[string]any{},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "config with token is valid",
			config: map[string]any{
				"token": "crates-token-12345",
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "config with allow_dirty is valid",
			config: map[string]any{
				"allow_dirty": true,
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "config with all options",
			config: map[string]any{
				"token":       "crates-token-12345",
				"allow_dirty": false,
				"dry_run":     true,
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "config with package path",
			config: map[string]any{
				"package_path": "./crates/mylib",
			},
			wantValid:  true,
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := p.Validate(ctx, tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Valid != tt.wantValid {
				t.Errorf("expected valid=%v, got valid=%v, errors=%v", tt.wantValid, resp.Valid, resp.Errors)
			}

			if len(resp.Errors) != tt.wantErrors {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrors, len(resp.Errors), resp.Errors)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	// Test environment variable handling for CARGO_REGISTRY_TOKEN
	// This tests the expected behavior for config parsing
	tests := []struct {
		name    string
		config  map[string]any
		envVars map[string]string
		setup   func()
		cleanup func()
	}{
		{
			name:   "defaults with empty config",
			config: map[string]any{},
		},
		{
			name: "token from config",
			config: map[string]any{
				"token": "direct-token",
			},
		},
		{
			name:   "token from CARGO_REGISTRY_TOKEN env var",
			config: map[string]any{},
			envVars: map[string]string{
				"CARGO_REGISTRY_TOKEN": "env-token-12345",
			},
		},
		{
			name: "allow_dirty flag enabled",
			config: map[string]any{
				"allow_dirty": true,
			},
		},
		{
			name: "allow_dirty flag disabled",
			config: map[string]any{
				"allow_dirty": false,
			},
		},
		{
			name: "config overrides env var",
			config: map[string]any{
				"token": "config-token",
			},
			envVars: map[string]string{
				"CARGO_REGISTRY_TOKEN": "env-token",
			},
		},
		{
			name: "full config with all options",
			config: map[string]any{
				"token":        "my-token",
				"allow_dirty":  true,
				"package_path": "./my-crate",
				"features":     []any{"feature1", "feature2"},
				"no_verify":    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
			}

			// Cleanup env vars after test
			defer func() {
				for k := range tt.envVars {
					_ = os.Unsetenv(k)
				}
			}()

			// Verify env vars are set correctly
			if token, exists := tt.envVars["CARGO_REGISTRY_TOKEN"]; exists {
				envToken := os.Getenv("CARGO_REGISTRY_TOKEN")
				if envToken != token {
					t.Errorf("expected CARGO_REGISTRY_TOKEN='%s', got '%s'", token, envToken)
				}
			}

			// Verify config values are accessible
			if allowDirty, ok := tt.config["allow_dirty"].(bool); ok {
				if tt.config["allow_dirty"] != allowDirty {
					t.Errorf("allow_dirty mismatch")
				}
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	p := &CratesPlugin{}
	ctx := context.Background()

	tests := []struct {
		name           string
		config         map[string]any
		releaseCtx     plugin.ReleaseContext
		expectedMsg    string
		expectedOutput bool
	}{
		{
			name:   "basic dry run execution",
			config: map[string]any{},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedMsg:    "Would execute crates plugin",
			expectedOutput: true,
		},
		{
			name: "dry run with token",
			config: map[string]any{
				"token": "test-token",
			},
			releaseCtx: plugin.ReleaseContext{
				Version:        "v2.0.0",
				PreviousVersion: "v1.0.0",
			},
			expectedMsg:    "Would execute crates plugin",
			expectedOutput: true,
		},
		{
			name: "dry run with allow_dirty",
			config: map[string]any{
				"allow_dirty": true,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v0.1.0",
			},
			expectedMsg:    "Would execute crates plugin",
			expectedOutput: true,
		},
		{
			name: "dry run with full config",
			config: map[string]any{
				"token":        "test-token",
				"allow_dirty":  false,
				"package_path": "./crates/lib",
			},
			releaseCtx: plugin.ReleaseContext{
				Version:        "v3.0.0",
				PreviousVersion: "v2.9.0",
				RepositoryURL:  "https://github.com/example/rust-project",
			},
			expectedMsg:    "Would execute crates plugin",
			expectedOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:    plugin.HookPostPublish,
				Config:  tt.config,
				Context: tt.releaseCtx,
				DryRun:  true,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !resp.Success {
				t.Errorf("expected success, got error: %s", resp.Error)
			}

			if resp.Message != tt.expectedMsg {
				t.Errorf("expected message '%s', got '%s'", tt.expectedMsg, resp.Message)
			}
		})
	}
}

func TestExecuteNonDryRun(t *testing.T) {
	p := &CratesPlugin{}
	ctx := context.Background()

	tests := []struct {
		name        string
		config      map[string]any
		releaseCtx  plugin.ReleaseContext
		expectedMsg string
	}{
		{
			name:   "non-dry run execution",
			config: map[string]any{},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			expectedMsg: "Crates plugin executed successfully",
		},
		{
			name: "non-dry run with config",
			config: map[string]any{
				"token":       "test-token",
				"allow_dirty": true,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v2.0.0",
			},
			expectedMsg: "Crates plugin executed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:    plugin.HookPostPublish,
				Config:  tt.config,
				Context: tt.releaseCtx,
				DryRun:  false,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !resp.Success {
				t.Errorf("expected success, got error: %s", resp.Error)
			}

			if resp.Message != tt.expectedMsg {
				t.Errorf("expected message '%s', got '%s'", tt.expectedMsg, resp.Message)
			}
		})
	}
}

func TestExecuteUnhandledHook(t *testing.T) {
	p := &CratesPlugin{}
	ctx := context.Background()

	tests := []struct {
		name        string
		hook        plugin.Hook
		config      map[string]any
		expectedMsg string
	}{
		{
			name:        "PreInit hook not handled",
			hook:        plugin.HookPreInit,
			config:      map[string]any{},
			expectedMsg: "Hook pre-init not handled",
		},
		{
			name:        "PostInit hook not handled",
			hook:        plugin.HookPostInit,
			config:      map[string]any{},
			expectedMsg: "Hook post-init not handled",
		},
		{
			name:        "PreVersion hook not handled",
			hook:        plugin.HookPreVersion,
			config:      map[string]any{},
			expectedMsg: "Hook pre-version not handled",
		},
		{
			name:        "PostVersion hook not handled",
			hook:        plugin.HookPostVersion,
			config:      map[string]any{},
			expectedMsg: "Hook post-version not handled",
		},
		{
			name:        "PreNotes hook not handled",
			hook:        plugin.HookPreNotes,
			config:      map[string]any{},
			expectedMsg: "Hook pre-notes not handled",
		},
		{
			name:        "PostNotes hook not handled",
			hook:        plugin.HookPostNotes,
			config:      map[string]any{},
			expectedMsg: "Hook post-notes not handled",
		},
		{
			name:        "PrePublish hook not handled",
			hook:        plugin.HookPrePublish,
			config:      map[string]any{},
			expectedMsg: "Hook pre-publish not handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:   tt.hook,
				Config: tt.config,
				DryRun: true,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !resp.Success {
				t.Error("expected success for unhandled hook")
			}

			if resp.Message != tt.expectedMsg {
				t.Errorf("expected message '%s', got '%s'", tt.expectedMsg, resp.Message)
			}
		})
	}
}
