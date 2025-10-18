package pfsense

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antst/pfsense-dhcp-updater/internal/logger"
	"github.com/antst/pfsense-dhcp-updater/internal/models"
)

// TestCheckHostnameConflict tests hostname conflict detection.
func TestCheckHostnameConflict(t *testing.T) {
	tests := []struct {
		name          string
		iface         *models.Interface
		hostname      string
		macAddress    string
		expectError   bool
		errorContains string
	}{
		{
			name: "no conflict - hostname not used",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
					{
						Hostname:   "host2",
						MACAddress: "aa:bb:cc:dd:ee:22",
						IPAddress:  net.ParseIP("192.168.1.11"),
					},
				},
			},
			hostname:    "host3",
			macAddress:  "aa:bb:cc:dd:ee:33",
			expectError: false,
		},
		{
			name: "no conflict - same hostname with same MAC (idempotent)",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			hostname:    "host1",
			macAddress:  "aa:bb:cc:dd:ee:11",
			expectError: false,
		},
		{
			name: "conflict - hostname exists with different MAC",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			hostname:      "host1",
			macAddress:    "aa:bb:cc:dd:ee:99",
			expectError:   true,
			errorContains: "hostname 'host1' is already in use by MAC address aa:bb:cc:dd:ee:11",
		},
		{
			name: "no conflict - empty static mappings",
			iface: &models.Interface{
				ID:             "lan",
				StaticMappings: []models.StaticMapping{},
			},
			hostname:    "host1",
			macAddress:  "aa:bb:cc:dd:ee:11",
			expectError: false,
		},
		{
			name: "conflict - hostname differs only in case",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						Hostname:   "Host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			hostname:      "host1",
			macAddress:    "aa:bb:cc:dd:ee:99",
			expectError:   true,
			errorContains: "hostname 'host1' is already in use",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckHostnameConflict(tt.iface, tt.hostname, tt.macAddress)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestFindMappingByMAC tests finding a static mapping by MAC address.
func TestFindMappingByMAC(t *testing.T) {
	tests := []struct {
		name        string
		iface       *models.Interface
		macAddress  string
		expectFound bool
		expectedID  string
		expectedIP  string
	}{
		{
			name: "mapping found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
					{
						ID:         "2",
						Hostname:   "host2",
						MACAddress: "aa:bb:cc:dd:ee:22",
						IPAddress:  net.ParseIP("192.168.1.11"),
					},
				},
			},
			macAddress:  "aa:bb:cc:dd:ee:22",
			expectFound: true,
			expectedID:  "2",
			expectedIP:  "192.168.1.11",
		},
		{
			name: "mapping not found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			macAddress:  "aa:bb:cc:dd:ee:99",
			expectFound: false,
		},
		{
			name: "empty mappings",
			iface: &models.Interface{
				ID:             "lan",
				StaticMappings: []models.StaticMapping{},
			},
			macAddress:  "aa:bb:cc:dd:ee:11",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := FindMappingByMAC(tt.iface, tt.macAddress)

			if tt.expectFound {
				if mapping == nil {
					t.Errorf("expected mapping but got nil")
					return
				}
				if mapping.ID != tt.expectedID {
					t.Errorf("expected ID %q but got %q", tt.expectedID, mapping.ID)
				}
				if mapping.IPAddress.String() != tt.expectedIP {
					t.Errorf("expected IP %q but got %q", tt.expectedIP, mapping.IPAddress.String())
				}
			} else {
				if mapping != nil {
					t.Errorf("expected nil but got mapping: %+v", mapping)
				}
			}
		})
	}
}

// TestFindMappingByHostname tests finding a static mapping by hostname.
func TestFindMappingByHostname(t *testing.T) {
	tests := []struct {
		name        string
		iface       *models.Interface
		hostname    string
		expectFound bool
		expectedID  string
		expectedMAC string
	}{
		{
			name: "mapping found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
					{
						ID:         "2",
						Hostname:   "host2",
						MACAddress: "aa:bb:cc:dd:ee:22",
						IPAddress:  net.ParseIP("192.168.1.11"),
					},
				},
			},
			hostname:    "host2",
			expectFound: true,
			expectedID:  "2",
			expectedMAC: "aa:bb:cc:dd:ee:22",
		},
		{
			name: "mapping found - case insensitive",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "Host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			hostname:    "host1",
			expectFound: true,
			expectedID:  "1",
			expectedMAC: "aa:bb:cc:dd:ee:11",
		},
		{
			name: "mapping not found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			hostname:    "host99",
			expectFound: false,
		},
		{
			name: "empty mappings",
			iface: &models.Interface{
				ID:             "lan",
				StaticMappings: []models.StaticMapping{},
			},
			hostname:    "host1",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := FindMappingByHostname(tt.iface, tt.hostname)

			if tt.expectFound {
				if mapping == nil {
					t.Errorf("expected mapping but got nil")
					return
				}
				if mapping.ID != tt.expectedID {
					t.Errorf("expected ID %q but got %q", tt.expectedID, mapping.ID)
				}
				if mapping.MACAddress != tt.expectedMAC {
					t.Errorf("expected MAC %q but got %q", tt.expectedMAC, mapping.MACAddress)
				}
			} else {
				if mapping != nil {
					t.Errorf("expected nil but got mapping: %+v", mapping)
				}
			}
		})
	}
}

