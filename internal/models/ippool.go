// Package models provides domain entities for DHCP static mappings.
package models

import (
	"bytes"
	"fmt"
	"net"
	"sort"
)

// IPPool represents the calculated pool of available IP addresses for assignment.
type IPPool struct {
	AvailableIPs    []net.IP       // Sorted ascending
	Exclusions      []net.IP       // IPs to exclude from pool
	TotalCapacity   int            // Total IPs in range (before exclusions)
	AvailableCount  int            // Number of available IPs
	ConfiguredRange *IPRangeConfig // Optional: configured range from config
}

// IPRangeConfig represents a configured IP range.
type IPRangeConfig struct {
	StartIP net.IP
	EndIP   net.IP
}

// CalculateIPPool computes available IPs for the given interface.
func CalculateIPPool(iface *Interface, configuredRange *IPRangeConfig) (*IPPool, error) {
	if iface == nil {
		return nil, fmt.Errorf("interface is required")
	}

	pool := &IPPool{
		Exclusions:      []net.IP{},
		ConfiguredRange: configuredRange,
	}

	var startIP, endIP net.IP

	// Determine IP range: configured range takes precedence over subnet range
	if configuredRange != nil {
		startIP = configuredRange.StartIP.To4()
		endIP = configuredRange.EndIP.To4()
	} else {
		// Use subnet range
		ipNet := iface.GetSubnetCIDR()
		if ipNet == nil {
			return nil, fmt.Errorf("cannot determine subnet range")
		}
		startIP = ipNet.IP.To4()
		endIP = broadcastIP(ipNet)
	}

	// Build exclusion set
	exclusionSet := make(map[string]bool)

	// Exclude network address (first IP in subnet)
	if configuredRange == nil {
		ipNet := iface.GetSubnetCIDR()
		exclusionSet[ipNet.IP.String()] = true
		// Exclude broadcast address (last IP in subnet)
		exclusionSet[broadcastIP(ipNet).String()] = true
	}

	// Exclude interface IP
	if iface.IPAddress != nil {
		exclusionSet[iface.IPAddress.String()] = true
	}

	// Exclude gateway IP
	if iface.Gateway != nil {
		exclusionSet[iface.Gateway.String()] = true
	}

	// Exclude static mappings
	for _, mapping := range iface.StaticMappings {
		if mapping.IPAddress != nil {
			exclusionSet[mapping.IPAddress.String()] = true
		}
	}

	// Exclude ALL dynamic leases (CRITICAL for preventing conflicts)
	// This includes leases that may exist outside the current DHCP pool range.
	//
	// Example scenario requiring this:
	// 1. Original DHCP pool: 192.168.1.100-200
	// 2. VM gets dynamic lease: 192.168.1.150
	// 3. Admin changes pool to: 192.168.1.50-100 (shrinks range)
	// 4. Lease 192.168.1.150 still exists but is now OUTSIDE the pool range
	// 5. Without this exclusion, we might assign static IP 192.168.1.150 → CONFLICT!
	//
	// Note: We fetch leases from a separate API endpoint (/status/dhcp/leases)
	// to ensure we have ALL current leases, not just those reported by the
	// DHCP config endpoint.
	for _, lease := range iface.DynamicLeases {
		if lease.IPAddress != nil {
			exclusionSet[lease.IPAddress.String()] = true
		}
	}

	// Exclude ENTIRE DHCP pool ranges (all IPs in pool ranges)
	// This prevents assigning static IPs that could be dynamically assigned in the future.
	//
	// Rationale: The DHCP pool range is reserved for dynamic allocation.
	// If we only excluded active leases, we could assign a static IP (e.g., 192.168.1.150)
	// that hasn't been leased yet. When the next VM boots and requests DHCP, the DHCP
	// server might try to assign 192.168.1.150, creating a conflict.
	//
	// Static mappings should ONLY be assigned from IPs outside the DHCP pool range.
	for _, dhcpPool := range iface.DHCPPools {
		poolIPs := ipRange(dhcpPool.StartIP, dhcpPool.EndIP)
		for _, ip := range poolIPs {
			exclusionSet[ip.String()] = true
		}
	}

	// Calculate available IPs
	var availableIPs []net.IP
	allIPs := ipRange(startIP, endIP)

	for _, ip := range allIPs {
		if !exclusionSet[ip.String()] {
			availableIPs = append(availableIPs, ip)
		} else {
			pool.Exclusions = append(pool.Exclusions, ip)
		}
	}

	// Sort IPs in ascending order
	sort.Slice(availableIPs, func(i, j int) bool {
		return bytes.Compare(availableIPs[i], availableIPs[j]) < 0
	})

	pool.AvailableIPs = availableIPs
	pool.TotalCapacity = len(allIPs)
	pool.AvailableCount = len(availableIPs)

	return pool, nil
}

