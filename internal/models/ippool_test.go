package models

import (
	"net"
	"testing"
)

// TestCalculateIPPool tests the IP pool calculation logic.
func TestCalculateIPPool(t *testing.T) {
	tests := []struct {
		name             string
		iface            *Interface
		configuredRange  *IPRangeConfig
		wantAvailableMin int // Minimum expected available IPs
		wantErr          bool
	}{
		{
			name: "simple /24 network with no exclusions",
			iface: &Interface{
				ID:             "ID0",
				IPAddress:      net.ParseIP("192.168.1.1"),
				Subnet:         net.ParseIP("192.168.1.0"),
				SubnetMask:     24,
				StaticMappings: []StaticMapping{},
				DynamicLeases:  []DynamicLease{},
				DHCPPools:      []DHCPPool{},
			},
			configuredRange:  nil,
			wantAvailableMin: 250, // 254 usable IPs minus network, broadcast, interface IP
			wantErr:          false,
		},
		{
			name: "/24 network with static mappings",
			iface: &Interface{
				ID:         "ID0",
				IPAddress:  net.ParseIP("192.168.1.1"),
				Subnet:     net.ParseIP("192.168.1.0"),
				SubnetMask: 24,
				StaticMappings: []StaticMapping{
					{IPAddress: net.ParseIP("192.168.1.10")},
					{IPAddress: net.ParseIP("192.168.1.11")},
					{IPAddress: net.ParseIP("192.168.1.12")},
				},
				DynamicLeases: []DynamicLease{},
				DHCPPools:     []DHCPPool{},
			},
			configuredRange:  nil,
			wantAvailableMin: 247, // Minus 3 static mappings
			wantErr:          false,
		},
		{
			name: "/24 network with dynamic leases",
			iface: &Interface{
				ID:             "ID0",
				IPAddress:      net.ParseIP("192.168.1.1"),
				Subnet:         net.ParseIP("192.168.1.0"),
				SubnetMask:     24,
				StaticMappings: []StaticMapping{},
				DynamicLeases: []DynamicLease{
					{IPAddress: net.ParseIP("192.168.1.100")},
					{IPAddress: net.ParseIP("192.168.1.101")},
				},
				DHCPPools: []DHCPPool{},
			},
			configuredRange:  nil,
			wantAvailableMin: 248, // Minus 2 dynamic leases
			wantErr:          false,
		},
		{
			name: "/24 network with DHCP pool",
			iface: &Interface{
				ID:             "ID0",
				IPAddress:      net.ParseIP("192.168.1.1"),
				Subnet:         net.ParseIP("192.168.1.0"),
				SubnetMask:     24,
				StaticMappings: []StaticMapping{},
				DynamicLeases:  []DynamicLease{},
				DHCPPools: []DHCPPool{
					{
						StartIP: net.ParseIP("192.168.1.100"),
						EndIP:   net.ParseIP("192.168.1.200"),
					},
				},
			},
			configuredRange:  nil,
			wantAvailableMin: 149, // Only 250 - 101 (pool size)
			wantErr:          false,
		},
		{
			name: "configured range overrides netmask",
			iface: &Interface{
				ID:             "ID0",
				IPAddress:      net.ParseIP("192.168.1.1"),
				Subnet:         net.ParseIP("192.168.1.0"),
				SubnetMask:     24,
				StaticMappings: []StaticMapping{},
				DynamicLeases:  []DynamicLease{},
				DHCPPools:      []DHCPPool{},
			},
			configuredRange: &IPRangeConfig{
				StartIP: net.ParseIP("192.168.1.50"),
				EndIP:   net.ParseIP("192.168.1.99"),
			},
			wantAvailableMin: 49, // Only 50 IPs in configured range (minus interface IP if in range)
			wantErr:          false,
		},
		{
			name: "small /28 network",
			iface: &Interface{
				ID:             "ID0",
				IPAddress:      net.ParseIP("10.0.10.1"),
				Subnet:         net.ParseIP("10.0.10.0"),
				SubnetMask:     28, // Only 16 IPs total
				StaticMappings: []StaticMapping{},
				DynamicLeases:  []DynamicLease{},
				DHCPPools:      []DHCPPool{},
			},
			configuredRange:  nil,
			wantAvailableMin: 12, // 14 usable minus interface IP and maybe others
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := CalculateIPPool(tt.iface, tt.configuredRange)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateIPPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if pool == nil {
					t.Error("CalculateIPPool() returned nil pool")
					return
				}

				if pool.AvailableCount < tt.wantAvailableMin {
					t.Errorf("CalculateIPPool() AvailableCount = %d, want at least %d",
						pool.AvailableCount, tt.wantAvailableMin)
				}

				// Verify IPs are sorted
				if !isIPsSorted(pool.AvailableIPs) {
					t.Error("CalculateIPPool() IPs are not sorted in ascending order")
				}
			}
			// Expected to fail in Red phase - calculation not implemented yet
		})
	}
}

