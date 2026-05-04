package impersonation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	"soli/formations/src/auth/routes/impersonationRoutes"
	"soli/formations/src/auth/services"
)

// ---------------------------------------------------------------------------
// ImpersonationController tests (Slice B3 — TDD)
//
// These tests describe the contract for the upcoming
// impersonationRoutes.NewController and its three handlers:
//   POST /admin/impersonate/start
//   POST /admin/impersonate/stop
//   GET  /admin/impersonate/active
//
// The controller does not yet exist; running these tests must fail at compile
// time until the next slice implements the production code.
// ---------------------------------------------------------------------------

const (
	ctrlAdminUser   = "admin-ctrl-1"
	ctrlTargetUser  = "user-ctrl-1"
	ctrlOtherTarget = "user-ctrl-2"
	ctrlClientIP    = "203.0.113.42"
	ctrlUserAgent   = "Mozilla/5.0 (controller-test)"

	startPath  = "/admin/impersonate/start"
	stopPath   = "/admin/impersonate/stop"
	activePath = "/admin/impersonate/active"

	headerImpersonate = "X-Impersonate-User"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeUserValidator is the test stub for impersonationRoutes.UserValidator.
type fakeUserValidator struct {
	knownUsers map[string]bool
	err        error
}

func (f *fakeUserValidator) UserExists(userID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.knownUsers[userID], nil
}

func (f *fakeUserValidator) GetUser(userID string) (*impersonationRoutes.TargetUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	if !f.knownUsers[userID] {
		return nil, nil
	}
	return &impersonationRoutes.TargetUser{
		ID:          userID,
		Username:    "fake-" + userID,
		DisplayName: "Fake " + userID,
		Email:       userID + "@example.com",
	}, nil
}

func newKnownValidator(ids ...string) *fakeUserValidator {
	known := make(map[string]bool, len(ids))
	for _, id := range ids {
		known[id] = true
	}
	return &fakeUserValidator{knownUsers: known}
}

// ---------------------------------------------------------------------------
// Helpers — context injection + router wiring
// ---------------------------------------------------------------------------

// ctxInjector simulates upstream middleware (AuthManagement +
// ImpersonationMiddleware) by populating the gin context with the values the
// controller reads.
type ctxInjector struct {
	userID         string
	userRoles      []string
	impersonatorID string // set when simulating post-ImpersonationMiddleware state
}

func (i ctxInjector) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if i.userID != "" {
			c.Set("userId", i.userID)
		}
		if len(i.userRoles) > 0 {
			c.Set("userRoles", i.userRoles)
		}
		if i.impersonatorID != "" {
			c.Set("impersonatorId", i.impersonatorID)
		}
		c.Next()
	}
}

// buildControllerRouter wires fakeAuth/ctx middleware → controller handlers.
// All three routes are registered so each test can hit the right one.
func buildControllerRouter(t *testing.T, ctrl *impersonationRoutes.Controller, inj ctxInjector) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(inj.middleware())
	r.POST(startPath, ctrl.StartImpersonation)
	r.POST(stopPath, ctrl.StopImpersonation)
	r.GET(activePath, ctrl.GetActiveImpersonation)
	return r
}

// startBody builds a JSON body for POST /start, given a target user id.
func startBody(targetID string) *bytes.Buffer {
	body, _ := json.Marshal(map[string]string{"target_user_id": targetID})
	return bytes.NewBuffer(body)
}

// startSuccessPayload is what the controller returns on a successful start.
type startSuccessPayload struct {
	SessionID    string                          `json:"session_id"`
	TargetUserID string                          `json:"target_user_id"`
	StartedAt    time.Time                       `json:"started_at"`
	Target       *impersonationRoutes.TargetUser `json:"target,omitempty"`
}

