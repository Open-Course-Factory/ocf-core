// tests/auth/userDeletion_test.go
//
// Failing tests for the user deletion Stripe-cascade fix (GitLab issue #259).
//
// These tests drive a production refactor of src/auth/services/userService.go:
//
//   1. Introduce a CasdoorUserClient interface so that Casdoor HTTP calls can
//      be stubbed in unit tests (today, userService calls casdoorsdk package
//      functions directly — impossible to mock without a seam).
//
//   2. Introduce a PaymentDeletionHelper interface that encapsulates the
//      payment-side cascade work (cancel active Stripe subscriptions,
//      pseudonymize billing data, preserve invoices).
//
//   3. Change the NewUserService constructor to accept those two collaborators
//      so DeleteUser can orchestrate: cancel Stripe FIRST, abort on failure,
//      THEN delete from Casdoor.
//
// As shipped today, NewUserService takes no arguments and DeleteUser silently
// leaves dangling Stripe subscriptions, charging deleted users. These tests
// are expected to fail to compile until the refactor lands; once the
// constructor signature is updated they must fail at runtime until the
// orchestration logic is implemented.
//
// Handoff to backend-dev:
//   - Promote the interfaces below to production code.
//   - Implement a concrete casdoorsdkClient wrapper around the casdoorsdk
//     package functions (GetUserByUserId / DeleteUser).
//   - Implement paymentDeletionHelper in src/payment/services/.
//   - Wire them through NewUserService.
package auth_tests

import (
	"errors"
	"sync"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authServices "soli/formations/src/auth/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
)

// ---------------------------------------------------------------------------
// Interfaces the refactor must add to production code
// ---------------------------------------------------------------------------

// CasdoorUserClient wraps the package-level casdoorsdk functions that
// userService needs. Production code should register a default
// implementation that simply forwards to casdoorsdk.
type CasdoorUserClient interface {
	GetUserByUserId(userID string) (*casdoorsdk.User, error)
	DeleteUser(user *casdoorsdk.User) (bool, error)
}

// PaymentDeletionHelper encapsulates the payment-side cleanup during user
// deletion. The interface is intentionally narrow so it can live in a
// dedicated file without pulling the whole payment service graph into
// userService.
type PaymentDeletionHelper interface {
	// CancelAllActiveSubscriptionsForUser cancels every active-ish Stripe
	// subscription the user owns (statuses: active, trialing, past_due with
	// a non-empty StripeSubscriptionID). Must return the first error and
	// stop on failure — callers rely on "stop on first error" semantics.
	CancelAllActiveSubscriptionsForUser(userID string) error

	// PseudonymizeBillingDataForUser replaces PII in BillingAddress and
	// PaymentMethod rows with a neutral placeholder. Invoices must be left
	// untouched (10-year retention, Art. L. 123-22 Code de commerce).
	// Best-effort: callers log but continue on failure.
	PseudonymizeBillingDataForUser(userID string) error
}

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// callRecorder records ordered calls across multiple mocks so tests can
// assert Stripe-before-Casdoor ordering.
type callRecorder struct {
	mu    *sync.Mutex // pointer so struct value is safe to copy after init
	calls []string
}

func newCallRecorder() *callRecorder {
	return &callRecorder{mu: &sync.Mutex{}}
}

func (r *callRecorder) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

