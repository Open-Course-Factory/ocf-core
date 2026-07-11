// tests/entityManagement/ownerReadScope_test.go
//
// RED tests for MR C — reusable owner read-scope on the generic read path.
//
// These drive the REAL generic router (controller.GetEntities / GetEntity)
// against an entity registered with:
//
//	OwnershipConfig{OwnerField:"UserID", Operations:["read"], AdminBypass:true}
//
// Expected end state (implemented by another agent):
//   - Non-admin LIST injects filters[owner_column]=userID → only own rows.
//   - Non-admin GET /:id of another user's row → 404 (no owner-only fields leak).
//   - Admins (userRoles contains "administrator") bypass the scope entirely.
//
// Assertions are USER-OBSERVABLE (HTTP status + response body rows), never
// "a mock was called". Tests 1, 2, 6 are RED against current code (no scoping);
// tests 3, 4, 5 are green today and act as regression guards against
// over-restriction (owner still sees own row; admin still sees everything).
package entityManagement_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	access "soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// OwnerScopeEntity carries a UserID owner column plus an owner-only Secret
// field used to prove that a non-owner 404 does not leak the row's contents.
type OwnerScopeEntity struct {
	entityManagementModels.BaseModel
	UserID string `json:"userId" gorm:"type:varchar(255);index"`
	Label  string `json:"label"`
	Secret string `json:"secret"` // owner-only; must never surface to a non-owner
}

type OwnerScopeEntityInput struct {
	UserID string `json:"userId"`
	Label  string `json:"label"`
	Secret string `json:"secret"`
}

type OwnerScopeEntityOutput struct {
	ID       string   `json:"id"`
	UserID   string   `json:"userId"`
	Label    string   `json:"label"`
	Secret   string   `json:"secret"`
	OwnerIDs []string `json:"ownerIDs"`
}

// registerOwnerScopeEntity registers OwnerScopeEntity with a read-scope
// OwnershipConfig into a fresh global registration service. Hooks are disabled
// so async ownership hooks never interfere with the direct db.Create seeding.
func registerOwnerScopeEntity(t *testing.T) {
	t.Helper()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)

	ems.RegisterTypedEntity[OwnerScopeEntity, OwnerScopeEntityInput, OwnerScopeEntityInput, OwnerScopeEntityOutput](
		ems.GlobalEntityRegistrationService,
		"OwnerScopeEntity",
		entityManagementInterfaces.TypedEntityRegistration[OwnerScopeEntity, OwnerScopeEntityInput, OwnerScopeEntityInput, OwnerScopeEntityOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[OwnerScopeEntity, OwnerScopeEntityInput, OwnerScopeEntityInput, OwnerScopeEntityOutput]{
				ModelToDto: func(e *OwnerScopeEntity) (OwnerScopeEntityOutput, error) {
					return OwnerScopeEntityOutput{
						ID:       e.ID.String(),
						UserID:   e.UserID,
						Label:    e.Label,
						Secret:   e.Secret,
						OwnerIDs: e.OwnerIDs,
					}, nil
				},
				DtoToModel: func(dto OwnerScopeEntityInput) *OwnerScopeEntity {
					return &OwnerScopeEntity{UserID: dto.UserID, Label: dto.Label, Secret: dto.Secret}
				},
			},
			OwnershipConfig: &access.OwnershipConfig{
				OwnerField:  "UserID",
				Operations:  []string{"read"},
				AdminBypass: true,
			},
		},
	)

	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService.UnregisterEntity("OwnerScopeEntity")
	})
}

func setupOwnerScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&OwnerScopeEntity{}))
	return db
}

func seedOwnerScopeRow(t *testing.T, db *gorm.DB, userID, label string) OwnerScopeEntity {
	t.Helper()
	e := OwnerScopeEntity{UserID: userID, Label: label, Secret: "secret-of-" + userID}
	require.NoError(t, db.Create(&e).Error)
	return e
}

// ownerScopeRouter mounts the real generic LIST + GET-by-id handlers behind a
// middleware that stamps the acting identity, exactly as the auth middleware
// does in production (ctx "userId" + "userRoles").
func ownerScopeRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
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
	r.GET("/api/v1/owner-scope-entities/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/owner-scope-entities/:id", func(c *gin.Context) { gc.GetEntity(c) })
	return r
}

type ownerScopeListResponse struct {
	Data  []OwnerScopeEntityOutput `json:"data"`
	Total int64                    `json:"total"`
}

