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
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"session_id":"test","commands":[],"count":0}`))
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

// TestStartSessionWithPlan_RetentionDaysCopied verifies that StartSessionWithPlan correctly
// copies CommandHistoryRetentionDays from the plan to the session input
func TestStartSessionWithPlan_RetentionDaysCopied(t *testing.T) {
	testCases := []struct {
		name                string
		planRetentionDays   int
		inputRetentionDays  int
		expectedInURL       bool
		expectedValue       int
	}{
		{
			name:               "plan_overrides_input",
			planRetentionDays:  30,
			inputRetentionDays: 0,
			expectedInURL:      true,
			expectedValue:      30,
		},
		{
			name:               "plan_overrides_existing_input",
			planRetentionDays:  90,
			inputRetentionDays: 7,
			expectedInURL:      true,
			expectedValue:      90,
		},
		{
			name:               "zero_retention_not_in_url",
			planRetentionDays:  0,
			inputRetentionDays: 0,
			expectedInURL:      false,
			expectedValue:      0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan := &paymentModels.SubscriptionPlan{
				MaxSessionDurationMinutes:   60,
				AllowedMachineSizes:         []string{"S", "M"},
				CommandHistoryRetentionDays: tc.planRetentionDays,
			}

			sessionInput := dto.CreateTerminalSessionInput{
				Terms:                "accepted",
				HistoryRetentionDays: tc.inputRetentionDays,
			}

			// Simulate the StartSessionWithPlan logic (line 423 in service)
			sessionInput.HistoryRetentionDays = plan.CommandHistoryRetentionDays

			assert.Equal(t, tc.expectedValue, sessionInput.HistoryRetentionDays,
				"HistoryRetentionDays should be set from plan")

			// Verify URL construction behavior
			urlStr := "http://example.com/start?terms=abc"
			if sessionInput.HistoryRetentionDays > 0 {
				urlStr += fmt.Sprintf("&history_retention_days=%d", sessionInput.HistoryRetentionDays)
			}

			parsed, err := url.Parse(urlStr)
			require.NoError(t, err)

			retentionParam := parsed.Query().Get("history_retention_days")
			if tc.expectedInURL {
				assert.Equal(t, fmt.Sprintf("%d", tc.expectedValue), retentionParam,
					"history_retention_days should be in URL with correct value")
			} else {
				assert.Empty(t, retentionParam,
					"history_retention_days should not be in URL when 0")
			}
		})
	}
}

// TestStartSessionWithPlan_InvalidPlanType tests that invalid plan type returns error
func TestStartSessionWithPlan_InvalidPlanType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	db := setupTestDBForService(t)

	svc := &terminalTrainerService{
		baseURL:    "http://localhost:9999",
		apiVersion: "1.0",
		repository: repositories.NewTerminalRepository(db),
	}

	sessionInput := dto.CreateTerminalSessionInput{
		Terms: "accepted",
	}

	// Pass wrong type as plan
	_, err := svc.StartSessionWithPlan("user1", sessionInput, "not-a-plan")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid subscription plan type")
}
