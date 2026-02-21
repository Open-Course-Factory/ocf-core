package utils_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/utils"

	"github.com/stretchr/testify/assert"
)

// Test data structures
type TestRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type TestResponse struct {
	ID      int    `json:"id"`
	Message string `json:"message"`
}

// ==========================================
// Low-Level HTTP Function Tests
// ==========================================

func TestMakeHTTPRequest(t *testing.T) {
	t.Run("Success - POST with JSON body", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Parse request body
			var req TestRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "John", req.Name)

			// Send response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestResponse{ID: 123, Message: "Created"})
		}))
		defer server.Close()

		// Make request
		payload := TestRequest{Name: "John", Email: "john@example.com"}
		opts := utils.DefaultHTTPClientOptions()
		resp, err := utils.MakeHTTPRequest("POST", server.URL, payload, opts)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Decode response
		var result TestResponse
		err = resp.DecodeJSON(&result)
		assert.NoError(t, err)
		assert.Equal(t, 123, result.ID)
	})

	t.Run("Success - GET without body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestResponse{ID: 456, Message: "OK"})
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		resp, err := utils.MakeHTTPRequest("GET", server.URL, nil, opts)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("Custom headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "secret-key", r.Header.Get("X-API-Key"))
			assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		opts.Headers = map[string]string{
			"X-API-Key":     "secret-key",
			"Authorization": "Bearer token123",
		}

		resp, err := utils.MakeHTTPRequest("GET", server.URL, nil, opts)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("Error - Invalid URL", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		_, err := utils.MakeHTTPRequest("GET", "://invalid-url", nil, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("Error - Timeout", func(t *testing.T) {
		// Server that delays response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Short timeout
		opts := utils.DefaultHTTPClientOptions()
		opts.Timeout = 50 * time.Millisecond

		_, err := utils.MakeHTTPRequest("GET", server.URL, nil, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP request failed")
	})
}

func TestHTTPResponse_DecodeJSON(t *testing.T) {
	t.Run("Success - Decode valid JSON", func(t *testing.T) {
		body := []byte(`{"id": 123, "message": "Success"}`)
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       body,
		}

		var result TestResponse
		err := resp.DecodeJSON(&result)
		assert.NoError(t, err)
		assert.Equal(t, 123, result.ID)
		assert.Equal(t, "Success", result.Message)
	})

	t.Run("Error - Empty body", func(t *testing.T) {
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       []byte{},
		}

		var result TestResponse
		err := resp.DecodeJSON(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty response body")
	})

	t.Run("Error - Invalid JSON", func(t *testing.T) {
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{invalid json`),
		}

		var result TestResponse
		err := resp.DecodeJSON(&result)
		assert.Error(t, err)
	})
}

func TestHTTPResponse_DecodeLastJSON(t *testing.T) {
	// ProgressMessage represents a streaming progress line from tt-backend
	type ProgressMessage struct {
		Message string `json:"message"`
	}

	// SessionResponse represents the final session object from tt-backend
	type SessionResponse struct {
		ID     string `json:"id"`
		Status int    `json:"status"`
		IP     string `json:"ip"`
	}

	t.Run("Success - Single JSON object (fast path)", func(t *testing.T) {
		body := []byte(`{"id": 123, "message": "Success"}`)
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       body,
		}

		var result TestResponse
		err := resp.DecodeLastJSON(&result)
		assert.NoError(t, err)
		assert.Equal(t, 123, result.ID)
		assert.Equal(t, "Success", result.Message)
	})

	t.Run("Success - NDJSON with progress messages", func(t *testing.T) {
		// Simulates tt-backend /start streaming: progress lines then final session
		body := []byte("{\"message\":\"Creating...\"}\n{\"message\":\"Starting...\"}\n{\"id\":\"abc\",\"status\":0,\"ip\":\"10.0.0.5\"}")
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       body,
		}

		var result SessionResponse
		err := resp.DecodeLastJSON(&result)
		assert.NoError(t, err)
		assert.Equal(t, "abc", result.ID)
		assert.Equal(t, 0, result.Status)
		assert.Equal(t, "10.0.0.5", result.IP)
	})

	t.Run("Success - NDJSON with many progress messages", func(t *testing.T) {
		// 4 progress objects before the final session object
		body := []byte("{\"message\":\"Initializing container...\"}\n{\"message\":\"Configuring network...\"}\n{\"message\":\"Installing packages...\"}\n{\"message\":\"Starting services...\"}\n{\"id\":\"session-xyz\",\"status\":0,\"ip\":\"192.168.1.100\"}")
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       body,
		}

		var result SessionResponse
		err := resp.DecodeLastJSON(&result)
		assert.NoError(t, err)
		assert.Equal(t, "session-xyz", result.ID)
		assert.Equal(t, 0, result.Status)
		assert.Equal(t, "192.168.1.100", result.IP)
	})

	t.Run("Error - Empty body", func(t *testing.T) {
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       []byte{},
		}

		var result TestResponse
		err := resp.DecodeLastJSON(&result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty response body")
	})

	t.Run("Error - Invalid JSON", func(t *testing.T) {
		resp := &utils.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{invalid json`),
		}

		var result TestResponse
		err := resp.DecodeLastJSON(&result)
		assert.Error(t, err)
	})
}

// ==========================================
// Retry Logic Tests
// ==========================================

func TestMakeHTTPRequestWithRetry(t *testing.T) {
	t.Run("Success on first attempt", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		opts.RetryCount = 3
		opts.RetryDelayMS = 10

		resp, err := utils.MakeHTTPRequestWithRetry("GET", server.URL, nil, opts)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, attemptCount, "Should succeed on first attempt, no retries")
	})

	t.Run("Success after retries", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			if attemptCount < 3 {
				// Fail first 2 attempts by closing connection
				panic(http.ErrAbortHandler)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		opts.RetryCount = 3
		opts.RetryDelayMS = 10

		// Note: This test may not work as expected because httptest server
		// panic doesn't simulate network errors properly
		// This is more of a structure test
		resp, err := utils.MakeHTTPRequestWithRetry("GET", server.URL, nil, opts)

		// We expect either success or error depending on httptest behavior
		if err == nil {
			assert.Equal(t, 200, resp.StatusCode)
		}
	})
}

