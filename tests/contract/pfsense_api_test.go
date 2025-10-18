package contract

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
)

// TestGetDHCPConfig tests the GET /services/dhcpd API endpoint.
func TestGetDHCPConfig(t *testing.T) {
	tests := []struct {
		name           string
		interfaceID    string
		responseStatus int
		responseBody   map[string]interface{}
		wantErr        bool
	}{
		{
			name:           "successful retrieval - base interface",
			interfaceID:    "ID0",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"interface":   "ID0",
				"enable":      true,
				"gateway":     "192.168.1.1",
				"subnet":      "192.168.1.0",
				"subnet_bits": 24,
				"range": map[string]interface{}{
					"from": "192.168.1.100",
					"to":   "192.168.1.200",
				},
				"staticmap": []map[string]interface{}{
					{
						"mac":      "00:11:22:33:44:55",
						"ipaddr":   "192.168.1.50",
						"hostname": "existing-vm",
						"descr":    "Existing mapping",
					},
				},
				"pool": []map[string]interface{}{
					{
						"start": "192.168.1.100",
						"end":   "192.168.1.200",
					},
				},
			},
			wantErr: false,
		},
		{
			name:           "successful retrieval - VLAN interface",
			interfaceID:    "ID0.10",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"interface":   "ID0.10",
				"enable":      true,
				"gateway":     "10.0.10.1",
				"subnet":      "10.0.10.0",
				"subnet_bits": 24,
				"range": map[string]interface{}{
					"from": "10.0.10.100",
					"to":   "10.0.10.200",
				},
				"staticmap": []map[string]interface{}{},
				"pool":      []map[string]interface{}{},
			},
			wantErr: false,
		},
		{
			name:           "interface not found",
			interfaceID:    "ID0.999",
			responseStatus: http.StatusNotFound,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "Interface not found",
				"code":    404,
			},
			wantErr: true,
		},
		{
			name:           "authentication failure",
			interfaceID:    "ID0",
			responseStatus: http.StatusUnauthorized,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "Unauthorized",
				"code":    401,
			},
			wantErr: true,
		},
		{
			name:           "DHCP server disabled",
			interfaceID:    "ID0",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"interface":   "ID0",
				"enable":      false, // DHCP disabled
				"subnet":      "192.168.1.0",
				"subnet_bits": 24,
			},
			wantErr: false, // API succeeds, but application should detect disabled DHCP
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				// Verify API key header
				apiKey := r.Header.Get("X-API-Key")
				if apiKey != "test-api-key" {
					t.Errorf("Expected API key header, got %s", apiKey)
				}

				// Verify query parameter
				interfaceParam := r.URL.Query().Get("interface")
				if interfaceParam != tt.interfaceID {
					t.Errorf("Expected interface=%s, got %s", tt.interfaceID, interfaceParam)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create client
			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-api-key",
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
			})

			// Make request
			_, err := client.Get(context.Background(), "/services/dhcpd?interface="+tt.interfaceID)

			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Expected to pass with mock server (tests API client contract)
		})
	}
}

