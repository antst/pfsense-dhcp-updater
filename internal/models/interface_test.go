package models

import (
	"net"
	"testing"
)

// TestInterfaceValidation tests the Validate method for Interface.
func TestInterfaceValidation(t *testing.T) {
	tests := []struct {
		name     string
		iface    Interface
		wantErr  bool
		errorMsg string
	}{
		{
			name: "valid base interface",
			iface: Interface{
				ID:          "ID0",
				IPAddress:   net.ParseIP("192.168.1.1"),
				Subnet:      net.ParseIP("192.168.1.0"),
				SubnetMask:  24,
				VLANTag:     0,
				DHCPEnabled: true,
				Gateway:     net.ParseIP("192.168.1.1"),
			},
			wantErr: false,
		},
		{
			name: "valid VLAN interface",
			iface: Interface{
				ID:          "ID0.10",
				IPAddress:   net.ParseIP("10.0.10.1"),
				Subnet:      net.ParseIP("10.0.10.0"),
				SubnetMask:  24,
				VLANTag:     10,
				DHCPEnabled: true,
				Gateway:     net.ParseIP("10.0.10.1"),
			},
			wantErr: false,
		},
		{
			name: "DHCP disabled",
			iface: Interface{
				ID:          "ID0",
				IPAddress:   net.ParseIP("192.168.1.1"),
				Subnet:      net.ParseIP("192.168.1.0"),
				SubnetMask:  24,
				VLANTag:     0,
				DHCPEnabled: false,
				Gateway:     net.ParseIP("192.168.1.1"),
			},
			wantErr:  true,
			errorMsg: "DHCP server not enabled",
		},
		{
			name: "nil IP address",
			iface: Interface{
				ID:          "ID0",
				IPAddress:   nil,
				Subnet:      net.ParseIP("192.168.1.0"),
				SubnetMask:  24,
				VLANTag:     0,
				DHCPEnabled: true,
				Gateway:     net.ParseIP("192.168.1.1"),
			},
			wantErr:  true,
			errorMsg: "IP address is required",
		},
		{
			name: "invalid subnet mask",
			iface: Interface{
				ID:          "ID0",
				IPAddress:   net.ParseIP("192.168.1.1"),
				Subnet:      net.ParseIP("192.168.1.0"),
				SubnetMask:  33, // Invalid: must be 0-32
				VLANTag:     0,
				DHCPEnabled: true,
				Gateway:     net.ParseIP("192.168.1.1"),
			},
			wantErr:  true,
			errorMsg: "invalid subnet mask",
		},
		{
			name: "invalid VLAN tag",
			iface: Interface{
				ID:          "ID0.5000",
				IPAddress:   net.ParseIP("10.0.10.1"),
				Subnet:      net.ParseIP("10.0.10.0"),
				SubnetMask:  24,
				VLANTag:     5000, // Invalid: must be 0-4094
				DHCPEnabled: true,
				Gateway:     net.ParseIP("10.0.10.1"),
			},
			wantErr:  true,
			errorMsg: "invalid VLAN tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.iface.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Interface.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Expected to fail in Red phase - validation not implemented yet
		})
	}
}

// TestGetSubnetCIDR tests the GetSubnetCIDR method.
func TestGetSubnetCIDR(t *testing.T) {
	tests := []struct {
		name     string
		iface    Interface
		wantCIDR string
	}{
		{
			name: "class C network",
			iface: Interface{
				Subnet:     net.ParseIP("192.168.1.0"),
				SubnetMask: 24,
			},
			wantCIDR: "192.168.1.0/24",
		},
		{
			name: "class B network",
			iface: Interface{
				Subnet:     net.ParseIP("172.16.0.0"),
				SubnetMask: 16,
			},
			wantCIDR: "172.16.0.0/16",
		},
		{
			name: "small subnet",
			iface: Interface{
				Subnet:     net.ParseIP("10.0.10.0"),
				SubnetMask: 28,
			},
			wantCIDR: "10.0.10.0/28",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipNet := tt.iface.GetSubnetCIDR()
			if ipNet == nil {
				t.Error("GetSubnetCIDR() returned nil, expected IPNet")
				return
			}
			if ipNet.String() != tt.wantCIDR {
				t.Errorf("GetSubnetCIDR() = %v, want %v", ipNet.String(), tt.wantCIDR)
			}
			// Expected to fail in Red phase - method not implemented yet
		})
	}
}
