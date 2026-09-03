package organizations_tests

// Student offboarding with organization-controlled retention and erasure (#492).
//
// An offboarded member keeps its OrganizationMember row (is_active=false, left_at set,
// scheduled_erasure_at = left_at + the organization's retention) so the class roster
// stays as evidence; the Casdoor account is forbidden; running terminals and assigned
// seats are released. Reinstating (manual route or CSV re-import by email) reverses it
// through ONE helper. Erasure (owner "erase now" or the daily job) runs the shared
// EraseUser cascade and is refused while the user is still active elsewhere.
//
// Shared helpers reused from siblings: installMockEnforcer / registerOrgHooks
// (organizationMemberPermissionSync_test.go), platformMember
// (organizationMemberRoleCap_test.go).

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	auditModels "soli/formations/src/audit/models"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	authServices "soli/formations/src/auth/services"
	"soli/formations/src/cron"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/models"
	organizationRoutes "soli/formations/src/organizations/routes"
	"soli/formations/src/organizations/services"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioModels "soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeIdentity is an in-memory Casdoor: it satisfies authServices.CasdoorUserClient
// so the same instance serves the offboarding service (SetForbidden), the import
// service (GetUserByEmail / UpdateUserForColumns) and userService.DeleteUser.
type fakeIdentity struct {
	mu        sync.Mutex
	users     map[string]*casdoorsdk.User // by id
	forbidden map[string]bool
	deleted   []string
	columns   [][]string
	affected  bool
}

func newFakeIdentity(userIDs ...string) *fakeIdentity {
	f := &fakeIdentity{users: map[string]*casdoorsdk.User{}, forbidden: map[string]bool{}, affected: true}
	for _, id := range userIDs {
		f.users[id] = &casdoorsdk.User{Id: id, Name: "user-" + id, Email: id + "@example.com"}
	}
	return f
}

func (f *fakeIdentity) GetUserByUserId(userID string) (*casdoorsdk.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.users[userID], nil
}

func (f *fakeIdentity) GetUserByEmail(email string) (*casdoorsdk.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user not found: %s", email)
}

func (f *fakeIdentity) DeleteUser(user *casdoorsdk.User) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, user.Id)
	delete(f.users, user.Id)
	return true, nil
}

func (f *fakeIdentity) UpdateUserForColumns(user *casdoorsdk.User, columns []string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.columns = append(f.columns, columns)
	return f.affected, nil
}

func (f *fakeIdentity) SetForbidden(userID string, forbidden bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[userID]; !ok {
		return fmt.Errorf("user not found: %s", userID)
	}
	if !f.affected {
		return errors.New("identity provider did not persist the change")
	}
	f.forbidden[userID] = forbidden
	return nil
}

type fakePaymentHelper struct{}

func (fakePaymentHelper) CancelAllActiveSubscriptionsForUser(string) error { return nil }
func (fakePaymentHelper) DeleteStripeCustomersForUser(string) error        { return nil }
func (fakePaymentHelper) PseudonymizeBillingDataForUser(string) error      { return nil }

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

var offboardingDBSeq int

// newOffboardingDB opens a shared-cache in-memory SQLite so that the import
// service's explicit transaction and its plain db writes see one database.
func newOffboardingDB(t *testing.T) *gorm.DB {
	t.Helper()
	offboardingDBSeq++
	dsn := fmt.Sprintf("file:offboarding_%d?mode=memory&cache=shared", offboardingDBSeq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&terminalModels.UserTerminalKey{},
		&terminalModels.Terminal{},
		&paymentModels.SubscriptionPlan{},
		&paymentModels.SubscriptionBatch{},
		&paymentModels.UserSubscription{},
		&paymentModels.OrganizationSubscription{},
		&paymentModels.OrganizationRolePlan{},
		&authModels.UserSettings{},
		&authModels.TokenBlacklist{},
		&authModels.EmailVerificationToken{},
		&authModels.PasswordResetToken{},
		&authModels.SshKey{},
		&auditModels.AuditLog{},
		&scenarioModels.Scenario{},
		&scenarioModels.ScenarioSession{},
		&scenarioModels.ScenarioStepProgress{},
		&scenarioModels.ScenarioFlag{},
		&scenarioModels.ScenarioAssignment{},
	))
	return db
}

