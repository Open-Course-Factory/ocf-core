package groups_tests

// Tests for GET /groups/me/memberships — returns the authenticated user's
// group memberships with their role per group. Frontend uses this to gate
// per-group "Create Scenario" actions without N round-trips per group.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/groups/models"
	groupRoutes "soli/formations/src/groups/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMyGroupMembershipsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ClassGroup{}, &models.GroupMember{}))
	return db
}

func setupMyGroupMembershipsRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	groupCtrl := groupRoutes.NewGroupController(db)

	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})
	api.GET("/groups/me/memberships", groupCtrl.GetMyGroupMemberships)
	return r
}

func seedGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role models.GroupMemberRole, active bool) {
	t.Helper()
	m := &models.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
		IsActive: active,
	}
	require.NoError(t, db.Omit("Metadata").Create(m).Error)
	if !active {
		// GORM applies the column default (true) on the zero value (false).
		require.NoError(t, db.Model(m).Update("is_active", false).Error)
	}
}

func TestGetMyGroupMemberships_ReturnsAllRoles(t *testing.T) {
	db := setupMyGroupMembershipsDB(t)
	userID := "user-multi-groups"

	g1, g2, g3 := uuid.New(), uuid.New(), uuid.New()
	seedGroupMember(t, db, g1, userID, models.GroupMemberRoleOwner, true)
	seedGroupMember(t, db, g2, userID, models.GroupMemberRoleManager, true)
	seedGroupMember(t, db, g3, userID, models.GroupMemberRoleMember, true)

	// Other user's membership — must NOT be returned.
	seedGroupMember(t, db, g1, "other-user", models.GroupMemberRoleManager, true)

	router := setupMyGroupMembershipsRouter(db, userID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 3, "should return only the 3 memberships of the authenticated user")

	roleByGroup := make(map[string]string, len(response))
	for _, m := range response {
		roleByGroup[m["group_id"].(string)] = m["role"].(string)
	}
	assert.Equal(t, "owner", roleByGroup[g1.String()])
	assert.Equal(t, "manager", roleByGroup[g2.String()])
	assert.Equal(t, "member", roleByGroup[g3.String()])
}

func TestGetMyGroupMemberships_ExcludesInactive(t *testing.T) {
	db := setupMyGroupMembershipsDB(t)
	userID := "user-mixed-groups"

	gActive, gInactive := uuid.New(), uuid.New()
	seedGroupMember(t, db, gActive, userID, models.GroupMemberRoleManager, true)
	seedGroupMember(t, db, gInactive, userID, models.GroupMemberRoleOwner, false)

	router := setupMyGroupMembershipsRouter(db, userID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, gActive.String(), response[0]["group_id"])
}

func TestGetMyGroupMemberships_EmptyForNonMember(t *testing.T) {
	db := setupMyGroupMembershipsDB(t)
	router := setupMyGroupMembershipsRouter(db, "user-no-groups")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Len(t, response, 0)
}

func TestGetMyGroupMemberships_UnauthenticatedReturns401(t *testing.T) {
	db := setupMyGroupMembershipsDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	groupCtrl := groupRoutes.NewGroupController(db)
	api := r.Group("/api/v1")
	// no userId middleware
	api.GET("/groups/me/memberships", groupCtrl.GetMyGroupMemberships)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/me/memberships", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