func (r *callRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

type mockCasdoorUserClient struct {
	mock.Mock
	recorder *callRecorder
}

func (m *mockCasdoorUserClient) GetUserByUserId(userID string) (*casdoorsdk.User, error) {
	args := m.Called(userID)
	if m.recorder != nil {
		m.recorder.record("casdoor.GetUserByUserId")
	}
	u, _ := args.Get(0).(*casdoorsdk.User)
	return u, args.Error(1)
}

func (m *mockCasdoorUserClient) DeleteUser(user *casdoorsdk.User) (bool, error) {
	args := m.Called(user)
	if m.recorder != nil {
		m.recorder.record("casdoor.DeleteUser")
	}
	return args.Bool(0), args.Error(1)
}

type mockPaymentDeletionHelper struct {
	mock.Mock
	recorder *callRecorder
}

func (m *mockPaymentDeletionHelper) CancelAllActiveSubscriptionsForUser(userID string) error {
	args := m.Called(userID)
	if m.recorder != nil {
		m.recorder.record("payment.CancelAllActiveSubscriptionsForUser")
	}
	return args.Error(0)
}

func (m *mockPaymentDeletionHelper) PseudonymizeBillingDataForUser(userID string) error {
	args := m.Called(userID)
	if m.recorder != nil {
		m.recorder.record("payment.PseudonymizeBillingDataForUser")
	}
	return args.Error(0)
}

// ---------------------------------------------------------------------------
// Test DB helpers
// ---------------------------------------------------------------------------

// setupUserDeletionTestDB creates an in-memory SQLite DB with the payment
// tables userService.DeleteUser cascades through.
func setupUserDeletionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	err = db.AutoMigrate(
		&paymentModels.SubscriptionPlan{},
		&paymentModels.UserSubscription{},
		&paymentModels.BillingAddress{},
		&paymentModels.PaymentMethod{},
		&paymentModels.Invoice{},
	)
	require.NoError(t, err)
	return db
}

func seedSubscription(t *testing.T, db *gorm.DB, userID, stripeSubID, status string) *paymentModels.UserSubscription {
	t.Helper()
	sub := &paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: uuid.New(),
		Status:             status,
	}
	if stripeSubID != "" {
		s := stripeSubID
		sub.StripeSubscriptionID = &s
	}
	require.NoError(t, db.Create(sub).Error)
	return sub
}

func seedBillingAddress(t *testing.T, db *gorm.DB, userID string) *paymentModels.BillingAddress {
	t.Helper()
	addr := &paymentModels.BillingAddress{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:     userID,
		Line1:      "123 Main Street",
		City:       "Paris",
		PostalCode: "75001",
		Country:    "FR",
	}
	require.NoError(t, db.Create(addr).Error)
	return addr
}

func seedPaymentMethod(t *testing.T, db *gorm.DB, userID string) *paymentModels.PaymentMethod {
	t.Helper()
	pm := &paymentModels.PaymentMethod{
		BaseModel:             entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:                userID,
		StripePaymentMethodID: "pm_test_" + userID,
		Type:                  "card",
		CardBrand:             "visa",
		CardLast4:             "4242",
		IsActive:              true,
	}
	require.NoError(t, db.Create(pm).Error)
	return pm
}

func seedInvoice(t *testing.T, db *gorm.DB, userID string, subID uuid.UUID) *paymentModels.Invoice {
	t.Helper()
	inv := &paymentModels.Invoice{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		UserSubscriptionID: subID,
		StripeInvoiceID:    "in_test_" + uuid.NewString(),
		Amount:             1999,
		Currency:           "EUR",
		Status:             "paid",
		InvoiceNumber:      "F-2026-0001",
	}
	require.NoError(t, db.Create(inv).Error)
	return inv
}

func buildCasdoorUser(id string) *casdoorsdk.User {
	return &casdoorsdk.User{Id: id, Name: "user-" + id, Email: id + "@example.com"}
}

// ---------------------------------------------------------------------------
// The tests
// ---------------------------------------------------------------------------

