package models

import (
	"net"
	"testing"
)

func TestParseIPRange(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart string
		wantEnd   string
		wantErr   bool
	}{
		{
			name:      "valid range",
			input:     "192.168.1.10-192.168.1.20",
			wantStart: "192.168.1.10",
			wantEnd:   "192.168.1.20",
			wantErr:   false,
		},
		{
			name:      "single IP",
			input:     "192.168.1.10",
			wantStart: "192.168.1.10",
			wantEnd:   "192.168.1.10",
			wantErr:   false,
		},
		{
			name:      "range with spaces",
			input:     "192.168.1.10 - 192.168.1.20",
			wantStart: "192.168.1.10",
			wantEnd:   "192.168.1.20",
			wantErr:   false,
		},
		{
			name:    "invalid start IP",
			input:   "invalid-192.168.1.20",
			wantErr: true,
		},
		{
			name:    "invalid end IP",
			input:   "192.168.1.10-invalid",
			wantErr: true,
		},
		{
			name:    "start > end",
			input:   "192.168.1.20-192.168.1.10",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too many parts",
			input:   "192.168.1.10-192.168.1.20-192.168.1.30",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startIP, endIP, err := ParseIPRange(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseIPRange() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseIPRange() unexpected error: %v", err)
				return
			}

			if startIP.String() != tt.wantStart {
				t.Errorf("ParseIPRange() startIP = %v, want %v", startIP, tt.wantStart)
			}

			if endIP.String() != tt.wantEnd {
				t.Errorf("ParseIPRange() endIP = %v, want %v", endIP, tt.wantEnd)
			}

			// Verify IPv4
			if startIP.To4() == nil {
				t.Errorf("ParseIPRange() startIP is not IPv4")
			}
			if endIP.To4() == nil {
				t.Errorf("ParseIPRange() endIP is not IPv4")
			}
		})
	}
}

func TestParseIPRange_RealWorldExamples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		count int // Expected number of IPs in range
	}{
		{
			name:  "config.test.yaml VLAN 0",
			input: "192.168.2.181-192.168.2.220",
			count: 40,
		},
		{
			name:  "small range",
			input: "10.0.0.1-10.0.0.10",
			count: 10,
		},
		{
			name:  "single IP",
			input: "172.16.0.50",
			count: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startIP, endIP, err := ParseIPRange(tt.input)
			if err != nil {
				t.Fatalf("ParseIPRange() error = %v", err)
			}

			// Count IPs in range
			count := 0
			for ip := startIP; ipLessThanOrEqual(ip, endIP); ip = nextIP(ip) {
				count++
			}

			if count != tt.count {
				t.Errorf("ParseIPRange() IP count = %v, want %v", count, tt.count)
			}
		})
	}
}

// Helper function to check if IP1 <= IP2
func ipLessThanOrEqual(ip1, ip2 net.IP) bool {
	for i := 0; i < 4; i++ {
		if ip1[i] < ip2[i] {
			return true
		}
		if ip1[i] > ip2[i] {
			return false
		}
	}
	return true // Equal
}

// Helper function to get next IP
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}
