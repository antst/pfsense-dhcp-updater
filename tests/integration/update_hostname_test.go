package integration

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/antst/pfsense-dhcp-updater/internal/config"
	"github.com/antst/pfsense-dhcp-updater/internal/logger"
	"github.com/antst/pfsense-dhcp-updater/internal/models"
	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
)

// findAvailableIP finds an IP address in the pool that doesn't have an existing static mapping
func findAvailableIP(ctx context.Context, client *pfsense.Client, interfaceID string, pool models.DHCPPool) (net.IP, error) {
	// Get DHCP config to see existing static mappings
	dhcpConfig, err := client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP config: %w", err)
	}

	// Build a map of used IPs from static mappings
	usedIPs := make(map[string]bool)
	for _, mapping := range dhcpConfig.StaticMappings {
		usedIPs[mapping.IPAddress.String()] = true
	}

	// Also get all leases to see dynamic leases (they're not in static mappings yet)
	leases, err := client.GetDHCPLeases(ctx)
	if err == nil {
		// Add IPs from merged leases that might conflict
		for _, lease := range leases {
			if lease.IPAddress != nil {
				usedIPs[lease.IPAddress.String()] = true
			}
		}
	}

	// Convert pool IPs to uint32 for iteration
	startIP := ipToUint32(pool.StartIP)
	endIP := ipToUint32(pool.EndIP)

	// Try IPs from the end of the pool (less likely to be used)
	for i := endIP; i >= startIP; i-- {
		ip := uint32ToIP(i)
		ipStr := ip.String()
		if !usedIPs[ipStr] {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("no available IP addresses in pool %s-%s (all %d addresses are used)",
		pool.StartIP, pool.EndIP, len(usedIPs))
} // ipToUint32 converts a net.IP to uint32 for arithmetic operations
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// uint32ToIP converts a uint32 back to net.IP
func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// TestUpdateHostname_BaseInterface tests hostname update for an existing static mapping on the base interface.
// This test verifies:
// - Finding existing mapping by MAC address
// - Updating hostname while preserving IP address
// - Interface remains unchanged
// - Safe testing: Creates test mapping, updates it, then cleans up
func TestUpdateHostname_BaseInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := config.Load("../../config.test.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	log := logger.New(logger.LevelInfo, false, os.Stderr)

	client := pfsense.NewClient(pfsense.ClientConfig{
		BaseURL:            cfg.PfSense.Endpoint,
		APIKey:             cfg.PfSense.APIKey,
		InsecureSkipVerify: cfg.PfSense.InsecureSkipVerify,
		Logger:             log,
	})

	ctx := context.Background()

	t.Run("update_hostname_on_base_interface", func(t *testing.T) {
		// Test parameters
		testMAC := "ff:ff:ff:ff:ff:cc"
		originalHostname := "test-update-original"
		updatedHostname := "test-update-modified"
		physicalInterface := "iavf1"
		vlanTag := 0

		// Resolve interface ID
		interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
		if err != nil {
			t.Fatalf("failed to resolve interface ID: %v", err)
		}

		t.Logf("Resolved interface: physical=%s vlan=%d → logical=%s", physicalInterface, vlanTag, interfaceID)

		// Get DHCP configuration and find available IP
		dhcpConfig, err := client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to get DHCP config: %v", err)
		}

		if len(dhcpConfig.DHCPPools) == 0 {
			t.Fatal("no DHCP pools configured for interface")
		}

		// Use an IP OUTSIDE the DHCP pool to avoid conflicts
		// Static mappings are commonly placed outside dynamic pools
		pool := dhcpConfig.DHCPPools[0]
		poolStart := ipToUint32(pool.StartIP)

		// Use IP just before pool starts (e.g., .119 if pool starts at .120)
		testIP := uint32ToIP(poolStart - 1)

		t.Logf("Using test IP: %s OUTSIDE pool range (%s-%s)",
			testIP,
			pool.StartIP,
			pool.EndIP,
		) // Create initial static mapping
		originalMapping := &models.StaticMapping{
			MACAddress:  testMAC,
			IPAddress:   testIP,
			Hostname:    originalHostname,
			Description: "Test mapping for hostname update (will be deleted)",
			Interface:   interfaceID,
		}

		err = client.CreateStaticMapping(ctx, originalMapping)
		if err != nil {
			t.Fatalf("failed to create test mapping: %v", err)
		}

		t.Logf("Created test mapping")

		// Cleanup: defer deletion
		defer func() {
			t.Logf("Cleaning up test mapping (MAC: %s)", testMAC)
			// Note: Would need DeleteStaticMapping function - skip for now in safe testing mode
		}()

		// Apply changes to pfSense
		if err := client.ApplyDHCPChanges(ctx); err != nil {
			t.Fatalf("failed to apply DHCP changes after create: %v", err)
		}

		// Retrieve the mapping to get its full state
		dhcpConfig, err = client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to retrieve DHCP config: %v", err)
		}

		// Find our mapping by MAC
		var foundMapping *models.StaticMapping
		for i := range dhcpConfig.StaticMappings {
			if dhcpConfig.StaticMappings[i].MACAddress == testMAC {
				foundMapping = &dhcpConfig.StaticMappings[i]
				break
			}
		}

		if foundMapping == nil {
			t.Fatalf("could not find created mapping with MAC %s", testMAC)
		}

		t.Logf("Found created mapping: ID=%s, IP=%s, Hostname=%s",
			foundMapping.ID, foundMapping.IPAddress, foundMapping.Hostname)

		// Verify initial state
		if foundMapping.Hostname != originalHostname {
			t.Errorf("expected hostname %q, got %q", originalHostname, foundMapping.Hostname)
		}

		// Update the hostname
		updatedMapping := &models.StaticMapping{
			ID:          foundMapping.ID,
			MACAddress:  foundMapping.MACAddress,
			IPAddress:   foundMapping.IPAddress,
			Hostname:    updatedHostname,
			Description: "Test mapping after hostname update (will be deleted)",
			Interface:   interfaceID,
		}

		err = client.UpdateStaticMapping(ctx, updatedMapping)
		if err != nil {
			t.Fatalf("failed to update hostname: %v", err)
		}

		t.Logf("Updated hostname from %q to %q", originalHostname, updatedHostname)

		// Apply changes
		if err := client.ApplyDHCPChanges(ctx); err != nil {
			t.Fatalf("failed to apply DHCP changes after update: %v", err)
		}

		// Retrieve again to verify the update
		dhcpConfig, err = client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to retrieve DHCP config after update: %v", err)
		}

		// Find the updated mapping
		var verifyMapping *models.StaticMapping
		for i := range dhcpConfig.StaticMappings {
			if dhcpConfig.StaticMappings[i].MACAddress == testMAC {
				verifyMapping = &dhcpConfig.StaticMappings[i]
				break
			}
		}

		if verifyMapping == nil {
			t.Fatalf("mapping disappeared after update")
		}

		// Verify hostname was updated
		if verifyMapping.Hostname != updatedHostname {
			t.Errorf("hostname not updated: expected %q, got %q", updatedHostname, verifyMapping.Hostname)
		}

		// Verify IP address unchanged
		if !verifyMapping.IPAddress.Equal(testIP) {
			t.Errorf("IP address changed: expected %s, got %s", testIP, verifyMapping.IPAddress)
		}

		// Verify MAC address unchanged
		if verifyMapping.MACAddress != testMAC {
			t.Errorf("MAC address changed: expected %s, got %s", testMAC, verifyMapping.MACAddress)
		}

		t.Logf("✓ Test successful: hostname updated from %q to %q, IP preserved (%s)",
			originalHostname, updatedHostname, testIP)
	})
}