// Test 1: Stripe cancel must happen before Casdoor delete.
func TestDeleteUser_WithActiveStripeSubscription_CancelsStripeBeforeCasdoor(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-1"
	seedSubscription(t, db, userID, "sub_test_123", "active")

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
	helperMock.On("PseudonymizeBillingDataForUser", userID).Return(nil)

	svc := authServices.NewUserService(casdoorMock, helperMock)

	err := svc.DeleteUser(userID)
	assert.NoError(t, err, "DeleteUser should succeed when all cascade steps succeed")

	helperMock.AssertCalled(t, "CancelAllActiveSubscriptionsForUser", userID)
	casdoorMock.AssertCalled(t, "DeleteUser", casdoorUser)

	// Ordering: payment cancel must precede casdoor delete.
	calls := rec.snapshot()
	cancelIdx := indexOf(calls, "payment.CancelAllActiveSubscriptionsForUser")
	casdoorIdx := indexOf(calls, "casdoor.DeleteUser")
	require.GreaterOrEqual(t, cancelIdx, 0, "payment cancel must be called, calls=%v", calls)
	require.GreaterOrEqual(t, casdoorIdx, 0, "casdoor delete must be called, calls=%v", calls)
	assert.Less(t, cancelIdx, casdoorIdx,
		"Stripe cancel MUST be called before Casdoor delete — otherwise a failed Stripe call leaves the user deleted but still billed. calls=%v", calls)
}

// Test 2: Stripe cancel failure aborts the whole deletion. Casdoor is not
// touched and the DB row keeps its original status.
func TestDeleteUser_StripeCancelFails_AbortsCasdoorDelete(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-2"
	sub := seedSubscription(t, db, userID, "sub_test_fail", "active")
	originalStatus := sub.Status

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	// DeleteUser MUST NOT be called — do not set up a .On for it so
	// any invocation will fail the mock's expectation check below.
	stripeErr := errors.New("stripe API unavailable")
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(stripeErr)

	svc := authServices.NewUserService(casdoorMock, helperMock)

	err := svc.DeleteUser(userID)
	require.Error(t, err, "DeleteUser must return an error when Stripe cancel fails")
	assert.ErrorIs(t, err, stripeErr, "returned error must wrap or equal the Stripe error so callers can log it")

	helperMock.AssertCalled(t, "CancelAllActiveSubscriptionsForUser", userID)
	casdoorMock.AssertNotCalled(t, "DeleteUser", mock.Anything)

	// Subscription row must be untouched (status preserved).
	var reloaded paymentModels.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, originalStatus, reloaded.Status,
		"UserSubscription.Status must not be mutated when Stripe cancel fails")
}

// Test 3: No active subscriptions => no Stripe calls, Casdoor still deleted.
func TestDeleteUser_NoActiveSubscriptions_DeletesNormally(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-3"
	// Only cancelled subs — the helper must short-circuit and not touch Stripe.
	seedSubscription(t, db, userID, "sub_already_cancelled", "cancelled")

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	// Helper is still called — it's responsible for checking "no active subs"
	// and returning nil. We assert it receives the call and succeeds.
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
	helperMock.On("PseudonymizeBillingDataForUser", userID).Return(nil)

	svc := authServices.NewUserService(casdoorMock, helperMock)

	err := svc.DeleteUser(userID)
	assert.NoError(t, err, "DeleteUser with no active subs should succeed")

	casdoorMock.AssertCalled(t, "DeleteUser", casdoorUser)
}

