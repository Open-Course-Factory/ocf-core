package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDBForService creates an in-memory SQLite database with required tables
func setupTestDBForService(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.UserTerminalKey{}, &models.Terminal{})
	require.NoError(t, err)

	return db
}

// createTestTerminalForService creates a test terminal session with an associated user key
func createTestTerminalForService(t *testing.T, db *gorm.DB, userID, sessionID, instanceType string) *models.Terminal {
	userKey := &models.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-api-key-" + userID,
		KeyName:     "test-key",
		IsActive:    true,
	}
	err := db.Create(userKey).Error
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      instanceType,
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}
	err = db.Create(terminal).Error
	require.NoError(t, err)

	return terminal
}

// newCommandHistoryTestService builds a fully-wired terminalTrainerService for
// the command-history tests. These tests control baseURL/apiVersion/repository
// to assert URL construction and history logic, but the public history methods
// now delegate to the embedded terminalHistoryService (which itself reaches tt-backend
// through the proxy's buildAPIPath). A bare struct literal leaves history (and
// proxy) nil → SIGSEGV, so this helper constructs both collaborators with the
// SAME baseURL/apiVersion the test passes in.
//
// The proxy's apiVersion/terminalType are forced to match the test inputs so
// buildAPIPath yields the asserted "/<apiVersion>/<instanceType>/history" path
// regardless of any TERMINAL_TRAINER_* env vars in the runner's environment.
func newCommandHistoryTestService(baseURL, apiVersion string, db *gorm.DB) *terminalTrainerService {
	repo := repositories.NewTerminalRepository(db)

	proxy := newTerminalProxyClient(repo)
	proxy.baseURL = baseURL
	proxy.apiVersion = apiVersion
	proxy.terminalType = ""

	tts := &terminalTrainerService{
		baseURL:    baseURL,
		apiVersion: apiVersion,
		repository: repo,
		db:         db,
		terminalProxyClient: proxy,
	}
	tts.terminalHistoryService = newTerminalHistoryService(proxy, repo, db, baseURL, apiVersion, tts.adminKey)
	return tts
}

// TestGetSessionCommandHistory_URLConstruction tests that the correct URL is built with query params
func TestGetSessionCommandHistory_URLConstruction(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test-session","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "test-session-1", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	since := int64(1700000000)
	body, contentType, err := svc.GetSessionCommandHistory("test-session-1", &since, "json", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "/1.0/alp/history", capturedPath)
	assert.Contains(t, capturedQuery, "id=test-session-1")
	assert.Contains(t, capturedQuery, "since=1700000000")
	assert.Contains(t, capturedQuery, "format=json")
	assert.Equal(t, "test-api-key-user1", capturedAPIKey)
}

// TestGetSessionCommandHistory_NoOptionalParams tests URL construction without optional params
func TestGetSessionCommandHistory_NoOptionalParams(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test-session","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-no-params", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("session-no-params", nil, "", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "id=session-no-params", capturedQuery)
}

// TestGetSessionCommandHistory_CSVFormat tests that CSV format is correctly passed and content type set
func TestGetSessionCommandHistory_CSVFormat(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte("sequence_num,command,executed_at\n1,ls,1700000000\n"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-csv", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("session-csv", nil, "csv", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "text/csv", contentType)
	assert.Contains(t, capturedQuery, "format=csv")
}

// TestGetSessionCommandHistory_FormatWhitelist_InvalidFormat tests that invalid format is defaulted to json
func TestGetSessionCommandHistory_FormatWhitelist_InvalidFormat(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-bad-format", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("session-bad-format", nil, "xml", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	// Invalid format "xml" should be defaulted to "json"
	assert.Contains(t, capturedQuery, "format=json")
	assert.NotContains(t, capturedQuery, "format=xml")
}

// TestGetSessionCommandHistory_FormatWhitelist_InjectionAttempt tests that URL injection via format is blocked
func TestGetSessionCommandHistory_FormatWhitelist_InjectionAttempt(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-inject", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	// Attempt URL parameter injection via format
	body, _, err := svc.GetSessionCommandHistory("session-inject", nil, "json&admin=true&delete=all", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	// Injection attempt should be neutralized - format defaulted to "json"
	assert.Contains(t, capturedQuery, "format=json")
	assert.NotContains(t, capturedQuery, "admin=true")
}

// TestGetSessionCommandHistory_FormatWhitelist_ValidFormats tests all valid format values
func TestGetSessionCommandHistory_FormatWhitelist_ValidFormats(t *testing.T) {
	validFormats := []struct {
		input       string
		expectInURL string
		contentType string
	}{
		{"json", "format=json", "application/json"},
		{"csv", "format=csv", "text/csv"},
		{"", "", "application/json"}, // empty format should not be added to URL
	}

	for _, tc := range validFormats {
		t.Run("format_"+tc.input, func(t *testing.T) {
			var capturedQuery string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = r.URL.RawQuery
				if r.URL.Query().Get("format") == "csv" {
					w.Header().Set("Content-Type", "text/csv")
					w.Write([]byte("timestamp,command\n"))
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
				}
			}))
			defer server.Close()

			db := setupTestDBForService(t)
			sessionID := "session-valid-" + tc.input
			if tc.input == "" {
				sessionID = "session-valid-empty"
			}
			_ = createTestTerminalForService(t, db, "user1", sessionID, "alp")

			svc := newCommandHistoryTestService(server.URL, "1.0", db)

			_, contentType, err := svc.GetSessionCommandHistory(sessionID, nil, tc.input, 0, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.contentType, contentType)

			if tc.expectInURL != "" {
				assert.Contains(t, capturedQuery, tc.expectInURL)
			} else {
				assert.NotContains(t, capturedQuery, "format=")
			}
		})
	}
}

