package auth_tests

// ---------------------------------------------------------------------------
// Slice B5 — /auth/me must include `impersonated_by` (TDD)
//
// These tests describe the contract for two changes to the auth module:
//
//   1. A new `dto.UserSummary` type with the JSON shape:
//        { "id", "username", "display_name", "email", "avatar?" }
//
//   2. A new `dto.CurrentUserOutput.ImpersonatedBy *UserSummary` field with
//      the JSON tag `json:"impersonated_by,omitempty"`. When the request was
//      made under an active impersonation session, the handler must populate
//      this field with a UserSummary describing the original admin (the
//      impersonator). When NOT impersonating, the field is omitted.
//
// Tests 1–4 are pure DTO/JSON tests — they will fail at compile time until
// the new types are added.
//
// Test 5 covers the handler behaviour (populating `ImpersonatedBy` from
// `ctx.GetString("impersonatorId")` via Casdoor lookup). It depends on a
// small injection point in production code: a package-level function variable
// `userController.LookupCasdoorUser` that the test can swap. The pattern
// mirrors `casdoorUserCache.fetchUserByID` in
// `src/groups/entityRegistration/groupMemberRegistration.go`.
// ---------------------------------------------------------------------------

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/dto"
	userController "soli/formations/src/auth/routes/usersRoutes"
)

// ---------------------------------------------------------------------------
// 1. UserSummary JSON shape
// ---------------------------------------------------------------------------

func TestUserSummary_StructFields(t *testing.T) {
	summary := &dto.UserSummary{
		ID:          "admin-uuid",
		Username:    "alice",
		DisplayName: "Alice Admin",
		Email:       "alice@example.com",
		Avatar:      "https://example.com/avatar.png",
	}

	jsonBytes, err := json.Marshal(summary)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	assert.Equal(t, "admin-uuid", parsed["id"])
	assert.Equal(t, "alice", parsed["username"])
	assert.Equal(t, "Alice Admin", parsed["display_name"])
	assert.Equal(t, "alice@example.com", parsed["email"])
	assert.Equal(t, "https://example.com/avatar.png", parsed["avatar"])

	// Ensure the JSON object has exactly these keys (snake_case)
	expectedKeys := map[string]struct{}{
		"id": {}, "username": {}, "display_name": {}, "email": {}, "avatar": {},
	}
	for k := range parsed {
		_, ok := expectedKeys[k]
		assert.True(t, ok, "unexpected key in UserSummary JSON: %q", k)
	}
}

// ---------------------------------------------------------------------------
// 2. ImpersonatedBy is omitted when nil
// ---------------------------------------------------------------------------

func TestCurrentUserOutput_ImpersonatedBy_OmitemptyWhenNil(t *testing.T) {
	output := &dto.CurrentUserOutput{
		UserID:         "target-uuid",
		UserName:       "bob",
		DisplayName:    "Bob",
		Email:          "bob@example.com",
		Roles:          []string{"member"},
		ImpersonatedBy: nil,
	}

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.NotContains(t, jsonStr, "impersonated_by",
		"impersonated_by must be omitted from JSON when ImpersonatedBy is nil (omitempty contract)")

	// Defensive parse: also confirm via map that the key is absent.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))
	_, ok := parsed["impersonated_by"]
	assert.False(t, ok, "impersonated_by key must not appear in JSON when nil")
}

// ---------------------------------------------------------------------------
// 3. ImpersonatedBy is included when set
// ---------------------------------------------------------------------------