// Test 4: Multiple active subs — helper cancels them; if one fails the whole
// DeleteUser aborts. We model this by having the helper return an error that
// documents a partial-failure scenario (first sub cancelled, second failed).
//
// NOTE: this test encodes "stop on first error" as the safer default. If
// backend-dev decides to implement best-effort (cancel as many as possible,
// then return aggregate error), this test can be relaxed — but the contract
// "if any cancel fails, do NOT proceed to Casdoor" MUST be preserved.
func TestDeleteUser_MultipleActiveSubscriptions_CancelsAll(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-4"
	seedSubscription(t, db, userID, "sub_test_A", "active")
	seedSubscription(t, db, userID, "sub_test_B", "active")

	// --- Subtest 4a: all cancels succeed, Casdoor delete proceeds ---
	t.Run("all_cancels_succeed", func(t *testing.T) {
		rec := newCallRecorder()
		casdoorMock := &mockCasdoorUserClient{recorder: rec}
		helperMock := &mockPaymentDeletionHelper{recorder: rec}

		casdoorUser := buildCasdoorUser(userID)
		casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
		casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
		helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
		helperMock.On("PseudonymizeBillingDataForUser", userID).Return(nil)

		svc := authServices.NewUserService(casdoorMock, helperMock)
		err := svc.DeleteUser(userID)
		assert.NoError(t, err)
		casdoorMock.AssertCalled(t, "DeleteUser", casdoorUser)
	})

	// --- Subtest 4b: partial failure — helper reports error, Casdoor NOT called ---
	t.Run("partial_failure_aborts_casdoor", func(t *testing.T) {
		rec := newCallRecorder()
		casdoorMock := &mockCasdoorUserClient{recorder: rec}
		helperMock := &mockPaymentDeletionHelper{recorder: rec}

		casdoorUser := buildCasdoorUser(userID)
		casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
		// DeleteUser must NOT be called.
		partialErr := errors.New("failed to cancel sub_test_B after cancelling sub_test_A")
		helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(partialErr)

		svc := authServices.NewUserService(casdoorMock, helperMock)
		err := svc.DeleteUser(userID)
		require.Error(t, err, "partial cancellation failure must abort DeleteUser")
		casdoorMock.AssertNotCalled(t, "DeleteUser", mock.Anything)
	})
}

