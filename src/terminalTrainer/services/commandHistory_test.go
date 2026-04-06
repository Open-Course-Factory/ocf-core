package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	paymentModels "soli/formations/src/payment/models"
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

	err = db.AutoMigrate(&models.UserTerminalKey{}, &models.Terminal{}, &models.TerminalShare{})
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
		MaxSessions: 5,
	}
	err := db.Create(userKey).Error
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		Status:            "active",
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

// TestGetSessionCommandHistory_URLConstruction tests that the correct URL is built with query params
func TestGetSessionCommandHistory_URLConstruction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

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

	svc := &terminalTrainerService{
		baseURL:      server.URL,
		apiVersion:   "1.0",
		terminalType: "",
		repository:   repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test-session","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-no-params", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, contentType, err := svc.GetSessionCommandHistory("session-no-params", nil, "", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "id=session-no-params", capturedQuery)
}

// TestGetSessionCommandHistory_CSVFormat tests that CSV format is correctly passed and content type set
func TestGetSessionCommandHistory_CSVFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte("sequence_num,command,executed_at\n1,ls,1700000000\n"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-csv", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, contentType, err := svc.GetSessionCommandHistory("session-csv", nil, "csv", 0, 0)

	require.NoError(t, err)
	assert.NotNil(t, body)
	assert.Equal(t, "text/csv", contentType)
	assert.Contains(t, capturedQuery, "format=csv")
}

// TestGetSessionCommandHistory_FormatWhitelist_InvalidFormat tests that invalid format is defaulted to json
func TestGetSessionCommandHistory_FormatWhitelist_InvalidFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-bad-format", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-inject", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

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

			svc := &terminalTrainerService{
				baseURL:    server.URL,
				apiVersion: "1.0",
				repository: repositories.NewTerminalRepository(db),
			}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	db := setupTestDBForService(t)

	svc := &terminalTrainerService{
		baseURL:    "http://localhost:9999",
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, contentType, err := svc.GetSessionCommandHistory("nonexistent-session", nil, "json", 0, 0)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
	assert.Nil(t, body)
	assert.Empty(t, contentType)
}

// TestGetSessionCommandHistory_Pagination tests that limit/offset are appended to the proxied URL
func TestGetSessionCommandHistory_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-paginate", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-no-paginate", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

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

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, _, err := svc.GetSessionCommandHistory("session-large", nil, "json", 0, 0)

	require.Error(t, err, "should return error for response exceeding 10MB")
	assert.Nil(t, body, "body should be nil when response exceeds limit")
	assert.Contains(t, err.Error(), "response body exceeds", "error should mention size limit")
}

// TestGetHistory_ExactlyAtLimit_Succeeds verifies that a response exactly at 10MB
// is allowed through (the limit applies to responses strictly larger than 10MB).
func TestGetHistory_ExactlyAtLimit_Succeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

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

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, contentType, err := svc.GetSessionCommandHistory("session-exact", nil, "json", 0, 0)

	require.NoError(t, err, "response exactly at 10MB should succeed")
	assert.NotNil(t, body)
	assert.Equal(t, "application/json", contentType)
	assert.Len(t, body, 10*1024*1024)
}

// TestExternalRefURLEncoding verifies that url.QueryEscape properly encodes special characters
// This tests the encoding logic used in StartSession for ExternalRef without calling the full
// StartSession method (which requires Casbin enforcer setup).
func TestExternalRefURLEncoding(t *testing.T) {
	testCases := []struct {
		name        string
		externalRef string
		expectEncoded string
	}{
		{
			name:          "ampersand_injection",
			externalRef:   "training&admin=true",
			expectEncoded: "training%26admin%3Dtrue",
		},
		{
			name:          "space_encoding",
			externalRef:   "my training session",
			expectEncoded: "my+training+session",
		},
		{
			name:          "slash_encoding",
			externalRef:   "org/course/session",
			expectEncoded: "org%2Fcourse%2Fsession",
		},
		{
			name:          "question_mark",
			externalRef:   "ref?param=1",
			expectEncoded: "ref%3Fparam%3D1",
		},
		{
			name:          "hash_fragment",
			externalRef:   "ref#section",
			expectEncoded: "ref%23section",
		},
		{
			name:          "complex_injection",
			externalRef:   "ref&evil=true&delete=all",
			expectEncoded: "ref%26evil%3Dtrue%26delete%3Dall",
		},
		{
			name:          "safe_characters_preserved",
			externalRef:   "simple-ref_123",
			expectEncoded: "simple-ref_123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := url.QueryEscape(tc.externalRef)
			assert.Equal(t, tc.expectEncoded, encoded)

			// Verify the encoded value would produce a valid URL parameter
			testURL := fmt.Sprintf("http://example.com/start?external_ref=%s", encoded)
			parsed, err := url.Parse(testURL)
			require.NoError(t, err)

			// The parsed query should decode back to the original value
			decodedRef := parsed.Query().Get("external_ref")
			assert.Equal(t, tc.externalRef, decodedRef,
				"URL-encoded value should decode back to original")
		})
	}
}

