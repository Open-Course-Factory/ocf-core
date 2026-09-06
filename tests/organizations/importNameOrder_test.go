package organizations_tests

// The `name_order` import option: a class list often carries a single "name"
// column, and whether it reads "DUPONT Marie" or "Marie DUPONT" depends on who
// exported it. The option tells the parser which way round the column is.
//
// Shared helpers reused from siblings: newOffboardingDB / seedTeamOrg /
// newFakeIdentity / newOffboardingService (memberOffboarding_test.go),
// installMockEnforcer (organizationMemberPermissionSync_test.go).

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/auth/errors"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"
)

func TestParseNameOrder(t *testing.T) {
	cases := []struct {
		raw     string
		want    dto.NameOrder
		wantErr bool
	}{
		{raw: "", want: dto.NameOrderLastFirst},
		{raw: "last_first", want: dto.NameOrderLastFirst},
		{raw: "first_last", want: dto.NameOrderFirstLast},
		{raw: "garbage", wantErr: true},
		{raw: "FIRST_LAST", wantErr: true},
	}
	for _, tc := range cases {
		t.Run("raw="+tc.raw, func(t *testing.T) {
			got, err := dto.ParseNameOrder(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "name_order")
				assert.Contains(t, err.Error(), "last_first")
				assert.Contains(t, err.Error(), "first_last")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Controller: the form field reaches the service, an invalid value is a 400
// ---------------------------------------------------------------------------

// recordingImportService captures the arguments the controller forwards.
type recordingImportService struct {
	called    bool
	nameOrder dto.NameOrder
}

func (r *recordingImportService) ImportOrganizationData(
	_ uuid.UUID, _ string,
	_, _, _ *multipart.FileHeader,
	_, _ bool, _ string, nameOrder dto.NameOrder,
) (*dto.ImportOrganizationDataResponse, error) {
	r.called = true
	r.nameOrder = nameOrder
	return &dto.ImportOrganizationDataResponse{Success: true}, nil
}

func importRouter(t *testing.T, db *gorm.DB, ownerID string, importer services.ImportService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctrl := controller.NewOrganizationController(services.NewOrganizationService(db), importer, nil, nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", ownerID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.POST("/organizations/:id/import", ctrl.ImportOrganizationData)
	return router
}

// importRequest builds the multipart POST the front sends: the users file plus
// the given form fields.
func importRequest(t *testing.T, orgID uuid.UUID, csv string, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("users", "users.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(csv))
	require.NoError(t, err)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/import", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestImportController_NameOrderParameter(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", nil)
	csv := "email,name\nmarie@example.com,Marie DUPONT\n"

	cases := []struct {
		name      string
		fields    map[string]string
		wantOrder dto.NameOrder
	}{
		{name: "absent defaults to last_first", fields: map[string]string{}, wantOrder: dto.NameOrderLastFirst},
		{name: "first_last reaches the service", fields: map[string]string{"name_order": "first_last"}, wantOrder: dto.NameOrderFirstLast},
		{name: "last_first reaches the service", fields: map[string]string{"name_order": "last_first"}, wantOrder: dto.NameOrderLastFirst},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			importer := &recordingImportService{}
			rec := httptest.NewRecorder()
			importRouter(t, db, "owner-1", importer).ServeHTTP(rec, importRequest(t, orgID, csv, tc.fields))

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.True(t, importer.called)
			assert.Equal(t, tc.wantOrder, importer.nameOrder)
		})
	}
}

func TestImportController_InvalidNameOrder_Returns400WithoutImporting(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", nil)
	importer := &recordingImportService{}

	rec := httptest.NewRecorder()
	importRouter(t, db, "owner-1", importer).ServeHTTP(rec,
		importRequest(t, orgID, "email,name\nmarie@example.com,Marie DUPONT\n", map[string]string{"name_order": "sideways"}))

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.False(t, importer.called, "an invalid name_order must not start the import")
	var apiErr errors.APIError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &apiErr))
	assert.Equal(t, http.StatusBadRequest, apiErr.ErrorCode)
	assert.Contains(t, apiErr.ErrorMessage, "name_order")
	assert.Contains(t, apiErr.ErrorMessage, "last_first")
	assert.Contains(t, apiErr.ErrorMessage, "first_last")
}

// ---------------------------------------------------------------------------
// Service: the order applied to the name column ends up on the account
// ---------------------------------------------------------------------------

func usersCSVWithHeader(t *testing.T, header, rows string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("users", "users.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(header + "\n" + rows))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.File["users"][0]
}

func TestImportService_FirstLastNameOrder_WritesNamesTheRightWayRound(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))

	importer := services.NewImportService(db, identity, newOffboardingService(db, identity))
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithHeader(t, "email,name,role", "student-a@example.com,Marie DUPONT,member\n"),
		nil, nil, false, true, "", dto.NameOrderFirstLast)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	account := identity.users["student-a"]
	assert.Equal(t, "Marie", account.FirstName)
	assert.Equal(t, "DUPONT", account.LastName)
	assert.Equal(t, "Marie DUPONT", account.DisplayName)
}
