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
	"sync"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"soli/formations/src/auth/dto"
	registration "soli/formations/src/auth/entityRegistration"
	"soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	userController "soli/formations/src/auth/routes/usersRoutes"
	authServices "soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	controller "soli/formations/src/entityManagement/routes"
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
	assert.Equal(t, "/terminal-creation", body["default_landing_page"])
	assert.Equal(t, "fr", body["preferred_language"])
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

	// Signup is what every new account gets, so its values are the reference.
	t.Setenv("TERMINAL_TRAINER_URL", "") // keep the key-creation step offline
	authServices.ProvisionNewUser(mocks.NewMockEnforcer(), db, &casdoorsdk.User{Id: "via-signup", EmailVerified: true}, "")
	var signupRow authModels.UserSettings
	require.NoError(t, db.Where("user_id = ?", "via-signup").First(&signupRow).Error)
	raw, err = json.Marshal(registration.UserSettingsToOutput(&signupRow))
	require.NoError(t, err)
	var fromSignup map[string]any
	require.NoError(t, json.Unmarshal(raw, &fromSignup))

	assert.Equal(t, pickDefaults(fromSignup), pickDefaults(fromGet), "lazy GET vs signup")
	assert.Equal(t, pickDefaults(fromSignup), pickDefaults(fromPatch), "lazy PATCH vs signup")
	assert.Equal(t, pickDefaults(fromSignup), pickDefaults(fromCurrentUser), "current user vs signup")
}

func TestUpdateMySettings_SecondPatch_KeepsEarlierValuesAndMatchesStoredRow(t *testing.T) {
	r, db := setupMySettingsRoutes(t)

	code, _ := callMySettings(t, r, http.MethodPatch, "sparse-user", `{"theme":"dark"}`)
	require.Equal(t, http.StatusOK, code)
	code, body := callMySettings(t, r, http.MethodPatch, "sparse-user", `{"preferred_language":"fr"}`)
	require.Equal(t, http.StatusOK, code, "body: %v", body)

	var stored authModels.UserSettings
	require.NoError(t, db.Where("user_id = ?", "sparse-user").First(&stored).Error)
	assert.Equal(t, "dark", stored.Theme)
	assert.Equal(t, "fr", stored.PreferredLanguage)
	assert.Equal(t, "/terminal-creation", stored.DefaultLandingPage)
	assert.Equal(t, stored.Theme, body["theme"])
	assert.Equal(t, stored.PreferredLanguage, body["preferred_language"])
	assert.Equal(t, stored.ID.String(), body["id"])
}

func TestUpdateMySettings_BodyNamesAnotherUser_StaysOnCaller(t *testing.T) {
	r, db := setupMySettingsRoutes(t)

	code, body := callMySettings(t, r, http.MethodPatch, "caller",
		`{"user_id":"victim","id":"00000000-0000-0000-0000-000000000001","two_factor_enabled":true,"password_last_changed":"2020-01-01T00:00:00Z","theme":"dark"}`)

	require.Equal(t, http.StatusOK, code, "body: %v", body)
	assert.Equal(t, "caller", body["user_id"])
	var stored authModels.UserSettings
	require.NoError(t, db.Where("user_id = ?", "caller").First(&stored).Error)
	assert.False(t, stored.TwoFactorEnabled)
	assert.Nil(t, stored.PasswordLastChanged)
	var victimRows int64
	require.NoError(t, db.Model(&authModels.UserSettings{}).Where("user_id = ?", "victim").Count(&victimRows).Error)
	assert.Zero(t, victimRows)
}

func TestMySettings_NoUserID_Returns401AndCreatesNoRow(t *testing.T) {
	r, db := setupMySettingsRoutes(t)

	getCode, _ := callMySettings(t, r, http.MethodGet, "", "")
	patchCode, _ := callMySettings(t, r, http.MethodPatch, "", `{"theme":"dark"}`)

	assert.Equal(t, http.StatusUnauthorized, getCode)
	assert.Equal(t, http.StatusUnauthorized, patchCode)
	var rows int64
	require.NoError(t, db.Model(&authModels.UserSettings{}).Count(&rows).Error)
	assert.Zero(t, rows)
}

// The unique index on user_id keeps concurrent first requests to one row, and
// every request, the losing first insert included, must still answer 200.
func TestMySettings_ConcurrentFirstRequests_AllSucceedWithOneRow(t *testing.T) {
	r, _ := setupMySettingsRoutes(t)
	// A file DB: ":memory:" gives each pooled connection its own empty database.
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/settings.db?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authModels.UserSettings{}))
	sqldb.DB = db // setupMySettingsRoutes' cleanup restores the previous DB

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method, body := http.MethodPatch, `{"theme":"dark"}`
			if i%2 == 0 {
				method, body = http.MethodGet, ""
			}
			codes[i], _ = callMySettings(t, r, method, "racer", body)
		}(i)
	}
	wg.Wait()
	for i, code := range codes {
		assert.Equal(t, http.StatusOK, code, "request %d of %v", i, codes)
	}

	var rows int64
	require.NoError(t, db.Model(&authModels.UserSettings{}).Where("user_id = ?", "racer").Count(&rows).Error)
	assert.Equal(t, int64(1), rows)
}

