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
	t.Helper()
	cap := &subUpdateCapture{}

	const subJSON = `{"id":"sub_upg","object":"subscription","status":"active",` +
		`"items":{"object":"list","data":[{"id":"si_upg","object":"subscription_item"}]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") {
			body, _ := io.ReadAll(r.Body)
			cap.record(string(body))
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
	req, _ := http.NewRequest(http.MethodPost, "/upgrade", upgradeBody(newPlan.ID))
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

	// The forward Stripe upgrade to the NEW price must have happened (otherwise
	// there is nothing to compensate — this pins the premise of the bug).
	require.GreaterOrEqual(t, cap.count(), 1,
		"the forward Stripe subscription update (charge) must have been issued before the DB write")
	assert.Contains(t, cap.joined(), *newPlan.StripePriceID,
		"the forward upgrade must have set the NEW price on the Stripe subscription")

	// (c) THE RED SIGNAL — a compensating revert to the OLD price must have been
	// issued. Today no revert is made, so the customer is billed for the new plan
	// while the local plan is unchanged.
	assert.Contains(t, cap.joined(), *oldPlan.StripePriceID,
		"COMPENSATION: when persistence fails after the Stripe charge, the upgrade must "+
			"revert the Stripe subscription back to the OLD price (%s) so the customer is not "+
			"billed for a plan they never received. Today UpgradeUserPlan returns 500 with no "+
			"compensating call — mirror bulkLicenseService.UpdateBatchQuantity's revert.",
		*oldPlan.StripePriceID)
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
