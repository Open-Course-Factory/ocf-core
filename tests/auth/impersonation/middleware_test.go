package impersonation_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "soli/formations/src/auth"
	authMiddleware "soli/formations/src/auth/middleware"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
)

// ---------------------------------------------------------------------------
// ImpersonationMiddleware tests (Slice B2 — TDD)
//
// These tests describe the contract for the upcoming
// authMiddleware.ImpersonationMiddleware and the supporting
// authMiddleware.RolesResolver type. The middleware does not yet exist;
// running these tests must fail at compile time until Slice B3 implements
// the production code.
// ---------------------------------------------------------------------------

const (
	mwAdminUser    = "admin-mw-1"
	mwTargetUser   = "user-mw-1"
	mwOtherTarget  = "user-mw-2"
	mwTestIP       = "10.0.0.7"
	mwTestUA       = "Mozilla/5.0 (mw-test)"
	probeRoute     = "/probe"
	headerName     = "X-Impersonate-User"
	roleAdminCanon = "administrator"
	roleAdminCased = "Administrator"
	roleMember     = "member"
)

// fakeAuth simulates AuthManagement by setting userId/userRoles on the gin
// context. Pass the empty string for userID to leave the context unchanged
// (i.e. simulate a request with no upstream auth middleware).
func fakeAuth(userID string, roles []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
			c.Set("userRoles", roles)
		}
		c.Next()
	}
}

// fakeRolesFor returns a RolesResolver backed by a static map.
func fakeRolesFor(rolesByID map[string][]string) authMiddleware.RolesResolver {
	return func(uid string) ([]string, error) {
		return rolesByID[uid], nil
	}
}

// errorRolesResolver always returns an error.
func errorRolesResolver(err error) authMiddleware.RolesResolver {
	return func(uid string) ([]string, error) {
		return nil, err
	}
}

// probePayload mirrors what the sentinel handler writes back so each test can
// assert on what the downstream handler observed in the gin context after
// ImpersonationMiddleware ran.
type probePayload struct {
	UserID            string   `json:"user_id"`
	UserRoles         []string `json:"user_roles"`
	ImpersonatorID    string   `json:"impersonator_id"`
	ImpersonatorRoles []string `json:"impersonator_roles"`
}

// sentinelHandler writes back what it sees in the gin context so tests can
// assert that the middleware swapped (or did not swap) the identity.
func sentinelHandler(c *gin.Context) {
	c.JSON(http.StatusOK, probePayload{
		UserID:            c.GetString("userId"),
		UserRoles:         c.GetStringSlice("userRoles"),
		ImpersonatorID:    c.GetString("impersonatorId"),
		ImpersonatorRoles: c.GetStringSlice("impersonatorRoles"),
	})
}

// buildRouter wires up: fakeAuth (simulating AuthManagement) → middleware
// under test → sentinel handler at /probe. It returns the engine ready to
// serve requests.
func buildRouter(callerID string, callerRoles []string, svc services.ImpersonationService, resolver authMiddleware.RolesResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(fakeAuth(callerID, callerRoles))
	r.Use(authMiddleware.ImpersonationMiddleware(svc, resolver))
	r.GET(probeRoute, sentinelHandler)
	return r
}

// decodeError unmarshals a JSON `{"error": "..."}` body and returns the
// "error" field.
func decodeError(t *testing.T, body []byte) string {
	t.Helper()
	var resp map[string]string
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp["error"]
}

// decodeProbe unmarshals the sentinel handler's payload.
func decodeProbe(t *testing.T, body []byte) probePayload {
	t.Helper()
	var p probePayload
	require.NoError(t, json.Unmarshal(body, &p))
	return p
}

// ---------------------------------------------------------------------------
// 1. No header → passthrough
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_NoHeader_Passthrough(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := services.NewImpersonationService(freshTestDB(t))
	resolver := fakeRolesFor(nil)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	got := decodeProbe(t, w.Body.Bytes())
	assert.Equal(t, mwAdminUser, got.UserID, "userId must be unchanged when no impersonation header is present")
	assert.Equal(t, []string{roleAdminCanon}, got.UserRoles, "userRoles must be unchanged when no impersonation header is present")
}