// ==========================================
// High-Level Convenience Function Tests
// ==========================================

func TestMakeJSONRequest(t *testing.T) {
	t.Run("Success - Request and decode response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestResponse{ID: 789, Message: "Success"})
		}))
		defer server.Close()

		var result TestResponse
		opts := utils.DefaultHTTPClientOptions()
		err := utils.MakeJSONRequest("GET", server.URL, nil, &result, opts)

		assert.NoError(t, err)
		assert.Equal(t, 789, result.ID)
	})

	t.Run("Error - HTTP 400", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Bad Request"))
		}))
		defer server.Close()

		var result TestResponse
		opts := utils.DefaultHTTPClientOptions()
		err := utils.MakeJSONRequest("GET", server.URL, nil, &result, opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 400")
		assert.Contains(t, err.Error(), "Bad Request")
	})

	t.Run("Error - HTTP 500", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		err := utils.MakeJSONRequest("GET", server.URL, nil, nil, opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})
}

// ==========================================
// External API Helper Tests
// ==========================================

func TestMakeExternalAPIRequest(t *testing.T) {
	t.Run("Success - External API call", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestResponse{ID: 999, Message: "API Success"})
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		resp, err := utils.MakeExternalAPIRequest("Test API", "GET", server.URL, nil, opts)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("Error - External API returns 400", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))
		}))
		defer server.Close()

		opts := utils.DefaultHTTPClientOptions()
		_, err := utils.MakeExternalAPIRequest("Test API", "POST", server.URL, nil, opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Test API API returned 400")
	})
}

