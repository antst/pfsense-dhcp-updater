// Package models provides domain entities for DHCP static mappings.
package models

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/antst/pfsense-dhcp-updater/internal/validator"
)

// StaticMapping represents a DHCP static mapping (MAC-to-IP binding).
type StaticMapping struct {
	ID          string // pfSense internal ID (for updates)
	MACAddress  string // Colon-separated lowercase (00:11:22:33:44:55)
	IPAddress   net.IP // Assigned IP
	Hostname    string // DNS hostname
	Description string // Optional description
	Interface   string // Interface ID (e.g., "ID0.10")
}

// Validate checks if the static mapping is valid.
func (m *StaticMapping) Validate() error {
	// Validate MAC address
	if m.MACAddress == "" {
		return fmt.Errorf("MAC address is required")
	}

	macPattern := regexp.MustCompile(`^([0-9a-f]{2}:){5}([0-9a-f]{2})$`)
	if !macPattern.MatchString(m.MACAddress) {
		return fmt.Errorf("invalid MAC address format: must be lowercase with colons (e.g., 00:11:22:33:44:55)")
	}

	// Validate IP address
	if m.IPAddress == nil {
		return fmt.Errorf("IP address is required")
	}
	if m.IPAddress.To4() == nil {
		return fmt.Errorf("only IPv4 addresses are supported")
	}

	// Validate hostname
	if m.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if _, err := validator.ValidateHostname(m.Hostname); err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}

	// Validate interface
	if m.Interface == "" {
		return fmt.Errorf("interface is required")
	}

	return nil
}

// NormalizeMAC normalizes a MAC address to lowercase with colons.
func NormalizeMAC(mac string) string {
	normalized := strings.ToLower(mac)
	normalized = strings.ReplaceAll(normalized, "-", ":")
	return normalized
}