func seedTeamOrg(t *testing.T, db *gorm.DB, ownerID string, retentionDays *int) uuid.UUID {
	t.Helper()
	org := &models.Organization{
		Name:             "offboard-org-" + uuid.NewString()[:8],
		DisplayName:      "Offboarding Org",
		OwnerUserID:      ownerID,
		OrganizationType: models.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       100,
		IsActive:         true,
		RetentionDays:    retentionDays,
	}
	org.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata", "OwnerIDs", "Members", "Groups").Create(org).Error)
	seedMember(t, db, org.ID, ownerID, models.OrgRoleOwner)
	return org.ID
}

func seedMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role models.OrganizationMemberRole) {
	t.Helper()
	m := &models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: role, JoinedAt: time.Now(), IsActive: true}
	require.NoError(t, db.Omit("Metadata").Create(m).Error)
}

func loadMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) models.OrganizationMember {
	t.Helper()
	var m models.OrganizationMember
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&m).Error)
	return m
}

func countMemberRows(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", orgID, userID).Count(&n).Error)
	return n
}

func seedRunningTerminal(t *testing.T, db *gorm.DB, userID string) string {
	t.Helper()
	key := &terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + userID, KeyName: "k", IsActive: true}
	key.ID = uuid.New()
	require.NoError(t, db.Create(key).Error)
	term := &terminalModels.Terminal{
		SessionID:         "sess-" + userID,
		UserID:            userID,
		State:             terminalModels.StateRunning,
		ExpiresAt:         time.Now().Add(2 * time.Hour),
		LastStartedAt:     time.Now(),
		UserTerminalKeyID: key.ID,
	}
	term.ID = uuid.New()
	require.NoError(t, db.Create(term).Error)
	return term.SessionID
}

func seedAssignedLicence(t *testing.T, db *gorm.DB, purchaserID, userID string) uuid.UUID {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{Name: "Seat plan " + uuid.NewString()[:8], PriceAmount: 1000, IsActive: true}
	plan.ID = uuid.New()
	require.NoError(t, db.Create(plan).Error)
	batch := &paymentModels.SubscriptionBatch{
		PurchaserUserID: purchaserID, SubscriptionPlanID: plan.ID, StripeSubscriptionID: "sub_" + uuid.NewString(),
		TotalQuantity: 5, AssignedQuantity: 1, Status: "active",
	}
	batch.ID = uuid.New()
	require.NoError(t, db.Create(batch).Error)
	licence := &paymentModels.UserSubscription{
		UserID: userID, PurchaserUserID: &purchaserID, SubscriptionBatchID: &batch.ID,
		SubscriptionType: "assigned", SubscriptionPlanID: plan.ID, Status: "active",
	}
	licence.ID = uuid.New()
	require.NoError(t, db.Create(licence).Error)
	return licence.ID
}

func seedClassWithMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, teacherID, studentID string) uuid.UUID {
	t.Helper()
	class := &groupModels.ClassGroup{Name: "class-" + uuid.NewString()[:8], DisplayName: "Class", OwnerUserID: teacherID, OrganizationID: &orgID, IsActive: true}
	class.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(class).Error)
	gm := &groupModels.GroupMember{GroupID: class.ID, UserID: studentID, Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: true}
	require.NoError(t, db.Omit("Metadata").Create(gm).Error)
	return class.ID
}

func newDeletionService(db *gorm.DB, identity *fakeIdentity) authServices.UserDeletionService {
	return authServices.NewUserDeletionService(db, authServices.NewUserService(identity, fakePaymentHelper{}))
}

func newOffboardingService(db *gorm.DB, identity *fakeIdentity) services.MemberOffboardingService {
	return services.NewMemberOffboardingService(db, identity, paymentServices.NewBulkLicenseService(db), newDeletionService(db, identity))
}

func intPtr(v int) *int { return &v }

// ---------------------------------------------------------------------------
// Offboard
// ---------------------------------------------------------------------------

