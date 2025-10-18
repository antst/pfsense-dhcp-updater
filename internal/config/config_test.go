package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid configuration",
			configYAML: `
pfsense:
  endpoint: "https://pfsense.example.com"
  api_key: "test-api-key-123"
  base_interface: "igb0"
  insecure_skip_verify: false
vlan_ip_ranges:
  0:
    - "192.168.1.100-192.168.1.200"
  10:
    - "10.0.10.10-10.0.10.50"
timeouts:
  api_request: 5s
  total_execution: 10s
logging:
  level: "INFO"
  format: "text"
`,
			wantErr: false,
		},
		{
			name: "environment variable expansion",
			configYAML: `
pfsense:
  endpoint: "https://pfsense.example.com"
  api_key: "${TEST_API_KEY}"
  base_interface: "igb0"
timeouts:
  api_request: 5s
  total_execution: 10s
logging:
  level: "INFO"
  format: "text"
`,
			envVars: map[string]string{
				"TEST_API_KEY": "expanded-api-key",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			configYAML: `
pfsense:
  api_key: "test-api-key"
  base_interface: "igb0"
`,
			wantErr:     true,
			errContains: "endpoint is required",
		},
		{
			name: "missing api_key",
			configYAML: `
pfsense:
  endpoint: "https://pfsense.example.com"
  base_interface: "igb0"
`,
			wantErr:     true,
			errContains: "api_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				if err := os.Setenv(key, value); err != nil {
					t.Fatalf("Failed to set env var: %v", err)
				}
				defer func(k string) {
					if err := os.Unsetenv(k); err != nil {
						t.Logf("Failed to unset env var: %v", err)
					}
				}(key)
			}

			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0600); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			} // Load configuration
			cfg, err := Load(configPath)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify defaults were applied
			if cfg.Timeouts.APIRequest == 0 {
				t.Errorf("Expected default APIRequest timeout to be set")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: Config{
				PfSense: PfSenseConfig{
					Endpoint:      "https://pfsense.example.com",
					APIKey:        "test-key",
					BaseInterface: "igb0",
				},
				Timeouts: TimeoutConfig{
					APIRequest:     5 * time.Second,
					TotalExecution: 10 * time.Second,
				},
				Logging: LoggingConfig{
					Level:  "INFO",
					Format: "text",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCheckConfigPermissions(t *testing.T) {
	tests := []struct {
		name            string
		fileMode        os.FileMode
		config          *Config
		expectWarning   bool
		warningContains string
	}{
		{
			name:     "secure permissions with plaintext key (600)",
			fileMode: 0600,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "plaintext-key-123",
				},
			},
			expectWarning: false,
		},
		{
			name:     "world-readable with plaintext key (644)",
			fileMode: 0644,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "plaintext-key-123",
				},
			},
			expectWarning:   true,
			warningContains: "world-readable",
		},
		{
			name:     "group-readable with plaintext key (640)",
			fileMode: 0640,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "plaintext-key-123",
				},
			},
			expectWarning:   true,
			warningContains: "group-readable",
		},
		{
			name:     "world-readable but using environment variable (644)",
			fileMode: 0644,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "${PFSENSE_API_KEY}",
				},
			},
			expectWarning: false, // No warning when using env var
		},
		{
			name:     "group-readable but using environment variable (640)",
			fileMode: 0640,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "${API_KEY}",
				},
			},
			expectWarning: false, // No warning when using env var
		},
		{
			name:     "world and group readable with plaintext (666)",
			fileMode: 0666,
			config: &Config{
				PfSense: PfSenseConfig{
					APIKey: "plaintext-key-123",
				},
			},
			expectWarning:   true,
			warningContains: "world-readable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file with specified permissions
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			testContent := []byte("pfsense:\n  api_key: test\n")
			if err := os.WriteFile(configPath, testContent, tt.fileMode); err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			// Check permissions
			warning, err := CheckConfigPermissions(configPath, tt.config)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectWarning {
				if warning == "" {
					t.Errorf("Expected warning but got none")
				} else if tt.warningContains != "" && !contains(warning, tt.warningContains) {
					t.Errorf("Expected warning containing %q, got: %s", tt.warningContains, warning)
				}
			} else {
				if warning != "" {
					t.Errorf("Expected no warning but got: %s", warning)
				}
			}
		})
	}
}
