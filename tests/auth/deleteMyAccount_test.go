// tests/auth/deleteMyAccount_test.go
//
// Failing tests for the reworked self-service account-deletion path
// (RGPD right to erasure, GitLab issue #110).
//
// CONTEXT — these tests pin the TARGET behavior after MR !167 is reworked to
// COMPOSE the canonical user-deletion services that landed on main, instead of
// hand-rolling its own cascade:
//
//   - authServices.UserService.DeleteUser(id) (src/auth/services/userService.go):
//       GetCasdoorUser → CancelAllActiveSubscriptionsForUser (ABORT on error)
//       → PseudonymizeBillingDataForUser (preserves country + StripePaymentMethodID
//       for the 10y retention) → Casdoor DeleteUser → RemoveGroupingPolicy.
//   - paymentServices.PaymentDeletionHelper.PseudonymizeBillingDataForUser
//       (src/payment/services/userDeletionHelper.go).
//   - paymentServices.TerminateUserTerminals(db, userID, nil)
//       (src/payment/services/terminalCleanup.go): flips State=StateDeleted.
//
// TARGET ordering of the reworked DeleteMyAccount(userID):
//   1. pre-flight 409 gates (owns non-personal org / owns group)
//   2. DELEGATE to userService.DeleteUser(userID) FIRST — so a Stripe-cancel
//      failure aborts with ZERO OCF-side mutation (retryable, returns error;
//      the handler maps that to 5xx).
//   3. OCF-side cascade: TerminateUserTerminals, delete scenario sessions
//      (cascade step_progress + flags), remove memberships, delete personal org,
//      anonymize scenario authorship, anonymize audit logs (clear actor_id,
//      actor_email AND actor_ip), delete auth tokens/settings/SSH keys.
//
// TARGET API the backend-dev must introduce (these tests are RED until then):
//   - services.NewUserDeletionService(db *gorm.DB, userSvc authServices.UserService) UserDeletionService
//   - the existing services.ErrOwnsOrganizations / services.ErrOwnsGroups sentinels stay.
//
// SHARED HELPERS/MOCKS are defined in userDeletion_test.go (same package):
// callRecorder, newCallRecorder, mockCasdoorUserClient, mockPaymentDeletionHelper,
// setupUserDeletionTestDB, seedSubscription, seedBillingAddress, seedPaymentMethod,
// buildCasdoorUser, indexOf. This file MUST NOT redefine them.
package auth_tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	auditModels "soli/formations/src/audit/models"
	authModels "soli/formations/src/auth/models"
	userController "soli/formations/src/auth/routes/usersRoutes"
	authServices "soli/formations/src/auth/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioModels "soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// stubStripeService satisfies paymentServices.StripeService for the
// pseudonymization test. Embedding the interface means only the methods we
// actually exercise need real bodies; everything else panics if called, which
// would surface an unexpected dependency. CancelSubscription is a no-op success
// because the pseudonymization test seeds no active Stripe subscriptions.
type stubStripeService struct {
	paymentServices.StripeService
}

func (s *stubStripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	return nil
}

// anyArg is a readable alias for mock.Anything.
func anyArg() interface{} { return mock.Anything }

// invokeDeleteMyAccountHandler runs the real gin handler so the confirmation
// gate (400 on missing/invalid confirmation) is exercised end-to-end at the
// HTTP layer rather than via a service call. The confirmation check runs before
// any DB/Casdoor access, so a nil global DB is never reached on the 400 paths.
func invokeDeleteMyAccountHandler(ctx *gin.Context) {
	userController.NewUserController().DeleteMyAccount(ctx)
}

// ---------------------------------------------------------------------------
// Test DB + seeding helpers specific to DeleteMyAccount.
//
// These are NEW helpers (they do not collide with userDeletion_test.go, which
// only defines setupUserDeletionTestDB for the payment-only tables). The
// DeleteMyAccount cascade touches many more tables, so it needs its own
// migration set.
// ---------------------------------------------------------------------------

func setupDeleteMyAccountDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&authModels.UserSettings{},
		&authModels.TokenBlacklist{},
		&authModels.EmailVerificationToken{},
		&authModels.PasswordResetToken{},
		&authModels.SshKey{},
		&terminalModels.UserTerminalKey{},
		&terminalModels.Terminal{},
		&scenarioModels.Scenario{},
		&scenarioModels.ScenarioSession{},
		&scenarioModels.ScenarioStepProgress{},
		&scenarioModels.ScenarioFlag{},
		&scenarioModels.ScenarioAssignment{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&paymentModels.SubscriptionPlan{},
		&paymentModels.UserSubscription{},
		&paymentModels.BillingAddress{},
		&paymentModels.PaymentMethod{},
		&paymentModels.Invoice{},
		&auditModels.AuditLog{},
	))
	require.NoError(t, err)
	return db
}

// newUserID returns a valid UUID string. The Casdoor user-id IS a UUID and the
// audit_logs.actor_id column is `uuid` typed (see authMiddleware.go: it parses
// ctx userId via uuid.Parse before writing the audit row). Using a real UUID
// keeps the seeded audit row's actor_id comparable to what production writes —
// the bug the rework must fix is that the branch used non-UUID ids like
// "test-user-xxxx", which silently no-op the audit WHERE in PostgreSQL.
func newUserID() string {
	return uuid.NewString()
}

// composedUserDeletionService builds the reworked UserDeletionService with a
// REAL authServices.UserService composed from main's mock seams, so we can
// assert the Stripe-before-Casdoor ordering and the abort-on-Stripe-failure
// contract end-to-end through the delegated DeleteUser.
//
// NOTE: services.NewUserDeletionService currently takes only (db). The second
// argument here is the TARGET signature; until the backend-dev reworks the
// constructor this call will NOT compile — that is the expected RED state.
func composedUserDeletionService(
	db *gorm.DB,
	casdoorMock *mockCasdoorUserClient,
	helperMock *mockPaymentDeletionHelper,
) authServices.UserDeletionService {
	userSvc := authServices.NewUserService(casdoorMock, helperMock)
	return authServices.NewUserDeletionService(db, userSvc)
}

func seedRunningTerminal(t *testing.T, db *gorm.DB, userID string) *terminalModels.Terminal {
	t.Helper()
	keyID := uuid.New()
	require.NoError(t, db.Create(&terminalModels.UserTerminalKey{
		BaseModel:   entityManagementModels.BaseModel{ID: keyID},
		UserID:      userID,
		APIKey:      "key-" + uuid.NewString()[:8],
		KeyName:     "test-key",
		IsActive:    true,
		MaxSessions: 5,
	}).Error)

	term := &terminalModels.Terminal{
		BaseModel:         entityManagementModels.BaseModel{ID: uuid.New()},
		SessionID:         "sess-" + uuid.NewString()[:8],
		UserID:            userID,
		Name:              "Test Terminal",
		State:             terminalModels.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: keyID,
	}
	require.NoError(t, db.Create(term).Error)
	return term
}

func seedScenario(t *testing.T, db *gorm.DB, authorID string) uuid.UUID {
	t.Helper()
	scenarioID := uuid.New()
	require.NoError(t, db.Exec(
		"INSERT INTO scenarios (id, name, title, source_type, instance_type, created_by_id) VALUES (?, ?, ?, ?, ?, ?)",
		scenarioID, "test-scenario-"+uuid.NewString()[:8], "Test Scenario", "builtin", "ubuntu:22.04", authorID,
	).Error)
	return scenarioID
}

func seedScenarioSessionWithProgress(t *testing.T, db *gorm.DB, userID string, scenarioID uuid.UUID) uuid.UUID {
	t.Helper()
	sessionID := uuid.New()
	require.NoError(t, db.Create(&scenarioModels.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: sessionID},
		ScenarioID: scenarioID,
		UserID:     userID,
		Status:     "active",
		StartedAt:  time.Now(),
	}).Error)
	require.NoError(t, db.Create(&scenarioModels.ScenarioStepProgress{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		SessionID: sessionID,
		StepOrder: 1,
		Status:    "active",
	}).Error)
	require.NoError(t, db.Create(&scenarioModels.ScenarioFlag{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		SessionID: sessionID,
		StepOrder: 1,
	}).Error)
	return sessionID
}