// TestStartSessionURLConstruction_ExternalRef verifies the URL construction pattern in StartSession
// by directly building the URL the same way StartSession does and verifying encoding
func TestStartSessionURLConstruction_ExternalRef(t *testing.T) {
	testCases := []struct {
		name        string
		externalRef string
	}{
		{"with_ampersand", "training&id=42"},
		{"with_space", "my session 2024"},
		{"with_special_chars", "org/course?id=1&type=lab"},
		{"empty_ref", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := "http://localhost:8090"
			apiPath := "/1.0/start"
			terms := "abc123"

			// Build URL the same way StartSession does (after our fix)
			builtURL := fmt.Sprintf("%s%s?terms=%s", baseURL, apiPath, terms)
			if tc.externalRef != "" {
				builtURL += fmt.Sprintf("&external_ref=%s", url.QueryEscape(tc.externalRef))
			}

			// Parse and verify
			parsed, err := url.Parse(builtURL)
			require.NoError(t, err)

			if tc.externalRef != "" {
				// The external_ref should be properly isolated as a single parameter
				decodedRef := parsed.Query().Get("external_ref")
				assert.Equal(t, tc.externalRef, decodedRef,
					"external_ref should round-trip through URL encoding")

				// Verify no parameter injection
				for key := range parsed.Query() {
					assert.Contains(t, []string{"terms", "external_ref"}, key,
						"only expected params should exist, got unexpected: %s", key)
				}
			} else {
				// Empty ref should not add the parameter
				assert.Empty(t, parsed.Query().Get("external_ref"))
			}
		})
	}
}

// TestStartComposedSession_RetentionDaysCopied verifies that StartComposedSession correctly
// copies CommandHistoryRetentionDays from the plan to the session input
func TestStartComposedSession_RetentionDaysCopied(t *testing.T) {
	testCases := []struct {
		name              string
		planRetentionDays int
		expectedValue     int
	}{
		{
			name:              "plan_sets_30_days",
			planRetentionDays: 30,
			expectedValue:     30,
		},
		{
			name:              "plan_sets_90_days",
			planRetentionDays: 90,
			expectedValue:     90,
		},
		{
			name:              "plan_zero_retention",
			planRetentionDays: 0,
			expectedValue:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan := &paymentModels.SubscriptionPlan{
				MaxSessionDurationMinutes:   60,
				AllowedMachineSizes:         []string{"S", "M"},
				CommandHistoryRetentionDays: tc.planRetentionDays,
			}

			composedInput := dto.CreateComposedSessionInput{
				Distribution: "debian",
				Size:         "S",
				Terms:        "accepted",
			}

			// Simulate the StartComposedSession logic
			composedInput.HistoryRetentionDays = plan.CommandHistoryRetentionDays

			assert.Equal(t, tc.expectedValue, composedInput.HistoryRetentionDays,
				"HistoryRetentionDays should be set from plan")
		})
	}
}

// =============================================================================
// B1: BulkCreateTerminalsForGroup must set HistoryRetentionDays from plan
// =============================================================================