// activeSuccessPayload is what GET /active returns when a session is open.
type activeSuccessPayload struct {
	SessionID      string    `json:"session_id"`
	TargetUserID   string    `json:"target_user_id"`
	StartedAt      time.Time `json:"started_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// errPayload mirrors the {"error": "..."} contract.
type errPayload struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func decodeJSONErr(t *testing.T, body []byte) errPayload {
	t.Helper()
	var p errPayload
	require.NoError(t, json.Unmarshal(body, &p))
	return p
}

// newCtrl builds a fresh controller with a known-validator and a real service.
func newCtrl(t *testing.T, validator impersonationRoutes.UserValidator) (*impersonationRoutes.Controller, services.ImpersonationService, *gorm.DB) {
	t.Helper()
	db := freshTestDB(t)
	svc := services.NewImpersonationService(db)
	return impersonationRoutes.NewController(svc, validator), svc, db
}

// ---------------------------------------------------------------------------
// StartImpersonation
// ---------------------------------------------------------------------------

func TestStartImpersonation_Success_Returns201WithSessionInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code,
		"expected 201 Created on successful start; body=%s", w.Body.String())

	var got startSuccessPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.NotEmpty(t, got.SessionID, "session_id must be returned")
	_, err := uuid.Parse(got.SessionID)
	assert.NoError(t, err, "session_id must be a valid UUID")
	assert.Equal(t, ctrlTargetUser, got.TargetUserID)
	assert.False(t, got.StartedAt.IsZero(), "started_at must be a valid timestamp")
}

// TestStartImpersonation_Success_IncludesTargetProfile_InResponse asserts
// that /start returns the target user's profile alongside the session info,
// so the frontend can populate the impersonation banner without a follow-up
// lookup. Bug 2 from B3 manual testing — banner showed empty target name.
func TestStartImpersonation_Success_IncludesTargetProfile_InResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var got startSuccessPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	require.NotNil(t, got.Target, "response must include the target profile")
	assert.Equal(t, ctrlTargetUser, got.Target.ID,
		"target.id must match the requested target_user_id")
	assert.NotEmpty(t, got.Target.Username,
		"target.username must be populated by the validator lookup")
}

func TestStartImpersonation_Success_PersistsSessionInDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, db := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var stored authModels.ImpersonationSession
	require.NoError(t, db.
		Where("impersonator_id = ? AND ended_at IS NULL", ctrlAdminUser).
		First(&stored).Error,
		"a row must exist with this impersonator + nil ended_at")

	assert.Equal(t, ctrlAdminUser, stored.ImpersonatorID)
	assert.Equal(t, ctrlTargetUser, stored.TargetID)
}

func TestStartImpersonation_Success_RecordsClientIPAndUserAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, db := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	// Force the engine to honour X-Forwarded-For so ctx.ClientIP() returns it.
	r.SetTrustedProxies([]string{"0.0.0.0/0"}) //nolint:errcheck
	r.ForwardedByClientIP = true
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ctrlClientIP)
	req.Header.Set("User-Agent", ctrlUserAgent)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var stored authModels.ImpersonationSession
	require.NoError(t, db.
		Where("impersonator_id = ?", ctrlAdminUser).
		First(&stored).Error)

	assert.Equal(t, ctrlClientIP, stored.ActorIP,
		"controller must capture ctx.ClientIP() and pass it to StartSession")
	assert.Equal(t, ctrlUserAgent, stored.ActorUserAgent,
		"controller must capture User-Agent header and pass it to StartSession")
}

func TestStartImpersonation_HeaderPresent_Returns409Chaining(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	// The presence of this header indicates the caller is already inside an
	// impersonation context — chaining is forbidden.
	req.Header.Set(headerImpersonate, ctrlOtherTarget)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "impersonation_chaining_forbidden", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_MissingBody_Returns400(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	// No body at all.
	req := httptest.NewRequest(http.MethodPost, startPath, nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "invalid_request", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_MissingTargetUserID_Returns400(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	// Body is well-formed JSON but the target_user_id field is empty.
	req := httptest.NewRequest(http.MethodPost, startPath, strings.NewReader(`{"target_user_id":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "invalid_request", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_SelfTarget_Returns400(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlAdminUser)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	// target_user_id equals the caller's own id.
	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlAdminUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "self_impersonation_forbidden", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_TargetNotFound_Returns404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	// Validator does NOT know the target.
	validator := newKnownValidator()
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "target_not_found", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_UserValidatorError_Returns500(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := &fakeUserValidator{err: errors.New("casdoor unreachable")}
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "user_validation_failed", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStartImpersonation_AlreadyImpersonating_Returns409(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	// Both targets are known so the validator does not block either request.
	validator := newKnownValidator(ctrlTargetUser, ctrlOtherTarget)
	ctrl, _, _ := newCtrl(t, validator)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	// First start succeeds.
	first := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlTargetUser))
	first.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, first)
	require.Equal(t, http.StatusCreated, w1.Code, "first start must succeed; body=%s", w1.Body.String())

	// Second start (different target, same admin) must be rejected by the
	// service's ErrAlreadyImpersonating guard.
	second := httptest.NewRequest(http.MethodPost, startPath, startBody(ctrlOtherTarget))
	second.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, second)

	assert.Equal(t, http.StatusConflict, w2.Code)
	assert.Equal(t, "already_impersonating", decodeJSONErr(t, w2.Body.Bytes()).Error)
}

// ---------------------------------------------------------------------------
// StopImpersonation
// ---------------------------------------------------------------------------

func TestStopImpersonation_Success_Returns204_AndClosesSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, svc, _ := newCtrl(t, validator)

	// Pre-create an active session so Stop has something to close.
	_, err := svc.StartSession(ctrlAdminUser, ctrlTargetUser, ctrlClientIP, ctrlUserAgent)
	require.NoError(t, err)

	// AuthManagement always sets userId to the authenticated admin's id —
	// the handler reads userId, NOT impersonatorId.
	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, stopPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, "expected 204 No Content; body=%s", w.Body.String())

	// After stop, no active session must remain for this admin.
	_, getErr := svc.GetActiveSession(ctrlAdminUser)
	assert.ErrorIs(t, getErr, services.ErrNoActiveSession,
		"controller must close the session via service.StopSession")
}