// ---------------------------------------------------------------------------
// 2. No header → impersonator metadata MUST NOT leak into the context
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_NoHeader_DoesNotTouchContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := services.NewImpersonationService(freshTestDB(t))
	resolver := fakeRolesFor(nil)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := decodeProbe(t, w.Body.Bytes())
	assert.Empty(t, got.ImpersonatorID, "impersonatorId must NOT be set when there is no impersonation header")
	assert.Empty(t, got.ImpersonatorRoles, "impersonatorRoles must NOT be set when there is no impersonation header")
}

// ---------------------------------------------------------------------------
// 3. Header but no upstream auth → 401 unauthenticated
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_HeaderWithoutAuth_Returns401Unauthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := services.NewImpersonationService(freshTestDB(t))
	resolver := fakeRolesFor(nil)

	// Empty callerID → fakeAuth does not populate userId/userRoles.
	r := buildRouter("", nil, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "unauthenticated", decodeError(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// 4. Header + caller is not admin → 403 impersonation_forbidden
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_NonAdmin_Returns403Forbidden(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := services.NewImpersonationService(freshTestDB(t))
	resolver := fakeRolesFor(nil)

	r := buildRouter("regular-user", []string{roleMember}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "impersonation_forbidden", decodeError(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// 5. Admin caller, but no active session → 401 impersonation_invalid
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_AdminButNoActiveSession_Returns401Invalid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := services.NewImpersonationService(freshTestDB(t))
	resolver := fakeRolesFor(nil)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "impersonation_invalid", decodeError(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// 6. Admin caller, active session, but header value != session.TargetID
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_TargetMismatch_Returns401Invalid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	resolver := fakeRolesFor(nil)

	// Admin started a session targeting mwTargetUser, but the request asks
	// to impersonate mwOtherTarget.
	_, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwOtherTarget)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "impersonation_invalid", decodeError(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// 7. Admin caller, active session matches, but the session is stale
//
// Expectation: 401 impersonation_expired AND the session row is closed
// (StopSession with reason "expired"), so a follow-up GetActiveSession returns
// ErrNoActiveSession.
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_StaleSession_Returns401Expired_AndClosesSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	resolver := fakeRolesFor(nil)

	session, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	// Force the session to look idle for longer than ImpersonationIdleTimeout.
	past := time.Now().Add(-(services.ImpersonationIdleTimeout + time.Minute))
	require.NoError(t,
		db.Model(&authModels.ImpersonationSession{}).
			Where("id = ?", session.ID).
			Update("last_activity_at", past).Error,
	)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "impersonation_expired", decodeError(t, w.Body.Bytes()))

	// The middleware MUST close the session: GetActiveSession should now
	// return ErrNoActiveSession.
	_, getErr := svc.GetActiveSession(mwAdminUser)
	assert.ErrorIs(t, getErr, services.ErrNoActiveSession,
		"middleware must close the stale session via StopSession; no active session should remain")

	// Defensively check the row is marked ended with reason "expired".
	var stored authModels.ImpersonationSession
	require.NoError(t, db.First(&stored, "id = ?", session.ID).Error)
	assert.NotNil(t, stored.EndedAt, "stale session must be marked ended")
	assert.Equal(t, "expired", stored.EndReason)
}

// ---------------------------------------------------------------------------
// 8. Admin caller, valid fresh session → context is swapped, Next runs
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_ValidSession_SwapsContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	targetRoles := []string{roleMember}
	resolver := fakeRolesFor(map[string][]string{
		mwTargetUser: targetRoles,
	})

	_, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	adminRoles := []string{roleAdminCanon}
	r := buildRouter(mwAdminUser, adminRoles, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "valid impersonation must let the request through")
	got := decodeProbe(t, w.Body.Bytes())

	assert.Equal(t, mwTargetUser, got.UserID, "downstream handler must see the target's userId")
	assert.Equal(t, targetRoles, got.UserRoles, "downstream handler must see the target's roles (from RolesResolver)")
	assert.Equal(t, mwAdminUser, got.ImpersonatorID, "impersonatorId must be the original admin userId")
	assert.Equal(t, adminRoles, got.ImpersonatorRoles, "impersonatorRoles must be the original admin roles")
}

// ---------------------------------------------------------------------------
// 9. Valid session → middleware bumps LastActivityAt (Touch)
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_ValidSession_TouchesSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	resolver := fakeRolesFor(map[string][]string{
		mwTargetUser: {roleMember},
	})

	session, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)
	originalActivity := session.LastActivityAt

	// Tiny sleep so the post-request timestamp is provably later even on
	// platforms with coarse monotonic clock granularity.
	time.Sleep(2 * time.Millisecond)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var refreshed authModels.ImpersonationSession
	require.NoError(t, db.First(&refreshed, "id = ?", session.ID).Error)
	assert.True(t, refreshed.LastActivityAt.After(originalActivity),
		"middleware must call Touch to bump LastActivityAt; got %v <= %v",
		refreshed.LastActivityAt, originalActivity,
	)
}

// ---------------------------------------------------------------------------
// 10. Resolver returns an error → 500 impersonation_role_lookup_failed
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_ResolveRolesError_Returns500(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	resolver := errorRolesResolver(assertSentinelError("casdoor unreachable"))

	_, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	r := buildRouter(mwAdminUser, []string{roleAdminCanon}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "impersonation_role_lookup_failed", decodeError(t, w.Body.Bytes()))
}

// ---------------------------------------------------------------------------
// 11. Admin role detection is case-insensitive ("Administrator" works too)
// ---------------------------------------------------------------------------

func TestImpersonationMiddleware_AdminUppercase_StillRecognized(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	targetRoles := []string{roleMember}
	resolver := fakeRolesFor(map[string][]string{
		mwTargetUser: targetRoles,
	})

	_, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	// Caller has the capitalised "Administrator" form (matches Casdoor casing).
	r := buildRouter(mwAdminUser, []string{roleAdminCased}, svc, resolver)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"the middleware must accept any case-form of \"administrator\" (delegates to access.IsAdmin)")
	got := decodeProbe(t, w.Body.Bytes())
	assert.Equal(t, mwTargetUser, got.UserID)
	assert.Equal(t, targetRoles, got.UserRoles)
	assert.Equal(t, mwAdminUser, got.ImpersonatorID)
}

