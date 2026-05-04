package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/admin/routes/adminUsersRoutes"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
)

// ---------------------------------------------------------------------------
// Slice B6 — TDD tests for the admin "users with memberships" listing.
//
// The production package src/admin/routes/adminUsersRoutes does not yet
// exist; running this file must FAIL at compile time until the next slice
// implements:
//
//   - adminUsersRoutes.BuildUserListings(users, db, isAdminFn) ([]UserListing, error)
//   - adminUsersRoutes.UserListing            (DTO)
//   - adminUsersRoutes.OrgMembership          (nested DTO)
//   - adminUsersRoutes.GroupMembership        (nested DTO)
//   - adminUsersRoutes.ListAllCasdoorUsers    (swappable var)
//   - adminUsersRoutes.NewListUsersHandler(db) gin.HandlerFunc
//
// The DTO field names are pinned by the JSON tags asserted in the handler
// test below (id / username / display_name / email / avatar / is_active /
// is_admin / organizations / groups, plus role / name on each membership).
// ---------------------------------------------------------------------------

const (
	listPath = "/admin/users-with-memberships"

	// Casdoor IDs used across the tests.
	uidAlice   = "alice-id"
	uidBob     = "bob-id"
	uidCarol   = "carol-id"
	uidNobody  = "nobody-id"
	uidAdminOp = "admin-op-id"
)

// ---------------------------------------------------------------------------
// Helpers — fake Casdoor users + isAdmin function
// ---------------------------------------------------------------------------

// fakeUser builds a minimal *casdoorsdk.User suitable for the listing.
func fakeUser(id, name, displayName, email, avatar string) *casdoorsdk.User {
	return &casdoorsdk.User{
		Id:          id,
		Name:        name,
		DisplayName: displayName,
		Email:       email,
		Avatar:      avatar,
		IsForbidden: false,
	}
}

// noAdminFn returns false for every userID — the typical case for a
// listing where no Casbin admin role is bound.
func noAdminFn() func(string) bool {
	return func(string) bool { return false }
}

// adminForFn returns true only for the given userIDs.
func adminForFn(adminIDs ...string) func(string) bool {
	set := map[string]bool{}
	for _, id := range adminIDs {
		set[id] = true
	}
	return func(uid string) bool { return set[uid] }
}

// ---------------------------------------------------------------------------
// Seed helpers — Organization + OrganizationMember + ClassGroup + GroupMember
//
// All seeds Omit jsonb Metadata and pq text[] OwnerIDs fields to keep
// SQLite happy.
// ---------------------------------------------------------------------------

func seedOrg(t *testing.T, db *gorm.DB, name, ownerID string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	org := &orgModels.Organization{
		Name:             name,
		DisplayName:      name,
		OwnerUserID:      ownerID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	org.ID = id
	require.NoError(t, db.Omit("Metadata", "OwnerIDs", "Members", "Groups").Create(org).Error,
		"failed to seed organization %q", name)
	return id
}

func seedOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role orgModels.OrganizationMemberRole, active bool) {
	t.Helper()
	m := &orgModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       active,
	}
	require.NoError(t, db.Omit("Metadata").Create(m).Error,
		"failed to seed organization_member user=%s org=%s", userID, orgID)
	if !active {
		// GORM `default:true` on IsActive overrides the literal `false` on insert;
		// force the column to false with a follow-up update.
		require.NoError(t, db.Model(m).Update("is_active", false).Error,
			"failed to force is_active=false on organization_member user=%s org=%s", userID, orgID)
	}
}

func seedGroup(t *testing.T, db *gorm.DB, name, ownerID string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	g := &groupModels.ClassGroup{
		Name:        name,
		DisplayName: name,
		OwnerUserID: ownerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	g.ID = id
	require.NoError(t, db.Omit("Metadata", "OwnerIDs", "Members", "SubGroups", "ParentGroup").Create(g).Error,
		"failed to seed class_group %q", name)
	return id
}

func seedGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole, active bool) {
	t.Helper()
	m := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
		IsActive: active,
	}
	require.NoError(t, db.Omit("Metadata").Create(m).Error,
		"failed to seed group_member user=%s group=%s", userID, groupID)
	if !active {
		// GORM `default:true` on IsActive overrides the literal `false` on insert;
		// force the column to false with a follow-up update.
		require.NoError(t, db.Model(m).Update("is_active", false).Error,
			"failed to force is_active=false on group_member user=%s group=%s", userID, groupID)
	}
}

// ---------------------------------------------------------------------------
// findListing returns the UserListing entry for the given Casdoor user id.
// Uses generics-free reflection-light access via the package's exported DTO.
// ---------------------------------------------------------------------------

func findListing(t *testing.T, listings []adminUsersRoutes.UserListing, id string) adminUsersRoutes.UserListing {
	t.Helper()
	for _, l := range listings {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("no listing found with id=%q (have %d listings)", id, len(listings))
	return adminUsersRoutes.UserListing{}
}

// ---------------------------------------------------------------------------
// Service-layer tests (BuildUserListings)
// ---------------------------------------------------------------------------

func TestBuildUserListings_EmptyUsers_ReturnsEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	listings, err := adminUsersRoutes.BuildUserListings(nil, db, noAdminFn())
	require.NoError(t, err)
	assert.Len(t, listings, 0, "empty input must produce empty output")
}

