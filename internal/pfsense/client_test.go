package pfsense

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient(ClientConfig{
		BaseURL: "https://pfsense.example.com",
		APIKey:  "test-key",
	})

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.baseURL == "" {
		t.Error("Expected baseURL to be set")
	}
}

func TestClient_Get(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		response    string
		wantErr     bool
		errContains string
	}{
		{"success", http.StatusOK, `{"status": "ok"}`, false, ""},
		{"not found", http.StatusNotFound, `{"error": "not found"}`, true, "404"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") == "" {
					t.Error("Expected X-API-Key header")
				}
				if r.Method == http.MethodGet && r.Header.Get("Content-Type") != "" {
					t.Error("GET should not have Content-Type")
				}
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.response)); err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			}))
			defer server.Close()

			client := NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test",
				Logger:  slog.Default(),
			})

			_, err := client.Get(context.Background(), "/test")

			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Error should contain %q", tt.errContains)
			}
		})
	}
}

func TestClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json")
		}
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{"status": "created"}`)); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test",
	})

	_, err := client.Post(context.Background(), "/test", []byte(`{"data": "test"}`))
	if err != nil {
		t.Errorf("Post() unexpected error: %v", err)
	}
}

func TestClient_RetryLogic(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"status": "ok"}`)); err != nil {
				t.Errorf("Failed to write response: %v", err)
			}
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test",
		Timeout: 5 * time.Second,
	})

	_, err := client.Get(context.Background(), "/test")
	if err != nil {
		t.Errorf("Get() should succeed after retries: %v", err)
	}
	if attempts < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", attempts)
	}
}

func TestClient_BaseURLTrimming(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/", "https://example.com"},
		{"https://example.com", "https://example.com"},
	}

	for _, tt := range tests {
		client := NewClient(ClientConfig{
			BaseURL: tt.input,
			APIKey:  "test",
		})
		if client.baseURL != tt.want {
			t.Errorf("baseURL = %q, want %q", client.baseURL, tt.want)
		}
	}
}