func TestOffboardMembers_SetsLeftAtInactiveAndScheduledErasureFromOrgRetention(t *testing.T) {
	t.Setenv("OCF_DEFAULT_RETENTION_DAYS", "")
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a", "student-b")
	svc := newOffboardingService(db, identity)

	orgWithRetention := seedTeamOrg(t, db, "owner-1", intPtr(30))
	seedMember(t, db, orgWithRetention, "student-a", models.OrgRoleMember)
	orgWithDefault := seedTeamOrg(t, db, "owner-2", nil)
	seedMember(t, db, orgWithDefault, "student-b", models.OrgRoleMember)

	before := time.Now()
	require.NoError(t, svc.Offboard(orgWithRetention, []string{"student-a"}, "owner-1"))
	require.NoError(t, svc.Offboard(orgWithDefault, []string{"student-b"}, "owner-2"))

	a := loadMember(t, db, orgWithRetention, "student-a")
	assert.False(t, a.IsActive, "an offboarded member grants nothing: is_active must be false")
	require.NotNil(t, a.LeftAt)
	require.NotNil(t, a.ScheduledErasureAt)
	assert.WithinDuration(t, before, *a.LeftAt, 5*time.Second)
	assert.WithinDuration(t, before.AddDate(0, 0, 30), *a.ScheduledErasureAt, 5*time.Second,
		"scheduled_erasure_at must be left_at + the organization's retention_days")
	assert.True(t, a.IsOffboarded())

	b := loadMember(t, db, orgWithDefault, "student-b")
	require.NotNil(t, b.ScheduledErasureAt)
	assert.WithinDuration(t, before.AddDate(0, 0, 365), *b.ScheduledErasureAt, 5*time.Second,
		"an organization without retention_days falls back to the platform default (365)")
}

func TestOffboardMembers_ForbidsTheCasdoorAccount_WithColumnList(t *testing.T) {
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	// The real client, with the SDK seams swapped: the assertion is on the exact
	// column list the production code sends, not on a fake's own bookkeeping.
	var writtenColumns []string
	var writtenUser *casdoorsdk.User
	affected := true
	restore := authServices.SwapCasdoorUserWriter(
		func(userID string) (*casdoorsdk.User, error) {
			return &casdoorsdk.User{Id: userID, Name: "user-" + userID}, nil
		},
		func(user *casdoorsdk.User, columns []string) (bool, error) {
			writtenUser, writtenColumns = user, columns
			return affected, nil
		},
	)
	t.Cleanup(restore)

	realClient := authServices.NewCasdoorUserClient()
	svc := services.NewMemberOffboardingService(db, realClient, paymentServices.NewBulkLicenseService(db), nil)

	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))
	require.NotNil(t, writtenUser)
	assert.True(t, writtenUser.IsForbidden, "the account must be sent with is_forbidden=true")
	assert.Equal(t, []string{"is_forbidden"}, writtenColumns,
		"UpdateUserForColumns must name the column: a bare UpdateUser silently drops it")

	// affected=false with a nil error means Casdoor changed nothing: the offboarding
	// must fail and the membership must be rolled back to active.
	seedMember(t, db, orgID, "student-b", models.OrgRoleMember)
	affected = false
	err := svc.Offboard(orgID, []string{"student-b"}, "owner-1")
	require.Error(t, err, "affected=false must be treated as a failure")
	b := loadMember(t, db, orgID, "student-b")
	assert.True(t, b.IsActive, "the transaction must be rolled back when the account was not forbidden")
	assert.Nil(t, b.LeftAt)
}

func TestOffboardedMember_GetsNoEffectivePlanFromTheOrg(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	plans := paymentServices.NewEffectivePlanService(db)
	_, errBefore := plans.GetUserEffectivePlan("student-a", &orgID)
	if errBefore != nil {
		assert.NotContains(t, errBefore.Error(), "is not a member", "sanity: the student is a member before offboarding")
	}

	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))

	_, errAfter := plans.GetUserEffectivePlan("student-a", &orgID)
	require.Error(t, errAfter)
	assert.Contains(t, errAfter.Error(), "is not a member",
		"the existing is_active gate is the one owner of 'offboarded grants nothing'")
}

