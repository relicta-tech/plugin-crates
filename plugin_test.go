// Package main provides tests for the Crates plugin.
package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// MockCommandExecutor is a mock implementation of CommandExecutor for testing.
type MockCommandExecutor struct {
	RunFunc      func(ctx context.Context, name string, args ...string) ([]byte, error)
	RunInDirFunc func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	calls        []ExecutorCall
}

// ExecutorCall records a call to the executor.
type ExecutorCall struct {
	Method string
	Dir    string
	Name   string
	Args   []string
}

// Run implements CommandExecutor.Run.
func (m *MockCommandExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, ExecutorCall{Method: "Run", Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(ctx, name, args...)
	}
	return []byte("success"), nil
}

// RunInDir implements CommandExecutor.RunInDir.
func (m *MockCommandExecutor) RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, ExecutorCall{Method: "RunInDir", Dir: dir, Name: name, Args: args})
	if m.RunInDirFunc != nil {
		return m.RunInDirFunc(ctx, dir, name, args...)
	}
	return []byte("success"), nil
}

// GetCalls returns all recorded calls.
func (m *MockCommandExecutor) GetCalls() []ExecutorCall {
	return m.calls
}

// Reset clears all recorded calls.
func (m *MockCommandExecutor) Reset() {
	m.calls = nil
}

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

	// Check config schema contains expected properties
	t.Run("config schema contains expected properties", func(t *testing.T) {
		expectedProps := []string{
			"token",
			"registry",
			"allow_dirty",
			"no_verify",
			"manifest_path",
			"features",
			"all_features",
			"no_default_features",
			"jobs",
		}
		for _, prop := range expectedProps {
			if !strings.Contains(info.ConfigSchema, prop) {
				t.Errorf("config schema missing property: %s", prop)
			}
		}
	})
}

