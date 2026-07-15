// tests/payment/upgradeStripeCompensation_test.go
//
// RED-phase tests for the 2026-07-10 review finding I3: the plan-upgrade path
// (UpgradeUserPlan controller, userSubscriptionController.go) charges Stripe
// FIRST — UpdateSubscription with the default "always_invoice" proration issues
// an immediate proration invoice — and only THEN persists the new plan on the
// local subscription row. If the DB write fails, the customer has been billed
// but the local plan is unchanged and the endpoint returns 500 with NO
// compensating Stripe call. The sibling bulk path already compensates
// (bulkLicenseService.go:UpdateBatchQuantity: Stripe first, DB, revert Stripe on
// DB failure); the single-user upgrade path does not.
//
// Driven through the real controller + real StripeService pointed at a fake
// Stripe backend (captures the subscription-update POST bodies, like the
// checkout POST-capture tests) + a deterministic SQLite BEFORE UPDATE trigger to
// force the persistence write to fail AFTER the Stripe charge succeeded.
package payment_tests

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"
)

// subUpdateCapture records the form-encoded POST bodies of every Stripe
// subscription-update request (POST /v1/subscriptions/{id}). The forward upgrade
// sends the NEW price; a compensating revert would send the OLD price.
type subUpdateCapture struct {
	mu     sync.Mutex
	bodies []string
}

func (c *subUpdateCapture) record(body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, body)
}

// joined returns all captured subscription-update bodies concatenated.
func (c *subUpdateCapture) joined() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.bodies, "&")
}

func (c *subUpdateCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

// installUpgradeCapturingStripe points the global Stripe backend at a local
// server. GET /v1/subscriptions/{id} returns a subscription carrying one item so
// StripeService.UpdateSubscription (Get then Update) succeeds; POST bodies to the
// same path are captured. Globals are restored on cleanup.
func installUpgradeCapturingStripe(t *testing.T) *subUpdateCapture {
	return installUpgradeCapturingStripeFailingOn(t, "")
}

// installUpgradeCapturingStripeFailingOn behaves like installUpgradeCapturingStripe
// but returns HTTP 500 on any subscription-update POST whose form body contains
// failIfBodyContains. Passing the OLD plan's Stripe price id makes the compensating
// revert call fail (the forward call carries the NEW price and still succeeds).
// Matching on body content rather than a call counter keeps the injection
// deterministic even if the Stripe SDK retries the failed request. Passing ""
// never fails (the plain capture used by the happy/forward paths).
func installUpgradeCapturingStripeFailingOn(t *testing.T, failIfBodyContains string) *subUpdateCapture {
	t.Helper()
	cap := &subUpdateCapture{}

	const subJSON = `{"id":"sub_upg","object":"subscription","status":"active",` +
		`"items":{"object":"list","data":[{"id":"si_upg","object":"subscription_item"}]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") {
			body, _ := io.ReadAll(r.Body)
			cap.record(string(body))
			if failIfBodyContains != "" && strings.Contains(string(body), failIfBodyContains) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error":{"message":"injected stripe revert failure","type":"api_error"}}`)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, subJSON)
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_upgrade_capture"

	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return cap
}

// lockedBuffer is a goroutine-safe sink for captured log output so the race
// detector stays quiet even if an async hook logs during the request.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureUpgradeLogs redirects the standard logger (which backs utils.Error) to a
// buffer for the duration of the test and returns an accessor for what was logged.
func captureUpgradeLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prevOut := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prevOut) })
	return buf
}

