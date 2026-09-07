package terminalTrainer_tests

// #501: POST /user-terminal-keys/regenerate is self-scoped and irreversible
// (the old key is disabled, a new one minted). Under impersonation the
// handler acted as the target, so an admin "acting as" a learner rotated that
// learner's key. It must refuse before touching either database.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

func TestRegenerateKey_DuringImpersonation_ForbiddenAndKeyUntouched(t *testing.T) {
	db := freshTestDB(t)
	key, err := createTestUserKey(db, "victim-501")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("userId", "victim-501")
	ctx.Set("impersonatorId", "admin-501")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/user-terminal-keys/regenerate", nil)

	terminalController.NewUserTerminalKeyController(db).RegenerateKey(ctx)

	assert.Equal(t, http.StatusForbidden, w.Code, "Body: %s", w.Body.String())

	var stored models.UserTerminalKey
	require.NoError(t, db.First(&stored, "id = ?", key.ID).Error)
	assert.True(t, stored.IsActive, "the refused regeneration must not disable the existing key")
	assert.Equal(t, key.APIKey, stored.APIKey)
}
