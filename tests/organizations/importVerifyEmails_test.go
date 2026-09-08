// tests/organizations/importVerifyEmails_test.go
//
// An account created by an organization's import was vouched for by that
// organization: the teacher holds the class list. Sending thirteen students to
// a verification screen for an address the teacher typed is a wall, not a
// safeguard — so the import marks the address verified unless asked not to,
// with exactly the writes the product's own verification flow makes.
package organizations_tests

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
)

func assertVerifiedLikeTheProductDoes(t *testing.T, u *casdoorsdk.User) {
	t.Helper()
	assert.True(t, u.EmailVerified, "%s: the native flag is what the login flow reads", u.Email)
	stamp := u.Properties["email_verified_at"]
	require.NotEmpty(t, stamp, "%s: email_verified_at is the product's own stamp", u.Email)
	_, err := time.Parse(time.RFC3339, stamp)
	assert.NoError(t, err, "email_verified_at is RFC3339 like the verification flow writes it")
}

func TestCsvImport_VerifyEmails_CreatedAccountIsVerified(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "ada@example.com,Ada,Lovelace,member\n"), nil, nil, false, false, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	require.Len(t, identity.created, 1)
	assertVerifiedLikeTheProductDoes(t, identity.created[0])
	assert.Equal(t, "true", identity.created[0].Properties["force_password_reset"], "the other properties are still there")
}

func TestCsvImport_VerifyEmails_UpdatedAccountIsVerifiedAndKeepsItsProperties(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	identity.users["student-a"].Properties = map[string]string{"tos_version": "2026-01-01"}
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role,force_reset", "student-a@example.com,Ada,Lovelace,member,true\n"), nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	existing := identity.users["student-a"]
	assertVerifiedLikeTheProductDoes(t, existing)
	assert.Equal(t, "2026-01-01", existing.Properties["tos_version"], "the properties map is merged into, not replaced")

	assert.Equal(t, "true", existing.Properties["force_password_reset"])

	require.Len(t, identity.columns, 1)
	assert.Equal(t, []string{"first_name", "last_name", "display_name", "properties", "email_verified"}, identity.columns[0],
		"Casdoor's default whitelist drops email_verified, so it must be named; force_reset and the verified mark share one properties column")
}

func TestCsvImport_VerifyEmailsOff_LeavesVerificationUntouched(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "student-a@example.com,Ada,Lovelace,member\nnew@example.com,Grace,Hopper,member\n"),
		nil, nil, false, true, "", false)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	require.Len(t, identity.created, 1)
	assert.False(t, identity.created[0].EmailVerified)
	assert.NotContains(t, identity.created[0].Properties, "email_verified_at")

	existing := identity.users["student-a"]
	assert.False(t, existing.EmailVerified)
	assert.NotContains(t, existing.Properties, "email_verified_at")
	require.Len(t, identity.columns, 1)
	assert.NotContains(t, identity.columns[0], "email_verified")
}

// recordingImportService captures the options the controller hands the service.
type recordingImportService struct {
	verifyEmails bool
}

func (r *recordingImportService) ImportOrganizationData(
	_ uuid.UUID, _ string, _, _, _ *multipart.FileHeader,
	_, _ bool, _ string, verifyEmails bool,
) (*dto.ImportOrganizationDataResponse, error) {
	r.verifyEmails = verifyEmails
	return &dto.ImportOrganizationDataResponse{Success: true}, nil
}

func postImport(t *testing.T, router *gin.Engine, orgID uuid.UUID, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("users", "users.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte("email,first_name,last_name,role\nada@example.com,Ada,Lovelace,member\n"))
	require.NoError(t, err)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/import", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestImportController_VerifyEmailsDefaultsToTrue(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := &recordingImportService{}
	ctrl := orgController.NewOrganizationController(services.NewOrganizationService(db), importer, nil, nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "owner-1"); c.Next() })
	router.POST("/organizations/:id/import", ctrl.ImportOrganizationData)

	rec := postImport(t, router, orgID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, importer.verifyEmails, "absent verify_emails means the organization vouches for its addresses")

	rec = postImport(t, router, orgID, map[string]string{"verify_emails": "false"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.False(t, importer.verifyEmails, "verify_emails=false reaches the service")
}
