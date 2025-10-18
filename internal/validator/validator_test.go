package validator

import (
	"net"
	"testing"
)

func TestValidateMAC(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid colon", "AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff", false},
		{"valid hyphen", "AA-BB-CC-DD-EE-FF", "aa:bb:cc:dd:ee:ff", false},
		{"empty", "", "", true},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateMAC(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMAC() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateMAC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"simple", "webserver", "webserver", false},
		{"FQDN", "web.example.com", "web.example.com", false},
		{"uppercase", "WebServer", "webserver", false},
		{"empty", "", "", true},
		{"invalid", "-invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateHostname() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateVLAN(t *testing.T) {
	tests := []struct {
		input   int
		want    int
		wantErr bool
	}{
		{0, 0, false},
		{10, 10, false},
		{4094, 4094, false},
		{-1, 0, true},
		{4095, 0, true},
	}

	for _, tt := range tests {
		got, err := ValidateVLAN(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateVLAN(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ValidateVLAN(%d) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"192.168.1.1", "192.168.1.1", false},
		{"10.0.0.1", "10.0.0.1", false},
		{"", "", true},
		{"invalid", "", true},
		{"2001:db8::1", "", true},
	}

	for _, tt := range tests {
		got, err := ValidateIP(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateIP(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if !tt.wantErr && got.String() != tt.want {
			t.Errorf("ValidateIP(%q) = %v, want %v", tt.input, got.String(), tt.want)
		}
	}
}

func TestValidateIPInSubnet(t *testing.T) {
	_, subnet, _ := net.ParseCIDR("192.168.1.0/24")

	tests := []struct {
		ip      string
		wantErr bool
	}{
		{"192.168.1.50", false},
		{"192.168.2.50", true},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		err := ValidateIPInSubnet(ip, subnet)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateIPInSubnet(%s) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
		}
	}
}