// TestCreateStaticMapping tests the POST /services/dhcpd/static_mapping API endpoint.
func TestCreateStaticMapping(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		responseStatus int
		responseBody   map[string]interface{}
		wantErr        bool
	}{
		{
			name: "successful creation",
			requestBody: map[string]interface{}{
				"interface": "ID0.10",
				"mac":       "11:22:33:44:55:66",
				"ipaddr":    "10.0.10.51",
				"hostname":  "new-vm",
				"descr":     "Auto-created by pfsense-dhcp-updater",
			},
			responseStatus: http.StatusCreated,
			responseBody: map[string]interface{}{
				"id":      "abc123def456",
				"status":  "success",
				"message": "Static mapping created",
			},
			wantErr: false,
		},
		{
			name: "duplicate MAC address",
			requestBody: map[string]interface{}{
				"interface": "ID0.10",
				"mac":       "00:11:22:33:44:55", // Already exists
				"ipaddr":    "10.0.10.52",
				"hostname":  "duplicate-vm",
				"descr":     "Test",
			},
			responseStatus: http.StatusBadRequest,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "MAC address already exists in static mappings",
				"code":    400,
			},
			wantErr: true,
		},
		{
			name: "invalid MAC address format",
			requestBody: map[string]interface{}{
				"interface": "ID0.10",
				"mac":       "invalid-mac",
				"ipaddr":    "10.0.10.53",
				"hostname":  "test-vm",
				"descr":     "Test",
			},
			responseStatus: http.StatusBadRequest,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "Invalid MAC address format",
				"code":    400,
			},
			wantErr: true,
		},
		{
			name: "IP address conflict",
			requestBody: map[string]interface{}{
				"interface": "ID0.10",
				"mac":       "aa:bb:cc:dd:ee:ff",
				"ipaddr":    "10.0.10.50", // Already assigned
				"hostname":  "conflict-vm",
				"descr":     "Test",
			},
			responseStatus: http.StatusBadRequest,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "IP address already in use",
				"code":    400,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				// Verify content type
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", contentType)
				}

				// Parse request body
				var reqBody map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create client
			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-api-key",
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
			})

			// Make request
			reqJSON, _ := json.Marshal(tt.requestBody)
			_, err := client.Post(context.Background(), "/services/dhcpd/static_mapping", reqJSON)

			if (err != nil) != tt.wantErr {
				t.Errorf("Post() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Expected to pass with mock server (tests API client contract)
		})
	}
}

// TestUpdateStaticMapping tests the PATCH /services/dhcpd/static_mapping/{id} API endpoint.
func TestUpdateStaticMapping(t *testing.T) {
	tests := []struct {
		name           string
		mappingID      string
		requestBody    map[string]interface{}
		responseStatus int
		responseBody   map[string]interface{}
		wantErr        bool
	}{
		{
			name:      "successful hostname update",
			mappingID: "abc123def456",
			requestBody: map[string]interface{}{
				"hostname": "updated-hostname",
				"descr":    "Updated by pfsense-dhcp-updater",
			},
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"id":      "abc123def456",
				"status":  "success",
				"message": "Static mapping updated",
			},
			wantErr: false,
		},
		{
			name:      "mapping not found",
			mappingID: "nonexistent",
			requestBody: map[string]interface{}{
				"hostname": "updated-hostname",
			},
			responseStatus: http.StatusNotFound,
			responseBody: map[string]interface{}{
				"status":  "error",
				"message": "Static mapping not found",
				"code":    404,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				if r.Method != http.MethodPatch {
					t.Errorf("Expected PATCH request, got %s", r.Method)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create client
			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-api-key",
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
			})

			// Make request
			reqJSON, _ := json.Marshal(tt.requestBody)
			_, err := client.Patch(context.Background(), "/services/dhcpd/static_mapping/"+tt.mappingID, reqJSON)

			if (err != nil) != tt.wantErr {
				t.Errorf("Patch() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Expected to pass with mock server (tests API client contract)
		})
	}
}

