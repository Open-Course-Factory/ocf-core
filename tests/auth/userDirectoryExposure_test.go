// tests/auth/userDirectoryExposure_test.go
//
// A member must not be able to read the platform's user directory. Before this
// fix, GET /users returned every Casdoor account with its email to any signed-in
// member, GET /users/:id returned any user's email and memberships, and
// GET /users/search enumerated accounts from a two-letter query.
package auth_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/dto"
	userController "soli/formations/src/auth/routes/usersRoutes"
	authServices "soli/formations/src/auth/services"
)

var directoryUser = dto.UserOutput{
	Id:          uuid.MustParse("5766880f-2fef-4348-9802-c857969408a1"),
	UserName:    "yanis",
	DisplayName: "Yanis Bouchard",
	Email:       "yanis@example.test",
}

// directoryUserService answers every lookup with directoryUser; the embedded
// interface panics on any method a test did not expect to be called.
type directoryUserService struct {
	authServices.UserService
	searched bool
}

func (s *directoryUserService) GetUserById(string) (*dto.UserOutput, error) {
	u := directoryUser
	return &u, nil
}

func (s *directoryUserService) SearchUsers(string) (*[]dto.UserOutput, error) {
	s.searched = true
	return &[]dto.UserOutput{directoryUser}, nil
}

func (s *directoryUserService) FindUserByEmail(email string) (*dto.UserOutput, error) {
	if email == "" {
		panic("FindUserByEmail must never be asked for an empty email")
	}
	if email != directoryUser.Email {
		return nil, nil
	}
	u := directoryUser
	return &u, nil
}

func runUsersHandler(t *testing.T, svc authServices.UserService, roles []string, path string, params gin.Params,
	handler func(*userController.UserController, *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("userId", uuid.NewString())
	ctx.Set("userRoles", roles)
	ctx.Params = params
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	handler(userController.NewUserControllerWithServices(svc, nil, nil), ctx)
	return w
}

func TestGetUser_OtherUserSeenByMember_HidesEmailAndMemberships(t *testing.T) {
	w := runUsersHandler(t, &directoryUserService{}, []string{"member"},
		"/users/"+directoryUser.Id.String()+"?includes=organization_memberships",
		gin.Params{{Key: "id", Value: directoryUser.Id.String()}},
		(*userController.UserController).GetUser)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "Yanis Bouchard", body["display_name"], "names stay visible: class pages show the owner")
	assert.Empty(t, body["email"])
	assert.NotContains(t, body, "organization_memberships")
}

func TestGetUser_OtherUserSeenByAdministrator_KeepsEmail(t *testing.T) {
	w := runUsersHandler(t, &directoryUserService{}, []string{"administrator"},
		"/users/"+directoryUser.Id.String(),
		gin.Params{{Key: "id", Value: directoryUser.Id.String()}},
		(*userController.UserController).GetUser)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), directoryUser.Email)
}

func TestSearchUsers_Member_PartialQueryFindsNobody(t *testing.T) {
	svc := &directoryUserService{}
	w := runUsersHandler(t, svc, []string{"member"}, "/users/search?q=ya", nil,
		(*userController.UserController).SearchUsers)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
	assert.False(t, svc.searched, "a member must never reach the substring search")
}

func TestSearchUsers_Member_BlankQueryNeverReachesCasdoor(t *testing.T) {
	w := runUsersHandler(t, &directoryUserService{}, []string{"member"}, "/users/search?q=++++", nil,
		(*userController.UserController).SearchUsers)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
}

func TestSearchUsers_Member_ExactEmailFindsTheUser(t *testing.T) {
	w := runUsersHandler(t, &directoryUserService{}, []string{"member"}, "/users/search?q=+yanis@example.test+", nil,
		(*userController.UserController).SearchUsers)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), directoryUser.Id.String())
}

// Casdoor runs on PostgreSQL in production, where its email lookup is
// case-sensitive, and phones capitalise the first letter of a text field.
func TestFindUserByEmail_CapitalisedQueryFindsTheLowercaseAccount(t *testing.T) {
	client := &mockCasdoorUserClient{}
	client.On("GetUserByEmail", "Yanis@Example.test").Return(nil, nil)
	client.On("GetUserByEmail", "yanis@example.test").Return(&casdoorsdk.User{Id: directoryUser.Id.String(), Email: "yanis@example.test"}, nil)

	found, err := authServices.NewUserService(client, nil).FindUserByEmail("Yanis@Example.test")

	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, directoryUser.Id, found.Id)
}

func TestFindUserByEmail_ExactMixedCaseAccountIsFoundAsTyped(t *testing.T) {
	client := &mockCasdoorUserClient{}
	client.On("GetUserByEmail", "Yanis@Example.test").Return(&casdoorsdk.User{Id: directoryUser.Id.String(), Email: "Yanis@Example.test"}, nil)

	found, err := authServices.NewUserService(client, nil).FindUserByEmail("Yanis@Example.test")

	require.NoError(t, err)
	require.NotNil(t, found)
	client.AssertNumberOfCalls(t, "GetUserByEmail", 1)
}

func TestSearchUsers_Administrator_KeepsSubstringSearch(t *testing.T) {
	svc := &directoryUserService{}
	w := runUsersHandler(t, svc, []string{"administrator"}, "/users/search?q=ya", nil,
		(*userController.UserController).SearchUsers)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, svc.searched)
}