// activeStripePlanNoPrice seeds an active plan WITHOUT a Stripe price id — used as
// the OLD plan in the un-revertable case (the compensation cannot revert because
// there is no old price to revert to).
func activeStripePlanNoPrice(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		Name:            name,
		PriceAmount:     999,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// newUpgradeRouter mounts the real subscription controller's upgrade endpoint
// behind a stub auth middleware that injects the authenticated user id.
func newUpgradeRouter(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := paymentController.NewSubscriptionController(db)
	r.POST("/upgrade", func(c *gin.Context) { c.Set("userId", userID) }, ctrl.UpgradeUserPlan)
	return r
}

// seedUpgradeSubscription creates an active personal subscription for userID on
// oldPlan with a real Stripe subscription id (so the upgrade reaches the Stripe
// call rather than the "free subscription" early return).
func seedUpgradeSubscription(t *testing.T, db *gorm.DB, userID string, oldPlan *models.SubscriptionPlan) *models.UserSubscription {
	t.Helper()
	stripeSubID := "sub_upg_" + uuid.NewString()
	customerID := "cus_upg_" + uuid.NewString()
	sub := &models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   oldPlan.ID,
		SubscriptionType:     "personal",
		StripeSubscriptionID: &stripeSubID,
		StripeCustomerID:     &customerID,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)
	return sub
}

func upgradeBody(newPlanID uuid.UUID) io.Reader {
	return strings.NewReader(`{"new_plan_id":"` + newPlanID.String() + `"}`)
}

// upgradeBodyWithProration drives a caller-chosen proration_behavior so tests can
// assert the SAME behavior is threaded through both the forward charge and the
// compensating revert (rather than the "always_invoice" default).
func upgradeBodyWithProration(newPlanID uuid.UUID, proration string) io.Reader {
	return strings.NewReader(`{"new_plan_id":"` + newPlanID.String() + `","proration_behavior":"` + proration + `"}`)
}