// TestGetDHCPLeases tests the GET /status/dhcp/leases API endpoint.
func TestGetDHCPLeases(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   map[string]interface{}
		wantLeaseCount int
		wantErr        bool
	}{
		{
			name:           "successful retrieval with multiple leases",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"status":      "ok",
				"code":        200,
				"response_id": "DHCP_LEASES_GET",
				"data": []map[string]interface{}{
					{
						"mac":      "00:11:22:33:44:55",
						"ip":       "192.168.1.100",
						"hostname": "laptop-01",
						"start":    "2025-10-17 10:00:00",
						"end":      "2025-10-17 18:00:00",
					},
					{
						"mac":      "aa:bb:cc:dd:ee:ff",
						"ip":       "192.168.1.101",
						"hostname": "phone-01",
						"start":    "2025-10-17 11:00:00",
						"end":      "2025-10-17 19:00:00",
					},
					{
						"mac":      "11:22:33:44:55:66",
						"ipaddr":   "192.168.1.102", // Test alternate field name
						"hostname": "tablet-01",
					},
				},
			},
			wantLeaseCount: 3,
			wantErr:        false,
		},
		{
			name:           "empty leases",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"status":      "ok",
				"code":        200,
				"response_id": "DHCP_LEASES_GET",
				"data":        []map[string]interface{}{},
			},
			wantLeaseCount: 0,
			wantErr:        false,
		},
		{
			name:           "API error",
			responseStatus: http.StatusInternalServerError,
			responseBody: map[string]interface{}{
				"status":  "error",
				"code":    500,
				"message": "Internal server error",
			},
			wantLeaseCount: 0,
			wantErr:        true,
		},
		{
			name:           "authentication failure",
			responseStatus: http.StatusUnauthorized,
			responseBody: map[string]interface{}{
				"status":  "error",
				"code":    401,
				"message": "Unauthorized",
			},
			wantLeaseCount: 0,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				// Verify path
				expectedPath := "/api/v2/status/dhcp_server/leases"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(tt.responseBody); err != nil {
					t.Fatalf("Failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-api-key",
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			})

			// Make request
			leases, err := client.GetDHCPLeases(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetDHCPLeases() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(leases) != tt.wantLeaseCount {
				t.Errorf("GetDHCPLeases() returned %d leases, want %d", len(leases), tt.wantLeaseCount)
			}

			// Verify lease data structure for successful cases
			if !tt.wantErr && len(leases) > 0 {
				for i, lease := range leases {
					if lease.MACAddress == "" {
						t.Errorf("Lease %d missing MAC address", i)
					}
					if lease.IPAddress == nil {
						t.Errorf("Lease %d missing IP address", i)
					}
				}
			}
		})
	}
}

// TestDeleteStaticMapping tests the DELETE /services/dhcpd/static_mapping API endpoint.
// T133 [P] [US6] Write contract test for pfSense DELETE /services/dhcpd/static_mapping/{id} API in tests/contract/pfsense_api_test.go
func TestDeleteStaticMapping(t *testing.T) {
	tests := []struct {
		name           string
		interfaceID    string
		mappingID      string
		responseStatus int
		responseBody   map[string]interface{}
		wantErr        bool
	}{
		{
			name:           "successful deletion",
			interfaceID:    "lan",
			mappingID:      "1",
			responseStatus: http.StatusOK,
			responseBody: map[string]interface{}{
				"status":      "ok",
				"code":        200,
				"response_id": "test-delete-123",
				"message":     "Static mapping deleted successfully",
			},
			wantErr: false,
		},
		{
			name:           "mapping not found",
			interfaceID:    "lan",
			mappingID:      "999",
			responseStatus: http.StatusNotFound,
			responseBody: map[string]interface{}{
				"status":  "error",
				"code":    404,
				"message": "Static mapping not found",
			},
			wantErr: true,
		},
		{
			name:           "invalid interface",
			interfaceID:    "invalid",
			mappingID:      "1",
			responseStatus: http.StatusBadRequest,
			responseBody: map[string]interface{}{
				"status":  "error",
				"code":    400,
				"message": "Invalid interface",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify HTTP method
				if r.Method != http.MethodDelete {
					t.Errorf("Expected DELETE method, got %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				// Verify endpoint path
				expectedPath := "/api/v2/services/dhcp_server/static_mapping"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// Verify query parameters
				query := r.URL.Query()
				if query.Get("parent_id") != tt.interfaceID {
					t.Errorf("Expected parent_id=%s, got %s", tt.interfaceID, query.Get("parent_id"))
				}
				if query.Get("id") != tt.mappingID {
					t.Errorf("Expected id=%s, got %s", tt.mappingID, query.Get("id"))
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create a client
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-key",
				InsecureSkipVerify: true,
				Logger:             logger,
			})

			// Execute the delete
			ctx := context.Background()
			err := client.DeleteStaticMapping(ctx, tt.interfaceID, tt.mappingID)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("DeleteStaticMapping() expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("DeleteStaticMapping() unexpected error: %v", err)
				}
			}
		})
	}
}