// GetNextAvailableIP returns the next available IP from the pool.
func (p *IPPool) GetNextAvailableIP() (net.IP, error) {
	if p.AvailableCount == 0 || len(p.AvailableIPs) == 0 {
		return nil, fmt.Errorf("IP pool exhausted: no available IPs")
	}

	// Return the first (lowest) available IP
	return p.AvailableIPs[0], nil
}

// GetNextAvailableIPExcluding returns the next available IP from the pool,
// excluding IPs in the provided exclusion map (typically IPs that have already been attempted).
// This is useful for retry logic when concurrent IP assignment conflicts occur.
//
// Parameters:
//   - excludedIPs: Map of IP addresses (as strings) that should be skipped
//
// Returns:
//   - net.IP: The next available IP that is not in the exclusion map
//   - error: An error if no suitable IP is available
//
// Example:
//
//	attemptedIPs := map[string]bool{"192.168.1.100": true, "192.168.1.101": true}
//	nextIP, err := pool.GetNextAvailableIPExcluding(attemptedIPs)
func (p *IPPool) GetNextAvailableIPExcluding(excludedIPs map[string]bool) (net.IP, error) {
	if p.AvailableCount == 0 || len(p.AvailableIPs) == 0 {
		return nil, fmt.Errorf("IP pool exhausted: no available IPs")
	}

	// Find first available IP that is not in the exclusion map
	for _, ip := range p.AvailableIPs {
		if !excludedIPs[ip.String()] {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("IP pool exhausted: all %d available IPs have been attempted", len(p.AvailableIPs))
}

// Helper functions

// ipRange generates all IPs between start and end (inclusive).
func ipRange(start, end net.IP) []net.IP {
	var ips []net.IP

	current := make(net.IP, len(start))
	copy(current, start)

	for {
		ip := make(net.IP, len(current))
		copy(ip, current)
		ips = append(ips, ip)

		if current.Equal(end) {
			break
		}

		// Increment IP
		incIP(current)
	}

	return ips
}

// incIP increments an IP address by 1.
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// broadcastIP calculates the broadcast address for a subnet.
func broadcastIP(ipNet *net.IPNet) net.IP {
	ip := ipNet.IP.To4()
	mask := ipNet.Mask

	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip[i] | ^mask[i]
	}

	return broadcast
}

// ParseIPRange parses an IP range string in the format "start-end" or "start".
// Returns the start and end IPs. If only one IP is provided, start and end are the same.
func ParseIPRange(rangeStr string) (net.IP, net.IP, error) {
	// Split by hyphen
	parts := bytes.Split([]byte(rangeStr), []byte("-"))

	if len(parts) == 0 || len(parts) > 2 {
		return nil, nil, fmt.Errorf("invalid IP range format: %s (expected 'start-end' or 'start')", rangeStr)
	}

	// Parse start IP
	startIP := net.ParseIP(string(bytes.TrimSpace(parts[0])))
	if startIP == nil {
		return nil, nil, fmt.Errorf("invalid start IP in range: %s", string(parts[0]))
	}
	startIP = startIP.To4()
	if startIP == nil {
		return nil, nil, fmt.Errorf("IPv6 not supported: %s", string(parts[0]))
	}

	// Parse end IP (or use start if not provided)
	var endIP net.IP
	if len(parts) == 2 {
		endIP = net.ParseIP(string(bytes.TrimSpace(parts[1])))
		if endIP == nil {
			return nil, nil, fmt.Errorf("invalid end IP in range: %s", string(parts[1]))
		}
		endIP = endIP.To4()
		if endIP == nil {
			return nil, nil, fmt.Errorf("IPv6 not supported: %s", string(parts[1]))
		}
	} else {
		endIP = startIP
	}

	// Validate start <= end
	if bytes.Compare(startIP, endIP) > 0 {
		return nil, nil, fmt.Errorf("start IP %s is greater than end IP %s", startIP, endIP)
	}

	return startIP, endIP, nil
}