func TestOffboardedMember_TerminalsAreTerminated_AndLicenceReleased(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	sessionID := seedRunningTerminal(t, db, "student-a")
	licenceID := seedAssignedLicence(t, db, "owner-1", "student-a")

	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))

	var term terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&term).Error)
	assert.NotEqual(t, terminalModels.StateRunning, term.State, "a running terminal must be terminated on offboarding")

	var licence paymentModels.UserSubscription
	require.NoError(t, db.First(&licence, "id = ?", licenceID).Error)
	assert.Equal(t, "unassigned", licence.Status, "the assigned seat must be released to the pool")
	assert.Empty(t, licence.UserID)
	var batch paymentModels.SubscriptionBatch
	require.NoError(t, db.First(&batch, "id = ?", *licence.SubscriptionBatchID).Error)
	assert.Equal(t, 0, batch.AssignedQuantity, "the batch counter must reflect the released seat")

	assert.True(t, identity.forbidden["student-a"], "the account must be forbidden")
}

func TestOffboardedMember_GroupRosterRowsAreUntouched(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	classID := seedClassWithMember(t, db, orgID, "owner-1", "student-a")

	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))

	var roster groupModels.GroupMember
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", classID, "student-a").First(&roster).Error)
	assert.True(t, roster.IsActive, "the class roster is evidence: offboarding must not touch group_members")
}

// ---------------------------------------------------------------------------
// Reinstate — one helper for the manual route, the CSV re-import and the add path
// ---------------------------------------------------------------------------

func TestReinstate_ClearsBothFieldsAndUnforbidsTheAccount(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))
	require.True(t, identity.forbidden["student-a"])

	require.NoError(t, svc.Reinstate(orgID, "student-a"))

	m := loadMember(t, db, orgID, "student-a")
	assert.True(t, m.IsActive)
	assert.Nil(t, m.LeftAt)
	assert.Nil(t, m.ScheduledErasureAt)
	assert.False(t, m.IsOffboarded())
	assert.False(t, identity.forbidden["student-a"], "the account must be usable again")
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "student-a"), "reinstating reuses the row")
}

func usersCSV(t *testing.T, rows string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("users", "users.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte("email,first_name,last_name,role\n" + rows))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.File["users"][0]
}

func TestCsvImport_ReenrolsAnOffboardedStudentByEmail(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	require.NoError(t, offboarding.Offboard(orgID, []string{"student-a"}, "owner-1"))

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "student-a@example.com,Ada,Lovelace,member\n"), nil, nil, false, true, "")
	require.NoError(t, err, "errors=%+v", resp.Errors)

	m := loadMember(t, db, orgID, "student-a")
	assert.True(t, m.IsActive, "re-importing an offboarded student by email must reinstate the membership")
	assert.Nil(t, m.LeftAt)
	assert.Nil(t, m.ScheduledErasureAt)
	assert.False(t, identity.forbidden["student-a"])
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "student-a"), "no duplicate membership row")

	require.NotEmpty(t, identity.columns, "the existing account is updated with an explicit column list")
	for _, cols := range identity.columns {
		assert.NotEmpty(t, cols, "processUser must never fall back to a bare UpdateUser")
	}
}

func TestManualAddMember_ReenrolsAnOffboardedStudent(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))
	registerOrgHooks(t, db)

	// The manual add is the generic POST /organization-members create, whose
	// BeforeCreate hook cannot turn an insert into an update. It must refuse
	// (pointing at the reinstate action) rather than insert a second row for a
	// user whose membership still exists.
	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: "student-a", Role: models.OrgRoleMember},
		UserID:     "owner-1",
		UserRoles:  platformMember,
	}
	err := hooks.GlobalHookRegistry.ExecuteHooks(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrMemberOffboarded)
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "student-a"), "no duplicate membership row")

	require.NoError(t, svc.Reinstate(orgID, "student-a"))
	assert.True(t, loadMember(t, db, orgID, "student-a").IsActive)
}

// ---------------------------------------------------------------------------
// Authority
// ---------------------------------------------------------------------------