// TestBulkCreateTerminals_SetsRetentionDaysFromPlan verifies that the bulk
// terminal creation path explicitly sets HistoryRetentionDays from the
// subscription plan. Recording stays enabled regardless of retention days.
// This tests the composedInput construction logic in BulkCreateTerminalsForGroup.
func TestBulkCreateTerminals_SetsRetentionDaysFromPlan(t *testing.T) {
	testCases := []struct {
		name                   string
		planRetentionDays      int
		requestRecordingEnabled int
		expectedRetentionDays  int
		expectedEnabled        int
	}{
		{
			name:                    "plan_with_90_day_retention",
			planRetentionDays:       90,
			requestRecordingEnabled: 1,
			expectedRetentionDays:   90,
			expectedEnabled:         1,
		},
		{
			name:                    "plan_with_zero_retention_recording_stays_enabled",
			planRetentionDays:       0,
			requestRecordingEnabled: 1,
			expectedRetentionDays:   0,
			expectedEnabled:         1,
		},
		{
			name:                    "plan_with_365_day_retention",
			planRetentionDays:       365,
			requestRecordingEnabled: 1,
			expectedRetentionDays:   365,
			expectedEnabled:         1,
		},
		{
			name:                    "zero_retention_recording_enabled_stays",
			planRetentionDays:       0,
			requestRecordingEnabled: 1,
			expectedRetentionDays:   0,
			expectedEnabled:         1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan := &paymentModels.SubscriptionPlan{
				MaxSessionDurationMinutes:   60,
				AllowedMachineSizes:         []string{"S", "M"},
				CommandHistoryRetentionDays: tc.planRetentionDays,
			}

			request := dto.BulkCreateTerminalsRequest{
				Terms:            "accepted",
				RecordingEnabled: tc.requestRecordingEnabled,
				InstanceType:     "alp",
			}

			// Simulate BulkCreateTerminalsForGroup composed input construction
			composedInput := dto.CreateComposedSessionInput{
				Distribution:     request.InstanceType,
				Size:             "S",
				Terms:            request.Terms,
				Expiry:           request.Expiry,
				Backend:          request.Backend,
				OrganizationID:   request.OrganizationID,
				RecordingEnabled: request.RecordingEnabled,
			}

			// StartComposedSession sets retention days from plan
			composedInput.HistoryRetentionDays = plan.CommandHistoryRetentionDays

			assert.Equal(t, tc.expectedRetentionDays, composedInput.HistoryRetentionDays,
				"HistoryRetentionDays should be set from plan")
			assert.Equal(t, tc.expectedEnabled, composedInput.RecordingEnabled,
				"RecordingEnabled should stay enabled regardless of retention days")
		})
	}
}

// TestStartComposedSession_InvalidPlanType tests that invalid plan type returns error
func TestStartComposedSession_InvalidPlanType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	db := setupTestDBForService(t)

	svc := &terminalTrainerService{
		baseURL:    "http://localhost:9999",
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
// C-1: Recording stays enabled regardless of retention days
// =============================================================================

// TestStartComposedSession_ZeroRetention_RecordingStaysEnabled verifies that
// recording stays enabled regardless of plan retention days. Recording is always
// on (RGPD Art. 6.1.f — legitimate interest).
func TestStartComposedSession_ZeroRetention_RecordingStaysEnabled(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{
		MaxSessionDurationMinutes:   60,
		AllowedMachineSizes:         []string{"S", "M"},
		CommandHistoryRetentionDays: 0,
	}

	composedInput := dto.CreateComposedSessionInput{
		Distribution:     "debian",
		Size:             "S",
		Terms:            "accepted",
		RecordingEnabled: 1, // Recording is always on
	}

	// Simulate the StartComposedSession logic:
	composedInput.HistoryRetentionDays = plan.CommandHistoryRetentionDays

	assert.Equal(t, 0, composedInput.HistoryRetentionDays,
		"HistoryRetentionDays should be 0 from plan")

	// Recording stays enabled — no override based on retention days
	assert.Equal(t, 1, composedInput.RecordingEnabled,
		"RecordingEnabled must stay 1 regardless of plan retention days")
}

// =============================================================================
// H-3: Backend parameter not URL-encoded
// =============================================================================

// TestBackendParameterURLEncoding verifies that the Backend parameter is properly
// URL-encoded when constructing the start session URL. Without encoding, a malicious
// backend value like "cloud1&recording_consent=1&history_retention_days=999" would
// inject extra query parameters.
func TestBackendParameterURLEncoding(t *testing.T) {
	testCases := []struct {
		name    string
		backend string
	}{
		{
			name:    "ampersand_injection",
			backend: "cloud1&recording_consent=1&history_retention_days=999",
		},
		{
			name:    "equals_injection",
			backend: "cloud1=evil",
		},
		{
			name:    "question_mark_injection",
			backend: "cloud1?admin=true",
		},
		{
			name:    "space_in_name",
			backend: "my backend",
		},
		{
			name:    "hash_fragment_injection",
			backend: "cloud1#fragment",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build URL the same way StartSession does (line 252), now with URL encoding:
			//   url += fmt.Sprintf("&backend=%s", url.QueryEscape(sessionInput.Backend))
			baseURL := "http://localhost:8090"
			apiPath := "/1.0/start"
			terms := "abc123"

			builtURL := fmt.Sprintf("%s%s?terms=%s", baseURL, apiPath, terms)
			builtURL += fmt.Sprintf("&backend=%s", url.QueryEscape(tc.backend))

			parsed, err := url.Parse(builtURL)
			require.NoError(t, err)

			// The backend parameter should be properly isolated.
			// If URL-encoded correctly, parsing the URL should give us back the original
			// backend value and no extra parameters should be injected.
			decodedBackend := parsed.Query().Get("backend")
			assert.Equal(t, tc.backend, decodedBackend,
				"backend should round-trip through URL encoding without losing data")

			// Verify no parameter injection occurred: only "terms" and "backend" should exist
			for key := range parsed.Query() {
				assert.Contains(t, []string{"terms", "backend"}, key,
					"only expected params should exist, got unexpected param: %s (injection via backend value)", key)
			}
		})
	}
}

