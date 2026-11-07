package groups_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	ems "soli/formations/src/entityManagement/entityManagementService"
	controller "soli/formations/src/entityManagement/routes"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
)

func readByIDAs(t *testing.T, db *gorm.DB, path, userID string, roles ...string) *httptest.ResponseRecorder {
	t.Helper()
	generic := controller.NewGenericController(db, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
	})
	router.GET("/api/v1/group-members/:id", generic.GetEntity)
	router.GET("/api/v1/class-groups/:id", generic.GetEntity)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestClassAndMember_ReadByID_FollowTheListRule(t *testing.T) {
	// The member converter looks the user up; unreachable just leaves the name out.
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
	original := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	t.Cleanup(func() { ems.GlobalEntityRegistrationService = original })
	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)
	groupRegistration.RegisterGroupMember(ems.GlobalEntityRegistrationService)

	db := setupGroupMemberVisibilityDB(t)
	orgOne, orgOther, groupID := seedFixture(t, db)
	addOrgMember(t, db, orgOne, "orgManager", orgModels.OrgRoleManager)
	addOrgMember(t, db, orgOne, "plainOrgMember", orgModels.OrgRoleMember)
	addOrgMember(t, db, orgOther, "otherOrgOwner", orgModels.OrgRoleOwner)

	var student groupModels.GroupMember
	require.NoError(t, db.First(&student, "user_id = ?", "userStudent2").Error)

	for _, path := range []string{
		"/api/v1/group-members/" + student.ID.String(),
		"/api/v1/class-groups/" + groupID.String(),
	} {
		for _, caller := range []string{"userStudent1", "orgManager"} {
			assert.Equal(t, http.StatusOK, readByIDAs(t, db, path, caller, "member").Code, "%s reading %s", caller, path)
		}
		for _, caller := range []string{"plainOrgMember", "otherOrgOwner", "stranger"} {
			response := readByIDAs(t, db, path, caller, "member")
			assert.Equal(t, http.StatusNotFound, response.Code, "%s reading %s", caller, path)
			assert.NotContains(t, response.Body.String(), "userStudent")
		}
		assert.Equal(t, http.StatusOK, readByIDAs(t, db, path, "platform-admin", "administrator").Code)
	}

	assert.Equal(t, http.StatusNotFound, readByIDAs(t, db, "/api/v1/group-members/"+uuid.NewString(), "userStudent1", "member").Code)
}
