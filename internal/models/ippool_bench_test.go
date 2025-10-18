package models

import (
	"net"
	"testing"
)

// BenchmarkIPPoolCalculation benchmarks IP pool calculation
func BenchmarkIPPoolCalculation(b *testing.B) {
	_, ipNet, _ := net.ParseCIDR("10.0.10.0/24")
	iface := &Interface{
		ID:             "lan",
		IPAddress:      net.ParseIP("10.0.10.1"),
		Subnet:         ipNet.IP,
		SubnetMask:     24,
		DHCPEnabled:    true,
		StaticMappings: []StaticMapping{},
		DynamicLeases:  []DynamicLease{},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool, _ := CalculateIPPool(iface, nil)
		_ = pool
	}
}

// BenchmarkGetNextAvailableIP benchmarks IP selection from pool
func BenchmarkGetNextAvailableIP(b *testing.B) {
	_, ipNet, _ := net.ParseCIDR("10.0.10.0/24")
	iface := &Interface{
		ID:             "lan",
		IPAddress:      net.ParseIP("10.0.10.1"),
		Subnet:         ipNet.IP,
		SubnetMask:     24,
		DHCPEnabled:    true,
		StaticMappings: []StaticMapping{},
		DynamicLeases:  []DynamicLease{},
	}

	pool, _ := CalculateIPPool(iface, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = pool.GetNextAvailableIP()
	}
}
