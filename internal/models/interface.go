// Package models provides domain entities for DHCP static mappings.
package models

import (
	"fmt"
	"net"
	"time"
)

// Interface represents a pfSense network interface with DHCP server configuration.
type Interface struct {
	ID             string          // e.g., "ID0", "ID0.10"
	IPAddress      net.IP          // Interface's IP address
	Subnet         net.IP          // Network address
	SubnetMask     int             // CIDR notation (e.g., 24 for /24)
	VLANTag        int             // 0 for base interface, >0 for VLAN
	DHCPEnabled    bool            // Whether DHCP server is active
	Gateway        net.IP          // Gateway IP
	StaticMappings []StaticMapping // Existing static mappings
	DynamicLeases  []DynamicLease  // Active DHCP leases
	DHCPPools      []DHCPPool      // DHCP pool ranges
}

// DynamicLease represents an active DHCP lease.
type DynamicLease struct {
	MACAddress string
	IPAddress  net.IP
	Hostname   string
	Expires    time.Time
}

// DHCPPool represents a DHCP pool range.
type DHCPPool struct {
	StartIP net.IP
	EndIP   net.IP
}

// Validate checks if the interface configuration is valid.
func (i *Interface) Validate() error {
	// Validate DHCP enabled
	if !i.DHCPEnabled {
		return fmt.Errorf("DHCP server not enabled on interface %s", i.ID)
	}

	// Validate IP address
	if i.IPAddress == nil {
		return fmt.Errorf("IP address is required")
	}
	if i.IPAddress.To4() == nil {
		return fmt.Errorf("only IPv4 addresses are supported")
	}

	// Validate subnet mask
	if i.SubnetMask < 0 || i.SubnetMask > 32 {
		return fmt.Errorf("invalid subnet mask: must be 0-32")
	}

	// Validate VLAN tag
	if i.VLANTag < 0 || i.VLANTag > 4094 {
		return fmt.Errorf("invalid VLAN tag: must be 0-4094")
	}

	return nil
}

// GetSubnetCIDR returns the subnet in CIDR notation.
func (i *Interface) GetSubnetCIDR() *net.IPNet {
	if i.Subnet == nil || i.SubnetMask == 0 {
		return nil
	}

	// Create CIDR from subnet and mask
	mask := net.CIDRMask(i.SubnetMask, 32)
	return &net.IPNet{
		IP:   i.Subnet.To4(),
		Mask: mask,
	}
}