func TestBuildUserListings_UserWithNoMemberships_ReturnsEmptyArrays(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", "https://avatar/alice.png"),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)
	require.Len(t, listings, 1)

	got := findListing(t, listings, uidAlice)
	assert.Equal(t, uidAlice, got.ID)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, "Alice", got.DisplayName)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, "https://avatar/alice.png", got.Avatar)
	assert.True(t, got.IsActive, "non-forbidden Casdoor user must be active")
	assert.False(t, got.IsAdmin, "isAdminFn returned false for this user")
	assert.NotNil(t, got.Organizations, "organizations must be non-nil (use empty slice, not nil) so JSON is [] not null")
	assert.NotNil(t, got.Groups, "groups must be non-nil (use empty slice, not nil) so JSON is [] not null")
	assert.Len(t, got.Organizations, 0)
	assert.Len(t, got.Groups, 0)
}

func TestBuildUserListings_UserWithOrgMembership_IncludesOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	orgID := seedOrg(t, db, "Acme Corp", uidAlice)
	seedOrgMember(t, db, orgID, uidAlice, orgModels.OrgRoleOwner, true)

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)
	require.Len(t, got.Organizations, 1, "alice should have exactly one org membership")

	orgMb := got.Organizations[0]
	assert.Equal(t, orgID.String(), orgMb.ID, "org id must be the seeded uuid as string")
	assert.Equal(t, "Acme Corp", orgMb.Name, "org name must come from organizations.name")
	assert.Equal(t, "owner", orgMb.Role)
	assert.Len(t, got.Groups, 0)
}

func TestBuildUserListings_UserWithGroupMembership_IncludesGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	groupID := seedGroup(t, db, "Linux 101", uidAlice)
	seedGroupMember(t, db, groupID, uidAlice, groupModels.GroupMemberRoleManager, true)

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)
	require.Len(t, got.Groups, 1, "alice should have exactly one group membership")

	gMb := got.Groups[0]
	assert.Equal(t, groupID.String(), gMb.ID)
	assert.Equal(t, "Linux 101", gMb.Name, "group name must come from class_groups.name")
	assert.Equal(t, "manager", gMb.Role)
	assert.Len(t, got.Organizations, 0)
}

func TestBuildUserListings_UserWithMultipleOrgsAndGroups_AllReturned(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	// Two orgs: alice is owner of one, member of the other.
	org1 := seedOrg(t, db, "OrgOne", uidAlice)
	org2 := seedOrg(t, db, "OrgTwo", uidBob)
	seedOrgMember(t, db, org1, uidAlice, orgModels.OrgRoleOwner, true)
	seedOrgMember(t, db, org2, uidAlice, orgModels.OrgRoleMember, true)

	// Two groups: alice is manager of one, plain member of the other.
	g1 := seedGroup(t, db, "GroupOne", uidAlice)
	g2 := seedGroup(t, db, "GroupTwo", uidBob)
	seedGroupMember(t, db, g1, uidAlice, groupModels.GroupMemberRoleManager, true)
	seedGroupMember(t, db, g2, uidAlice, groupModels.GroupMemberRoleMember, true)

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)
	assert.Len(t, got.Organizations, 2, "alice must surface BOTH org memberships")
	assert.Len(t, got.Groups, 2, "alice must surface BOTH group memberships")

	// Roles: extract them into a name->role map for stable assertions.
	orgRoles := map[string]string{}
	for _, o := range got.Organizations {
		orgRoles[o.Name] = o.Role
	}
	assert.Equal(t, "owner", orgRoles["OrgOne"])
	assert.Equal(t, "member", orgRoles["OrgTwo"])

	groupRoles := map[string]string{}
	for _, g := range got.Groups {
		groupRoles[g.Name] = g.Role
	}
	assert.Equal(t, "manager", groupRoles["GroupOne"])
	assert.Equal(t, "member", groupRoles["GroupTwo"])
}

func TestBuildUserListings_AdminFlag_TrueWhenAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	users := []*casdoorsdk.User{
		fakeUser(uidAdminOp, "platformadmin", "Platform Admin", "admin@ocf.local", ""),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, adminForFn(uidAdminOp))
	require.NoError(t, err)

	got := findListing(t, listings, uidAdminOp)
	assert.True(t, got.IsAdmin, "isAdminFn returned true for this user — IsAdmin must be true")
}

func TestBuildUserListings_AdminFlag_FalseForRegularUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""),
	}

	// Admin function is set up to recognize a DIFFERENT user as admin.
	listings, err := adminUsersRoutes.BuildUserListings(users, db, adminForFn(uidAdminOp))
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)
	assert.False(t, got.IsAdmin, "isAdminFn returned false for this user — IsAdmin must be false")
}