// TestIPPoolExclusions tests that network, broadcast, and interface IPs are excluded.
func TestIPPoolExclusions(t *testing.T) {
	iface := &Interface{
		ID:             "ID0",
		IPAddress:      net.ParseIP("192.168.1.1"),
		Subnet:         net.ParseIP("192.168.1.0"),
		SubnetMask:     24,
		StaticMappings: []StaticMapping{},
		DynamicLeases:  []DynamicLease{},
		DHCPPools:      []DHCPPool{},
	}

	pool, err := CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("CalculateIPPool() unexpected error: %v", err)
	}

	// Verify exclusions
	excludedIPs := []net.IP{
		net.ParseIP("192.168.1.0"),   // Network address
		net.ParseIP("192.168.1.255"), // Broadcast address
		net.ParseIP("192.168.1.1"),   // Interface IP
	}

	for _, excludedIP := range excludedIPs {
		if containsIP(pool.AvailableIPs, excludedIP) {
			t.Errorf("AvailableIPs contains excluded IP %s", excludedIP.String())
		}
	}
	// Expected to fail in Red phase
}

// TestIPPoolExhaustion tests the IP pool exhaustion scenario.
func TestIPPoolExhaustion(t *testing.T) {
	// Create /30 network (only 4 IPs total: network, 2 usable, broadcast)
	iface := &Interface{
		ID:         "ID0",
		IPAddress:  net.ParseIP("10.0.10.1"),
		Subnet:     net.ParseIP("10.0.10.0"),
		SubnetMask: 30, // /30 = 4 IPs total
		StaticMappings: []StaticMapping{
			{IPAddress: net.ParseIP("10.0.10.2")}, // Take the only other usable IP
		},
		DynamicLeases: []DynamicLease{},
		DHCPPools:     []DHCPPool{},
	}

	pool, err := CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("CalculateIPPool() unexpected error: %v", err)
	}

	if pool.AvailableCount > 0 {
		t.Errorf("Expected exhausted pool, but got %d available IPs", pool.AvailableCount)
	}

	// Try to get next available IP
	_, err = pool.GetNextAvailableIP()
	if err == nil {
		t.Error("GetNextAvailableIP() should return error when pool is exhausted")
	}
	// Expected to fail in Red phase
}

// TestGetNextAvailableIP tests sequential IP assignment.
func TestGetNextAvailableIP(t *testing.T) {
	iface := &Interface{
		ID:             "ID0",
		IPAddress:      net.ParseIP("10.0.10.1"),
		Subnet:         net.ParseIP("10.0.10.0"),
		SubnetMask:     28, // Small subnet for testing
		StaticMappings: []StaticMapping{},
		DynamicLeases:  []DynamicLease{},
		DHCPPools:      []DHCPPool{},
	}

	pool, err := CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("CalculateIPPool() unexpected error: %v", err)
	}

	// Get first available IP
	ip1, err := pool.GetNextAvailableIP()
	if err != nil {
		t.Fatalf("GetNextAvailableIP() unexpected error: %v", err)
	}

	if ip1 == nil {
		t.Error("GetNextAvailableIP() returned nil")
	}

	// Should return the lowest available IP (sorted ascending)
	expectedFirst := pool.AvailableIPs[0]
	if !ip1.Equal(expectedFirst) {
		t.Errorf("GetNextAvailableIP() = %v, want %v", ip1, expectedFirst)
	}
	// Expected to fail in Red phase
}

// Helper functions

func isIPsSorted(ips []net.IP) bool {
	for i := 1; i < len(ips); i++ {
		if compareIPs(ips[i-1], ips[i]) > 0 {
			return false
		}
	}
	return true
}

func compareIPs(ip1, ip2 net.IP) int {
	ip1v4 := ip1.To4()
	ip2v4 := ip2.To4()

	for i := 0; i < 4; i++ {
		if ip1v4[i] < ip2v4[i] {
			return -1
		}
		if ip1v4[i] > ip2v4[i] {
			return 1
		}
	}
	return 0
}

func containsIP(ips []net.IP, target net.IP) bool {
	for _, ip := range ips {
		if ip.Equal(target) {
			return true
		}
	}
	return false
}
