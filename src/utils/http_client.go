package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClientOptions configures HTTP client behavior
type HTTPClientOptions struct {
	Timeout      time.Duration
	Headers      map[string]string
	RetryCount   int
	RetryDelayMS int
}

// DefaultHTTPClientOptions returns sensible defaults
func DefaultHTTPClientOptions() HTTPClientOptions {
	return HTTPClientOptions{
		Timeout:      30 * time.Second,
		Headers:      make(map[string]string),
		RetryCount:   0, // No retries by default
		RetryDelayMS: 1000,
	}
}

// HTTPResponse represents an HTTP response with parsed body
type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// DecodeJSON decodes the response body into the provided interface
func (r *HTTPResponse) DecodeJSON(v any) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("empty response body")
	}
	return json.Unmarshal(r.Body, v)
}

// DecodeLastJSON decodes the last JSON object from a response body that may
// contain multiple newline-delimited JSON objects (NDJSON). This is needed for
// streaming endpoints like tt-backend's /start which writes progress messages
// as separate JSON objects before the final response.
func (r *HTTPResponse) DecodeLastJSON(v any) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("empty response body")
	}
	// Try standard unmarshal first (fast path for single JSON object)
	if err := json.Unmarshal(r.Body, v); err == nil {
		return nil
	}
	// Fall back to reading last JSON object from NDJSON stream
	decoder := json.NewDecoder(bytes.NewReader(r.Body))
	var lastRaw json.RawMessage
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		lastRaw = raw
	}
	if lastRaw == nil {
		return fmt.Errorf("no JSON objects found in response body")
	}
	return json.Unmarshal(lastRaw, v)
}

// ==========================================
// Low-Level HTTP Functions
// ==========================================

// MakeHTTPRequest makes an HTTP request with the given method, URL, body, and options
//
// Example:
//
//	resp, err := MakeHTTPRequest("POST", "https://api.example.com/users", payload, opts)
func MakeHTTPRequest(method, url string, body any, opts HTTPClientOptions) (*HTTPResponse, error) {
	// Marshal body to JSON if provided
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBytes)
	}

	// Create HTTP request
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default Content-Type for JSON
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply custom headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	// Debug: Log headers being sent (especially for Terminal Trainer requests)
	if len(opts.Headers) > 0 {
		Debug("HTTP %s %s - Headers: %v", method, url, opts.Headers)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: opts.Timeout,
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

// MakeHTTPRequestWithRetry makes an HTTP request with automatic retries
//
// Example:
//
//	resp, err := MakeHTTPRequestWithRetry("POST", url, payload, opts)
func MakeHTTPRequestWithRetry(method, url string, body any, opts HTTPClientOptions) (*HTTPResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= opts.RetryCount; attempt++ {
		if attempt > 0 {
			// Delay before retry
			time.Sleep(time.Duration(opts.RetryDelayMS) * time.Millisecond)
			Debug("Retrying HTTP request (attempt %d/%d): %s %s", attempt+1, opts.RetryCount+1, method, url)
		}

		resp, err := MakeHTTPRequest(method, url, body, opts)
		if err == nil {
			// Success
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable (network errors, timeouts)
		// HTTP errors with responses are not retried
		if resp != nil {
			// Got a response (even if error status), don't retry
			return resp, err
		}
	}

	return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", opts.RetryCount+1, lastErr)
}

// ==========================================
// High-Level Convenience Functions
// ==========================================

// MakeJSONRequest makes a JSON request and checks for error status codes
//
// Example:
//
//	var result UserResponse
//	err := MakeJSONRequest("POST", url, payload, &result, opts)
func MakeJSONRequest(method, url string, body any, result any, opts HTTPClientOptions) error {
	resp, err := MakeHTTPRequest(method, url, body, opts)
	if err != nil {
		return err
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(resp.Body))
	}

	// Decode response if result pointer provided
	if result != nil {
		if err := resp.DecodeJSON(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// MakeJSONRequestWithRetry makes a JSON request with automatic retries
//
// Example:
//
//	var result UserResponse
//	err := MakeJSONRequestWithRetry("POST", url, payload, &result, opts)
func MakeJSONRequestWithRetry(method, url string, body any, result any, opts HTTPClientOptions) error {
	resp, err := MakeHTTPRequestWithRetry(method, url, body, opts)
	if err != nil {
		return err
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(resp.Body))
	}

	// Decode response if result pointer provided
	if result != nil {
		if err := resp.DecodeJSON(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ==========================================
// External API Helpers
// ==========================================

// MakeExternalAPIRequest makes a request to an external API service
//
// Example:
//
//	resp, err := MakeExternalAPIRequest("Terminal Trainer", "POST", url, payload, opts)
func MakeExternalAPIRequest(serviceName, method, url string, body any, opts HTTPClientOptions) (*HTTPResponse, error) {
	resp, err := MakeHTTPRequest(method, url, body, opts)
	if err != nil {
		return nil, ExternalAPIError(serviceName, fmt.Sprintf("%s %s", method, url), err)
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		return resp, ExternalAPIStatusError(serviceName, fmt.Sprintf("%s %s", method, url), resp.StatusCode, string(resp.Body))
	}

	return resp, nil
}

// MakeExternalAPIJSONRequest makes a JSON request to an external API and decodes the response
//
// Example:
//
//	var result SessionResponse
//	err := MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, payload, &result, opts)
func MakeExternalAPIJSONRequest(serviceName, method, url string, body any, result any, opts HTTPClientOptions) error {
	resp, err := MakeExternalAPIRequest(serviceName, method, url, body, opts)
	if err != nil {
		return err
	}

	// Decode response if result pointer provided
	if result != nil {
		if err := resp.DecodeJSON(result); err != nil {
			return ExternalAPIError(serviceName, "decode response", err)
		}
	}

	return nil
}

// ==========================================
// HTTP Client Option Builders
// ==========================================

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		opts.Timeout = timeout
	}
}

// WithHeader adds a header to the HTTP request
func WithHeader(key, value string) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		opts.Headers[key] = value
	}
}

// WithHeaders sets multiple headers
func WithHeaders(headers map[string]string) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		for k, v := range headers {
			opts.Headers[k] = v
		}
	}
}

// WithRetry sets retry behavior
func WithRetry(count, delayMS int) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		opts.RetryCount = count
		opts.RetryDelayMS = delayMS
	}
}

// WithAPIKey adds an API key header
func WithAPIKey(apiKey string) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		opts.Headers["X-API-Key"] = apiKey
	}
}

// WithBearerToken adds a Bearer token header
func WithBearerToken(token string) func(*HTTPClientOptions) {
	return func(opts *HTTPClientOptions) {
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		opts.Headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}
}

// ApplyOptions applies option functions to HTTPClientOptions
//
// Example:
//
//	opts := DefaultHTTPClientOptions()
//	ApplyOptions(&opts, WithTimeout(10*time.Second), WithAPIKey("secret"))
func ApplyOptions(opts *HTTPClientOptions, optFuncs ...func(*HTTPClientOptions)) {
	for _, fn := range optFuncs {
		fn(opts)
	}
}
