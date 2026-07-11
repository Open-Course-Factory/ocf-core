// tests/auth/userSettingsReadScope_test.go
//
// RED tests for MR G at the UserSetting layer: the REAL UserSettings entity
// registration must be owner-read-scoped on the generic read path. Today
// RegisterUserSettings carries no read OwnershipConfig, so a member can list
// and fetch OTHER users' settings rows through the generic
// GET /user-settings and GET /user-settings/:id handlers.
//
// These drive the real generic router (controller.GetEntities / GetEntity)
// over the real registration.RegisterUserSettings, and assert USER-OBSERVABLE
// state (HTTP status + response body rows), never a mock call.
//
// Rows are seeded via direct db.Create of the model — NOT the generic create
// path — so the UserSettings auto-create hook never fires and these tests
// exercise READ scope in isolation.
package auth_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	registration "soli/formations/src/auth/entityRegistration"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// registerUserSettingsForReadScope registers the REAL UserSettings entity into
// a fresh global registration service and restores the previous service (and
// the prior hook-enable state) afterwards, so sibling auth tests are
// unaffected.
func registerUserSettingsForReadScope(t *testing.T) {
	t.Helper()
	prev := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	registration.RegisterUserSettings(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = prev
		hooks.GlobalHookRegistry.DisableAllHooks(false)
	})
}

func setupUserSettingsReadScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authModels.UserSettings{}))
	return db
}

// seedUserSettingsRow inserts a UserSettings row owned by userID directly via
// db.Create, bypassing the generic create path and its auto-create hook. The
// distinctive landingPage doubles as the owner-only field whose non-leak is
// asserted on the non-owner 404.
func seedUserSettingsRow(t *testing.T, db *gorm.DB, userID, landingPage string) *authModels.UserSettings {
	t.Helper()
	settings := &authModels.UserSettings{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		DefaultLandingPage: landingPage,
		PreferredLanguage:  "en",
		Timezone:           "UTC",
		Theme:              "light",
	}
	require.NoError(t, db.Create(settings).Error)
	return settings
}

func userSettingsReadScopeRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	gc := controller.NewGenericController(db, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
		}
		c.Set("userRoles", roles)
		c.Next()
	})
	r.GET("/api/v1/user-settings/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/user-settings/:id", func(c *gin.Context) { gc.GetEntity(c) })
	return r
}

type userSettingsReadScopeListResponse struct {
	Data []struct {
		ID                 string `json:"id"`
		UserID             string `json:"user_id"`
		DefaultLandingPage string `json:"default_landing_page"`
	} `json:"data"`
	Total int64 `json:"total"`
}

// --- UserSetting LIST as a non-owner returns only the caller's rows ----------

func TestUserSettingsReadScope_ListAsNonOwner_ReturnsOnlyOwnRows(t *testing.T) {
	registerUserSettingsForReadScope(t)
	db := setupUserSettingsReadScopeDB(t)

	sa := seedUserSettingsRow(t, db, "user-A", "/dashboard")
	sb := seedUserSettingsRow(t, db, "user-B", "/secret-of-user-B")

	r := userSettingsReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/user-settings/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp userSettingsReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[sa.ID.String()], "A must see own settings row")
	assert.False(t, ids[sb.ID.String()], "A must NOT see B's settings row (cross-user read)")
	assert.Equal(t, int64(1), resp.Total, "Total must be owner-scoped to A's single row")
}

// --- UserSetting GET another user's row → 404, no owner-only fields leak -----

func TestUserSettingsReadScope_GetByIdOtherUsersRow_Returns404(t *testing.T) {
	registerUserSettingsForReadScope(t)
	db := setupUserSettingsReadScopeDB(t)

	_ = seedUserSettingsRow(t, db, "user-A", "/dashboard")
	sb := seedUserSettingsRow(t, db, "user-B", "/secret-of-user-B")

	r := userSettingsReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/user-settings/"+sb.ID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A requesting B's settings row by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	// The 404 body must not disclose B's owner-only content.
	assert.NotContains(t, w.Body.String(), "/secret-of-user-B",
		"404 body must not leak the non-owner row's DefaultLandingPage")
}

// --- Admin LIST returns all rows (regression guard: AdminBypass) -------------

func TestUserSettingsReadScope_ListAsAdmin_ReturnsAllRows(t *testing.T) {
	registerUserSettingsForReadScope(t)
	db := setupUserSettingsReadScopeDB(t)

	sa := seedUserSettingsRow(t, db, "user-A", "/dashboard")
	sb := seedUserSettingsRow(t, db, "user-B", "/secret-of-user-B")

	r := userSettingsReadScopeRouter(db, "admin-1", []string{"administrator"})
	req := httptest.NewRequest("GET", "/api/v1/user-settings/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp userSettingsReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[sa.ID.String()], "admin must see A's row")
	assert.True(t, ids[sb.ID.String()], "admin must see B's row (AdminBypass)")
	assert.Equal(t, int64(2), resp.Total, "admin Total must count all rows")
}