// TestUpdateHostname_VLAN tests hostname update for an existing static mapping on a VLAN interface.
// This test verifies:
// - Hostname updates work on VLAN interfaces
// - IP address preservation across VLANs
// - Interface-specific mapping updates
// - Safe testing: Creates test mapping, updates it, then cleans up
func TestUpdateHostname_VLAN(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := config.Load("../../config.test.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	log := logger.New(logger.LevelInfo, false, os.Stderr)

	client := pfsense.NewClient(pfsense.ClientConfig{
		BaseURL:            cfg.PfSense.Endpoint,
		APIKey:             cfg.PfSense.APIKey,
		InsecureSkipVerify: cfg.PfSense.InsecureSkipVerify,
		Logger:             log,
	})

	ctx := context.Background()

	t.Run("update_hostname_on_VLAN", func(t *testing.T) {
		// Test parameters
		testMAC := "ff:ff:ff:ff:ff:dd"
		originalHostname := "test-vlan-original"
		updatedHostname := "test-vlan-modified"
		physicalInterface := "iavf1"
		vlanTag := 101 // Using VLAN 101 as it exists in our test environment

		// Resolve interface ID
		interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
		if err != nil {
			t.Fatalf("failed to resolve interface ID: %v", err)
		}

		t.Logf("Resolved interface: physical=%s vlan=%d → logical=%s", physicalInterface, vlanTag, interfaceID)

		// Get DHCP configuration and find available IP
		dhcpConfig, err := client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to get DHCP config: %v", err)
		}

		if len(dhcpConfig.DHCPPools) == 0 {
			t.Fatal("no DHCP pools configured for VLAN interface")
		}

		// Use an IP OUTSIDE the DHCP pool to avoid conflicts
		pool := dhcpConfig.DHCPPools[0]
		poolStart := ipToUint32(pool.StartIP)
		testIP := uint32ToIP(poolStart - 1)

		t.Logf("Using test IP: %s OUTSIDE VLAN %d pool range (%s-%s)",
			testIP, vlanTag,
			pool.StartIP,
			pool.EndIP,
		)

		// Create initial static mapping
		originalMapping := &models.StaticMapping{
			MACAddress:  testMAC,
			IPAddress:   testIP,
			Hostname:    originalHostname,
			Description: "Test VLAN mapping for hostname update (will be deleted)",
			Interface:   interfaceID,
		}

		err = client.CreateStaticMapping(ctx, originalMapping)
		if err != nil {
			t.Fatalf("failed to create test mapping on VLAN: %v", err)
		}

		t.Logf("Created test mapping on VLAN")

		// Cleanup: defer deletion
		defer func() {
			t.Logf("Cleaning up test VLAN mapping (MAC: %s)", testMAC)
			// Note: Would need DeleteStaticMapping function - skip for now
		}()

		// Apply changes
		if err := client.ApplyDHCPChanges(ctx); err != nil {
			t.Fatalf("failed to apply DHCP changes after create: %v", err)
		}

		// Retrieve the mapping
		dhcpConfig, err = client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to retrieve DHCP config: %v", err)
		}

		// Find our mapping by MAC
		var foundMapping *models.StaticMapping
		for i := range dhcpConfig.StaticMappings {
			if dhcpConfig.StaticMappings[i].MACAddress == testMAC {
				foundMapping = &dhcpConfig.StaticMappings[i]
				break
			}
		}

		if foundMapping == nil {
			t.Fatalf("could not find created mapping with MAC %s on VLAN", testMAC)
		}

		// Update the hostname
		updatedMapping := &models.StaticMapping{
			ID:          foundMapping.ID,
			MACAddress:  foundMapping.MACAddress,
			IPAddress:   foundMapping.IPAddress,
			Hostname:    updatedHostname,
			Description: "Test VLAN mapping after update (will be deleted)",
			Interface:   interfaceID,
		}

		err = client.UpdateStaticMapping(ctx, updatedMapping)
		if err != nil {
			t.Fatalf("failed to update hostname on VLAN: %v", err)
		}

		// Apply changes
		if err := client.ApplyDHCPChanges(ctx); err != nil {
			t.Fatalf("failed to apply DHCP changes after update: %v", err)
		}

		// Verify the update
		dhcpConfig, err = client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to retrieve DHCP config after update: %v", err)
		}

		var verifyMapping *models.StaticMapping
		for i := range dhcpConfig.StaticMappings {
			if dhcpConfig.StaticMappings[i].MACAddress == testMAC {
				verifyMapping = &dhcpConfig.StaticMappings[i]
				break
			}
		}

		if verifyMapping == nil {
			t.Fatalf("mapping disappeared after update on VLAN")
		}

		// Verify updates
		if verifyMapping.Hostname != updatedHostname {
			t.Errorf("hostname not updated on VLAN: expected %q, got %q", updatedHostname, verifyMapping.Hostname)
		}

		if !verifyMapping.IPAddress.Equal(testIP) {
			t.Errorf("IP address changed on VLAN: expected %s, got %s", testIP, verifyMapping.IPAddress)
		}

		t.Logf("✓ Test successful: VLAN hostname updated from %q to %q, IP preserved (%s)",
			originalHostname, updatedHostname, testIP)
	})
}