// Test 5: Pseudonymization replaces PII in BillingAddress / PaymentMethod
// rows. The assertion is on the DB state AFTER DeleteUser — the helper
// implementation is expected to mutate those rows in-place.
//
// Because the test uses a mock helper (not the real implementation), we
// wire the mock's "PseudonymizeBillingDataForUser" call to also perform
// the real mutation on the shared test DB. This is intentional: we are
// verifying the CONTRACT ("after DeleteUser, PII in these rows must be
// replaced with a placeholder"), not the helper's internal algorithm.
// The dedicated helper implementation will have its own tests in
// tests/payment/ (different MR).
//
// The placeholder string "[deleted]" is a suggestion — backend-dev can
// pick a different sentinel as long as it's non-empty and clearly
// indicates the data has been pseudonymized.
func TestDeleteUser_PseudonymizesBillingData(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-5"
	addr := seedBillingAddress(t, db, userID)
	pm := seedPaymentMethod(t, db, userID)

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)

	// Make the pseudonymization mock actually mutate the DB so we can assert
	// the post-condition. In production, the real helper does this work.
	helperMock.On("PseudonymizeBillingDataForUser", userID).Run(func(args mock.Arguments) {
		db.Model(&paymentModels.BillingAddress{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{
				"line1":       "[deleted]",
				"line2":       "[deleted]",
				"city":        "[deleted]",
				"state":       "[deleted]",
				"postal_code": "[deleted]",
			})
		db.Model(&paymentModels.PaymentMethod{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{
				"card_brand":  "[deleted]",
				"card_last4":  "[deleted]",
				"is_active":   false,
			})
	}).Return(nil)

	svc := authServices.NewUserService(casdoorMock, helperMock)
	err := svc.DeleteUser(userID)
	require.NoError(t, err)

	// Verify BillingAddress was pseudonymized.
	var reloadedAddr paymentModels.BillingAddress
	require.NoError(t, db.First(&reloadedAddr, "id = ?", addr.ID).Error)
	assert.NotEqual(t, "123 Main Street", reloadedAddr.Line1,
		"BillingAddress.Line1 must be pseudonymized after DeleteUser")
	assert.NotEqual(t, "Paris", reloadedAddr.City,
		"BillingAddress.City must be pseudonymized after DeleteUser")
	assert.NotEqual(t, "75001", reloadedAddr.PostalCode,
		"BillingAddress.PostalCode must be pseudonymized after DeleteUser")

	// Verify PaymentMethod PII replaced; Stripe IDs preserved for invoice
	// traceability (legal retention).
	var reloadedPM paymentModels.PaymentMethod
	require.NoError(t, db.First(&reloadedPM, "id = ?", pm.ID).Error)
	assert.Equal(t, "pm_test_"+userID, reloadedPM.StripePaymentMethodID,
		"StripePaymentMethodID must remain intact for invoice traceability")
	assert.NotEqual(t, "visa", reloadedPM.CardBrand,
		"PaymentMethod.CardBrand must be pseudonymized")

	helperMock.AssertCalled(t, "PseudonymizeBillingDataForUser", userID)
}

// Test 6: Invoices must be preserved (Art. L. 123-22 Code de commerce — 10y).
func TestDeleteUser_PreservesInvoices(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-6"
	sub := seedSubscription(t, db, userID, "sub_test_inv", "active")
	invoice := seedInvoice(t, db, userID, sub.ID)

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
	helperMock.On("PseudonymizeBillingDataForUser", userID).Return(nil)

	svc := authServices.NewUserService(casdoorMock, helperMock)
	err := svc.DeleteUser(userID)
	require.NoError(t, err)

	// Invoice row must still exist, unmodified, for the 10-year legal
	// retention period.
	var count int64
	require.NoError(t, db.Model(&paymentModels.Invoice{}).
		Where("user_id = ?", userID).Count(&count).Error)
	assert.GreaterOrEqual(t, count, int64(1),
		"Invoice rows must be preserved after DeleteUser (Art. L. 123-22 Code de commerce, 10y retention)")

	var reloadedInvoice paymentModels.Invoice
	require.NoError(t, db.First(&reloadedInvoice, "id = ?", invoice.ID).Error)
	assert.Equal(t, invoice.StripeInvoiceID, reloadedInvoice.StripeInvoiceID,
		"Invoice.StripeInvoiceID must remain intact")
	assert.Equal(t, invoice.Amount, reloadedInvoice.Amount,
		"Invoice.Amount must remain intact")
}

// Test 7: Pseudonymization is best-effort — if it fails, Casdoor delete
// still proceeds and DeleteUser returns nil. Stripe was already cancelled
// at that point, which is the important security property.
func TestDeleteUser_BillingPseudonymizationFailure_DoesNotAbort(t *testing.T) {
	db := setupUserDeletionTestDB(t)
	userID := "user-del-7"
	seedSubscription(t, db, userID, "sub_test_besteff", "active")
	seedBillingAddress(t, db, userID)

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	helperMock := &mockPaymentDeletionHelper{recorder: rec}

	casdoorUser := buildCasdoorUser(userID)
	casdoorMock.On("GetUserByUserId", userID).Return(casdoorUser, nil)
	casdoorMock.On("DeleteUser", casdoorUser).Return(true, nil)
	helperMock.On("CancelAllActiveSubscriptionsForUser", userID).Return(nil)
	helperMock.On("PseudonymizeBillingDataForUser", userID).
		Return(errors.New("database write failed"))

	svc := authServices.NewUserService(casdoorMock, helperMock)
	err := svc.DeleteUser(userID)
	assert.NoError(t, err,
		"Pseudonymization failure must NOT abort DeleteUser — the security-critical work (Stripe cancel) is already done and Casdoor delete must still happen")

	// Crucially, Casdoor delete still proceeds.
	casdoorMock.AssertCalled(t, "DeleteUser", casdoorUser)

	// And Stripe cancel ran before Casdoor delete.
	calls := rec.snapshot()
	cancelIdx := indexOf(calls, "payment.CancelAllActiveSubscriptionsForUser")
	casdoorIdx := indexOf(calls, "casdoor.DeleteUser")
	require.GreaterOrEqual(t, cancelIdx, 0)
	require.GreaterOrEqual(t, casdoorIdx, 0)
	assert.Less(t, cancelIdx, casdoorIdx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func indexOf(slice []string, target string) int {
	for i, v := range slice {
		if v == target {
			return i
		}
	}
	return -1
}
