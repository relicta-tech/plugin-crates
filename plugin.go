// Package main implements the Crates plugin for Relicta.
package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// RealCommandExecutor executes actual system commands.
type RealCommandExecutor struct{}

// Run executes a command and returns combined output.
func (e *RealCommandExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// RunInDir executes a command in a specific directory.
func (e *RealCommandExecutor) RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// CratesPlugin implements the Publish crates to crates.io (Rust) plugin.
type CratesPlugin struct {
	// cmdExecutor is used for executing shell commands. If nil, uses RealCommandExecutor.
	cmdExecutor CommandExecutor
}

// getExecutor returns the command executor, defaulting to RealCommandExecutor.
func (p *CratesPlugin) getExecutor() CommandExecutor {
	if p.cmdExecutor != nil {
		return p.cmdExecutor
	}
	return &RealCommandExecutor{}
}

// Config represents the Crates plugin configuration.
type Config struct {
	Token             string
	Registry          string
	AllowDirty        bool
	NoVerify          bool
	ManifestPath      string
	Features          []string
	AllFeatures       bool
	NoDefaultFeatures bool
	Jobs              int
}

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
			"properties": {
				"token": {"type": "string", "description": "Crates.io API token (or use CARGO_REGISTRY_TOKEN env)"},
				"registry": {"type": "string", "description": "Registry to publish to (optional, for private registries)"},
				"allow_dirty": {"type": "boolean", "description": "Allow publishing with uncommitted changes", "default": false},
				"no_verify": {"type": "boolean", "description": "Skip crate verification", "default": false},
				"manifest_path": {"type": "string", "description": "Path to Cargo.toml", "default": "Cargo.toml"},
				"features": {"type": "array", "items": {"type": "string"}, "description": "Features to activate"},
				"all_features": {"type": "boolean", "description": "Activate all available features", "default": false},
				"no_default_features": {"type": "boolean", "description": "Do not activate the default feature", "default": false},
				"jobs": {"type": "integer", "description": "Number of parallel jobs"}
			}
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *CratesPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	cfg := p.parseConfig(req.Config)

	switch req.Hook {
	case plugin.HookPostPublish:
		return p.publish(ctx, cfg, req.Context, req.DryRun)
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// publish executes the cargo publish command.
func (p *CratesPlugin) publish(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	// Validate configuration
	if err := p.validateConfig(cfg); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("configuration validation failed: %v", err),
		}, nil
	}

	// Build cargo publish command arguments
	args := p.buildPublishArgs(cfg)

	version := strings.TrimPrefix(releaseCtx.Version, "v")

	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Would publish crate version %s to %s", version, p.getRegistryName(cfg)),
			Outputs: map[string]any{
				"version":       version,
				"registry":      cfg.Registry,
				"manifest_path": cfg.ManifestPath,
				"allow_dirty":   cfg.AllowDirty,
				"no_verify":     cfg.NoVerify,
				"command":       "cargo publish " + strings.Join(args, " "),
			},
		}, nil
	}

	// Check if token is available
	if cfg.Token == "" {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   "no API token provided: set token in config or CARGO_REGISTRY_TOKEN environment variable",
		}, nil
	}

	// Execute cargo publish
	executor := p.getExecutor()

	// Determine working directory from manifest path
	workDir := ""
	if cfg.ManifestPath != "" && cfg.ManifestPath != "Cargo.toml" {
		workDir = filepath.Dir(cfg.ManifestPath)
	}

	var output []byte
	var err error
	if workDir != "" {
		output, err = executor.RunInDir(ctx, workDir, "cargo", args...)
	} else {
		output, err = executor.Run(ctx, "cargo", args...)
	}

	if err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("cargo publish failed: %v\nOutput: %s", err, string(output)),
		}, nil
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Published crate version %s to %s", version, p.getRegistryName(cfg)),
		Outputs: map[string]any{
			"version":  version,
			"registry": cfg.Registry,
			"output":   string(output),
		},
	}, nil
}

// buildPublishArgs constructs the cargo publish command arguments.
func (p *CratesPlugin) buildPublishArgs(cfg *Config) []string {
	args := []string{"publish"}

	// Token is passed via argument (cargo handles it securely)
	if cfg.Token != "" {
		args = append(args, "--token", cfg.Token)
	}

	// Registry for private registries
	if cfg.Registry != "" {
		args = append(args, "--registry", cfg.Registry)
	}

	// Allow dirty working directory
	if cfg.AllowDirty {
		args = append(args, "--allow-dirty")
	}

	// Skip verification
	if cfg.NoVerify {
		args = append(args, "--no-verify")
	}

	// Manifest path
	if cfg.ManifestPath != "" && cfg.ManifestPath != "Cargo.toml" {
		args = append(args, "--manifest-path", cfg.ManifestPath)
	}

	// Features
	if len(cfg.Features) > 0 {
		args = append(args, "--features", strings.Join(cfg.Features, ","))
	}

	// All features
	if cfg.AllFeatures {
		args = append(args, "--all-features")
	}

	// No default features
	if cfg.NoDefaultFeatures {
		args = append(args, "--no-default-features")
	}

	// Parallel jobs
	if cfg.Jobs > 0 {
		args = append(args, "--jobs", fmt.Sprintf("%d", cfg.Jobs))
	}

	return args
}