func doGET(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Test 1: non-owner LIST returns only own rows ---------------------------

func TestOwnerReadScope_ListAsNonOwner_ReturnsOnlyOwnRows(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	a1 := seedOwnerScopeRow(t, db, "user-A", "A one")
	a2 := seedOwnerScopeRow(t, db, "user-A", "A two")
	b1 := seedOwnerScopeRow(t, db, "user-B", "B one")

	r := ownerScopeRouter(db, "user-A", []string{"member"})
	w := doGET(r, "/api/v1/owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp ownerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[a1.ID.String()], "A must see own row a1")
	assert.True(t, ids[a2.ID.String()], "A must see own row a2")
	assert.False(t, ids[b1.ID.String()], "A must NOT see B's row (cross-user read / IDOR)")
	assert.Equal(t, int64(2), resp.Total, "Total must be owner-scoped to A's 2 rows, not the global 3")
}

// --- Test 2: GET another user's row → 404, no owner-only fields leak ---------

func TestOwnerReadScope_GetByIdOtherUsersRow_Returns404(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	_ = seedOwnerScopeRow(t, db, "user-A", "A one")
	b1 := seedOwnerScopeRow(t, db, "user-B", "B one")

	r := ownerScopeRouter(db, "user-A", []string{"member"})
	w := doGET(r, "/api/v1/owner-scope-entities/"+b1.ID.String())

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A requesting B's row by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	// The 404 body must not disclose B's owner-only content.
	assert.NotContains(t, w.Body.String(), "secret-of-user-B",
		"404 body must not leak the non-owner row's Secret field")
	assert.NotContains(t, w.Body.String(), "B one",
		"404 body must not leak the non-owner row's Label")
}

// --- Test 3: GET own row → 200 (regression guard: no over-restriction) ------

func TestOwnerReadScope_GetByIdOwnRow_Returns200(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	a1 := seedOwnerScopeRow(t, db, "user-A", "A one")

	r := ownerScopeRouter(db, "user-A", []string{"member"})
	w := doGET(r, "/api/v1/owner-scope-entities/"+a1.ID.String())

	require.Equal(t, http.StatusOK, w.Code,
		"owner must still read own row; body=%s", w.Body.String())
	var got OwnerScopeEntityOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, a1.ID.String(), got.ID)
	assert.Equal(t, "user-A", got.UserID)
	assert.Equal(t, "A one", got.Label)
}

// --- Test 4: admin LIST returns all rows (regression guard: AdminBypass) -----

func TestOwnerReadScope_ListAsAdmin_ReturnsAllRows(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	a1 := seedOwnerScopeRow(t, db, "user-A", "A one")
	b1 := seedOwnerScopeRow(t, db, "user-B", "B one")

	r := ownerScopeRouter(db, "admin-1", []string{"administrator"})
	w := doGET(r, "/api/v1/owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp ownerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[a1.ID.String()], "admin must see A's row")
	assert.True(t, ids[b1.ID.String()], "admin must see B's row (AdminBypass)")
	assert.Equal(t, int64(2), resp.Total, "admin Total must count all rows")
}

// --- Test 5: admin GET another user's row → 200 (regression guard) -----------

func TestOwnerReadScope_GetByIdAsAdmin_ReturnsOtherUsersRow(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	b1 := seedOwnerScopeRow(t, db, "user-B", "B one")

	r := ownerScopeRouter(db, "admin-1", []string{"administrator"})
	w := doGET(r, "/api/v1/owner-scope-entities/"+b1.ID.String())

	require.Equal(t, http.StatusOK, w.Code,
		"admin must read any user's row (AdminBypass); body=%s", w.Body.String())
	var got OwnerScopeEntityOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, b1.ID.String(), got.ID)
	assert.Equal(t, "user-B", got.UserID)
}

// --- Test 6: pagination COUNT is owner-scoped -------------------------------

func TestOwnerReadScope_ListPaginationCountIsOwnerScoped(t *testing.T) {
	registerOwnerScopeEntity(t)
	db := setupOwnerScopeDB(t)

	// Interleave A and B so B rows fall within the first (default 20) page —
	// this makes BOTH the COUNT assertion AND the per-row ownership assertion
	// fail today, rather than the ordering accidentally hiding B on page 1.
	for i := 0; i < 25; i++ {
		seedOwnerScopeRow(t, db, "user-A", fmt.Sprintf("A-%d", i))
		seedOwnerScopeRow(t, db, "user-B", fmt.Sprintf("B-%d", i))
	}

	r := ownerScopeRouter(db, "user-A", []string{"member"})
	w := doGET(r, "/api/v1/owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp ownerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Guards the COUNT query, not just the page slice.
	assert.Equal(t, int64(25), resp.Total, "Total must count only A's 25 rows, not the global 50")
	// Every row on the returned page must belong to A.
	for _, d := range resp.Data {
		assert.Equal(t, "user-A", d.UserID,
			"paginated page must contain only A's rows, found row owned by %q", d.UserID)
	}
}
