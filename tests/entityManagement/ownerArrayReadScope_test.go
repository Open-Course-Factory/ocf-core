// tests/entityManagement/ownerArrayReadScope_test.go
//
// RED tests for #406 — ARRAY-aware owner read-scope on the generic read path.
//
// Unlike ownerReadScope_test.go (a scalar UserID column), these drive an entity
// whose owner is the BaseModel.OwnerIDs *array* column (pq.StringArray). The
// entity registers:
//
//	OwnershipConfig{OwnerField:"OwnerIDs", Operations:["read"], AdminBypass:true, ArrayOwner:true}
//
// Expected end state (implemented by another agent for #406):
//   - Non-admin LIST filters on the owner_ids array → only rows whose owner_ids
//     CONTAINS the caller. Postgres uses `owner_ids && ARRAY[?]`; SQLite (these
//     tests) uses `owner_ids LIKE '%"<id>"%'` because pq.StringArray serialises a
//     one-element array as the literal `{"<id>"}`.
//   - Non-admin GET /:id of a row not containing the caller → 404 (no leak).
//   - Admins (userRoles contains "administrator") bypass the scope entirely.
//   - Empty actor (no userId) on a read-scoped entity → zero rows (fail-closed).
//
// Assertions are USER-OBSERVABLE (HTTP status + response body rows / DB round
// trip), never "a mock was called".
//
// RED signal is twofold:
//  1. COMPILE-FAIL: OwnershipConfig has no `ArrayOwner` field yet, so this file
//     does not build until the impl adds it. That breaks the whole package —
//     the intended first RED.
//  2. BEHAVIOURAL: once it compiles, with no array scoping in the read path a
//     non-owner still sees every row, so tests 1/2/6/7 fail. Tests 3/4/5/8 are
//     regression guards (green after impl; test 8 goes RED only if write
//     serialisation and the read pattern ever diverge).
package entityManagement_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authRegistration "soli/formations/src/auth/entityRegistration"
	authModels "soli/formations/src/auth/models"
	access "soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ArrayOwnerScopeEntity is owned via the BaseModel.OwnerIDs array column rather
// than a scalar owner field. Secret is owner-only and proves a non-owner 404 /
// filtered list never leaks the row's contents.
type ArrayOwnerScopeEntity struct {
	entityManagementModels.BaseModel
	Label  string `json:"label"`
	Secret string `json:"secret"` // owner-only; must never surface to a non-owner
}

type ArrayOwnerScopeEntityInput struct {
	Label  string `json:"label"`
	Secret string `json:"secret"`
}

type ArrayOwnerScopeEntityOutput struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Secret   string   `json:"secret"`
	OwnerIDs []string `json:"ownerIDs"`
}

// registerArrayOwnerScopeEntity registers ArrayOwnerScopeEntity with an
// array-aware read-scope OwnershipConfig into a fresh global registration
// service. Hooks are disabled so async ownership hooks never interfere with the
// direct db.Create seeding used by most tests.
func registerArrayOwnerScopeEntity(t *testing.T) {
	t.Helper()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)

	ems.RegisterTypedEntity[ArrayOwnerScopeEntity, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityOutput](
		ems.GlobalEntityRegistrationService,
		"ArrayOwnerScopeEntity",
		entityManagementInterfaces.TypedEntityRegistration[ArrayOwnerScopeEntity, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[ArrayOwnerScopeEntity, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityInput, ArrayOwnerScopeEntityOutput]{
				ModelToDto: func(e *ArrayOwnerScopeEntity) (ArrayOwnerScopeEntityOutput, error) {
					return ArrayOwnerScopeEntityOutput{
						ID:       e.ID.String(),
						Label:    e.Label,
						Secret:   e.Secret,
						OwnerIDs: e.OwnerIDs,
					}, nil
				},
				DtoToModel: func(dto ArrayOwnerScopeEntityInput) *ArrayOwnerScopeEntity {
					return &ArrayOwnerScopeEntity{Label: dto.Label, Secret: dto.Secret}
				},
			},
			OwnershipConfig: &access.OwnershipConfig{
				OwnerField:  "OwnerIDs",
				Operations:  []string{"read"},
				AdminBypass: true,
				ArrayOwner:  true,
			},
		},
	)

	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService.UnregisterEntity("ArrayOwnerScopeEntity")
	})
}

func setupArrayOwnerScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ArrayOwnerScopeEntity{}))
	return db
}

// seedArrayOwnerRow seeds a row whose owner_ids array contains exactly `owners`.
func seedArrayOwnerRow(t *testing.T, db *gorm.DB, owners []string, label string) ArrayOwnerScopeEntity {
	t.Helper()
	e := ArrayOwnerScopeEntity{
		BaseModel: entityManagementModels.BaseModel{OwnerIDs: pq.StringArray(owners)},
		Label:     label,
		Secret:    "secret-of-" + label,
	}
	require.NoError(t, db.Create(&e).Error)
	return e
}

// arrayOwnerScopeRouter mounts the real generic LIST + GET-by-id + CREATE
// handlers behind a middleware that stamps the acting identity (ctx "userId" +
// "userRoles"), exactly as the auth middleware does in production.
func arrayOwnerScopeRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
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
	r.GET("/api/v1/array-owner-scope-entities/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/array-owner-scope-entities/:id", func(c *gin.Context) { gc.GetEntity(c) })
	r.POST("/api/v1/array-owner-scope-entities/", func(c *gin.Context) { gc.AddEntity(c) })
	return r
}

type arrayOwnerScopeListResponse struct {
	Data  []ArrayOwnerScopeEntityOutput `json:"data"`
	Total int64                         `json:"total"`
}