// getRegistryName returns a human-readable registry name.
func (p *CratesPlugin) getRegistryName(cfg *Config) string {
	if cfg.Registry != "" {
		return cfg.Registry
	}
	return "crates.io"
}

// validateConfig validates the plugin configuration for security issues.
func (p *CratesPlugin) validateConfig(cfg *Config) error {
	// Validate manifest path
	if err := validatePath(cfg.ManifestPath); err != nil {
		return fmt.Errorf("invalid manifest_path: %w", err)
	}

	// Validate registry URL if provided
	if cfg.Registry != "" {
		if err := validateRegistryURL(cfg.Registry); err != nil {
			return fmt.Errorf("invalid registry: %w", err)
		}
	}

	return nil
}

// validatePath validates a file path to prevent path traversal.
func validatePath(path string) error {
	if path == "" {
		return nil
	}

	// Clean the path
	cleaned := filepath.Clean(path)

	// Check for absolute paths (potential escape from working directory)
	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("absolute paths are not allowed")
	}

	// Check for path traversal attempts
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return fmt.Errorf("path traversal detected: cannot use '..' to escape working directory")
	}

	return nil
}

// validateRegistryURL validates a registry URL for security (SSRF protection).
func validateRegistryURL(registryURL string) error {
	// If it's just a registry name (not a URL), allow it
	if !strings.Contains(registryURL, "://") {
		// Simple registry name validation (alphanumerics, dots, dashes)
		validName := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.-]*$`)
		if !validName.MatchString(registryURL) {
			return fmt.Errorf("invalid registry name format")
		}
		return nil
	}

	// Parse as URL
	parsedURL, err := url.Parse(registryURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Hostname()

	// Allow localhost for testing purposes
	isLocalhost := host == "localhost" || host == "127.0.0.1" || host == "::1"

	// Require HTTPS for non-localhost URLs
	if parsedURL.Scheme != "https" && !isLocalhost {
		if parsedURL.Scheme != "sparse+https" { // Cargo supports sparse+https protocol
			return fmt.Errorf("only HTTPS URLs are allowed (got %s)", parsedURL.Scheme)
		}
	}

	// For localhost, skip the private IP check
	if isLocalhost {
		return nil
	}

	// Resolve hostname to check for private IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS resolution might fail in some environments, allow it but log
		return nil
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URLs pointing to private networks are not allowed")
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	// Private IPv4 ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // Link-local
		"0.0.0.0/8",
	}

	// Cloud metadata endpoints
	cloudMetadata := []string{
		"169.254.169.254/32", // AWS/GCP/Azure metadata
		"fd00:ec2::254/128",  // AWS IMDSv2 IPv6
	}

	allRanges := append(privateRanges, cloudMetadata...)

	for _, cidr := range allRanges {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if block.Contains(ip) {
			return true
		}
	}

	// Check for IPv6 private ranges
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}

	return false
}

// parseConfig parses the raw configuration map into a Config struct.
func (p *CratesPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	return &Config{
		Token:             parser.GetString("token", "CARGO_REGISTRY_TOKEN", ""),
		Registry:          parser.GetString("registry", "", ""),
		AllowDirty:        parser.GetBool("allow_dirty", false),
		NoVerify:          parser.GetBool("no_verify", false),
		ManifestPath:      parser.GetString("manifest_path", "", "Cargo.toml"),
		Features:          parser.GetStringSlice("features", nil),
		AllFeatures:       parser.GetBool("all_features", false),
		NoDefaultFeatures: parser.GetBool("no_default_features", false),
		Jobs:              parser.GetInt("jobs", 0),
	}
}

// Validate validates the plugin configuration.
func (p *CratesPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	parser := helpers.NewConfigParser(config)

	// Validate manifest_path if provided
	manifestPath := parser.GetString("manifest_path", "", "Cargo.toml")
	if err := validatePath(manifestPath); err != nil {
		vb.AddError("manifest_path", err.Error())
	}

	// Validate registry URL if provided
	registry := parser.GetString("registry", "", "")
	if registry != "" {
		if err := validateRegistryURL(registry); err != nil {
			vb.AddError("registry", err.Error())
		}
	}

	// Jobs must be positive if specified
	if jobs, ok := config["jobs"].(float64); ok {
		if jobs < 0 {
			vb.AddError("jobs", "jobs must be a positive integer")
		}
	}

	// Token is optional during validation - it can be set via env at runtime
	// No warning needed here since it's checked at execution time

	return vb.Build(), nil
}