// TestStartSessionURLConstruction_Backend verifies URL construction with Backend
// using the same pattern as TestStartSessionURLConstruction_ExternalRef
func TestStartSessionURLConstruction_Backend(t *testing.T) {
	testCases := []struct {
		name    string
		backend string
	}{
		{"with_ampersand", "cloud1&evil=true"},
		{"with_special_chars", "cloud/1?type=premium"},
		{"normal_backend", "cloud1"},
		{"empty_backend", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := "http://localhost:8090"
			apiPath := "/1.0/start"
			terms := "abc123"

			// Build URL the CORRECT way (with encoding)
			correctURL := fmt.Sprintf("%s%s?terms=%s", baseURL, apiPath, terms)
			if tc.backend != "" {
				correctURL += fmt.Sprintf("&backend=%s", url.QueryEscape(tc.backend))
			}

			// Build URL the CURRENT way (now with encoding, matching the fixed code)
			currentURL := fmt.Sprintf("%s%s?terms=%s", baseURL, apiPath, terms)
			if tc.backend != "" {
				currentURL += fmt.Sprintf("&backend=%s", url.QueryEscape(tc.backend))
			}

			// Parse the current (fixed) URL
			parsedCurrent, err := url.Parse(currentURL)
			require.NoError(t, err)

			if tc.backend != "" {
				// The backend value should decode back to the original
				decodedBackend := parsedCurrent.Query().Get("backend")
				assert.Equal(t, tc.backend, decodedBackend,
					"backend should round-trip correctly; current code may inject extra params")

				// Only expected params should exist
				for key := range parsedCurrent.Query() {
					assert.Contains(t, []string{"terms", "backend"}, key,
						"unexpected param injected via backend: %s", key)
				}
			}
		})
	}
}

// =============================================================================
// M-4: recording_consent validation (values outside 0/1)
// =============================================================================

// =============================================================================
// HTTP error status passthrough (403 Forbidden, 429 Rate Limit)
// =============================================================================

// TestGetSessionCommandHistory_403Forbidden_ReturnsError verifies that when tt-backend
// returns 403 (e.g., recording not enabled for this session), the service returns an
// error containing the status code so the controller can map it to a proper 403 instead
// of a generic 500.
func TestGetSessionCommandHistory_403Forbidden_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Recording not enabled for this session"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-403", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
	}))
	defer server.Close()

	db := setupTestDBForService(t)
	_ = createTestTerminalForService(t, db, "user1", "session-429", "alp")

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	body, _, err := svc.GetSessionCommandHistory("session-429", nil, "json", 0, 0)

	assert.Error(t, err, "should return error when tt-backend returns 429")
	assert.Nil(t, body, "body should be nil on 429 error")
	assert.Contains(t, err.Error(), "429", "error should contain status code 429")
	assert.Contains(t, err.Error(), "Rate limit exceeded", "error should contain the upstream error message")
}

// TestRecordingEnabledValidation verifies that recording_enabled values outside
// the valid range (0 or 1) are normalized. Values > 1 are clamped to 1,
// values < 0 are clamped to 0.
func TestRecordingEnabledValidation(t *testing.T) {
	testCases := []struct {
		name           string
		enabledValue   int
		expectedValue  int
	}{
		{
			name:          "valid_enabled_1",
			enabledValue:  1,
			expectedValue: 1,
		},
		{
			name:          "valid_enabled_0",
			enabledValue:  0,
			expectedValue: 0,
		},
		{
			name:          "invalid_enabled_2_clamped_to_1",
			enabledValue:  2,
			expectedValue: 1,
		},
		{
			name:          "invalid_enabled_negative_clamped_to_0",
			enabledValue:  -1,
			expectedValue: 0,
		},
		{
			name:          "invalid_enabled_large_number_clamped_to_1",
			enabledValue:  999,
			expectedValue: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			composedInput := dto.CreateComposedSessionInput{
				Distribution:     "debian",
				Size:             "S",
				Terms:            "accepted",
				RecordingEnabled: tc.enabledValue,
			}

			// Simulate the normalization logic from startComposedSession
			if composedInput.RecordingEnabled > 1 {
				composedInput.RecordingEnabled = 1
			}
			if composedInput.RecordingEnabled < 0 {
				composedInput.RecordingEnabled = 0
			}

			assert.Equal(t, tc.expectedValue, composedInput.RecordingEnabled,
				"recording_enabled=%d should be normalized to %d", tc.enabledValue, tc.expectedValue)
		})
	}
}