func TestOffboard_RequiresOrgManager_EraseRequiresOrgOwner(t *testing.T) {
	enforcer := mocks.NewMockEnforcer()
	organizationRoutes.RegisterOrganizationPermissions(enforcer)

	cases := []struct {
		path    string
		minRole string
	}{
		{"/api/v1/organizations/:id/members/offboard", "manager"},
		{"/api/v1/organizations/:id/members/:userId/reinstate", "manager"},
		{"/api/v1/organizations/:id/members/:userId/erase", "owner"},
	}
	for _, c := range cases {
		perm, found := access.RouteRegistry.Lookup("POST", c.path)
		require.True(t, found, "route %s must be declared for Layer 2", c.path)
		assert.Equal(t, access.OrgRole, perm.Access.Type)
		assert.Equal(t, "id", perm.Access.Param)
		assert.Equal(t, c.minRole, perm.Access.MinRole, "%s", c.path)
		assert.Equal(t, access.RoleMember, perm.Role, "every real user is a platform member; authority is the org role")

		policyFound := false
		for _, call := range enforcer.AddPolicyCalls {
			if len(call) >= 3 && call[0] == "member" && call[1] == c.path && call[2] == "POST" {
				policyFound = true
			}
		}
		assert.True(t, policyFound, "missing Casbin policy for %s", c.path)
	}
}

func TestRetentionDays_OnlyOwnerMayChangeIt(t *testing.T) {
	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, "owner-1", map[string]models.OrganizationMemberRole{
		"owner-1":   models.OrgRoleOwner,
		"manager-1": models.OrgRoleManager,
	})
	registerOrgHooks(t, db)

	var org models.Organization
	require.NoError(t, db.First(&org, "id = ?", orgID).Error)

	patch := func(actor string, roles []string) error {
		return hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
			EntityName: "Organization",
			HookType:   hooks.BeforeUpdate,
			EntityID:   orgID,
			OldEntity:  &org,
			NewEntity:  map[string]any{"retention_days": 90},
			UserID:     actor,
			UserRoles:  roles,
		})
	}

	require.Error(t, patch("manager-1", platformMember), "a manager must not change the retention period")
	require.NoError(t, patch("owner-1", platformMember), "the owner (data controller) may")
	require.NoError(t, patch("platform-admin", []string{"administrator"}), "platform administrators bypass")

	// A PATCH that does not touch retention_days is not the hook's business.
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "Organization", HookType: hooks.BeforeUpdate, EntityID: orgID, OldEntity: &org,
		NewEntity: map[string]any{"display_name": "Renamed"}, UserID: "manager-1", UserRoles: platformMember,
	}))
}

// ---------------------------------------------------------------------------
// Erasure
// ---------------------------------------------------------------------------

func offboardedAt(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, leftAt, scheduled time.Time) {
	t.Helper()
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Updates(map[string]any{"is_active": false, "left_at": leftAt, "scheduled_erasure_at": scheduled}).Error)
}

func TestMemberErasureJob_ErasesOnlyDueMembers(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("due", "not-due", "elsewhere")
	eraser := newDeletionService(db, identity)
	now := time.Now()
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	otherOrgID := seedTeamOrg(t, db, "owner-2", intPtr(10))
	for _, u := range []string{"due", "not-due", "elsewhere"} {
		seedMember(t, db, orgID, u, models.OrgRoleMember)
		seedRunningTerminal(t, db, u)
	}
	seedMember(t, db, otherOrgID, "elsewhere", models.OrgRoleMember)
	offboardedAt(t, db, orgID, "due", now.AddDate(0, 0, -20), now.Add(-time.Hour))
	offboardedAt(t, db, orgID, "not-due", now.AddDate(0, 0, -2), now.AddDate(0, 0, 8))
	offboardedAt(t, db, orgID, "elsewhere", now.AddDate(0, 0, -20), now.Add(-time.Hour))

	report, err := cron.RunMemberErasure(db, eraser, now, 50)
	require.NoError(t, err)

	assert.Equal(t, 1, report.Erased)
	require.Len(t, report.Skipped, 1, "the member still active in another organization must be reported, not erased")
	assert.Equal(t, "elsewhere", report.Skipped[0].UserID)
	assert.ErrorIs(t, report.Skipped[0].Reason, authServices.ErrStillActiveElsewhere)

	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "due"), "the due member is erased")
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "not-due"), "a member inside retention is untouched")
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "elsewhere"), "the blocked member is untouched")
	assert.Equal(t, []string{"due"}, identity.deleted)

	t.Run("refuses to run above the per-run cap", func(t *testing.T) {
		offboardedAt(t, db, orgID, "not-due", now.AddDate(0, 0, -20), now.Add(-time.Hour))
		_, err := cron.RunMemberErasure(db, eraser, now, 1)
		require.Error(t, err, "two due members over a cap of one: the run must be refused, not partially executed")
		assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "not-due"))
		assert.Equal(t, []string{"due"}, identity.deleted, "nothing was erased by the refused run")
	})
}