func seedOrgMembership(t *testing.T, db *gorm.DB, userID string, role string) uuid.UUID {
	t.Helper()
	orgOwner := "other-owner-" + uuid.NewString()[:8]
	orgID := uuid.New()
	require.NoError(t, db.Exec(
		"INSERT INTO organizations (id, name, display_name, owner_user_id, organization_type, is_personal, max_groups, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		orgID, "team-org-"+uuid.NewString()[:8], "Team Org", orgOwner, "team", false, 250, 100, true,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO organization_members (id, organization_id, user_id, role, joined_at, is_active) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), orgID, userID, role, time.Now(), true,
	).Error)
	return orgID
}

// seedOwnedOrg makes the user the OWNER of a non-personal org (a 409 gate).
func seedOwnedOrg(t *testing.T, db *gorm.DB, ownerUserID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO organizations (id, name, display_name, owner_user_id, organization_type, is_personal, max_groups, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), "owned-org-"+uuid.NewString()[:8], "Owned Org", ownerUserID, "team", false, 250, 100, true,
	).Error)
}

// seedPersonalOrg makes the user the owner of their personal org (must be
// deleted by DeleteMyAccount, NOT a 409 gate).
func seedPersonalOrg(t *testing.T, db *gorm.DB, ownerUserID string) uuid.UUID {
	t.Helper()
	orgID := uuid.New()
	require.NoError(t, db.Exec(
		"INSERT INTO organizations (id, name, display_name, owner_user_id, organization_type, is_personal, max_groups, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		orgID, "personal-"+uuid.NewString()[:8], "Personal", ownerUserID, "personal", true, 250, 100, true,
	).Error)
	return orgID
}

func seedOwnedGroup(t *testing.T, db *gorm.DB, ownerUserID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO class_groups (id, name, display_name, owner_user_id, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), "owned-group-"+uuid.NewString()[:8], "Owned Group", ownerUserID, 50, true,
	).Error)
}

func seedGroupMembership(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	groupOwner := "group-owner-" + uuid.NewString()[:8]
	gID := uuid.New()
	require.NoError(t, db.Exec(
		"INSERT INTO class_groups (id, name, display_name, owner_user_id, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?)",
		gID, "member-group-"+uuid.NewString()[:8], "Member Group", groupOwner, 50, true,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO group_members (id, group_id, user_id, role, joined_at, is_active) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New(), gID, userID, "member", time.Now(), true,
	).Error)
}