// TestValidateDualIdentifiers tests validation of both MAC and hostname for deletion.
func TestValidateDualIdentifiers(t *testing.T) {
	tests := []struct {
		name          string
		iface         *models.Interface
		macAddress    string
		hostname      string
		expectError   bool
		errorContains string
	}{
		{
			name: "both identifiers reference same mapping - valid",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			macAddress:  "aa:bb:cc:dd:ee:11",
			hostname:    "host1",
			expectError: false,
		},
		{
			name: "both identifiers reference same mapping - case insensitive",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "Host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			macAddress:  "aa:bb:cc:dd:ee:11",
			hostname:    "host1",
			expectError: false,
		},
		{
			name: "identifiers reference different mappings - invalid",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
					{
						ID:         "2",
						Hostname:   "host2",
						MACAddress: "aa:bb:cc:dd:ee:22",
						IPAddress:  net.ParseIP("192.168.1.11"),
					},
				},
			},
			macAddress:    "aa:bb:cc:dd:ee:11",
			hostname:      "host2",
			expectError:   true,
			errorContains: "MAC address aa:bb:cc:dd:ee:11 is associated with hostname 'host1', not 'host2'",
		},
		{
			name: "MAC not found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			macAddress:    "aa:bb:cc:dd:ee:99",
			hostname:      "host1",
			expectError:   true,
			errorContains: "MAC address aa:bb:cc:dd:ee:99 not found",
		},
		{
			name: "hostname not found",
			iface: &models.Interface{
				ID: "lan",
				StaticMappings: []models.StaticMapping{
					{
						ID:         "1",
						Hostname:   "host1",
						MACAddress: "aa:bb:cc:dd:ee:11",
						IPAddress:  net.ParseIP("192.168.1.10"),
					},
				},
			},
			macAddress:    "aa:bb:cc:dd:ee:11",
			hostname:      "host99",
			expectError:   true,
			errorContains: "hostname 'host99' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDualIdentifiers(tt.iface, tt.macAddress, tt.hostname)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestResolveInterfaceID tests interface ID determination with VLAN support.
func TestResolveInterfaceID(t *testing.T) {
	tests := []struct {
		name              string
		physicalInterface string
		vlanTag           int
		manualMapping     map[string]string
		apiInterfaces     []map[string]interface{}
		expectedID        string
		expectError       bool
		errorContains     string
	}{
		{
			name:              "VLAN 0 - base interface - manual mapping",
			physicalInterface: "iavf1",
			vlanTag:           0,
			manualMapping: map[string]string{
				"iavf1": "lan",
			},
			expectedID:  "lan",
			expectError: false,
		},
		{
			name:              "VLAN 10 - tagged interface - manual mapping",
			physicalInterface: "iavf1",
			vlanTag:           10,
			manualMapping: map[string]string{
				"iavf1.10": "lan.10",
			},
			expectedID:  "lan.10",
			expectError: false,
		},
		{
			name:              "VLAN 0 - base interface - API discovery",
			physicalInterface: "iavf1",
			vlanTag:           0,
			manualMapping:     nil,
			apiInterfaces: []map[string]interface{}{
				{
					"id":     "lan",
					"if":     "iavf1",
					"descr":  "LAN",
					"enable": true,
				},
				{
					"id":     "wan",
					"if":     "iavf0",
					"descr":  "WAN",
					"enable": true,
				},
			},
			expectedID:  "lan",
			expectError: false,
		},
		{
			name:              "VLAN 101 - tagged interface - API discovery",
			physicalInterface: "iavf1",
			vlanTag:           101,
			manualMapping:     nil,
			apiInterfaces: []map[string]interface{}{
				{
					"id":     "lan",
					"if":     "iavf1",
					"descr":  "LAN",
					"enable": true,
				},
				{
					"id":     "opt1",
					"if":     "iavf1.101",
					"descr":  "VLAN101",
					"enable": true,
				},
			},
			expectedID:  "opt1",
			expectError: false,
		},
		{
			name:              "non-existent interface - API discovery",
			physicalInterface: "iavf2",
			vlanTag:           0,
			manualMapping:     nil,
			apiInterfaces: []map[string]interface{}{
				{
					"id":     "lan",
					"if":     "iavf1",
					"descr":  "LAN",
					"enable": true,
				},
			},
			expectError:   true,
			errorContains: "interface iavf2 not found",
		},
		{
			name:              "non-existent VLAN - API discovery",
			physicalInterface: "iavf1",
			vlanTag:           999,
			manualMapping:     nil,
			apiInterfaces: []map[string]interface{}{
				{
					"id":     "lan",
					"if":     "iavf1",
					"descr":  "LAN",
					"enable": true,
				},
			},
			expectError:   true,
			errorContains: "interface iavf1.999 not found",
		},
		{
			name:              "manual mapping takes precedence over API",
			physicalInterface: "iavf1",
			vlanTag:           0,
			manualMapping: map[string]string{
				"iavf1": "custom_lan",
			},
			apiInterfaces: []map[string]interface{}{
				{
					"id":     "lan",
					"if":     "iavf1",
					"descr":  "LAN",
					"enable": true,
				},
			},
			expectedID:  "custom_lan",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/interfaces" {
					t.Errorf("unexpected request path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				response := map[string]interface{}{
					"status":      "ok",
					"code":        200,
					"response_id": "test",
					"data":        tt.apiInterfaces,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create a client
			log := logger.New(logger.LevelError, false, io.Discard)
			client := NewClient(ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-key",
				InsecureSkipVerify: true,
				Logger:             log,
			})

			// Test the function
			ctx := context.Background()
			result, err := client.ResolveInterfaceID(ctx, tt.physicalInterface, tt.vlanTag, tt.manualMapping)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if result != tt.expectedID {
					t.Errorf("expected interface ID %q but got %q", tt.expectedID, result)
				}
			}
		})
	}
}

