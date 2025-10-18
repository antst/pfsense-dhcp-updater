package logger

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	logger := New(LevelInfo, false, &buf)

	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}

	logger.Info("test message")
	if !strings.Contains(buf.String(), "test message") {
		t.Error("Expected message in output")
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(LevelInfo, true, &buf)

	logger.Info("test", "key", "value")

	var jsonData map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &jsonData); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
}

func TestLevelFromVerbosity(t *testing.T) {
	tests := []struct {
		verbosity int
		want      LogLevel
	}{
		{0, LevelWarn},
		{1, LevelInfo},
		{2, LevelDebug},
	}

	for _, tt := range tests {
		got := LevelFromVerbosity(tt.verbosity)
		if got != tt.want {
			t.Errorf("LevelFromVerbosity(%d) = %v, want %v", tt.verbosity, got, tt.want)
		}
	}
}

func TestLogAudit(t *testing.T) {
	tests := []struct {
		name         string
		entry        AuditEntry
		jsonFormat   bool
		expectedKeys []string
	}{
		{
			name: "CREATE operation",
			entry: AuditEntry{
				Timestamp:  time.Now(),
				Operation:  "CREATE",
				MACAddress: "00:11:22:33:44:55",
				Hostname:   "test-vm",
				NewIP:      net.ParseIP("192.168.1.100"),
				VLAN:       10,
				Interface:  "opt7",
				Result:     "SUCCESS",
				DryRun:     false,
			},
			jsonFormat:   true,
			expectedKeys: []string{"operation", "mac", "hostname", "new_ip", "vlan", "interface", "result", "dry_run"},
		},
		{
			name: "UPDATE operation with old and new IP",
			entry: AuditEntry{
				Timestamp:  time.Now(),
				Operation:  "UPDATE",
				MACAddress: "00:11:22:33:44:55",
				Hostname:   "test-vm-renamed",
				OldIP:      net.ParseIP("192.168.1.100"),
				NewIP:      net.ParseIP("192.168.1.100"),
				VLAN:       10,
				Interface:  "opt7",
				Result:     "SUCCESS",
				DryRun:     false,
			},
			jsonFormat:   true,
			expectedKeys: []string{"operation", "mac", "hostname", "old_ip", "new_ip", "vlan", "interface", "result"},
		},
		{
			name: "DELETE operation",
			entry: AuditEntry{
				Timestamp:  time.Now(),
				Operation:  "DELETE",
				MACAddress: "00:11:22:33:44:55",
				Hostname:   "test-vm",
				OldIP:      net.ParseIP("192.168.1.100"),
				VLAN:       10,
				Interface:  "opt7",
				Result:     "SUCCESS",
				DryRun:     false,
			},
			jsonFormat:   true,
			expectedKeys: []string{"operation", "mac", "hostname", "old_ip", "vlan", "interface", "result"},
		},
		{
			name: "ERROR result with error message",
			entry: AuditEntry{
				Timestamp:  time.Now(),
				Operation:  "CREATE",
				MACAddress: "00:11:22:33:44:55",
				Hostname:   "test-vm",
				NewIP:      net.ParseIP("192.168.1.100"),
				VLAN:       10,
				Interface:  "opt7",
				Result:     "ERROR",
				ErrorMsg:   "IP pool exhausted",
				DryRun:     false,
			},
			jsonFormat:   true,
			expectedKeys: []string{"operation", "mac", "hostname", "new_ip", "vlan", "interface", "result", "error"},
		},
		{
			name: "DRY_RUN operation",
			entry: AuditEntry{
				Timestamp:  time.Now(),
				Operation:  "CREATE",
				MACAddress: "00:11:22:33:44:55",
				Hostname:   "test-vm",
				NewIP:      net.ParseIP("192.168.1.100"),
				VLAN:       10,
				Interface:  "opt7",
				Result:     "SUCCESS",
				DryRun:     true,
			},
			jsonFormat:   true,
			expectedKeys: []string{"operation", "mac", "hostname", "new_ip", "vlan", "interface", "result", "dry_run"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(LevelInfo, tt.jsonFormat, &buf)

			LogAudit(logger, tt.entry)

			output := buf.String()

			if tt.jsonFormat {
				// Parse JSON and verify all expected keys are present
				var jsonData map[string]interface{}
				if err := json.Unmarshal(buf.Bytes(), &jsonData); err != nil {
					t.Fatalf("Invalid JSON output: %v", err)
				}

				// Check for AUDIT message
				if msg, ok := jsonData["msg"].(string); !ok || msg != "AUDIT" {
					t.Errorf("Expected msg='AUDIT', got: %v", jsonData["msg"])
				}

				// Check all expected keys are present
				for _, key := range tt.expectedKeys {
					if _, ok := jsonData[key]; !ok {
						t.Errorf("Expected key %q not found in JSON output", key)
					}
				}
			} else {
				// For text format, just check key strings are present
				if !strings.Contains(output, "AUDIT") {
					t.Error("Expected 'AUDIT' in text output")
				}
				if !strings.Contains(output, tt.entry.MACAddress) {
					t.Errorf("Expected MAC address %q in output", tt.entry.MACAddress)
				}
			}
		})
	}
}
