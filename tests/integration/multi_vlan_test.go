package integration

import (
	"context"
	"os"
	"testing"

	"github.com/antst/pfsense-dhcp-updater/internal/config"
	"github.com/antst/pfsense-dhcp-updater/internal/logger"
	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
)

// TestMultiVLAN_BaseInterface tests interface resolution and DHCP config for base interface (VLAN 0).
func TestMultiVLAN_BaseInterface(t *testing.T) {
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

	t.Run("VLAN_0_base_interface", func(t *testing.T) {
		physicalInterface := "iavf1"
		vlanTag := 0

		interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
		if err != nil {
			t.Fatalf("failed to resolve interface ID: %v", err)
		}

		t.Logf("Resolved interface: physical=%s vlan=%d → logical=%s", physicalInterface, vlanTag, interfaceID)

		expectedID := "lan"
		if interfaceID != expectedID {
			t.Errorf("expected interface ID %q but got %q", expectedID, interfaceID)
		}

		dhcpConfig, err := client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to get DHCP config: %v", err)
		}

		t.Logf("DHCP config: interface=%s subnet=%s/%d pools=%d",
			dhcpConfig.ID,
			dhcpConfig.Subnet,
			dhcpConfig.SubnetMask,
			len(dhcpConfig.DHCPPools),
		)

		if !dhcpConfig.DHCPEnabled {
			t.Errorf("DHCP is not enabled on interface %s", interfaceID)
		}

		if len(dhcpConfig.DHCPPools) == 0 {
			t.Errorf("No DHCP pools configured for interface %s", interfaceID)
		} else {
			for i, pool := range dhcpConfig.DHCPPools {
				t.Logf("  Pool %d: %s - %s", i, pool.StartIP, pool.EndIP)
			}
		}

		leases, err := client.GetDHCPLeases(ctx)
		if err != nil {
			t.Fatalf("failed to get DHCP leases: %v", err)
		}

		t.Logf("Retrieved %d DHCP leases", len(leases))
		t.Logf("✓ Test successful: interface resolution and DHCP config validated")
	})
}

// TestMultiVLAN_TaggedInterface tests interface resolution for VLAN 101.
func TestMultiVLAN_TaggedInterface(t *testing.T) {
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

	t.Run("VLAN_101_tagged_interface", func(t *testing.T) {
		physicalInterface := "iavf1"
		vlanTag := 101

		interfaceID, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)
		if err != nil {
			t.Fatalf("failed to resolve interface ID: %v", err)
		}

		t.Logf("Resolved interface: physical=%s vlan=%d → logical=%s", physicalInterface, vlanTag, interfaceID)

		if interfaceID == "" {
			t.Errorf("interface ID is empty")
		}

		dhcpConfig, err := client.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			t.Fatalf("failed to get DHCP config: %v", err)
		}

		t.Logf("DHCP config: interface=%s subnet=%s/%d pools=%d",
			dhcpConfig.ID,
			dhcpConfig.Subnet,
			dhcpConfig.SubnetMask,
			len(dhcpConfig.DHCPPools),
		)

		if !dhcpConfig.DHCPEnabled {
			t.Errorf("DHCP is not enabled on interface %s", interfaceID)
		}

		if len(dhcpConfig.DHCPPools) == 0 {
			t.Errorf("No DHCP pools configured for interface %s", interfaceID)
		} else {
			for i, pool := range dhcpConfig.DHCPPools {
				t.Logf("  Pool %d: %s - %s", i, pool.StartIP, pool.EndIP)
			}
		}

		t.Logf("✓ Test successful: interface resolution for VLAN 101")
	})
} // TestMultiVLAN_NetmaskBasedAssignment checks for interfaces without explicit DHCP pools.
func TestMultiVLAN_NetmaskBasedAssignment(t *testing.T) {
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

	t.Run("check_netmask_based_interfaces", func(t *testing.T) {
		interfaces, err := client.GetInterfaces(ctx)
		if err != nil {
			t.Fatalf("failed to get interfaces: %v", err)
		}

		t.Logf("Found %d interfaces", len(interfaces))

		foundNetmaskBased := false
		for physIface, logicalID := range interfaces {
			dhcpConfig, err := client.GetDHCPConfig(ctx, logicalID)
			if err != nil {
				continue
			}

			if len(dhcpConfig.DHCPPools) == 0 {
				t.Logf("Interface %s (%s) has no explicit DHCP pools: subnet=%s/%d",
					physIface, logicalID, dhcpConfig.Subnet, dhcpConfig.SubnetMask)
				foundNetmaskBased = true
			}
		}

		if !foundNetmaskBased {
			t.Logf("All interfaces have explicit DHCP pools configured")
		}

		t.Logf("✓ Test successful: DHCP pool configuration checked")
	})
}

// TestMultiVLAN_NonExistentInterface tests error handling for non-existent VLAN.
func TestMultiVLAN_NonExistentInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := config.Load("../../config.test.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	log := logger.New(logger.LevelError, false, os.Stderr)

	client := pfsense.NewClient(pfsense.ClientConfig{
		BaseURL:            cfg.PfSense.Endpoint,
		APIKey:             cfg.PfSense.APIKey,
		InsecureSkipVerify: cfg.PfSense.InsecureSkipVerify,
		Logger:             log,
	})

	ctx := context.Background()

	t.Run("non_existent_VLAN", func(t *testing.T) {
		physicalInterface := "iavf1"
		vlanTag := 9999

		_, err := client.ResolveInterfaceID(ctx, physicalInterface, vlanTag, nil)

		if err == nil {
			t.Errorf("expected error for non-existent VLAN %d but got none", vlanTag)
			return
		}

		t.Logf("Got expected error: %v", err)

		expectedPhysical := "iavf1.9999"
		if !contains(err.Error(), expectedPhysical) && !contains(err.Error(), "not found") {
			t.Errorf("error message should mention interface %q or 'not found', got: %v", expectedPhysical, err)
		}

		t.Logf("✓ Test successful: proper error handling for non-existent VLAN")
	})
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