// TestGetNextAvailableIPExcluding tests the IP pool exclusion logic used for retry.
func TestGetNextAvailableIPExcluding(t *testing.T) {
	// Create a simple interface with a few available IPs
	iface := &models.Interface{
		ID:          "lan",
		IPAddress:   net.ParseIP("192.168.1.1"),
		Subnet:      net.ParseIP("192.168.1.0"),
		SubnetMask:  24,
		DHCPEnabled: true,
		StaticMappings: []models.StaticMapping{
			{IPAddress: net.ParseIP("192.168.1.10")},
		},
	}

	// Calculate IP pool (will have 192.168.1.2 - 192.168.1.254, excluding .1, .10, .0, .255)
	pool, err := models.CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("failed to calculate IP pool: %v", err)
	}

	tests := []struct {
		name        string
		excludedIPs map[string]bool
		expectError bool
		expectIP    string // Expected IP address (if not error)
	}{
		{
			name:        "no exclusions - returns first IP",
			excludedIPs: map[string]bool{},
			expectError: false,
			expectIP:    "192.168.1.2", // First available IP after .1 (interface)
		},
		{
			name: "exclude first IP - returns second",
			excludedIPs: map[string]bool{
				"192.168.1.2": true,
			},
			expectError: false,
			expectIP:    "192.168.1.3",
		},
		{
			name: "exclude first two IPs - returns third",
			excludedIPs: map[string]bool{
				"192.168.1.2": true,
				"192.168.1.3": true,
			},
			expectError: false,
			expectIP:    "192.168.1.4",
		},
		{
			name: "exclude many IPs but not all",
			excludedIPs: map[string]bool{
				"192.168.1.2": true,
				"192.168.1.3": true,
				"192.168.1.4": true,
				"192.168.1.5": true,
			},
			expectError: false,
			expectIP:    "192.168.1.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := pool.GetNextAvailableIPExcluding(tt.excludedIPs)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if ip.String() != tt.expectIP {
					t.Errorf("expected IP %s but got %s", tt.expectIP, ip.String())
				}
			}
		})
	}
}

// TestContainsAny tests the containsAny helper function.
func TestContainsAny(t *testing.T) {
	tests := []struct {
		name       string
		str        string
		substrings []string
		expected   bool
	}{
		{
			name:       "single match",
			str:        "IP address already in use",
			substrings: []string{"ip address already in use"},
			expected:   true,
		},
		{
			name:       "case insensitive match",
			str:        "IP ADDRESS ALREADY IN USE",
			substrings: []string{"ip address already in use"},
			expected:   true,
		},
		{
			name:       "multiple options - first matches",
			str:        "ipaddr is already in use",
			substrings: []string{"ip already in use", "ipaddr is already in use"},
			expected:   true,
		},
		{
			name:       "multiple options - second matches",
			str:        "A static mapping with this IP already exists",
			substrings: []string{"duplicate ip", "static mapping with this ip"},
			expected:   true,
		},
		{
			name:       "no match",
			str:        "Invalid MAC address format",
			substrings: []string{"ip address already in use", "duplicate ip"},
			expected:   false,
		},
		{
			name:       "partial match",
			str:        "IP conflict detected",
			substrings: []string{"ip address already in use"},
			expected:   false,
		},
		{
			name:       "empty substrings list",
			str:        "any string",
			substrings: []string{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.str, tt.substrings)
			if result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}