func TestEraseNow_RunsTheSharedCascade(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	classID := seedClassWithMember(t, db, orgID, "owner-1", "student-a")
	seedRunningTerminal(t, db, "student-a")
	require.NoError(t, db.Create(&authModels.UserSettings{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, UserID: "student-a", DefaultLandingPage: "/dashboard",
	}).Error)

	err := svc.EraseNow(orgID, "student-a")
	require.ErrorIs(t, err, services.ErrMemberNotOffboarded, "erasure is the end of offboarding, never a shortcut past it")

	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))
	require.NoError(t, svc.EraseNow(orgID, "student-a"))

	var orgMembers, groupMembers, settings, keys, liveTerminals int64
	db.Model(&models.OrganizationMember{}).Where("user_id = ?", "student-a").Count(&orgMembers)
	db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ?", classID, "student-a").Count(&groupMembers)
	db.Model(&authModels.UserSettings{}).Where("user_id = ?", "student-a").Count(&settings)
	db.Model(&terminalModels.UserTerminalKey{}).Where("user_id = ?", "student-a").Count(&keys)
	db.Model(&terminalModels.Terminal{}).Where("user_id = ?", "student-a").Count(&liveTerminals)
	assert.Equal(t, int64(0), orgMembers, "memberships removed by the cascade")
	assert.Equal(t, int64(0), groupMembers, "group memberships removed by the cascade")
	assert.Equal(t, int64(0), settings, "settings removed by the cascade")
	assert.Equal(t, int64(0), keys, "terminal credentials removed by the cascade")
	assert.Equal(t, int64(0), liveTerminals, "terminal rows no longer link to the erased user")
	assert.Equal(t, []string{"student-a"}, identity.deleted, "the Casdoor identity is deleted")

	var teamOrgCount int64
	db.Model(&models.Organization{}).Where("id = ?", orgID).Count(&teamOrgCount)
	assert.Equal(t, int64(1), teamOrgCount, "the organization survives")
}

func TestEraseNow_RefusedWhileActiveElsewhere_AndListedAsBlocked(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	otherOrgID := seedTeamOrg(t, db, "owner-2", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	seedMember(t, db, otherOrgID, "student-a", models.OrgRoleMember)
	require.NoError(t, svc.Offboard(orgID, []string{"student-a"}, "owner-1"))

	err := svc.EraseNow(orgID, "student-a")
	require.ErrorIs(t, err, authServices.ErrStillActiveElsewhere)
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "student-a"))

	blocked := newDeletionService(db, identity).CheckDepartedMemberErasable(orgID, "student-a")
	require.ErrorIs(t, blocked, authServices.ErrStillActiveElsewhere,
		"the members list reports the same reason the erasure would refuse with")
}

func TestOffboard_EmptySelectionAndOwner_AreRefused(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("owner-1", "student-a")
	svc := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	require.ErrorIs(t, svc.Offboard(orgID, nil, "owner-1"), services.ErrNoMembersSelected)
	require.ErrorIs(t, svc.Offboard(orgID, []string{"owner-1"}, "owner-1"), services.ErrCannotOffboardOwner)
	require.ErrorIs(t, svc.Offboard(orgID, []string{"nobody"}, "owner-1"), services.ErrMemberNotFound)
	assert.True(t, loadMember(t, db, orgID, "owner-1").IsActive)
	assert.False(t, identity.forbidden["owner-1"])
}