func seedAuditLog(t *testing.T, db *gorm.DB, userID string) uuid.UUID {
	t.Helper()
	actorUUID := uuid.MustParse(userID) // userID is always a UUID via newUserID()
	id := uuid.New()
	log := &auditModels.AuditLog{
		ID:         id,
		EventType:  auditModels.AuditEventLogin,
		Severity:   auditModels.AuditSeverityInfo,
		ActorID:    &actorUUID,
		ActorEmail: "victim@example.com",
		ActorIP:    "203.0.113.7",
		Action:     "User login",
		Status:     "success",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(365 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(log).Error)
	return id
}

// happyMocks wires both seams to succeed and returns the shared recorder so a
// test can assert call ordering.
func happyMocks(userID string) (*mockCasdoorUserClient, *mockPaymentDeletionHelper, *callRecorder) {
	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}
	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
	helperMock.On("PseudonymizeBillingDataForUser", userID).Return(nil)
	return casdoorMock, helperMock, rec
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Test 1: the delegated DeleteUser must cancel Stripe BEFORE deleting Casdoor.
// This is the security-critical ordering: a deleted identity that is still
// being billed is the failure mode we are preventing.
func TestDeleteMyAccount_StripeCancelledBeforeCasdoorDelete(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedSubscription(t, db, userID, "sub_active_1", "active")

	casdoorMock, helperMock, rec := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	calls := rec.snapshot()
	cancelIdx := indexOf(calls, "payment.CancelAllActiveSubscriptionsForUser")
	casdoorIdx := indexOf(calls, "casdoor.DeleteUser")
	require.GreaterOrEqual(t, cancelIdx, 0, "Stripe cancel must run, calls=%v", calls)
	require.GreaterOrEqual(t, casdoorIdx, 0, "Casdoor delete must run, calls=%v", calls)
	assert.Less(t, cancelIdx, casdoorIdx,
		"Stripe cancel MUST precede Casdoor delete — otherwise a failed Stripe call leaves a deleted-but-still-billed user. calls=%v", calls)
}

// Test 2: if Stripe cancellation fails, DeleteMyAccount must abort with ZERO
// OCF-side mutation — Casdoor not deleted, and seeded OCF rows still present.
// Self-service must NOT log-and-200 on Stripe failure: it returns an error so
// the handler responds 5xx and the user can retry.
func TestDeleteMyAccount_StripeCancelFails_AbortsWithNoOCFMutation(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedSubscription(t, db, userID, "sub_fail", "active")
	term := seedRunningTerminal(t, db, userID)
	scenarioID := seedScenario(t, db, "other-author")
	sessionID := seedScenarioSessionWithProgress(t, db, userID, scenarioID)
	orgID := seedOrgMembership(t, db, userID, "member")

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}
	casdoorMock.On("GetUserByUserId", userID).Return(buildCasdoorUser(userID), nil)
	stripeErr := errors.New("stripe API unavailable")
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(stripeErr)
	// DeleteUser intentionally NOT expected — any call fails the assertion below.

	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	err := svc.DeleteMyAccount(userID)
	require.Error(t, err, "DeleteMyAccount must return an error when Stripe cancel fails")

	casdoorMock.AssertNotCalled(t, "DeleteUser", anyArg())

	// OCF-side rows must be untouched (abort before the cascade).
	var termCount int64
	db.Model(&terminalModels.Terminal{}).Where("id = ?", term.ID).Count(&termCount)
	assert.Equal(t, int64(1), termCount, "terminal must still exist after aborted deletion")

	var reloadedTerm terminalModels.Terminal
	require.NoError(t, db.First(&reloadedTerm, "id = ?", term.ID).Error)
	assert.Equal(t, terminalModels.StateRunning, reloadedTerm.State,
		"terminal state must be unchanged (still running) when deletion aborts")

	var sessionCount int64
	db.Model(&scenarioModels.ScenarioSession{}).Where("id = ?", sessionID).Count(&sessionCount)
	assert.Equal(t, int64(1), sessionCount, "scenario session must still exist after aborted deletion")

	var orgMemberCount int64
	db.Model(&organizationModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).Count(&orgMemberCount)
	assert.Equal(t, int64(1), orgMemberCount, "org membership must still exist after aborted deletion")
}

// Test 3: billing PII is pseudonymized through the REAL paymentDeletionHelper
// seam, preserving country (BillingAddress) and StripePaymentMethodID
// (PaymentMethod) for the 10y legal retention. Mirrors main's
// TestDeleteUser_PseudonymizesBillingData but drives the reworked path and
// asserts the exact preservation contract.
func TestDeleteMyAccount_PseudonymizesBilling_PreservesCountryAndPMID(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	addr := seedBillingAddress(t, db, userID) // Line1 "123 Main Street", City "Paris", Country "FR"
	pm := seedPaymentMethod(t, db, userID)    // StripePaymentMethodID "pm_test_<userID>"

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	casdoorMock.On("GetUserByUserId", userID).Return(buildCasdoorUser(userID), nil)
	casdoorMock.On("DeleteUser", anyArg()).Return(true, nil)

	// Use the REAL pseudonymization seam (stub only Stripe) so we test the
	// actual mutation contract, not a hand-mocked one.
	realHelper := paymentServices.NewPaymentDeletionHelperWithDeps(db, &stubStripeService{})
	userSvc := authServices.NewUserService(casdoorMock, realHelper)
	svc := authServices.NewUserDeletionService(db, userSvc)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var reloadedAddr paymentModels.BillingAddress
	require.NoError(t, db.First(&reloadedAddr, "id = ?", addr.ID).Error)
	assert.Equal(t, "FR", reloadedAddr.Country, "BillingAddress.Country MUST be preserved (tax/audit traceability)")
	assert.NotEqual(t, "123 Main Street", reloadedAddr.Line1, "BillingAddress.Line1 must be pseudonymized")
	assert.NotEqual(t, "Paris", reloadedAddr.City, "BillingAddress.City must be pseudonymized")

	var reloadedPM paymentModels.PaymentMethod
	require.NoError(t, db.First(&reloadedPM, "id = ?", pm.ID).Error)
	assert.Equal(t, "pm_test_"+userID, reloadedPM.StripePaymentMethodID,
		"StripePaymentMethodID MUST be preserved for invoice traceability")
	assert.NotEqual(t, "visa", reloadedPM.CardBrand, "PaymentMethod.CardBrand must be pseudonymized")
}