func TestValidate(t *testing.T) {
	p := &CratesPlugin{}
	ctx := context.Background()

	tests := []struct {
		name        string
		config      map[string]any
		wantValid   bool
		wantErrors  int
		errorFields []string
	}{
		{
			name:       "empty config is valid with warning",
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
				"token":               "crates-token-12345",
				"allow_dirty":         false,
				"no_verify":           true,
				"manifest_path":       "crates/mylib/Cargo.toml",
				"registry":            "my-registry",
				"features":            []any{"feature1", "feature2"},
				"all_features":        false,
				"no_default_features": true,
				"jobs":                4,
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "invalid manifest_path with path traversal",
			config: map[string]any{
				"manifest_path": "../../../etc/passwd",
			},
			wantValid:   false,
			wantErrors:  1,
			errorFields: []string{"manifest_path"},
		},
		{
			name: "invalid manifest_path with absolute path",
			config: map[string]any{
				"manifest_path": "/etc/passwd",
			},
			wantValid:   false,
			wantErrors:  1,
			errorFields: []string{"manifest_path"},
		},
		{
			name: "invalid registry URL with HTTP",
			config: map[string]any{
				"registry": "http://insecure-registry.com",
			},
			wantValid:   false,
			wantErrors:  1,
			errorFields: []string{"registry"},
		},
		{
			name: "valid registry with HTTPS",
			config: map[string]any{
				"registry": "https://my-registry.com/crates",
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "valid registry name (not URL)",
			config: map[string]any{
				"registry": "my-private-registry",
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "valid sparse+https registry",
			config: map[string]any{
				"registry": "sparse+https://my-registry.com/index",
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "negative jobs value",
			config: map[string]any{
				"jobs": float64(-1),
			},
			wantValid:   false,
			wantErrors:  1,
			errorFields: []string{"jobs"},
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

			// Check error fields
			for _, field := range tt.errorFields {
				found := false
				for _, e := range resp.Errors {
					if e.Field == field {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field '%s', not found in %v", field, resp.Errors)
				}
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		envVars  map[string]string
		expected Config
	}{
		{
			name:   "defaults with empty config",
			config: map[string]any{},
			expected: Config{
				Token:             "",
				Registry:          "",
				AllowDirty:        false,
				NoVerify:          false,
				ManifestPath:      "Cargo.toml",
				Features:          nil,
				AllFeatures:       false,
				NoDefaultFeatures: false,
				Jobs:              0,
			},
		},
		{
			name: "token from config",
			config: map[string]any{
				"token": "direct-token",
			},
			expected: Config{
				Token:        "direct-token",
				ManifestPath: "Cargo.toml",
			},
		},
		{
			name:   "token from CARGO_REGISTRY_TOKEN env var",
			config: map[string]any{},
			envVars: map[string]string{
				"CARGO_REGISTRY_TOKEN": "env-token-12345",
			},
			expected: Config{
				Token:        "env-token-12345",
				ManifestPath: "Cargo.toml",
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
			expected: Config{
				Token:        "config-token",
				ManifestPath: "Cargo.toml",
			},
		},
		{
			name: "full config with all options",
			config: map[string]any{
				"token":               "my-token",
				"registry":            "my-registry",
				"allow_dirty":         true,
				"no_verify":           true,
				"manifest_path":       "./my-crate/Cargo.toml",
				"features":            []any{"feature1", "feature2"},
				"all_features":        true,
				"no_default_features": true,
				"jobs":                8,
			},
			expected: Config{
				Token:             "my-token",
				Registry:          "my-registry",
				AllowDirty:        true,
				NoVerify:          true,
				ManifestPath:      "./my-crate/Cargo.toml",
				Features:          []string{"feature1", "feature2"},
				AllFeatures:       true,
				NoDefaultFeatures: true,
				Jobs:              8,
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

			p := &CratesPlugin{}
			cfg := p.parseConfig(tt.config)

			if cfg.Token != tt.expected.Token {
				t.Errorf("Token: expected '%s', got '%s'", tt.expected.Token, cfg.Token)
			}
			if cfg.Registry != tt.expected.Registry {
				t.Errorf("Registry: expected '%s', got '%s'", tt.expected.Registry, cfg.Registry)
			}
			if cfg.AllowDirty != tt.expected.AllowDirty {
				t.Errorf("AllowDirty: expected %v, got %v", tt.expected.AllowDirty, cfg.AllowDirty)
			}
			if cfg.NoVerify != tt.expected.NoVerify {
				t.Errorf("NoVerify: expected %v, got %v", tt.expected.NoVerify, cfg.NoVerify)
			}
			if cfg.ManifestPath != tt.expected.ManifestPath {
				t.Errorf("ManifestPath: expected '%s', got '%s'", tt.expected.ManifestPath, cfg.ManifestPath)
			}
			if cfg.AllFeatures != tt.expected.AllFeatures {
				t.Errorf("AllFeatures: expected %v, got %v", tt.expected.AllFeatures, cfg.AllFeatures)
			}
			if cfg.NoDefaultFeatures != tt.expected.NoDefaultFeatures {
				t.Errorf("NoDefaultFeatures: expected %v, got %v", tt.expected.NoDefaultFeatures, cfg.NoDefaultFeatures)
			}
			if cfg.Jobs != tt.expected.Jobs {
				t.Errorf("Jobs: expected %d, got %d", tt.expected.Jobs, cfg.Jobs)
			}
		})
	}
}

func TestBuildPublishArgs(t *testing.T) {
	tests := []struct {
		name         string
		config       Config
		expectedArgs []string
		notExpected  []string
	}{
		{
			name: "minimal config",
			config: Config{
				Token:        "test-token",
				ManifestPath: "Cargo.toml",
			},
			expectedArgs: []string{"publish", "--token", "test-token"},
			notExpected:  []string{"--registry", "--allow-dirty", "--no-verify", "--manifest-path"},
		},
		{
			name: "with registry",
			config: Config{
				Token:    "test-token",
				Registry: "my-registry",
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--registry", "my-registry"},
		},
		{
			name: "with allow_dirty",
			config: Config{
				Token:      "test-token",
				AllowDirty: true,
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--allow-dirty"},
		},
		{
			name: "with no_verify",
			config: Config{
				Token:    "test-token",
				NoVerify: true,
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--no-verify"},
		},
		{
			name: "with custom manifest_path",
			config: Config{
				Token:        "test-token",
				ManifestPath: "crates/mylib/Cargo.toml",
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--manifest-path", "crates/mylib/Cargo.toml"},
		},
		{
			name: "with features",
			config: Config{
				Token:    "test-token",
				Features: []string{"feature1", "feature2"},
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--features", "feature1,feature2"},
		},
		{
			name: "with all_features",
			config: Config{
				Token:       "test-token",
				AllFeatures: true,
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--all-features"},
		},
		{
			name: "with no_default_features",
			config: Config{
				Token:             "test-token",
				NoDefaultFeatures: true,
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--no-default-features"},
		},
		{
			name: "with jobs",
			config: Config{
				Token: "test-token",
				Jobs:  4,
			},
			expectedArgs: []string{"publish", "--token", "test-token", "--jobs", "4"},
		},
		{
			name: "full config",
			config: Config{
				Token:             "test-token",
				Registry:          "my-registry",
				AllowDirty:        true,
				NoVerify:          true,
				ManifestPath:      "path/to/Cargo.toml",
				Features:          []string{"f1", "f2"},
				AllFeatures:       false,
				NoDefaultFeatures: true,
				Jobs:              8,
			},
			expectedArgs: []string{
				"publish",
				"--token", "test-token",
				"--registry", "my-registry",
				"--allow-dirty",
				"--no-verify",
				"--manifest-path", "path/to/Cargo.toml",
				"--features", "f1,f2",
				"--no-default-features",
				"--jobs", "8",
			},
		},
		{
			name: "without token",
			config: Config{
				Token: "",
			},
			expectedArgs: []string{"publish"},
			notExpected:  []string{"--token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CratesPlugin{}
			args := p.buildPublishArgs(&tt.config)

			// Check expected args are present
			argsStr := strings.Join(args, " ")
			for _, expected := range tt.expectedArgs {
				found := false
				for _, arg := range args {
					if arg == expected {
						found = true
						break
					}
				}
				if !found && !strings.Contains(argsStr, expected) {
					t.Errorf("expected argument '%s' not found in %v", expected, args)
				}
			}

			// Check not expected args are absent
			for _, notExpected := range tt.notExpected {
				for _, arg := range args {
					if arg == notExpected {
						t.Errorf("unexpected argument '%s' found in %v", notExpected, args)
					}
				}
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	tests := []struct {
		name            string
		config          map[string]any
		releaseCtx      plugin.ReleaseContext
		wantSuccess     bool
		wantMsgContains string
		wantOutputKeys  []string
	}{
		{
			name:   "basic dry run execution",
			config: map[string]any{},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			wantSuccess:     true,
			wantMsgContains: "Would publish crate version 1.0.0",
			wantOutputKeys:  []string{"version", "registry", "manifest_path", "command"},
		},
		{
			name: "dry run with token",
			config: map[string]any{
				"token": "test-token",
			},
			releaseCtx: plugin.ReleaseContext{
				Version:         "v2.0.0",
				PreviousVersion: "v1.0.0",
			},
			wantSuccess:     true,
			wantMsgContains: "Would publish crate version 2.0.0",
		},
		{
			name: "dry run with allow_dirty",
			config: map[string]any{
				"allow_dirty": true,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v0.1.0",
			},
			wantSuccess:     true,
			wantMsgContains: "Would publish crate version 0.1.0",
		},
		{
			name: "dry run with full config",
			config: map[string]any{
				"token":         "test-token",
				"allow_dirty":   false,
				"manifest_path": "crates/lib/Cargo.toml",
				"registry":      "my-registry",
			},
			releaseCtx: plugin.ReleaseContext{
				Version:         "v3.0.0",
				PreviousVersion: "v2.9.0",
				RepositoryURL:   "https://github.com/example/rust-project",
			},
			wantSuccess:     true,
			wantMsgContains: "Would publish crate version 3.0.0 to my-registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &CratesPlugin{}
			ctx := context.Background()

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

			if resp.Success != tt.wantSuccess {
				t.Errorf("expected success=%v, got success=%v, error=%s", tt.wantSuccess, resp.Success, resp.Error)
			}

			if !strings.Contains(resp.Message, tt.wantMsgContains) {
				t.Errorf("expected message to contain '%s', got '%s'", tt.wantMsgContains, resp.Message)
			}

			// Check output keys
			for _, key := range tt.wantOutputKeys {
				if _, ok := resp.Outputs[key]; !ok {
					t.Errorf("expected output key '%s' not found in %v", key, resp.Outputs)
				}
			}
		})
	}
}

func TestExecuteWithMockExecutor(t *testing.T) {
	tests := []struct {
		name              string
		config            map[string]any
		releaseCtx        plugin.ReleaseContext
		mockSetup         func(*MockCommandExecutor)
		wantSuccess       bool
		wantMsgContains   string
		wantErrorContains string
		checkCalls        func(*testing.T, []ExecutorCall)
	}{
		{
			name: "successful publish",
			config: map[string]any{
				"token": "test-token",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockSetup: func(m *MockCommandExecutor) {
				m.RunFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
					return []byte("Uploaded successfully"), nil
				}
			},
			wantSuccess:     true,
			wantMsgContains: "Published crate version 1.0.0",
			checkCalls: func(t *testing.T, calls []ExecutorCall) {
				if len(calls) != 1 {
					t.Errorf("expected 1 call, got %d", len(calls))
					return
				}
				if calls[0].Name != "cargo" {
					t.Errorf("expected cargo command, got %s", calls[0].Name)
				}
				if calls[0].Args[0] != "publish" {
					t.Errorf("expected publish subcommand, got %s", calls[0].Args[0])
				}
			},
		},
		{
			name: "publish fails",
			config: map[string]any{
				"token": "test-token",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockSetup: func(m *MockCommandExecutor) {
				m.RunFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
					return []byte("error: crate already published"), errors.New("exit status 1")
				}
			},
			wantSuccess:       false,
			wantErrorContains: "cargo publish failed",
		},
		{
			name: "publish with registry",
			config: map[string]any{
				"token":    "test-token",
				"registry": "my-registry",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockSetup: func(m *MockCommandExecutor) {
				m.RunFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
					return []byte("Uploaded successfully"), nil
				}
			},
			wantSuccess:     true,
			wantMsgContains: "Published crate version 1.0.0 to my-registry",
			checkCalls: func(t *testing.T, calls []ExecutorCall) {
				argsStr := strings.Join(calls[0].Args, " ")
				if !strings.Contains(argsStr, "--registry my-registry") {
					t.Errorf("expected --registry flag, got %s", argsStr)
				}
			},
		},
		{
			name: "publish with custom manifest path uses RunInDir",
			config: map[string]any{
				"token":         "test-token",
				"manifest_path": "crates/lib/Cargo.toml",
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockSetup: func(m *MockCommandExecutor) {
				m.RunInDirFunc = func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					return []byte("Uploaded successfully"), nil
				}
			},
			wantSuccess:     true,
			wantMsgContains: "Published crate version 1.0.0",
			checkCalls: func(t *testing.T, calls []ExecutorCall) {
				if len(calls) != 1 {
					t.Errorf("expected 1 call, got %d", len(calls))
					return
				}
				if calls[0].Method != "RunInDir" {
					t.Errorf("expected RunInDir, got %s", calls[0].Method)
				}
				if calls[0].Dir != "crates/lib" {
					t.Errorf("expected dir 'crates/lib', got '%s'", calls[0].Dir)
				}
			},
		},
		{
			name:   "missing token returns error",
			config: map[string]any{
				// no token
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			wantSuccess:       false,
			wantErrorContains: "no API token provided",
		},
		{
			name: "publish with all flags",
			config: map[string]any{
				"token":               "test-token",
				"allow_dirty":         true,
				"no_verify":           true,
				"features":            []any{"feature1"},
				"no_default_features": true,
				"jobs":                4,
			},
			releaseCtx: plugin.ReleaseContext{
				Version: "v1.0.0",
			},
			mockSetup: func(m *MockCommandExecutor) {
				m.RunFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
					return []byte("Uploaded successfully"), nil
				}
			},
			wantSuccess:     true,
			wantMsgContains: "Published crate version 1.0.0",
			checkCalls: func(t *testing.T, calls []ExecutorCall) {
				argsStr := strings.Join(calls[0].Args, " ")
				expectedFlags := []string{
					"--allow-dirty",
					"--no-verify",
					"--features",
					"--no-default-features",
					"--jobs",
				}
				for _, flag := range expectedFlags {
					if !strings.Contains(argsStr, flag) {
						t.Errorf("expected %s flag, got %s", flag, argsStr)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommandExecutor{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			p := &CratesPlugin{cmdExecutor: mock}
			ctx := context.Background()

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

			if resp.Success != tt.wantSuccess {
				t.Errorf("expected success=%v, got success=%v, error=%s", tt.wantSuccess, resp.Success, resp.Error)
			}

			if tt.wantMsgContains != "" && !strings.Contains(resp.Message, tt.wantMsgContains) {
				t.Errorf("expected message to contain '%s', got '%s'", tt.wantMsgContains, resp.Message)
			}

			if tt.wantErrorContains != "" && !strings.Contains(resp.Error, tt.wantErrorContains) {
				t.Errorf("expected error to contain '%s', got '%s'", tt.wantErrorContains, resp.Error)
			}

			if tt.checkCalls != nil {
				tt.checkCalls(t, mock.GetCalls())
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

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path is valid",
			path:    "",
			wantErr: false,
		},
		{
			name:    "simple relative path",
			path:    "Cargo.toml",
			wantErr: false,
		},
		{
			name:    "nested relative path",
			path:    "crates/lib/Cargo.toml",
			wantErr: false,
		},
		{
			name:    "path with dot",
			path:    "./Cargo.toml",
			wantErr: false,
		},
		{
			name:    "absolute path rejected",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal rejected",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "hidden path traversal rejected",
			path:    "crates/../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRegistryURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "simple registry name",
			url:     "my-registry",
			wantErr: false,
		},
		{
			name:    "registry name with dots",
			url:     "my.registry.com",
			wantErr: false,
		},
		{
			name:    "HTTPS URL",
			url:     "https://my-registry.com/crates",
			wantErr: false,
		},
		{
			name:    "sparse+https URL",
			url:     "sparse+https://my-registry.com/index",
			wantErr: false,
		},
		{
			name:    "HTTP URL rejected",
			url:     "http://insecure-registry.com",
			wantErr: true,
		},
		{
			name:    "localhost HTTP allowed",
			url:     "http://localhost:8080/index",
			wantErr: false,
		},
		{
			name:    "127.0.0.1 HTTP allowed",
			url:     "http://127.0.0.1:8080/index",
			wantErr: false,
		},
		{
			name:    "invalid registry name",
			url:     "my_registry!@#",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegistryURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRegistryURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestGetRegistryName(t *testing.T) {
	p := &CratesPlugin{}

	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name:     "empty registry returns crates.io",
			config:   Config{Registry: ""},
			expected: "crates.io",
		},
		{
			name:     "custom registry returns registry name",
			config:   Config{Registry: "my-registry"},
			expected: "my-registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.getRegistryName(&tt.config)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	p := &CratesPlugin{}

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid minimal config",
			config:  Config{ManifestPath: "Cargo.toml"},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: Config{
				ManifestPath: "crates/lib/Cargo.toml",
				Registry:     "my-registry",
			},
			wantErr: false,
		},
		{
			name:    "invalid manifest path",
			config:  Config{ManifestPath: "/etc/passwd"},
			wantErr: true,
		},
		{
			name:    "invalid registry URL",
			config:  Config{Registry: "http://insecure.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateConfig(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetExecutor(t *testing.T) {
	t.Run("returns RealCommandExecutor when no executor set", func(t *testing.T) {
		p := &CratesPlugin{}
		executor := p.getExecutor()
		if _, ok := executor.(*RealCommandExecutor); !ok {
			t.Error("expected RealCommandExecutor")
		}
	})

	t.Run("returns custom executor when set", func(t *testing.T) {
		mock := &MockCommandExecutor{}
		p := &CratesPlugin{cmdExecutor: mock}
		executor := p.getExecutor()
		if executor != mock {
			t.Error("expected mock executor")
		}
	})
}
