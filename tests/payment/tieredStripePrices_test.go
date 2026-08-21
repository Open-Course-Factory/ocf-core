// tests/payment/tieredStripePrices_test.go
//
// #442: volume degression was impossible in both directions.
//
//  1. OCF could never CREATE a tiered price. The create path emitted a flat
//     unit_amount and the words TiersMode / BillingScheme appeared nowhere
//     outside the import path, so a ladder could only ever reach Stripe by being
//     hand-built in the dashboard.
//  2. The sync then DESTROYED it. migratePriceIfDrifted compared
//     `current.UnitAmount != plan.PriceAmount`; Stripe returns unit_amount=null
//     (→ 0) for a tiered price while the import sets PriceAmount to the first
//     tier's amount, so drift was permanently true. The first executed sync
//     minted a flat price at the tier-1 amount, repointed the plan and archived
//     the tiered one — invisibly, since existing subscribers keep their price.
package payment_tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"
)

// tieredStripeFake captures request bodies and serves a configurable price.
type tieredStripeFake struct {
	mu       sync.Mutex
	bodies   map[string][]string
	getPaths []string
	// priceJSON is returned for GET /v1/prices/{id}.
	priceJSON string
}

func (f *tieredStripeFake) form(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.bodies[path], "&")
}

func (f *tieredStripeFake) sawGet(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.getPaths {
		if strings.Contains(p, substr) {
			return true
		}
	}
	return false
}

// gradTierJSON builds a Stripe tiered-price payload. up_to=0 means "inf".
func gradTierJSON(id string, tiers [][2]int64) string {
	parts := make([]string, 0, len(tiers))
	for _, t := range tiers {
		upTo := "null"
		if t[0] > 0 {
			upTo = jsonNum(t[0])
		}
		parts = append(parts, `{"up_to":`+upTo+`,"unit_amount":`+jsonNum(t[1])+`}`)
	}
	// A real price always states its tax behaviour; "exclusive" is what
	// ApplyPricing sets for a plan that declares none, so a price built here is
	// not drifted against the plan these tests seed.
	return `{"id":"` + id + `","object":"price","currency":"eur","unit_amount":null,` +
		`"billing_scheme":"tiered","tiers_mode":"graduated","tax_behavior":"exclusive",` +
		`"recurring":{"interval":"month"},"tiers":[` + strings.Join(parts, ",") + `]}`
}