// TestUpgradeUserPlan_DBFailureAfterStripeCharge_RevertsStripeSubscription is the
// core RED test. It charges Stripe (forward upgrade to the new price), then the
// local persistence fails (injected BEFORE UPDATE trigger). The endpoint must
// 500, the local row must still carry the OLD plan, AND a compensating Stripe
// call reverting the subscription to the OLD price must have been issued.
//
// RED today: (a) 500 and (b) old-plan-retained already hold (the persistence tx
// rolls back), but (c) NO revert is made — the customer is billed for the new
// plan while the local plan is unchanged (Stripe/DB divergence). The Contains
// check on the OLD price id is the RED signal.
func TestUpgradeUserPlan_DBFailureAfterStripeCharge_RevertsStripeSubscription(t *testing.T) {
	db := freshTestDB(t)
	cap := installUpgradeCapturingStripe(t)

	userID := "user_upg_" + uuid.NewString()
	oldPlan := activeStripePlan(t, db, "Upgrade Old Plan")
	newPlan := activeStripePlan(t, db, "Upgrade New Plan")
	sub := seedUpgradeSubscription(t, db, userID, oldPlan)

	// Force the persistence write to fail AFTER the Stripe charge. The service's
	// UpgradeUserPlan updates user_subscriptions inside a transaction; this
	// BEFORE UPDATE trigger aborts it. Installed after seeding (seeds use Create,
	// not Update) so only the upgrade write trips it. Dropped on cleanup so it
	// can't leak into other tests sharing the DB.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_upgrade_persist BEFORE UPDATE ON user_subscriptions
		BEGIN
			SELECT RAISE(ABORT, 'injected upgrade persistence failure');
		END;`).Error)
	t.Cleanup(func() { db.Exec(`DROP TRIGGER IF EXISTS fail_upgrade_persist`) })

	router := newUpgradeRouter(t, db, userID)
	w := httptest.NewRecorder()
	// Drive a NON-default proration behavior so we can assert the SAME behavior is
	// carried into the compensating revert (not silently reset to always_invoice).
	req, _ := http.NewRequest(http.MethodPost, "/upgrade", upgradeBodyWithProration(newPlan.ID, "create_prorations"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// (a) The failed persistence must surface as an error.
	assert.GreaterOrEqual(t, w.Code, 500,
		"a failed plan persistence after a successful Stripe charge must return 5xx (body: %s)",
		w.Body.String())

	// (b) The local row must still carry the OLD plan (the tx rolled back).
	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, oldPlan.ID, reloaded.SubscriptionPlanID,
		"the local subscription must still carry the OLD plan after a failed upgrade persistence")

	// Exactly two Stripe subscription-update calls: the forward charge (NEW price)
	// and the compensating revert (OLD price). No more, no fewer.
	assert.Equal(t, 2, cap.count(),
		"expected exactly two Stripe subscription-update calls (forward charge + revert), got %d (bodies: %s)",
		cap.count(), cap.joined())
	assert.Contains(t, cap.joined(), *newPlan.StripePriceID,
		"the forward upgrade must have set the NEW price on the Stripe subscription")

	// (c) A compensating revert to the OLD price must have been issued so the
	// customer is not billed for a plan they never received.
	assert.Contains(t, cap.joined(), *oldPlan.StripePriceID,
		"COMPENSATION: when persistence fails after the Stripe charge, the upgrade must "+
			"revert the Stripe subscription back to the OLD price (%s).",
		*oldPlan.StripePriceID)

	// The revert must reuse the same proration behavior as the forward charge so
	// the offsetting proration cancels out cleanly.
	assert.Contains(t, cap.joined(), "proration_behavior=create_prorations",
		"both the forward charge and the compensating revert must carry the caller's "+
			"proration_behavior (create_prorations); captured bodies: %s", cap.joined())
}

// TestUpgradeUserPlan_UnrevertableOldPlan_LogsForReconciliation pins the else-branch
// added for review finding: when the OLD plan has NO Stripe price, the customer has
// already been charged at the new price and the DB write failed, but the revert is
// impossible (no old price to revert to). The code must not fall through silently —
// it must emit a loud error log naming the Stripe subscription and the new price so
// the state can be reconciled manually. Only the forward charge is issued (no
// revert), and the endpoint still returns 5xx.
//
// RED before the fix: the else branch does not exist, so no reconciliation log is
// emitted (the call-count assertion already holds — the RED signal is the log).
func TestUpgradeUserPlan_UnrevertableOldPlan_LogsForReconciliation(t *testing.T) {
	db := freshTestDB(t)
	cap := installUpgradeCapturingStripe(t)
	logs := captureUpgradeLogs(t)

	userID := "user_upg_norevert_" + uuid.NewString()
	oldPlan := activeStripePlanNoPrice(t, db, "Upgrade Old Plan No Price")
	newPlan := activeStripePlan(t, db, "Upgrade New Plan For NoRevert")
	sub := seedUpgradeSubscription(t, db, userID, oldPlan)

	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_upgrade_persist_norevert BEFORE UPDATE ON user_subscriptions
		BEGIN
			SELECT RAISE(ABORT, 'injected upgrade persistence failure');
		END;`).Error)
	t.Cleanup(func() { db.Exec(`DROP TRIGGER IF EXISTS fail_upgrade_persist_norevert`) })

	router := newUpgradeRouter(t, db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/upgrade", upgradeBody(newPlan.ID))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// The failed persistence must still surface as a 5xx.
	assert.GreaterOrEqual(t, w.Code, 500,
		"a failed persistence after the Stripe charge must return 5xx (body: %s)", w.Body.String())

	// Only the forward charge — no revert is possible without an old price.
	assert.Equal(t, 1, cap.count(),
		"with no old Stripe price there is nothing to revert to; only the forward charge "+
			"must have been issued, got %d call(s) (bodies: %s)", cap.count(), cap.joined())
	assert.Contains(t, cap.joined(), *newPlan.StripePriceID,
		"the forward charge to the NEW price must still have happened")

	// THE RED SIGNAL — a loud reconciliation log must name the Stripe subscription
	// and the new price so the divergence can be fixed by hand.
	logged := logs.String()
	assert.Contains(t, logged, *sub.StripeSubscriptionID,
		"the un-revertable state must be logged with the Stripe subscription id for "+
			"manual reconciliation; captured logs: %s", logged)
	assert.Contains(t, logged, *newPlan.StripePriceID,
		"the un-revertable state log must carry the new price id; captured logs: %s", logged)
	assert.Contains(t, strings.ToLower(logged), "reconcil",
		"the un-revertable state log must flag that manual reconciliation is required; "+
			"captured logs: %s", logged)
}

