package integration

import (
	"context"
	"os"
	"testing"

	"github.com/antst/pfsense-dhcp-updater/internal/config"
	"github.com/antst/pfsense-dhcp-updater/internal/logger"
	"github.com/antst/pfsense-dhcp-updater/internal/models"
	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
)

// TestDeleteMappingByMAC tests deletion of a static mapping by MAC address on base interface.
// T134 [P] [US6] Write integration test for deletion by MAC on base interface in tests/integration/delete_mapping_test.go
func TestDeleteMappingByMAC(t *testing.T) {
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

	// Create a test mapping first
	testMAC := "00:11:22:33:44:99"
	testHostname := "delete-test-by-mac"
	physicalInterface := "iavf1"
	vlanTag := 0

	// Resolve interface ID
	interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
	if err != nil {
		t.Fatalf("Failed to resolve interface ID: %v", err)
	}

	t.Logf("Resolved interface: %s", interfaceID)

	// Get interface config to find available IP
	iface, err := client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config: %v", err)
	}

	// Calculate available IPs
	pool, err := models.CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("Failed to calculate available IPs: %v", err)
	}

	if len(pool.AvailableIPs) == 0 {
		t.Fatal("No available IPs in pool")
	}

	testIP := pool.AvailableIPs[0]

	// Create the mapping
	mapping := &models.StaticMapping{
		Interface:   interfaceID,
		MACAddress:  testMAC,
		IPAddress:   testIP,
		Hostname:    testHostname,
		Description: "Integration test - delete by MAC",
	}

	if err := client.CreateStaticMapping(ctx, mapping); err != nil {
		t.Fatalf("Failed to create test mapping: %v", err)
	}

	// Apply changes
	if err := client.ApplyDHCPChanges(ctx); err != nil {
		t.Fatalf("Failed to apply DHCP changes after create: %v", err)
	}

	t.Logf("Created test mapping: MAC=%s, IP=%s", testMAC, testIP)

	// Verify mapping was created
	iface, err = client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config after create: %v", err)
	}

	found := false
	var mappingID string
	for _, m := range iface.StaticMappings {
		if m.MACAddress == testMAC {
			found = true
			mappingID = m.ID
			break
		}
	}

	if !found {
		t.Fatal("Mapping was not created")
	}

	t.Logf("Found mapping with ID: %s", mappingID)

	// Now test deletion by MAC
	if err := client.DeleteStaticMapping(ctx, interfaceID, mappingID); err != nil {
		t.Fatalf("Failed to delete mapping by MAC: %v", err)
	}

	// Apply changes
	if err := client.ApplyDHCPChanges(ctx); err != nil {
		t.Fatalf("Failed to apply DHCP changes after delete: %v", err)
	}

	t.Logf("Deleted mapping")

	// Verify mapping was deleted
	iface, err = client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config after delete: %v", err)
	}

	for _, m := range iface.StaticMappings {
		if m.MACAddress == testMAC {
			t.Errorf("Mapping still exists after deletion: %+v", m)
		}
	}

	t.Log("Test completed successfully - mapping was deleted")
}

// TestDeleteMappingByHostname tests deletion of a static mapping by hostname on VLAN.
// T135 [P] [US6] Write integration test for deletion by hostname on VLAN in tests/integration/delete_mapping_test.go
func TestDeleteMappingByHostname(t *testing.T) {
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

	// Use VLAN interface
	physicalInterface := "iavf1"
	vlanTag := 101
	testMAC := "00:11:22:33:44:88"
	testHostname := "delete-test-by-hostname"

	// Resolve interface ID
	interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
	if err != nil {
		t.Fatalf("Failed to resolve interface ID: %v", err)
	}

	t.Logf("Resolved interface: %s", interfaceID)

	// Get interface config
	iface, err := client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config: %v", err)
	}

	// Calculate available IPs
	pool, err := models.CalculateIPPool(iface, nil)
	if err != nil {
		t.Fatalf("Failed to calculate available IPs: %v", err)
	}

	if len(pool.AvailableIPs) == 0 {
		t.Fatal("No available IPs in pool")
	}

	testIP := pool.AvailableIPs[0]

	// Create the mapping
	mapping := &models.StaticMapping{
		Interface:   interfaceID,
		MACAddress:  testMAC,
		IPAddress:   testIP,
		Hostname:    testHostname,
		Description: "Integration test - delete by hostname",
	}

	if err := client.CreateStaticMapping(ctx, mapping); err != nil {
		t.Fatalf("Failed to create test mapping: %v", err)
	}

	// Apply changes
	if err := client.ApplyDHCPChanges(ctx); err != nil {
		t.Fatalf("Failed to apply DHCP changes after create: %v", err)
	}

	t.Logf("Created test mapping: hostname=%s, IP=%s", testHostname, testIP)

	// Get mapping ID by hostname
	iface, err = client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config after create: %v", err)
	}

	found := false
	var mappingID string
	for _, m := range iface.StaticMappings {
		if pfsense.NormalizeHostname(m.Hostname) == pfsense.NormalizeHostname(testHostname) {
			found = true
			mappingID = m.ID
			break
		}
	}

	if !found {
		t.Fatal("Mapping was not created")
	}

	t.Logf("Found mapping with ID: %s", mappingID)

	// Now test deletion by hostname
	if err := client.DeleteStaticMapping(ctx, interfaceID, mappingID); err != nil {
		t.Fatalf("Failed to delete mapping by hostname: %v", err)
	}

	// Apply changes
	if err := client.ApplyDHCPChanges(ctx); err != nil {
		t.Fatalf("Failed to apply DHCP changes after delete: %v", err)
	}

	t.Logf("Deleted mapping")

	// Verify mapping was deleted
	iface, err = client.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		t.Fatalf("Failed to get DHCP config after delete: %v", err)
	}

	for _, m := range iface.StaticMappings {
		if pfsense.NormalizeHostname(m.Hostname) == pfsense.NormalizeHostname(testHostname) {
			t.Errorf("Mapping still exists after deletion: %+v", m)
		}
	}

	t.Log("Test completed successfully - mapping was deleted")
}