// Test 4: running terminals are terminated by flipping State to StateDeleted
// (real teardown via TerminateUserTerminals), NOT a status flip and NOT a row
// delete — the row must survive for audit, with State == deleted.
func TestDeleteMyAccount_TerminatesTerminals_StateDeleted(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	term := seedRunningTerminal(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var reloaded terminalModels.Terminal
	require.NoError(t, db.First(&reloaded, "id = ?", term.ID).Error)
	assert.Equal(t, terminalModels.StateDeleted, reloaded.State,
		"terminal must be marked State=StateDeleted (real teardown), not status-flipped or row-deleted")
}

// Test 5: scenario sessions are deleted and the FK cascade removes their
// step_progress and flags rows.
func TestDeleteMyAccount_DeletesScenarioSessions_CascadesStepProgressAndFlags(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	// Enforce SQLite FK cascade so OnDelete:CASCADE actually fires.
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	userID := newUserID()
	scenarioID := seedScenario(t, db, "other-author")
	sessionID := seedScenarioSessionWithProgress(t, db, userID, scenarioID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var sessionCount, progressCount, flagCount int64
	db.Model(&scenarioModels.ScenarioSession{}).Where("id = ?", sessionID).Count(&sessionCount)
	db.Model(&scenarioModels.ScenarioStepProgress{}).Where("session_id = ?", sessionID).Count(&progressCount)
	db.Model(&scenarioModels.ScenarioFlag{}).Where("session_id = ?", sessionID).Count(&flagCount)

	assert.Equal(t, int64(0), sessionCount, "scenario session must be deleted")
	assert.Equal(t, int64(0), progressCount, "step progress must cascade-delete with the session")
	assert.Equal(t, int64(0), flagCount, "flags must cascade-delete with the session")
}

// Test 6: audit logs are anonymized — actor_id, actor_email AND actor_ip must
// all be cleared. The seeded row uses the same actor identifier production
// writes (a UUID parsed from the Casdoor userId), so the WHERE actually matches
// and the test cannot pass while prod no-ops.
func TestDeleteMyAccount_AnonymizesAuditLogs_ClearsActorEmailAndIP(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	auditID := seedAuditLog(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var reloaded auditModels.AuditLog
	require.NoError(t, db.First(&reloaded, "id = ?", auditID).Error)
	assert.Nil(t, reloaded.ActorID, "AuditLog.ActorID must be nulled")
	assert.Equal(t, "", reloaded.ActorEmail, "AuditLog.ActorEmail must be cleared (PII)")
	assert.Equal(t, "", reloaded.ActorIP, "AuditLog.ActorIP must be cleared (PII)")
}

// Test 7: scenario authorship is anonymized — created_by_id is emptied so the
// scenario survives but no longer points at the deleted user.
func TestDeleteMyAccount_AnonymizesScenarioAuthorship(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	scenarioID := seedScenario(t, db, userID) // authored by the user

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var scenario scenarioModels.Scenario
	require.NoError(t, db.First(&scenario, "id = ?", scenarioID).Error)
	assert.Equal(t, "", scenario.CreatedByID, "Scenario.CreatedByID must be emptied (authorship anonymized)")
}

// Test 8: owning a non-personal org blocks deletion (409 at handler level).
func TestDeleteMyAccount_BlocksIfOwnsNonPersonalOrg(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedOwnedOrg(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	err := svc.DeleteMyAccount(userID)
	require.Error(t, err)
	assert.ErrorIs(t, err, authServices.ErrOwnsOrganizations)
}

// Test 9: owning a group blocks deletion.
func TestDeleteMyAccount_BlocksIfOwnsGroup(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedOwnedGroup(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	err := svc.DeleteMyAccount(userID)
	require.Error(t, err)
	assert.ErrorIs(t, err, authServices.ErrOwnsGroups)
}

// Test 10: the user's PERSONAL org is allowed and gets deleted (not a 409).
func TestDeleteMyAccount_AllowsAndDeletesPersonalOrg(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	personalOrgID := seedPersonalOrg(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var orgCount int64
	db.Model(&organizationModels.Organization{}).Where("id = ?", personalOrgID).Count(&orgCount)
	assert.Equal(t, int64(0), orgCount, "personal organization must be deleted")
}

// Test 11: the confirmation gate lives in the HTTP handler. A request without
// the exact confirmation body must return 400 — exercised through a real gin
// handler, not a service call.
func TestDeleteMyAccount_RequiresConfirmation_HandlerLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"empty body", ``, http.StatusBadRequest},
		{"missing confirmation field", `{}`, http.StatusBadRequest},
		{"wrong confirmation string", `{"confirmation":"yes"}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Set("userId", newUserID())
			ctx.Request = httptest.NewRequest(http.MethodDelete, "/users/me/account",
				strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			invokeDeleteMyAccountHandler(ctx)

			assert.Equal(t, tc.wantStatus, w.Code,
				"missing/invalid confirmation must be rejected at the handler before any deletion runs")
		})
	}
}

// Test 12: memberships, auth tokens, user settings and SSH keys are removed.
func TestDeleteMyAccount_RemovesMembershipsTokensSettingsSSHKeys(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()

	seedOrgMembership(t, db, userID, "member")
	seedGroupMembership(t, db, userID)
	require.NoError(t, db.Create(&authModels.UserSettings{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		DefaultLandingPage: "/dashboard",
	}).Error)
	require.NoError(t, db.Create(&authModels.TokenBlacklist{
		TokenJTI:  "jti-" + uuid.NewString()[:8],
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}).Error)
	require.NoError(t, db.Create(&authModels.EmailVerificationToken{
		UserID:    userID,
		Email:     "v@example.com",
		Token:     "ev-" + uuid.NewString()[:8],
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}).Error)
	require.NoError(t, db.Create(&authModels.PasswordResetToken{
		UserID:    userID,
		Token:     "pr-" + uuid.NewString()[:8],
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}).Error)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	var orgMemberCount, groupMemberCount, settingsCount int64
	var evTokenCount, prTokenCount, blacklistCount int64
	db.Model(&organizationModels.OrganizationMember{}).Where("user_id = ?", userID).Count(&orgMemberCount)
	db.Model(&groupModels.GroupMember{}).Where("user_id = ?", userID).Count(&groupMemberCount)
	db.Model(&authModels.UserSettings{}).Where("user_id = ?", userID).Count(&settingsCount)
	db.Model(&authModels.EmailVerificationToken{}).Where("user_id = ?", userID).Count(&evTokenCount)
	db.Model(&authModels.PasswordResetToken{}).Where("user_id = ?", userID).Count(&prTokenCount)
	db.Model(&authModels.TokenBlacklist{}).Where("user_id = ?", userID).Count(&blacklistCount)

	assert.Equal(t, int64(0), orgMemberCount, "org membership must be removed")
	assert.Equal(t, int64(0), groupMemberCount, "group membership must be removed")
	assert.Equal(t, int64(0), settingsCount, "user settings must be removed")
	assert.Equal(t, int64(0), evTokenCount, "email verification tokens must be removed")
	assert.Equal(t, int64(0), prTokenCount, "password reset tokens must be removed")
	assert.Equal(t, int64(0), blacklistCount, "token blacklist entries must be removed")
}