// ---------------------------------------------------------------------------
// 12. End-to-end wiring: auth.SetImpersonationHandler installs a handler that
// behaves correctly when invoked after AuthManagement has populated the
// context. This is the contract the production code relies on:
//
//   AuthManagement: ... ctx.Set("userId", ...); ctx.Set("userRoles", ...);
//                   if impersonationHandler != nil { impersonationHandler(ctx) }
//
// The test installs the same handler via the package-level setter, simulates
// the userId/userRoles assignment that happens at the end of AuthManagement,
// invokes the handler, and asserts that the identity swap was performed.
// ---------------------------------------------------------------------------

func TestImpersonationViaAuthManagement_E2E_SwapsContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	targetRoles := []string{roleMember}
	resolver := fakeRolesFor(map[string][]string{
		mwTargetUser: targetRoles,
	})

	_, err := svc.StartSession(mwAdminUser, mwTargetUser, mwTestIP, mwTestUA)
	require.NoError(t, err)

	// Install the impersonation handler at the package level (mirrors what
	// main.go does at startup via authController.SetImpersonationHandler).
	handler := authMiddleware.ImpersonationMiddleware(svc, resolver)
	auth.SetImpersonationHandler(handler)
	t.Cleanup(func() { auth.SetImpersonationHandler(nil) })

	// Build a tiny router whose middleware mimics the END of AuthManagement:
	// set userId / userRoles, then invoke the installed impersonation handler.
	// We invoke `handler` directly (the same callable we passed to the setter)
	// — this proves that what production stored on the package var is the
	// thing that performs the swap.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	adminRoles := []string{roleAdminCanon}
	r.Use(func(c *gin.Context) {
		c.Set("userId", mwAdminUser)
		c.Set("userRoles", adminRoles)
		handler(c)
	})
	r.GET(probeRoute, sentinelHandler)

	req := httptest.NewRequest(http.MethodGet, probeRoute, nil)
	req.Header.Set(headerName, mwTargetUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"AuthManagement-style chain + impersonation handler must let the request through")
	got := decodeProbe(t, w.Body.Bytes())

	assert.Equal(t, mwTargetUser, got.UserID,
		"downstream handler must see the target's userId after the swap")
	assert.Equal(t, targetRoles, got.UserRoles,
		"downstream handler must see the target's roles after the swap")
	assert.Equal(t, mwAdminUser, got.ImpersonatorID,
		"impersonatorId must be the original admin userId")
	assert.Equal(t, adminRoles, got.ImpersonatorRoles,
		"impersonatorRoles must be the original admin roles")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertSentinelError is a tiny wrapper so we don't need to import "errors"
// just to build a sentinel for the role-resolver-error test.
type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

func assertSentinelError(msg string) error { return sentinelErr(msg) }