func TestCurrentUserOutput_ImpersonatedBy_IncludedWhenSet(t *testing.T) {
	output := &dto.CurrentUserOutput{
		UserID:      "target-uuid",
		UserName:    "bob",
		DisplayName: "Bob",
		Email:       "bob@example.com",
		Roles:       []string{"member"},
		ImpersonatedBy: &dto.UserSummary{
			ID:          "admin-uuid",
			Username:    "alice",
			DisplayName: "Alice",
			Email:       "alice@example.com",
		},
	}

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"impersonated_by"`,
		"impersonated_by key must be present in JSON when ImpersonatedBy is set")
	assert.Contains(t, jsonStr, `"alice@example.com"`,
		"impersonator's email must be marshalled inside impersonated_by")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	impersonatedBy, ok := parsed["impersonated_by"].(map[string]any)
	require.True(t, ok, "impersonated_by must marshal to a JSON object")
	assert.Equal(t, "admin-uuid", impersonatedBy["id"])
	assert.Equal(t, "alice", impersonatedBy["username"])
	assert.Equal(t, "Alice", impersonatedBy["display_name"])
	assert.Equal(t, "alice@example.com", impersonatedBy["email"])
}

// ---------------------------------------------------------------------------
// 4. UserSummary.Avatar uses omitempty
// ---------------------------------------------------------------------------

func TestCurrentUserOutput_ImpersonatedBy_AvatarOmitempty(t *testing.T) {
	summary := &dto.UserSummary{
		ID:          "admin-uuid",
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		Avatar:      "",
	}

	jsonBytes, err := json.Marshal(summary)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.NotContains(t, jsonStr, "avatar",
		"avatar must be omitted from UserSummary JSON when empty (omitempty contract)")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))
	_, ok := parsed["avatar"]
	assert.False(t, ok, "avatar key must not appear when empty")
}

// ---------------------------------------------------------------------------
// 5. GetCurrentUser handler populates ImpersonatedBy from impersonatorId
//
// Contract for the production handler change:
//   * After computing the standard CurrentUserOutput for the (possibly
//     swapped) target user, the handler reads ctx.GetString("impersonatorId").
//   * If non-empty, it looks up the impersonator via the package-level
//     function variable `userController.LookupCasdoorUser` (default:
//     casdoorsdk.GetUserByUserId) and populates output.ImpersonatedBy with a
//     UserSummary built from the returned casdoor user.
//
// The injection point (`LookupCasdoorUser`) mirrors the pattern used by
// casdoorUserCache.fetchUserByID in groupMemberRegistration.go, so it is
// idiomatic for this codebase.
//
// If the production code chooses a different injection mechanism (interface,
// service, etc.), this test should be updated to use it. Until LookupCasdoorUser
// (or an equivalent swap point) exists, this test will fail to compile — that
// is the intended TDD signal.
// ---------------------------------------------------------------------------

func TestGetCurrentUser_WithImpersonatorContext_PopulatesImpersonatedBy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping handler test in short mode")
	}

	const (
		targetUserID    = "target-uuid"
		impersonatorID  = "admin-uuid"
		targetUsername  = "bob"
		targetDisplay   = "Bob"
		targetEmail     = "bob@example.com"
		adminUsername   = "alice"
		adminDisplay    = "Alice"
		adminEmail      = "alice@example.com"
	)

	// Swap the casdoor lookup with a fake that knows about both users.
	originalLookup := userController.LookupCasdoorUser
	userController.LookupCasdoorUser = func(id string) (*casdoorsdk.User, error) {
		switch id {
		case targetUserID:
			return &casdoorsdk.User{
				Id:          targetUserID,
				Name:        targetUsername,
				DisplayName: targetDisplay,
				Email:       targetEmail,
			}, nil
		case impersonatorID:
			return &casdoorsdk.User{
				Id:          impersonatorID,
				Name:        adminUsername,
				DisplayName: adminDisplay,
				Email:       adminEmail,
			}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() {
		userController.LookupCasdoorUser = originalLookup
	})

	// Build a gin context that simulates the post-impersonation state:
	// userId points to the target (the swap already happened), and
	// impersonatorId carries the original admin's id.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", targetUserID)
		c.Set("userRoles", []string{"member"})
		c.Set("impersonatorId", impersonatorID)
		c.Set("impersonatorRoles", []string{"administrator"})
	})
	r.GET("/auth/me", userController.GetCurrentUser)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"GetCurrentUser must return 200 when called under a valid impersonation context; body=%s",
		w.Body.String())

	body := w.Body.Bytes()

	// The response must mention the impersonated_by key (sanity check on raw
	// bytes — guards against the field being silently absent).
	require.True(t, strings.Contains(string(body), `"impersonated_by"`),
		"response body must contain impersonated_by key; got: %s", string(body))

	var output dto.CurrentUserOutput
	require.NoError(t, json.Unmarshal(body, &output))

	// The standard fields must reflect the TARGET user (not the impersonator),
	// because by the time GetCurrentUser runs the middleware has already
	// swapped userId.
	assert.Equal(t, targetUserID, output.UserID, "UserID must be the target's id")
	assert.Equal(t, targetUsername, output.UserName, "UserName must be the target's username")
	assert.Equal(t, targetEmail, output.Email, "Email must be the target's email")

	// The new impersonated_by field must describe the ORIGINAL admin.
	require.NotNil(t, output.ImpersonatedBy,
		"ImpersonatedBy must be populated when impersonatorId is set in the context")
	assert.Equal(t, impersonatorID, output.ImpersonatedBy.ID)
	assert.Equal(t, adminUsername, output.ImpersonatedBy.Username)
	assert.Equal(t, adminDisplay, output.ImpersonatedBy.DisplayName)
	assert.Equal(t, adminEmail, output.ImpersonatedBy.Email)
}