// TestGetSessionCommandHistory_SessionNotFound tests error when session doesn't exist
func TestGetSessionCommandHistory_SessionNotFound(t *testing.T) {
	db := setupTestDBForService(t)

	svc := newCommandHistoryTestService("http://localhost:9999", "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("nonexistent-session", nil, "json", 0, 0)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
	assert.Nil(t, body)
	assert.Empty(t, contentType)
}

// TestGetSessionCommandHistory_Pagination tests that limit/offset are appended to the proxied URL
func TestGetSessionCommandHistory_Pagination(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-paginate", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("session-paginate", nil, "json", 50, 10)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Contains(t, capturedQuery, "limit=50")
	assert.Contains(t, capturedQuery, "offset=10")
	assert.Contains(t, capturedQuery, "format=json")
}

// TestGetSessionCommandHistory_PaginationZeroValues tests that zero limit/offset are not appended
func TestGetSessionCommandHistory_PaginationZeroValues(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-no-paginate", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	_, _, err := svc.GetSessionCommandHistory("session-no-paginate", nil, "", 0, 0)

	require.NoError(t, err)
	assert.NotContains(t, capturedQuery, "limit=")
	assert.NotContains(t, capturedQuery, "offset=")
}

// =============================================================================
// B3: Response body size limit on proxy passthrough
// =============================================================================

// TestGetHistory_LargeResponse_LimitedTo10MB verifies that GetSessionCommandHistory
// returns an error when tt-backend returns a response larger than 10MB. This prevents
// an OOM scenario where a massive history payload could exhaust ocf-core's memory.
func TestGetHistory_LargeResponse_LimitedTo10MB(t *testing.T) {
	// Create a mock tt-backend that returns a response just over 10MB
	oversizedBody := make([]byte, 10*1024*1024+1) // 10MB + 1 byte
	for i := range oversizedBody {
		oversizedBody[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(oversizedBody)
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-large", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, _, err := svc.GetSessionCommandHistory("session-large", nil, "json", 0, 0)

	require.Error(t, err, "should return error for response exceeding 10MB")
	assert.Nil(t, body, "body should be nil when response exceeds limit")
	assert.Contains(t, err.Error(), "response body exceeds", "error should mention size limit")
}

// TestGetHistory_ExactlyAtLimit_Succeeds verifies that a response exactly at 10MB
// is allowed through (the limit applies to responses strictly larger than 10MB).
func TestGetHistory_ExactlyAtLimit_Succeeds(t *testing.T) {
	// Create a response exactly at 10MB
	exactBody := make([]byte, 10*1024*1024) // exactly 10MB
	for i := range exactBody {
		exactBody[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(exactBody)
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-exact", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, contentType, err := svc.GetSessionCommandHistory("session-exact", nil, "json", 0, 0)

	require.NoError(t, err, "response exactly at 10MB should succeed")
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Len(t, body, 10*1024*1024)
}

// TestStartComposedSession_InvalidPlanType tests that invalid plan type returns error
func TestStartComposedSession_InvalidPlanType(t *testing.T) {
	db := setupTestDBForService(t)

	// Use the production constructor so the composer collaborator is wired;
	// StartComposedSession delegates to it. The invalid-plan-type guard fires
	// before any tt-backend call, so baseURL is irrelevant here.
	svc := NewTerminalTrainerService(db).(*terminalTrainerService)

	composedInput := dto.CreateComposedSessionInput{
		Distribution: "debian",
		Size:         "S",
		Terms:        "accepted",
	}

	// Pass wrong type as plan
	_, err := svc.StartComposedSession("user1", composedInput, "not-a-plan")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid subscription plan type")
}

// =============================================================================
// HTTP error status passthrough (403 Forbidden, 429 Rate Limit)
// =============================================================================

// TestGetSessionCommandHistory_403Forbidden_ReturnsError verifies that when tt-backend
// returns 403 (e.g., recording not enabled for this session), the service returns an
// error containing the status code so the controller can map it to a proper 403 instead
// of a generic 500.
func TestGetSessionCommandHistory_403Forbidden_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Recording not enabled for this session"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-403", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, _, err := svc.GetSessionCommandHistory("session-403", nil, "json", 0, 0)

	assert.Error(t, err, "should return error when tt-backend returns 403")
	assert.Nil(t, body, "body should be nil on 403 error")
	assert.Contains(t, err.Error(), "403", "error should contain status code 403")
	assert.Contains(t, err.Error(), "Recording not enabled", "error should contain the upstream error message")
}

// TestGetSessionCommandHistory_429RateLimit_ReturnsError verifies that when tt-backend
// returns 429 (rate limit exceeded), the service returns an error containing the status
// code so the controller can map it appropriately instead of returning a generic 500.
func TestGetSessionCommandHistory_429RateLimit_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-429", "alp")

	svc := newCommandHistoryTestService(server.URL, "1.0", db)

	body, _, err := svc.GetSessionCommandHistory("session-429", nil, "json", 0, 0)

	assert.Error(t, err, "should return error when tt-backend returns 429")
	assert.Nil(t, body, "body should be nil on 429 error")
	assert.Contains(t, err.Error(), "429", "error should contain status code 429")
	assert.Contains(t, err.Error(), "Rate limit exceeded", "error should contain the upstream error message")
}
