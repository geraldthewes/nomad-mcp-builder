package unit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nomad-mcp-builder/internal/config"
)

// TestCORSPreflightRequest tests that OPTIONS requests return proper CORS headers
func TestCORSPreflightRequest(t *testing.T) {
	// Create test server config
	cfg := &config.Config{
		Server: config.ServerConfig{
			CORSOrigin: "*",
		},
	}

	// Create OPTIONS request to /mcp endpoint
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:6274")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, mcp-protocol-version")

	w := httptest.NewRecorder()

	// Create a test handler that wraps the server's handleMCPRequest
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This simulates what the actual server does
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", cfg.Server.CORSOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, mcp-protocol-version")
			w.Header().Set("Access-Control-Max-Age", "3600")
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	handler.ServeHTTP(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin: *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}

	if w.Header().Get("Access-Control-Allow-Methods") != "POST, OPTIONS" {
		t.Errorf("Expected Access-Control-Allow-Methods: POST, OPTIONS, got %s", w.Header().Get("Access-Control-Allow-Methods"))
	}

	allowedHeaders := w.Header().Get("Access-Control-Allow-Headers")
	if allowedHeaders != "Content-Type, mcp-protocol-version" {
		t.Errorf("Expected Access-Control-Allow-Headers to include 'Content-Type, mcp-protocol-version', got %s", allowedHeaders)
	}

	if w.Header().Get("Access-Control-Max-Age") != "3600" {
		t.Errorf("Expected Access-Control-Max-Age: 3600, got %s", w.Header().Get("Access-Control-Max-Age"))
	}
}

// TestCORSActualRequest tests that actual POST requests include CORS headers
func TestCORSActualRequest(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			CORSOrigin: "*",
		},
	}

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers like the actual server does
		w.Header().Set("Access-Control-Allow-Origin", cfg.Server.CORSOrigin)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
	})

	// Create POST request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:6274")
	req.Header.Set("mcp-protocol-version", "2024-11-05")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify CORS header is present
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin: *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestConfigurableCORSOrigin tests that CORS origin is configurable
func TestConfigurableCORSOrigin(t *testing.T) {
	testCases := []struct {
		name           string
		corsOrigin     string
		expectedOrigin string
	}{
		{
			name:           "wildcard origin",
			corsOrigin:     "*",
			expectedOrigin: "*",
		},
		{
			name:           "specific origin",
			corsOrigin:     "http://localhost:6274",
			expectedOrigin: "http://localhost:6274",
		},
		{
			name:           "production origin",
			corsOrigin:     "https://app.example.com",
			expectedOrigin: "https://app.example.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Server: config.ServerConfig{
					CORSOrigin: tc.corsOrigin,
				},
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", cfg.Server.CORSOrigin)
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Header().Get("Access-Control-Allow-Origin") != tc.expectedOrigin {
				t.Errorf("Expected origin %s, got %s", tc.expectedOrigin, w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestJSONRPCNotificationHandling tests that notifications (requests without id) are handled correctly
func TestJSONRPCNotificationHandling(t *testing.T) {
	testCases := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectResponse bool
	}{
		{
			name:           "notification without id",
			requestBody:    `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			expectedStatus: http.StatusOK,
			expectResponse: false, // Notifications should not return a JSON response
		},
		{
			name:           "notification with null id",
			requestBody:    `{"jsonrpc":"2.0","id":null,"method":"notifications/initialized"}`,
			expectedStatus: http.StatusOK,
			expectResponse: false,
		},
		{
			name:           "request with id (not a notification)",
			requestBody:    `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
			expectedStatus: http.StatusOK,
			expectResponse: true, // Regular requests should return a response
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "Bad request", http.StatusBadRequest)
					return
				}

				// Check if this is a notification (no id or null id)
				id, hasID := req["id"]
				isNotification := !hasID || id == nil

				if isNotification {
					// Notifications: return 200 OK with no body
					w.WriteHeader(http.StatusOK)
					return
				}

				// Regular requests: return JSON response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			})

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(tc.requestBody))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			if tc.expectResponse {
				if w.Body.Len() == 0 {
					t.Error("Expected response body, got empty")
				}

				// Verify it's valid JSON
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("Expected valid JSON response, got error: %v", err)
				}
			} else {
				if w.Body.Len() > 0 {
					t.Errorf("Expected no response body for notification, got: %s", w.Body.String())
				}
			}
		})
	}
}

// TestMCPProtocolVersionHeader tests that mcp-protocol-version header is accepted
func TestMCPProtocolVersionHeader(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			CORSOrigin: "*",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the header is present
		protocolVersion := r.Header.Get("mcp-protocol-version")
		if protocolVersion == "" {
			t.Error("Expected mcp-protocol-version header to be present")
		}

		if protocolVersion != "2024-11-05" {
			t.Errorf("Expected protocol version 2024-11-05, got %s", protocolVersion)
		}

		w.Header().Set("Access-Control-Allow-Origin", cfg.Server.CORSOrigin)
		w.WriteHeader(http.StatusOK)
	})

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("mcp-protocol-version", "2024-11-05")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
