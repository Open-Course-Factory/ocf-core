// tests/auth/mySettingsPatch_test.go
//
// GET /users/me/settings creates the row with defaults on first read, but
// PATCH /users/me/settings only loads it, so a PATCH before any GET answers
// 404. The settings embedded in the current-user payload (fetchUserSettings)
// are hand-mapped and drop recording_acknowledged_at.
package auth_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	userController "soli/formations/src/auth/routes/usersRoutes"
	sqldb "soli/formations/src/db"
)

// defaultSettingKeys are the fields every settings-creation path fills with defaults.
var defaultSettingKeys = []string{
	"default_landing_page", "preferred_language", "timezone", "theme",
	"compact_mode", "email_notifications", "desktop_notifications", "two_factor_enabled",
}

// setupMySettingsRoutes points the production handlers at a fresh in-memory DB
// with the real UserSettings registration, and restores the globals afterwards.
func setupMySettingsRoutes(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	registerUserSettingsForReadScope(t)
	db := setupUserSettingsReadScopeDB(t)
	prevDB := sqldb.DB
	sqldb.DB = db
	t.Cleanup(func() { sqldb.DB = prevDB })

	uc := userController.NewUserControllerWithServices(nil, nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userId", c.GetHeader("X-Test-User")) })
	r.GET("/users/me/settings", uc.GetMySettings)
	r.PATCH("/users/me/settings", uc.UpdateMySettings)
	return r, db
}

func callMySettings(t *testing.T, r *gin.Engine, method, userID, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/users/me/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func pickDefaults(m map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range defaultSettingKeys {
		out[k] = m[k]
	}
	return out
}

func TestUpdateMySettings_BeforeAnyGet_CreatesSettingsWithDefaults(t *testing.T) {
	r, db := setupMySettingsRoutes(t)

	code, body := callMySettings(t, r, http.MethodPatch, "first-patch-user", `{"theme":"dark"}`)

	require.Equal(t, http.StatusOK, code, "body: %v", body)
	assert.Equal(t, "dark", body["theme"])
	assert.Equal(t, "/dashboard", body["default_landing_page"])
	assert.Equal(t, "en", body["preferred_language"])
	assert.Equal(t, true, body["email_notifications"])

	var count int64
	require.NoError(t, db.Model(&authModels.UserSettings{}).Where("user_id = ?", "first-patch-user").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestFetchUserSettings_RecordingAcknowledged_ReturnsIt(t *testing.T) {
	db := setupCurrentUserTestDB(t)
	ackAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&authModels.UserSettings{UserID: "ack-user", RecordingAcknowledgedAt: &ackAt}).Error)

	settings, err := userController.FetchUserSettings(db, "ack-user")

	require.NoError(t, err)
	require.NotNil(t, settings.RecordingAcknowledgedAt, "current-user payload must carry recording_acknowledged_at")
	assert.Equal(t, ackAt, settings.RecordingAcknowledgedAt.UTC().Truncate(time.Second))
}

func TestMySettings_DefaultsAreTheSameOnEveryCreationPath(t *testing.T) {
	r, db := setupMySettingsRoutes(t)

	code, fromGet := callMySettings(t, r, http.MethodGet, "via-get", "")
	require.Equal(t, http.StatusOK, code)

	// Patch a field outside the default set so every default stays observable.
	code, fromPatch := callMySettings(t, r, http.MethodPatch, "via-patch", `{"recording_acknowledged_at":"2026-01-01T00:00:00Z"}`)
	require.Equal(t, http.StatusOK, code, "body: %v", fromPatch)

	current, err := userController.FetchUserSettings(db, "via-current-user")
	require.NoError(t, err)
	raw, err := json.Marshal(current)
	require.NoError(t, err)
	var fromCurrentUser map[string]any
	require.NoError(t, json.Unmarshal(raw, &fromCurrentUser))

	assert.Equal(t, pickDefaults(fromGet), pickDefaults(fromCurrentUser))
	assert.Equal(t, pickDefaults(fromGet), pickDefaults(fromPatch))
}