func TestBuildUserListings_OnlyReturnsActiveOrgMemberships(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	activeOrg := seedOrg(t, db, "ActiveOrg", uidAlice)
	inactiveOrg := seedOrg(t, db, "InactiveOrg", uidBob)
	seedOrgMember(t, db, activeOrg, uidAlice, orgModels.OrgRoleOwner, true)
	seedOrgMember(t, db, inactiveOrg, uidAlice, orgModels.OrgRoleMember, false) // soft-removed

	activeGroup := seedGroup(t, db, "ActiveGroup", uidAlice)
	inactiveGroup := seedGroup(t, db, "InactiveGroup", uidBob)
	seedGroupMember(t, db, activeGroup, uidAlice, groupModels.GroupMemberRoleManager, true)
	seedGroupMember(t, db, inactiveGroup, uidAlice, groupModels.GroupMemberRoleMember, false) // soft-removed

	users := []*casdoorsdk.User{
		fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""),
	}

	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)

	require.Len(t, got.Organizations, 1, "inactive org membership must be filtered out")
	assert.Equal(t, "ActiveOrg", got.Organizations[0].Name)

	require.Len(t, got.Groups, 1, "inactive group membership must be filtered out")
	assert.Equal(t, "ActiveGroup", got.Groups[0].Name)
}

// ---------------------------------------------------------------------------
// Handler test — verifies the HTTP shape and the ListAllCasdoorUsers swap
// ---------------------------------------------------------------------------

// adminCtxInjector mimics AuthManagement by setting userId + administrator role.
func adminCtxInjector() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userId", uidAdminOp)
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	}
}

// handlerJSONUser is a minimal struct mirroring the JSON response shape.
// Keeping this local (instead of importing the package DTO) ensures we test
// the JSON contract — including the kebab/snake_case tags — and not just the
// Go field names.
type handlerJSONUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Email         string `json:"email"`
	Avatar        string `json:"avatar"`
	IsActive      bool   `json:"is_active"`
	IsAdmin       bool   `json:"is_admin"`
	Organizations []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"organizations"`
	Groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"groups"`
}

func TestAdminUsersHandler_OverrideListAllCasdoorUsers_ReturnsExpectedJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	// Seed alice as owner of OrgOne and manager of GroupOne.
	orgID := seedOrg(t, db, "OrgOne", uidAlice)
	seedOrgMember(t, db, orgID, uidAlice, orgModels.OrgRoleOwner, true)
	groupID := seedGroup(t, db, "GroupOne", uidAlice)
	seedGroupMember(t, db, groupID, uidAlice, groupModels.GroupMemberRoleManager, true)

	// Seed bob as a plain member of the same org (no group memberships).
	seedOrgMember(t, db, orgID, uidBob, orgModels.OrgRoleMember, true)

	// Override the swappable Casdoor lookup to return a controlled fake list.
	original := adminUsersRoutes.ListAllCasdoorUsers
	adminUsersRoutes.ListAllCasdoorUsers = func() ([]*casdoorsdk.User, error) {
		return []*casdoorsdk.User{
			fakeUser(uidAlice, "alice", "Alice", "alice@example.com", "https://avatar/alice.png"),
			fakeUser(uidBob, "bob", "Bob", "bob@example.com", ""),
		}, nil
	}
	t.Cleanup(func() { adminUsersRoutes.ListAllCasdoorUsers = original })

	// Build a router with the handler under test.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(adminCtxInjector())
	r.GET(listPath, adminUsersRoutes.NewListUsersHandler(db))

	req := httptest.NewRequest(http.MethodGet, listPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "handler must return 200; body=%s", w.Body.String())

	var got []handlerJSONUser
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got),
		"response must decode as []handlerJSONUser; raw=%s", w.Body.String())
	require.Len(t, got, 2, "must return both seeded users")

	// Find alice's entry.
	var alice handlerJSONUser
	var foundAlice bool
	for _, u := range got {
		if u.ID == uidAlice {
			alice = u
			foundAlice = true
			break
		}
	}
	require.True(t, foundAlice, "alice must be present in response; raw=%s", w.Body.String())

	assert.Equal(t, "alice", alice.Username)
	assert.Equal(t, "Alice", alice.DisplayName)
	assert.Equal(t, "alice@example.com", alice.Email)
	assert.Equal(t, "https://avatar/alice.png", alice.Avatar)
	assert.True(t, alice.IsActive)
	assert.False(t, alice.IsAdmin)

	require.Len(t, alice.Organizations, 1)
	assert.Equal(t, orgID.String(), alice.Organizations[0].ID)
	assert.Equal(t, "OrgOne", alice.Organizations[0].Name)
	assert.Equal(t, "owner", alice.Organizations[0].Role)

	require.Len(t, alice.Groups, 1)
	assert.Equal(t, groupID.String(), alice.Groups[0].ID)
	assert.Equal(t, "GroupOne", alice.Groups[0].Name)
	assert.Equal(t, "manager", alice.Groups[0].Role)
}
