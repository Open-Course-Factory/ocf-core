// tests/payment/injectOrgContext_membership_test.go
//
// Security regression tests: verifies that InjectOrgContext validates that the
// authenticated user is actually a member of the organization they are claiming
// via the organization_id query parameter or request body.
//
// Background (issue #242):
//   InjectOrgContext currently reads organization_id from the request and injects
//   it into the Gin context without verifying that the requesting user belongs to
//   that organization. Any authenticated user can pass ?organization_id=<X> and
//   the middleware will happily inject it, letting them route their session through
//   org X's quota and backend infrastructure.
//
// These tests FAIL before the fix (middleware accepts any org_id regardless of
// membership) and PASS after the fix (middleware rejects non-members with 403).
package payment_tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	paymentMiddleware "soli/formations/src/payment/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockMembershipChecker is a testify mock that implements access.MembershipChecker.
// It allows tests to control whether CheckOrgRole returns true or false without
// a real database.
type mockMembershipChecker struct {
	mock.Mock
}

func (m *mockMembershipChecker) CheckOrgRole(orgID string, userID string, minRole string) (bool, error) {
	args := m.Called(orgID, userID, minRole)
	return args.Bool(0), args.Error(1)
}

func (m *mockMembershipChecker) CheckGroupRole(groupID string, userID string, minRole string) (bool, error) {
	args := m.Called(groupID, userID, minRole)
	return args.Bool(0), args.Error(1)
}

// TestInjectOrgContext_NonMember_ShouldReject verifies that a user who is NOT a
// member of the requested organization is rejected with 403 Forbidden.
//
// EXPECTED: 403 Forbidden, org_context_id NOT set in context.
// CURRENT BUG: 200 OK, org_context_id is set — the non-member silently escalates.
func TestInjectOrgContext_NonMember_ShouldReject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	attackerUserID := "attacker-user-id"

	checker := new(mockMembershipChecker)
	// Attacker is NOT a member of the org — CheckOrgRole returns false
	checker.On("CheckOrgRole", orgID.String(), attackerUserID, "member").Return(false, nil)

	orgContextSet := false
	router := gin.New()
	router.GET("/test", func(ctx *gin.Context) {
		ctx.Set("userId", attackerUserID)
	}, paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		_, orgContextSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?organization_id="+orgID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"non-member should be rejected with 403 when requesting an org context they don't belong to")
	assert.False(t, orgContextSet,
		"org_context_id must NOT be injected into the context for a non-member")
	checker.AssertExpectations(t)
}

// TestInjectOrgContext_Member_ShouldInject verifies that a user who IS a member
// of the requested organization has org_context_id correctly injected.
//
// EXPECTED: 200 OK, org_context_id set to the organization's UUID.
func TestInjectOrgContext_Member_ShouldInject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	memberUserID := "legitimate-member-id"

	checker := new(mockMembershipChecker)
	// Member IS a member of the org — CheckOrgRole returns true
	checker.On("CheckOrgRole", orgID.String(), memberUserID, "member").Return(true, nil)

	var capturedOrgID string
	router := gin.New()
	router.GET("/test", func(ctx *gin.Context) {
		ctx.Set("userId", memberUserID)
	}, paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		val, exists := ctx.Get("org_context_id")
		if exists {
			capturedOrgID = val.(string)
		}
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?organization_id="+orgID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"organization member should get 200 OK")
	assert.Equal(t, orgID.String(), capturedOrgID,
		"org_context_id should be injected for a legitimate member")
	checker.AssertExpectations(t)
}

// TestInjectOrgContext_NoOrgParam_ShouldPassThrough verifies that when no
// organization_id is provided, the middleware passes through without calling
// the membership checker and without rejecting the request.
func TestInjectOrgContext_NoOrgParam_ShouldPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := "any-user-id"

	checker := new(mockMembershipChecker)
	// No org param → membership checker should NOT be called at all
	// (no mock setup = any call would cause the test to fail via unexpected call)

	orgContextSet := false
	router := gin.New()
	router.GET("/test", func(ctx *gin.Context) {
		ctx.Set("userId", userID)
	}, paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		_, orgContextSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"request without organization_id should pass through normally")
	assert.False(t, orgContextSet,
		"org_context_id should not be set when no organization_id is provided")
	checker.AssertExpectations(t)
}

// TestInjectOrgContext_NonMember_ViaBody_ShouldReject verifies that the membership
// check also applies when organization_id is sent in the JSON body (POST requests),
// not just as a query parameter.
func TestInjectOrgContext_NonMember_ViaBody_ShouldReject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	attackerUserID := "attacker-post-user"

	checker := new(mockMembershipChecker)
	checker.On("CheckOrgRole", orgID.String(), attackerUserID, "member").Return(false, nil)

	orgContextSet := false
	router := gin.New()
	router.POST("/test", func(ctx *gin.Context) {
		ctx.Set("userId", attackerUserID)
	}, paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		_, orgContextSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	body := `{"organization_id": "` + orgID.String() + `"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"non-member should be rejected with 403 even when organization_id is in the request body")
	assert.False(t, orgContextSet,
		"org_context_id must NOT be injected for a non-member even via POST body")
	checker.AssertExpectations(t)
}

// TestInjectOrgContext_MembershipCheckError_ShouldReject verifies that if the
// membership checker returns an error (e.g., database failure), the middleware
// rejects the request rather than silently granting access.
func TestInjectOrgContext_MembershipCheckError_ShouldReject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	userID := "some-user-id"

	checker := new(mockMembershipChecker)
	// Simulate a database error during membership lookup
	checker.On("CheckOrgRole", orgID.String(), userID, "member").
		Return(false, errors.New("database connection lost"))

	orgContextSet := false
	router := gin.New()
	router.GET("/test", func(ctx *gin.Context) {
		ctx.Set("userId", userID)
	}, paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		_, orgContextSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?organization_id="+orgID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a membership check error must NOT silently grant org context — fail closed")
	assert.False(t, orgContextSet,
		"org_context_id must NOT be set when the membership check fails with an error")
	checker.AssertExpectations(t)
}

// TestInjectOrgContext_NoUserID_ShouldPassThrough verifies that when no userId is
// in the context (unauthenticated request), the middleware does not attempt membership
// checking and defers to auth middleware to reject. This matches the behavior of
// InjectEffectivePlan which also skips processing when userId is absent.
func TestInjectOrgContext_NoUserID_ShouldPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()

	checker := new(mockMembershipChecker)
	// No userId in context → membership checker should NOT be called

	orgContextSet := false
	router := gin.New()
	// Note: no middleware that sets "userId" in ctx — simulates unauthenticated request
	router.GET("/test", paymentMiddleware.InjectOrgContext(checker), func(ctx *gin.Context) {
		_, orgContextSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?organization_id="+orgID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"unauthenticated request should pass through (auth middleware handles rejection)")
	assert.False(t, orgContextSet,
		"org_context_id should not be set when there is no authenticated user")
	checker.AssertExpectations(t)
}