func TestStopImpersonation_Success_RecordsManualReason(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, svc, db := newCtrl(t, validator)

	session, err := svc.StartSession(ctrlAdminUser, ctrlTargetUser, ctrlClientIP, ctrlUserAgent)
	require.NoError(t, err)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, stopPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())

	var stored authModels.ImpersonationSession
	require.NoError(t, db.First(&stored, "id = ?", session.ID).Error)
	assert.NotNil(t, stored.EndedAt, "ended_at must be set after a manual stop")
	assert.Equal(t, "manual", stored.EndReason,
		"controller must call StopSession with reason \"manual\"")
}

func TestStopImpersonation_NoUserId_Returns401Unauthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	// No userId injected — defence-in-depth case (AuthManagement should
	// always populate it in production, but the handler must not panic
	// or silently succeed if it is missing).
	r := buildControllerRouter(t, ctrl, ctxInjector{})

	req := httptest.NewRequest(http.MethodPost, stopPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "unauthenticated", decodeJSONErr(t, w.Body.Bytes()).Error)
}

func TestStopImpersonation_NoActiveSession_Returns404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	// Admin is authenticated (userId set) but there is no active session
	// row in the DB for them.
	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, stopPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "no_active_session", decodeJSONErr(t, w.Body.Bytes()).Error)
}

// TestStopImpersonation_HeaderNotPresent_StillStopsSessionByAuthenticatedAdmin
// proves the recovery contract: even when the frontend has lost its
// X-Impersonate-User header (e.g. after a transient 401 silent-stop or a
// page-navigation race), an admin can still call /stop and close their
// active session as long as they are authenticated. Bug 1 from B3 manual
// testing — DB row would otherwise stay open until the 30-min sweep.
func TestStopImpersonation_HeaderNotPresent_StillStopsSessionByAuthenticatedAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, svc, db := newCtrl(t, validator)

	// An active session exists in the DB for admin → target.
	session, err := svc.StartSession(ctrlAdminUser, ctrlTargetUser, ctrlClientIP, ctrlUserAgent)
	require.NoError(t, err)

	// Build a router that ONLY sets userId (no impersonatorId — simulating
	// the lost-header case). The request itself does not carry
	// X-Impersonate-User either.
	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodPost, stopPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code,
		"stop must succeed by authenticated admin id, regardless of header presence; body=%s", w.Body.String())

	var stored authModels.ImpersonationSession
	require.NoError(t, db.First(&stored, "id = ?", session.ID).Error)
	assert.NotNil(t, stored.EndedAt, "session row must be closed after stop")
	assert.Equal(t, "manual", stored.EndReason,
		"end reason must be \"manual\" for an explicit stop call")
}

// ---------------------------------------------------------------------------
// GetActiveImpersonation
// ---------------------------------------------------------------------------

func TestGetActiveImpersonation_NoSession_Returns204(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, _, _ := newCtrl(t, validator)

	// Admin queries their active session WITHOUT the impersonation header,
	// so userId is the admin's real id.
	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodGet, activePath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code,
		"no active session must yield 204; body=%s", w.Body.String())
}

func TestGetActiveImpersonation_HasSession_Returns200WithDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, svc, _ := newCtrl(t, validator)

	session, err := svc.StartSession(ctrlAdminUser, ctrlTargetUser, ctrlClientIP, ctrlUserAgent)
	require.NoError(t, err)

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodGet, activePath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var got activeSuccessPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Equal(t, session.ID.String(), got.SessionID)
	assert.Equal(t, ctrlTargetUser, got.TargetUserID)
	assert.False(t, got.StartedAt.IsZero(), "started_at must be returned")
	assert.False(t, got.LastActivityAt.IsZero(), "last_activity_at must be returned")
}

func TestGetActiveImpersonation_DoesNotReturnEndedSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	validator := newKnownValidator(ctrlTargetUser)
	ctrl, svc, _ := newCtrl(t, validator)

	_, err := svc.StartSession(ctrlAdminUser, ctrlTargetUser, ctrlClientIP, ctrlUserAgent)
	require.NoError(t, err)
	require.NoError(t, svc.StopSession(ctrlAdminUser, "manual"))

	r := buildControllerRouter(t, ctrl, ctxInjector{
		userID:    ctrlAdminUser,
		userRoles: []string{"administrator"},
	})

	req := httptest.NewRequest(http.MethodGet, activePath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code,
		"a previously-stopped session must not surface as active; body=%s", w.Body.String())
}