// TestUpgradeUserPlan_RevertItselfFails_StillReturnsOriginalError pins the path
// where the compensating revert ALSO fails at Stripe: the forward charge succeeded,
// the DB write failed, and the revert call errors too. The handler must not panic
// and must still return the original DB failure to the caller (5xx). Both the
// forward and the (failed) revert calls must have been attempted.
func TestUpgradeUserPlan_RevertItselfFails_StillReturnsOriginalError(t *testing.T) {
	db := freshTestDB(t)

	userID := "user_upg_revertfail_" + uuid.NewString()
	oldPlan := activeStripePlan(t, db, "Upgrade Old Plan RevertFail")
	newPlan := activeStripePlan(t, db, "Upgrade New Plan RevertFail")
	sub := seedUpgradeSubscription(t, db, userID, oldPlan)

	// Fail any Stripe subscription-update whose body carries the OLD price — i.e.
	// the compensating revert — while the forward charge (NEW price) succeeds.
	cap := installUpgradeCapturingStripeFailingOn(t, *oldPlan.StripePriceID)

	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_upgrade_persist_revertfail BEFORE UPDATE ON user_subscriptions
		BEGIN
			SELECT RAISE(ABORT, 'injected upgrade persistence failure');
		END;`).Error)
	t.Cleanup(func() { db.Exec(`DROP TRIGGER IF EXISTS fail_upgrade_persist_revertfail`) })

	router := newUpgradeRouter(t, db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/upgrade", upgradeBody(newPlan.ID))
	req.Header.Set("Content-Type", "application/json")

	// A failing revert must not panic — the handler completes and responds.
	require.NotPanics(t, func() { router.ServeHTTP(w, req) })

	// The original DB failure is what the caller must see (5xx), not the revert error.
	assert.GreaterOrEqual(t, w.Code, 500,
		"when both the persistence and the revert fail, the caller must still get the "+
			"original DB error as 5xx (body: %s)", w.Body.String())

	// The local row must still carry the OLD plan (persistence rolled back).
	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, oldPlan.ID, reloaded.SubscriptionPlanID,
		"the local subscription must still carry the OLD plan after a failed upgrade")

	// Both the forward charge and the (failed) revert must have been attempted.
	assert.GreaterOrEqual(t, cap.count(), 2,
		"both the forward charge (NEW price) and the compensating revert (OLD price) must "+
			"have been attempted, got %d call(s) (bodies: %s)", cap.count(), cap.joined())
	assert.Contains(t, cap.joined(), *newPlan.StripePriceID, "the forward charge must have happened")
	assert.Contains(t, cap.joined(), *oldPlan.StripePriceID, "the revert to the OLD price must have been attempted")
}

// TestUpgradeUserPlan_HappyPath_UpdatesLocalPlan pins the success path: with no
// injected failure, the upgrade must 200, persist the NEW plan locally, and have
// charged Stripe with the NEW price. GREEN today and after the fix.
func TestUpgradeUserPlan_HappyPath_UpdatesLocalPlan(t *testing.T) {
	db := freshTestDB(t)
	cap := installUpgradeCapturingStripe(t)

	userID := "user_upg_ok_" + uuid.NewString()
	oldPlan := activeStripePlan(t, db, "Upgrade OK Old Plan")
	newPlan := activeStripePlan(t, db, "Upgrade OK New Plan")
	sub := seedUpgradeSubscription(t, db, userID, oldPlan)

	router := newUpgradeRouter(t, db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/upgrade", upgradeBody(newPlan.ID))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"a successful upgrade must return 200 (body: %s)", w.Body.String())

	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, newPlan.ID, reloaded.SubscriptionPlanID,
		"a successful upgrade must persist the NEW plan on the local subscription")

	assert.Contains(t, cap.joined(), *newPlan.StripePriceID,
		"a successful upgrade must set the NEW price on the Stripe subscription")
}