func doArrayGET(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doArrayPOST(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Test 1: non-owner LIST returns only own rows ---------------------------

func TestOwnerArrayReadScope_ListAsNonOwner_ReturnsOnlyOwnRows(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	a1 := seedArrayOwnerRow(t, db, []string{"user-A"}, "A one")
	b1 := seedArrayOwnerRow(t, db, []string{"user-B"}, "B one")

	r := arrayOwnerScopeRouter(db, "user-A", []string{"member"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp arrayOwnerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[a1.ID.String()], "A must see own row a1")
	assert.False(t, ids[b1.ID.String()], "A must NOT see B's row (cross-user array read / IDOR)")
	assert.Equal(t, int64(1), resp.Total, "Total must be owner-scoped to A's 1 row, not the global 2")
	assert.NotContains(t, w.Body.String(), "secret-of-B one",
		"list body must not leak B's owner-only Secret")
}

// --- Test 2: GET another user's row → 404, no owner-only fields leak ---------

func TestOwnerArrayReadScope_GetByIdOtherUsersRow_Returns404(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	_ = seedArrayOwnerRow(t, db, []string{"user-A"}, "A one")
	b1 := seedArrayOwnerRow(t, db, []string{"user-B"}, "B one")

	r := arrayOwnerScopeRouter(db, "user-A", []string{"member"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/"+b1.ID.String())

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A requesting B's row by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret-of-B one",
		"404 body must not leak the non-owner row's Secret field")
	assert.NotContains(t, w.Body.String(), "B one",
		"404 body must not leak the non-owner row's Label")
}

// --- Test 3: GET own row → 200 (regression guard: no over-restriction) ------

func TestOwnerArrayReadScope_GetByIdOwnRow_Returns200(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	a1 := seedArrayOwnerRow(t, db, []string{"user-A"}, "A one")

	r := arrayOwnerScopeRouter(db, "user-A", []string{"member"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/"+a1.ID.String())

	require.Equal(t, http.StatusOK, w.Code,
		"owner must still read own row; body=%s", w.Body.String())
	var got ArrayOwnerScopeEntityOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, a1.ID.String(), got.ID)
	assert.Equal(t, "A one", got.Label)
	assert.Contains(t, got.OwnerIDs, "user-A", "own row must carry the caller in owner_ids")
}

// --- Test 4: admin LIST returns all rows (regression guard: AdminBypass) -----

func TestOwnerArrayReadScope_ListAsAdmin_ReturnsAllRows(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	a1 := seedArrayOwnerRow(t, db, []string{"user-A"}, "A one")
	b1 := seedArrayOwnerRow(t, db, []string{"user-B"}, "B one")

	r := arrayOwnerScopeRouter(db, "admin-1", []string{"administrator"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp arrayOwnerScopeListResponse
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

func TestOwnerArrayReadScope_GetByIdAsAdmin_ReturnsOtherUsersRow(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	b1 := seedArrayOwnerRow(t, db, []string{"user-B"}, "B one")

	r := arrayOwnerScopeRouter(db, "admin-1", []string{"administrator"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/"+b1.ID.String())

	require.Equal(t, http.StatusOK, w.Code,
		"admin must read any user's row (AdminBypass); body=%s", w.Body.String())
	var got ArrayOwnerScopeEntityOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, b1.ID.String(), got.ID)
}

// --- Test 6: pagination COUNT is owner-scoped -------------------------------

func TestOwnerArrayReadScope_ListPaginationCountIsOwnerScoped(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	// Interleave A and B so B rows fall within the first (default 20) page —
	// this makes BOTH the COUNT assertion AND the per-row ownership assertion
	// fail today, rather than ordering accidentally hiding B on page 1.
	for i := 0; i < 25; i++ {
		seedArrayOwnerRow(t, db, []string{"user-A"}, fmt.Sprintf("A-%d", i))
		seedArrayOwnerRow(t, db, []string{"user-B"}, fmt.Sprintf("B-%d", i))
	}

	r := arrayOwnerScopeRouter(db, "user-A", []string{"member"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp arrayOwnerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Guards the COUNT query, not just the page slice.
	assert.Equal(t, int64(25), resp.Total, "Total must count only A's 25 rows, not the global 50")
	// Every row on the returned page must be owned by A.
	for _, d := range resp.Data {
		assert.Contains(t, d.OwnerIDs, "user-A",
			"paginated page must contain only A's rows, found row with owner_ids %v", d.OwnerIDs)
	}
}

// --- Test 7: empty actor on a read-scoped list → zero rows (fail-closed) -----

func TestOwnerArrayReadScope_ListEmptyActor_ReturnsNoRows(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	seedArrayOwnerRow(t, db, []string{"user-A"}, "A one")
	seedArrayOwnerRow(t, db, []string{"user-B"}, "B one")

	// Empty userID → the middleware sets no "userId"; roles say non-admin member.
	r := arrayOwnerScopeRouter(db, "", []string{"member"})
	w := doArrayGET(r, "/api/v1/array-owner-scope-entities/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp arrayOwnerScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Empty(t, resp.Data,
		"an unknown actor (empty userId) on a read-scoped array entity must see ZERO rows")
	assert.Equal(t, int64(0), resp.Total,
		"Total must be 0 for an unknown actor, not the global 2 (fail-open IDOR)")
}

// --- Test 8: create-then-list round-trip (THE CRITICAL GUARD) ---------------
//
// Drives the REAL generic AddEntity handler, which calls addOwnerIDs →
// AddOwnerIDToEntity → writes owner_ids as pq.StringArray{"user-A"}. Then lists
// as A and asserts the new row IS returned. This proves owner_ids is written in
// a form the array read filter matches: if write serialisation (`{"user-A"}`)
// and the read pattern (`LIKE '%"user-A"%'` on SQLite / `&& ARRAY[?]` on PG)
// ever diverge, the scope returns empty for legit owners and this goes RED.
func TestOwnerArrayReadScope_CreateThenList_OwnerSeesOwnNewRow(t *testing.T) {
	registerArrayOwnerScopeEntity(t)
	db := setupArrayOwnerScopeDB(t)

	r := arrayOwnerScopeRouter(db, "user-A", []string{"member"})

	// Create through the real controller so owner_ids is written by the prod path.
	cw := doArrayPOST(r, "/api/v1/array-owner-scope-entities/", `{"label":"A fresh","secret":"secret-of-A fresh"}`)
	require.Equal(t, http.StatusCreated, cw.Code, "create should succeed; body=%s", cw.Body.String())

	var created ArrayOwnerScopeEntityOutput
	require.NoError(t, json.Unmarshal(cw.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID, "created row must have an ID")
	require.Contains(t, created.OwnerIDs, "user-A",
		"create path must stamp the caller into owner_ids")

	// List as A — the freshly created row must be visible through the array filter.
	lw := doArrayGET(r, "/api/v1/array-owner-scope-entities/")
	require.Equal(t, http.StatusOK, lw.Code, "list should succeed; body=%s", lw.Body.String())

	var resp arrayOwnerScopeListResponse
	require.NoError(t, json.Unmarshal(lw.Body.Bytes(), &resp))

	found := false
	for _, d := range resp.Data {
		if d.ID == created.ID {
			found = true
			break
		}
	}
	assert.True(t, found,
		"owner must see own freshly-created row through the array read scope (write/read serialisation must agree)")
	assert.Equal(t, int64(1), resp.Total, "only A's single new row is owned by A")
}

// --- Test 9: the REAL #406 target — SshKey generic list is array-scoped ------
//
// SshKey embeds BaseModel (OwnerIDs) and grants Casbin `member` GET on its
// generic list. #406 adds ArrayOwner read scope to its registration so a member
// only lists keys they own. Seeded directly (SshKey create/DTO is heavier); the
// throwaway-entity tests above are the mechanism proof.
func TestSshKeyReadScope_ListAsNonOwner_ReturnsOnlyOwnKeys(t *testing.T) {
	// SshKey.BeforeSave encrypts PrivateKey; seeding needs the field key set.
	t.Setenv("FIELD_ENCRYPTION_SECRET", "test-secret-for-ssh-key-scope")

	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	authRegistration.RegisterSshKey(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService.UnregisterEntity("SshKey")
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authModels.SshKey{}))

	keyA := authModels.SshKey{
		BaseModel:  entityManagementModels.BaseModel{OwnerIDs: pq.StringArray{"user-A"}},
		KeyName:    "A-key",
		PrivateKey: "priv-A",
	}
	keyB := authModels.SshKey{
		BaseModel:  entityManagementModels.BaseModel{OwnerIDs: pq.StringArray{"user-B"}},
		KeyName:    "B-key",
		PrivateKey: "priv-B",
	}
	require.NoError(t, db.Create(&keyA).Error)
	require.NoError(t, db.Create(&keyB).Error)

	gin.SetMode(gin.TestMode)
	gc := controller.NewGenericController(db, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", "user-A")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	r.GET("/api/v1/ssh-keys/", func(c *gin.Context) { gc.GetEntities(c) })

	w := doArrayGET(r, "/api/v1/ssh-keys/")
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	// PrivateKey is stored/returned encrypted, so assert on the plaintext KeyName
	// which reliably identifies each row. A must see only its own key; B's key
	// (name or ciphertext) must never appear in A's list.
	assert.Contains(t, w.Body.String(), "A-key", "A must see own key")
	assert.NotContains(t, w.Body.String(), "B-key",
		"A must NOT see B's SSH key (cross-user key exposure)")
	assert.NotContains(t, w.Body.String(), keyB.PrivateKey,
		"A's list must not leak B's private key material (encrypted ciphertext)")

	var resp struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Total, "Total must be owner-scoped to A's 1 key, not the global 2")
}