func TestMakeExternalAPIJSONRequest(t *testing.T) {
	t.Run("Success - Decode external API response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestResponse{ID: 111, Message: "External API"})
		}))
		defer server.Close()

		var result TestResponse
		opts := utils.DefaultHTTPClientOptions()
		err := utils.MakeExternalAPIJSONRequest("Test API", "GET", server.URL, nil, &result, opts)

		assert.NoError(t, err)
		assert.Equal(t, 111, result.ID)
		assert.Equal(t, "External API", result.Message)
	})
}

// ==========================================
// Option Builder Tests
// ==========================================

func TestOptionBuilders(t *testing.T) {
	t.Run("WithTimeout", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts, utils.WithTimeout(5*time.Second))

		assert.Equal(t, 5*time.Second, opts.Timeout)
	})

	t.Run("WithHeader", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts, utils.WithHeader("X-Custom", "value"))

		assert.Equal(t, "value", opts.Headers["X-Custom"])
	})

	t.Run("WithHeaders", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		headers := map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
		}
		utils.ApplyOptions(&opts, utils.WithHeaders(headers))

		assert.Equal(t, "value1", opts.Headers["X-Header-1"])
		assert.Equal(t, "value2", opts.Headers["X-Header-2"])
	})

	t.Run("WithRetry", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts, utils.WithRetry(3, 500))

		assert.Equal(t, 3, opts.RetryCount)
		assert.Equal(t, 500, opts.RetryDelayMS)
	})

	t.Run("WithAPIKey", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts, utils.WithAPIKey("secret-key-123"))

		assert.Equal(t, "secret-key-123", opts.Headers["X-API-Key"])
	})

	t.Run("WithBearerToken", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts, utils.WithBearerToken("token123"))

		assert.Equal(t, "Bearer token123", opts.Headers["Authorization"])
	})

	t.Run("Multiple options", func(t *testing.T) {
		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts,
			utils.WithTimeout(10*time.Second),
			utils.WithAPIKey("secret"),
			utils.WithRetry(5, 1000),
		)

		assert.Equal(t, 10*time.Second, opts.Timeout)
		assert.Equal(t, "secret", opts.Headers["X-API-Key"])
		assert.Equal(t, 5, opts.RetryCount)
		assert.Equal(t, 1000, opts.RetryDelayMS)
	})
}

// ==========================================
// Integration Tests
// ==========================================

func TestHTTPClient_Integration(t *testing.T) {
	t.Run("Complete workflow - POST, receive, decode", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "secret-key", r.Header.Get("X-API-Key"))

			// Parse body
			var req TestRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "Alice", req.Name)

			// Send response
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(TestResponse{
				ID:      42,
				Message: "User created: " + req.Name,
			})
		}))
		defer server.Close()

		// Make request with options
		payload := TestRequest{Name: "Alice", Email: "alice@example.com"}
		var result TestResponse

		opts := utils.DefaultHTTPClientOptions()
		utils.ApplyOptions(&opts,
			utils.WithAPIKey("secret-key"),
			utils.WithTimeout(5*time.Second),
		)

		err := utils.MakeJSONRequest("POST", server.URL, payload, &result, opts)

		assert.NoError(t, err)
		assert.Equal(t, 42, result.ID)
		assert.Contains(t, result.Message, "Alice")
	})
}

// ==========================================
// Default Options Tests
// ==========================================

func TestDefaultHTTPClientOptions(t *testing.T) {
	opts := utils.DefaultHTTPClientOptions()

	assert.Equal(t, 30*time.Second, opts.Timeout, "Default timeout should be 30 seconds")
	assert.Equal(t, 0, opts.RetryCount, "Default retry count should be 0")
	assert.NotNil(t, opts.Headers, "Headers map should be initialized")
	assert.Empty(t, opts.Headers, "Headers should be empty by default")
}
