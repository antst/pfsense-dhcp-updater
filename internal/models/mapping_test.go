package models

import (
	"net"
	"testing"
)

// TestStaticMappingValidation tests the Validate method for StaticMapping.
func TestStaticMappingValidation(t *testing.T) {
	tests := []struct {
		name    string
		mapping StaticMapping
		wantErr bool
	}{
		{
			name: "valid mapping",
			mapping: StaticMapping{
				MACAddress:  "00:11:22:33:44:55",
				IPAddress:   net.ParseIP("10.0.10.50"),
				Hostname:    "web-server-01",
				Interface:   "ID0.10",
				Description: "Test mapping",
			},
			wantErr: false,
		},
		{
			name: "empty MAC address",
			mapping: StaticMapping{
				MACAddress: "",
				IPAddress:  net.ParseIP("10.0.10.50"),
				Hostname:   "web-server-01",
				Interface:  "ID0.10",
			},
			wantErr: true,
		},
		{
			name: "invalid MAC format",
			mapping: StaticMapping{
				MACAddress: "00:11:22:33:44",
				IPAddress:  net.ParseIP("10.0.10.50"),
				Hostname:   "web-server-01",
				Interface:  "ID0.10",
			},
			wantErr: true,
		},
		{
			name: "nil IP address",
			mapping: StaticMapping{
				MACAddress: "00:11:22:33:44:55",
				IPAddress:  nil,
				Hostname:   "web-server-01",
				Interface:  "ID0.10",
			},
			wantErr: true,
		},
		{
			name: "empty hostname",
			mapping: StaticMapping{
				MACAddress: "00:11:22:33:44:55",
				IPAddress:  net.ParseIP("10.0.10.50"),
				Hostname:   "",
				Interface:  "ID0.10",
			},
			wantErr: true,
		},
		{
			name: "invalid hostname format",
			mapping: StaticMapping{
				MACAddress: "00:11:22:33:44:55",
				IPAddress:  net.ParseIP("10.0.10.50"),
				Hostname:   "invalid_hostname!",
				Interface:  "ID0.10",
			},
			wantErr: true,
		},
		{
			name: "empty interface",
			mapping: StaticMapping{
				MACAddress: "00:11:22:33:44:55",
				IPAddress:  net.ParseIP("10.0.10.50"),
				Hostname:   "web-server-01",
				Interface:  "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StaticMapping.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStaticMappingNormalization tests MAC address normalization.
func TestStaticMappingNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputMAC string
		wantMAC  string
	}{
		{
			name:     "uppercase with colons",
			inputMAC: "AA:BB:CC:DD:EE:FF",
			wantMAC:  "aa:bb:cc:dd:ee:ff",
		},
		{
			name:     "hyphens to colons",
			inputMAC: "00-11-22-33-44-55",
			wantMAC:  "00:11:22:33:44:55",
		},
		{
			name:     "mixed case with hyphens",
			inputMAC: "aA-Bb-Cc-Dd-Ee-Ff",
			wantMAC:  "aa:bb:cc:dd:ee:ff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: This will test the normalization logic once implemented
			// For now, this test is expected to fail (Red phase)
			t.Skip("Normalization not implemented yet - expected to fail in Red phase")
		})
	}
}
