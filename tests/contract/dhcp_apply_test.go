package contract

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
)

func TestApplyDHCPChanges(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "successful apply",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code":        200,
				"status":      "ok",
				"response_id": "SUCCESS",
				"message":     "",
				"data": map[string]interface{}{
					"applied": false,
				},
			},
			wantErr: false,
		},
		{
			name:       "already applied (no pending changes)",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code":        200,
				"status":      "ok",
				"response_id": "SUCCESS",
				"message":     "",
				"data": map[string]interface{}{
					"applied": true,
				},
			},
			wantErr: false,
		},
		{
			name:       "API error",
			statusCode: http.StatusBadRequest,
			response: map[string]interface{}{
				"code":        400,
				"status":      "error",
				"response_id": "ERROR",
				"message":     "Invalid request",
				"data":        []interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				if r.URL.Path != "/api/v2/services/dhcp_server/apply" {
					t.Errorf("Expected /api/v2/services/dhcp_server/apply, got %s", r.URL.Path)
				}

				// Check API key header
				apiKey := r.Header.Get("X-API-Key")
				if apiKey != "test-key" {
					t.Errorf("Expected X-API-Key header, got %s", apiKey)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-key",
				Timeout:            5 * time.Second,
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
			})

			ctx := context.Background()
			err := client.ApplyDHCPChanges(ctx)

			if tt.wantErr && err == nil {
				t.Errorf("ApplyDHCPChanges() expected error but got none")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("ApplyDHCPChanges() unexpected error: %v", err)
			}
		})
	}
}

func TestCheckPendingDHCPChanges(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		response    map[string]interface{}
		wantApplied bool
		wantErr     bool
	}{
		{
			name:       "no pending changes",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code":        200,
				"status":      "ok",
				"response_id": "SUCCESS",
				"message":     "",
				"data": map[string]interface{}{
					"applied": true,
				},
			},
			wantApplied: true,
			wantErr:     false,
		},
		{
			name:       "pending changes exist",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code":        200,
				"status":      "ok",
				"response_id": "SUCCESS",
				"message":     "",
				"data": map[string]interface{}{
					"applied": false,
				},
			},
			wantApplied: false,
			wantErr:     false,
		},
		{
			name:       "API error",
			statusCode: http.StatusInternalServerError,
			response: map[string]interface{}{
				"code":        500,
				"status":      "error",
				"response_id": "ERROR",
				"message":     "Internal error",
				"data":        map[string]interface{}{},
			},
			wantApplied: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				if r.URL.Path != "/api/v2/services/dhcp_server/apply" {
					t.Errorf("Expected /api/v2/services/dhcp_server/apply, got %s", r.URL.Path)
				}

				// Check API key header
				apiKey := r.Header.Get("X-API-Key")
				if apiKey != "test-key" {
					t.Errorf("Expected X-API-Key header, got %s", apiKey)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			client := pfsense.NewClient(pfsense.ClientConfig{
				BaseURL:            server.URL,
				APIKey:             "test-key",
				Timeout:            5 * time.Second,
				InsecureSkipVerify: true,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
			})

			ctx := context.Background()
			applied, err := client.CheckPendingDHCPChanges(ctx)

			if tt.wantErr && err == nil {
				t.Errorf("CheckPendingDHCPChanges() expected error but got none")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("CheckPendingDHCPChanges() unexpected error: %v", err)
			}

			if !tt.wantErr && applied != tt.wantApplied {
				t.Errorf("CheckPendingDHCPChanges() applied = %v, want %v", applied, tt.wantApplied)
			}
		})
	}
}