func jsonNum(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func installTieredStripeFake(t *testing.T, priceJSON string) *tieredStripeFake {
	t.Helper()
	fake := &tieredStripeFake{bodies: map[string][]string{}, priceJSON: priceJSON}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		if r.Method == http.MethodGet {
			fake.getPaths = append(fake.getPaths, r.URL.String())
		} else {
			fake.bodies[r.URL.Path] = append(fake.bodies[r.URL.Path], string(body))
		}
		payload := fake.priceJSON
		fake.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/prices/"):
			_, _ = io.WriteString(w, payload)
		case strings.HasPrefix(r.URL.Path, "/v1/prices"):
			_, _ = io.WriteString(w, `{"id":"price_new_tiered","object":"price"}`)
		case strings.HasPrefix(r.URL.Path, "/v1/products"):
			_, _ = io.WriteString(w, `{"id":"prod_tiered","object":"product"}`)
		default:
			_, _ = io.WriteString(w, `{"id":"obj_test","object":"thing"}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_tiered"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return fake
}

// agreedSeatLadder is the monthly seat ladder signed off for the offer (#442).
func agreedSeatLadder() []models.PricingTier {
	return []models.PricingTier{
		{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
		{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 700},
		{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 550},
	}
}

func seedTieredPlan(t *testing.T, db *gorm.DB, stripePriceID *string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             "Siège élève — mensuel",
		Currency:         "eur",
		BillingInterval:  "month",
		IsActive:         true,
		PriceAmount:      900,
		UseTieredPricing: true,
		PricingTiers:     agreedSeatLadder(),
	}
	if stripePriceID != nil {
		productID := "prod_tiered"
		plan.StripeProductID = &productID
		plan.StripePriceID = stripePriceID
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// TestCreateTieredPlan_EmitsGraduatedTiers pins that a ladder actually reaches
// Stripe. Without this the brackets are decoration: the admin configures a
// degression the customer is never charged.
func TestCreateTieredPlan_EmitsGraduatedTiers(t *testing.T) {
	db := freshTestDB(t)
	fake := installTieredStripeFake(t, "")
	plan := seedTieredPlan(t, db, nil)

	require.NoError(t, services.NewStripeService(db).CreateSubscriptionPlanInStripe(plan))

	form := fake.form("/v1/prices")
	require.NotEmpty(t, form, "a price must have been created")

	assert.Contains(t, form, "billing_scheme=tiered")
	assert.Contains(t, form, "tiers_mode=graduated",
		"graduated was the decision: brackets stack, so the bill never drops when buying more")

	// Each bracket, in order, with its ceiling and unit price.
	assert.Contains(t, form, "tiers[0][up_to]=5")
	assert.Contains(t, form, "tiers[0][unit_amount]=900")
	assert.Contains(t, form, "tiers[1][up_to]=15")
	assert.Contains(t, form, "tiers[1][unit_amount]=700")
	assert.Contains(t, form, "tiers[2][unit_amount]=550")
	assert.Contains(t, form, "tiers[2][up_to]=inf",
		"the open bracket must be unbounded, or Stripe refuses quantities beyond it")

	// A tiered price carries no flat unit_amount; sending one is a Stripe error.
	assert.NotContains(t, form, "unit_amount=900&",
		"a tiered price must not also send a flat unit_amount")
}

// TestCreateUntieredPlan_StaysFlat is the regression guard: ordinary plans must
// keep their simple flat price.
func TestCreateUntieredPlan_StaysFlat(t *testing.T) {
	db := freshTestDB(t)
	fake := installTieredStripeFake(t, "")

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Solo",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		PriceAmount:     1200,
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, services.NewStripeService(db).CreateSubscriptionPlanInStripe(plan))

	form := fake.form("/v1/prices")
	assert.Contains(t, form, "unit_amount=1200")
	assert.NotContains(t, form, "tiers_mode")
	assert.NotContains(t, form, "billing_scheme=tiered")
}

// TestSync_TieredPriceIsNotPermanentlyDrifted is the destructive bug. A tiered
// Stripe price matching the local ladder must report NO drift. Before the fix
// every sync saw drift — Stripe returns unit_amount=null for a tiered price
// while PriceAmount holds the tier-1 amount — and replaced the ladder with a
// flat price.
func TestSync_TieredPriceIsNotPermanentlyDrifted(t *testing.T) {
	db := freshTestDB(t)
	priceID := "price_existing_tiered"
	fake := installTieredStripeFake(t, gradTierJSON(priceID, [][2]int64{{5, 900}, {15, 700}, {0, 550}}))
	seedTieredPlan(t, db, &priceID)

	result, err := services.NewStripeService(db).SyncPlansToStripe(services.SyncToStripeOptions{})
	require.NoError(t, err)

	assert.Empty(t, result.PriceMigrated,
		"an unchanged tiered price must not be reported as drifted — the first executed sync "+
			"would otherwise replace the ladder with a flat price and archive it")
	assert.True(t, fake.sawGet("expand"),
		"the drift check must expand tiers, or Stripe returns none and every tiered price "+
			"looks empty")
}

// TestSync_TieredPriceDetectsARealChange keeps the check honest: editing a
// bracket must still migrate the price.
func TestSync_TieredPriceDetectsARealChange(t *testing.T) {
	db := freshTestDB(t)
	priceID := "price_existing_tiered"
	// Stripe still holds the OLD middle bracket (750 rather than 700).
	fake := installTieredStripeFake(t, gradTierJSON(priceID, [][2]int64{{5, 900}, {15, 750}, {0, 550}}))
	_ = fake
	seedTieredPlan(t, db, &priceID)

	result, err := services.NewStripeService(db).SyncPlansToStripe(services.SyncToStripeOptions{})
	require.NoError(t, err)

	assert.Len(t, result.PriceMigrated, 1,
		"a genuinely edited ladder must still be migrated to a new Stripe price")
}
