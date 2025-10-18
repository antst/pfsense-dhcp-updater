// Package validator provides input validation functions for MAC addresses, hostnames, VLANs, and IPs.
package validator

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var (
	// MAC address patterns: supports both colon and hyphen separators
	macPattern = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)

	// Hostname pattern: RFC 1123 compliant
	// - Alphanumeric and hyphens only
	// - No leading/trailing hyphens
	// - Case insensitive
	hostnamePattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
)

// ValidateMAC validates and normalizes a MAC address.
// Returns the normalized MAC address (lowercase with colons) or an error.
func ValidateMAC(mac string) (string, error) {
	if mac == "" {
		return "", fmt.Errorf("MAC address is required")
	}

	if !macPattern.MatchString(mac) {
		return "", fmt.Errorf("invalid MAC address format: must match XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX")
	}

	// Normalize: lowercase and replace hyphens with colons
	normalized := strings.ToLower(mac)
	normalized = strings.ReplaceAll(normalized, "-", ":")

	return normalized, nil
}

// ValidateHostname validates and normalizes a hostname according to RFC 1123.
// Returns the normalized hostname (lowercase) or an error.
func ValidateHostname(hostname string) (string, error) {
	if hostname == "" {
		return "", fmt.Errorf("hostname is required")
	}

	if len(hostname) > 253 {
		return "", fmt.Errorf("hostname exceeds maximum length of 253 characters")
	}

	// Check each label (dot-separated part)
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) > 63 {
			return "", fmt.Errorf("hostname label '%s' exceeds maximum length of 63 characters", label)
		}
	}

	if !hostnamePattern.MatchString(hostname) {
		return "", fmt.Errorf("invalid hostname format: must be RFC 1123 compliant (alphanumeric and hyphens, no leading/trailing hyphens)")
	}

	// Normalize: lowercase (DNS is case-insensitive)
	normalized := strings.ToLower(hostname)

	return normalized, nil
}

// ValidateVLAN validates a VLAN tag.
// Returns the VLAN tag or an error.
func ValidateVLAN(vlan int) (int, error) {
	if vlan < 0 || vlan > 4094 {
		return 0, fmt.Errorf("invalid VLAN tag %d: must be in range 0-4094", vlan)
	}
	return vlan, nil
}

// ValidateIP validates an IPv4 address.
// Returns the parsed IP or an error.
func ValidateIP(ipStr string) (net.IP, error) {
	if ipStr == "" {
		return nil, fmt.Errorf("IP address is required")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address format: %s", ipStr)
	}

	// Ensure it's IPv4
	if ip.To4() == nil {
		return nil, fmt.Errorf("only IPv4 addresses are supported: %s", ipStr)
	}

	return ip.To4(), nil
}

// ValidateIPInSubnet checks if an IP address is within a subnet.
func ValidateIPInSubnet(ip net.IP, subnet *net.IPNet) error {
	if !subnet.Contains(ip) {
		return fmt.Errorf("IP address %s is not in subnet %s", ip.String(), subnet.String())
	}
	return nil
}
