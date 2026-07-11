package auth_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authController "soli/formations/src/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestGetEntityIdFromContext_EmptyId_ReturnsBadRequest verifies that a request
// reaching the permission middleware without an :id route param results in a
// 400 response and a clean (uuid.Nil, false) return — NOT a process-killing
// log.Fatal / os.Exit(1). A malformed request must never take the server down.
func TestGetEntityIdFromContext_EmptyId_ReturnsBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	// No "id" param set — ctx.Param("id") returns "".

	id, ok := authController.GetEntityIdFromContext(ctx)

	assert.False(t, ok, "expected ok=false when the id param is empty")
	assert.Equal(t, uuid.Nil, id, "expected uuid.Nil when the id param is empty")
	assert.Equal(t, http.StatusBadRequest, recorder.Code, "expected HTTP 400 when the id param is empty")
}