// A soft-deleted row still holds the unique user_id, so the settings must be
// served from it (or replace it) instead of failing every call.
func TestMySettings_SoftDeletedRow_GetAndPatchStillWork(t *testing.T) {
	r, db := setupMySettingsRoutes(t)
	row := seedUserSettingsRow(t, db, "soft-deleted-user", "/dashboard")
	require.NoError(t, db.Delete(row).Error)

	code, body := callMySettings(t, r, http.MethodGet, "soft-deleted-user", "")
	require.Equal(t, http.StatusOK, code, "GET body: %v", body)
	assert.Equal(t, "soft-deleted-user", body["user_id"])
	// Deliberate: the row is restored as it was, so the user gets their old values back.
	assert.Equal(t, "/dashboard", body["default_landing_page"])
	assert.Equal(t, "en", body["preferred_language"])

	code, body = callMySettings(t, r, http.MethodPatch, "soft-deleted-user", `{"theme":"dark"}`)
	require.Equal(t, http.StatusOK, code, "PATCH body: %v", body)
	assert.Equal(t, "dark", body["theme"])

	code, body = callMySettings(t, r, http.MethodGet, "soft-deleted-user", "")
	require.Equal(t, http.StatusOK, code, "GET after PATCH body: %v", body)
	assert.Equal(t, "dark", body["theme"], "the PATCH must persist")
}

// A settings row created through the generic administrator POST must carry the
// same defaults as every other creation path, unless that route is not offered.
func TestUserSettings_GenericCreate_UnmountedOrUsesDefaultSettings(t *testing.T) {
	registerUserSettingsForReadScope(t)
	cfg := ems.GlobalEntityRegistrationService.GetSwaggerConfig("UserSetting")
	adminMethods := ems.GlobalEntityRegistrationService.GetAllEntityRoles()["UserSetting"].Roles[string(authModels.Admin)]
	if cfg == nil || cfg.Create == nil || !strings.Contains(adminMethods, http.MethodPost) {
		return // no generic create route is offered, so no other defaults can be written
	}

	db := setupUserSettingsReadScopeDB(t)
	gc := controller.NewGenericController(db, mocks.NewMockEnforcer())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", "admin-user")
		c.Set("userRoles", []string{string(authModels.Admin)})
	})
	r.POST("/api/v1/user-settings/", func(c *gin.Context) { gc.AddEntity(c) })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user-settings/", strings.NewReader(`{"user_id":"admin-created"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var gotMap map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gotMap))
	want := authModels.DefaultUserSettings("admin-created")
	raw, err := json.Marshal(registration.UserSettingsToOutput(&want))
	require.NoError(t, err)
	var wantMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &wantMap))
	assert.Equal(t, pickDefaults(wantMap), pickDefaults(gotMap))
}

// Signup over a leftover soft-deleted row must leave the user with a live settings row.
func TestProvisionNewUser_SoftDeletedSettingsRow_LeavesLiveRow(t *testing.T) {
	db := setupCurrentUserTestDB(t)
	row := seedUserSettingsRow(t, db, "returning-user", "/dashboard")
	require.NoError(t, db.Delete(row).Error)

	t.Setenv("TERMINAL_TRAINER_URL", "") // keep the key-creation step offline
	authServices.ProvisionNewUser(mocks.NewMockEnforcer(), db, &casdoorsdk.User{Id: "returning-user", EmailVerified: true}, "")

	var live int64
	require.NoError(t, db.Model(&authModels.UserSettings{}).Where("user_id = ?", "returning-user").Count(&live).Error)
	assert.Equal(t, int64(1), live)
}

// A forced password change must stamp password_last_changed even when the
// user has no live settings row yet.
func TestForceChangePassword_NoSettingsRow_StampsPasswordLastChanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/get-user" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": casdoorsdk.User{
				Id: "forced-user", Owner: "org", Name: "forced",
				Properties: map[string]string{"force_password_reset": "true"},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": "Affected"})
	}))
	casdoorsdk.InitConfig(srv.URL, "dummy", "dummy", "dummy", "dummy", "dummy")
	t.Cleanup(func() {
		casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
		srv.Close()
	})
	db := setupCurrentUserTestDB(t)
	prevDB := sqldb.DB
	sqldb.DB = db
	t.Cleanup(func() { sqldb.DB = prevDB })

	err := authServices.NewUserSettingsService().ForceChangePassword("forced-user",
		dto.ForceChangePasswordInput{NewPassword: "N3w-Passw0rd!", ConfirmPassword: "N3w-Passw0rd!"}, "")
	require.NoError(t, err)

	var stored authModels.UserSettings
	require.NoError(t, db.Where("user_id = ?", "forced-user").First(&stored).Error, "a settings row must hold the stamp")
	assert.NotNil(t, stored.PasswordLastChanged)
}
