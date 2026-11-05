// tests/terminalTrainer/disableUserKey_test.go
//
// DisableUserKey used to address tt-backend with the key value and without the
// required name, so tt-backend refused every call. The refusal was read as an
// orphaned key: ocf-core marked the key disabled while it stayed active.
package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

func localKeyIsActive(t *testing.T, db *gorm.DB, userID string) bool {
	t.Helper()
	var key models.UserTerminalKey
	require.NoError(t, db.Where("user_id = ?", userID).First(&key).Error)
	return key.IsActive
}

func TestDisableUserKey_DeactivatesTheKeyOnTTBackend(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, true)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-disable-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)

	require.NoError(t, services.NewTerminalTrainerService(db).DisableUserKey(userID))

	updates := srv.recordedUpdates()
	require.Len(t, updates, 1)
	assert.True(t, strings.HasSuffix(updates[0]["_path"].(string), "/admin/api-keys/42"), "addressed by tt-backend id, never by the key value")
	assert.Equal(t, false, updates[0]["is_active"])
	assert.Equal(t, "learner", updates[0]["name"], "tt-backend requires the name on every update")
	assert.Equal(t, true, updates[0]["limits_disabled"], "the operator switch survives the update")
	assert.False(t, localKeyIsActive(t, db, userID))
}

func TestDisableUserKey_KeyUnknownToTTBackend_DisablesLocally(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-disable-orphan-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)

	require.NoError(t, services.NewTerminalTrainerService(db).DisableUserKey(userID))

	assert.False(t, localKeyIsActive(t, db, userID), "an orphaned key must not block creating a new one")
}

func TestDisableUserKey_TTBackendFailure_ReportsAndKeepsTheKeyActiveLocally(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-disable-down-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)

	err := services.NewTerminalTrainerService(db).DisableUserKey(userID)

	require.Error(t, err, "a key still active on tt-backend must not be reported as revoked")
	assert.True(t, localKeyIsActive(t, db, userID), "the row stays active so a retry can still find the key")
}

func TestDisableUserKey_KeyWithoutTTBackendID_DisablesLocallyAndSendsNothing(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-disable-legacy-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 0)

	require.NoError(t, services.NewTerminalTrainerService(db).DisableUserKey(userID))

	assert.Empty(t, srv.recordedUpdates())
	assert.False(t, localKeyIsActive(t, db, userID), "regenerating is the documented way out for these keys")
}
