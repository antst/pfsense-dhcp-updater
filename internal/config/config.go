// Package config handles loading and validation of application configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	PfSense      PfSenseConfig    `yaml:"pfsense"`
	VLANIPRanges map[int][]string `yaml:"vlan_ip_ranges"`
	Timeouts     TimeoutConfig    `yaml:"timeouts"`
	Logging      LoggingConfig    `yaml:"logging"`
}

// PfSenseConfig holds pfSense connection settings.
type PfSenseConfig struct {
	Endpoint           string            `yaml:"endpoint"`
	APIKey             string            `yaml:"api_key"`
	BaseInterface      string            `yaml:"base_interface"`
	InsecureSkipVerify bool              `yaml:"insecure_skip_verify"`
	InterfaceMapping   map[string]string `yaml:"interface_mapping"` // Optional: physical -> logical name mapping
}

// TimeoutConfig holds timeout settings.
type TimeoutConfig struct {
	APIRequest     time.Duration `yaml:"api_request"`
	TotalExecution time.Duration `yaml:"total_execution"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load reads and parses the configuration file from the given path.
// It expands environment variables in the format ${VAR_NAME}.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	decoder := yaml.NewDecoder(strings.NewReader(expanded))
	decoder.KnownFields(true) // Strict mode: error on unknown fields

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	if cfg.Timeouts.APIRequest == 0 {
		cfg.Timeouts.APIRequest = 5 * time.Second
	}
	if cfg.Timeouts.TotalExecution == 0 {
		cfg.Timeouts.TotalExecution = 10 * time.Second
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "INFO"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	// Validate pfSense endpoint
	if c.PfSense.Endpoint == "" {
		return fmt.Errorf("pfsense.endpoint is required")
	}

	parsedURL, err := url.Parse(c.PfSense.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid pfsense.endpoint URL: %w", err)
	}

	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("pfsense.endpoint must use http or https scheme")
	}

	if parsedURL.Scheme == "http" && !c.PfSense.InsecureSkipVerify {
		return fmt.Errorf("http endpoint requires insecure_skip_verify=true for security acknowledgment")
	}

	// Validate API key
	if c.PfSense.APIKey == "" {
		return fmt.Errorf("pfsense.api_key is required (use ${ENV_VAR} syntax for environment variables)")
	}

	// Validate base interface format
	if c.PfSense.BaseInterface == "" {
		return fmt.Errorf("pfsense.base_interface is required")
	}

	// Accept physical interface names (e.g., iavf1, em0, igb0) or logical names (e.g., lan, opt1)
	// Validation is relaxed - actual interface existence will be verified via API
	if len(c.PfSense.BaseInterface) == 0 || len(c.PfSense.BaseInterface) > 64 {
		return fmt.Errorf("pfsense.base_interface length must be 1-64 characters")
	}

	// Validate VLAN IP ranges
	for vlan, ranges := range c.VLANIPRanges {
		if vlan < 0 || vlan > 4094 {
			return fmt.Errorf("invalid VLAN tag %d: must be 0-4094", vlan)
		}

		for _, r := range ranges {
			if err := validateIPRange(r); err != nil {
				return fmt.Errorf("invalid IP range for VLAN %d: %w", vlan, err)
			}
		}
	}

	// Validate timeouts
	if c.Timeouts.APIRequest <= 0 {
		return fmt.Errorf("timeouts.api_request must be > 0")
	}
	if c.Timeouts.TotalExecution <= 0 {
		return fmt.Errorf("timeouts.total_execution must be > 0")
	}

	// Validate logging config
	validLevels := map[string]bool{"ERROR": true, "WARN": true, "INFO": true, "DEBUG": true}
	if !validLevels[strings.ToUpper(c.Logging.Level)] {
		return fmt.Errorf("logging.level must be one of: ERROR, WARN, INFO, DEBUG")
	}

	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		return fmt.Errorf("logging.format must be one of: text, json")
	}

	return nil
}

// validateIPRange checks if a range string is valid (CIDR or start-end format).
func validateIPRange(rangeStr string) error {
	// Check if it's CIDR notation
	if strings.Contains(rangeStr, "/") {
		_, _, err := net.ParseCIDR(rangeStr)
		if err != nil {
			return fmt.Errorf("invalid CIDR notation: %w", err)
		}
		return nil
	}

	// Check if it's range notation (start-end)
	if strings.Contains(rangeStr, "-") {
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid range format: expected 'start-end'")
		}

		startIP := net.ParseIP(strings.TrimSpace(parts[0]))
		endIP := net.ParseIP(strings.TrimSpace(parts[1]))

		if startIP == nil || endIP == nil {
			return fmt.Errorf("invalid IP addresses in range")
		}

		if startIP.To4() == nil || endIP.To4() == nil {
			return fmt.Errorf("only IPv4 addresses are supported")
		}

		return nil
	}

	// Single IP address
	ip := net.ParseIP(rangeStr)
	if ip == nil {
		return fmt.Errorf("invalid IP address or range format")
	}

	if ip.To4() == nil {
		return fmt.Errorf("only IPv4 addresses are supported")
	}

	return nil
}

// DefaultConfigPath returns the default configuration file path.
func DefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".config/pfsense-dhcp-updater/config.yaml"
	}
	return fmt.Sprintf("%s/.config/pfsense-dhcp-updater/config.yaml", homeDir)
}

// CheckConfigPermissions validates that the config file has secure permissions.
// Per SEC-009: Config files containing API keys should not be world-readable.
//
// Returns:
//   - warning: A warning message if permissions are insecure (but not fatal)
//   - error: An error if the file cannot be checked
//
// Example:
//
//	warning, err := CheckConfigPermissions("/path/to/config.yaml", cfg)
//	if err != nil {
//	    log.Error("Failed to check config permissions", "error", err)
//	}
//	if warning != "" {
//	    log.Warn(warning)
//	}
func CheckConfigPermissions(configPath string, cfg *Config) (warning string, err error) {
	// Get file info
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat config file: %w", err)
	}

	// Get file permissions
	mode := fileInfo.Mode()
	perm := mode.Perm()

	// Check if API key is in plaintext (not using environment variable)
	hasPlaintextKey := cfg != nil && cfg.PfSense.APIKey != "" && !strings.HasPrefix(cfg.PfSense.APIKey, "${")

	// Check if file is world-readable (others have read permission)
	isWorldReadable := perm&0004 != 0

	// Check if file is group-readable
	isGroupReadable := perm&0040 != 0

	if hasPlaintextKey && (isWorldReadable || isGroupReadable) {
		permStr := fmt.Sprintf("%04o", perm)
		warning = fmt.Sprintf(
			"[SECURITY WARNING] Config file contains plaintext API key and has insecure permissions (%s). "+
				"Recommended actions:\n"+
				"  1. Use environment variable syntax: api_key: ${PFSENSE_API_KEY}\n"+
				"  2. OR restrict file permissions: chmod 600 %s\n"+
				"  Current permissions allow: ",
			permStr, configPath,
		)

		if isWorldReadable {
			warning += "world-readable "
		}
		if isGroupReadable {
			warning += "group-readable"
		}

		return warning, nil
	}

	// No warning needed
	return "", nil
}
