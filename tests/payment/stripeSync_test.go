// tests/payment/stripeSync_test.go
//
// RED-phase tests for issue #422: two-way Stripe sync
// (stripeService.SyncPlansToStripe). These pin the INTENDED behavior of a
// service method that does NOT exist yet:
//
//	type SyncToStripeOptions struct { Mirror bool; DryRun bool }
//	func (ss *stripeService) SyncPlansToStripe(opts SyncToStripeOptions) (*StripeSyncResult, error)
//
// Because neither SyncPlansToStripe, SyncToStripeOptions, nor StripeSyncResult
// exist in src/, this file FAILS TO COMPILE today — that is the expected RED
// state. Do NOT add stubs to src/ to make it compile; that is backend-dev's job.
//
// Behavior pinned (per the #422 plan):
//   - Safe mode (Mirror:false):
//       * plan WITHOUT StripePriceID  -> create product+price   (Created)
//       * plan WITH    StripePriceID  -> update product always  (Updated)
//       * price drift (local price != current Stripe price)     -> new price,
//         repoint plan, archive old price                       (PriceMigrated)
//       * price matches                                          -> NO migration
//       * IsFree() plan                                          -> skipped, never synced
//       * products are NEVER archived in safe mode
//   - Mirror mode (Mirror:true): everything safe does, PLUS reconcile Stripe ->
//       * active product whose plan_id metadata matches no LIVE local plan
//         (soft-deleted counts as not-live)                     -> archived (Archived)
//       * product WITHOUT plan_id metadata                      -> skipped, NEVER archived
//   - DryRun (Mirror:true, DryRun:true): reports what WOULD be archived but
//     performs ZERO Stripe writes.
//
// Test harness note: the existing stripeIdempotency_test.go harness
// (installFakeStripeBackend) only records customers + checkout sessions and is
// stateless. This feature needs a STATEFUL product/price catalog (create,
// update, get, list, archive) so drift and mirror reconciliation can be
// exercised. We therefore add a self-contained fake catalog backend here with
// DISTINCT type/function names (fakeStripeCatalog / installFakeStripeCatalog) so
// nothing in the shared package scope is redefined.
package payment_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"
)

// -----------------------------------------------------------------------------
// Stateful fake Stripe catalog backend (products + prices).
// -----------------------------------------------------------------------------

type fakeStripeProduct struct {
	ID          string
	Name        string
	Description string
	Active      bool
	Metadata    map[string]string
}

type fakeStripePrice struct {
	ID         string
	Product    string
	UnitAmount int64
	Currency   string
	Interval   string
	Active     bool
	Metadata   map[string]string
}

type fakeStripeCatalog struct {
	mu         sync.Mutex
	products   map[string]*fakeStripeProduct
	prices     map[string]*fakeStripePrice
	writePaths []string // paths of mutating (POST/DELETE) requests, in order
	prodSeq    int
	priceSeq   int
}

func (c *fakeStripeCatalog) recordWrite(path string) {
	c.writePaths = append(c.writePaths, path)
}

// writeCount returns how many mutating requests hit the backend. The DryRun
// contract requires this to be zero.
func (c *fakeStripeCatalog) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writePaths)
}

// seedProduct inserts a product as if it already existed in Stripe.
func (c *fakeStripeCatalog) seedProduct(name, desc string, active bool, metadata map[string]string) *fakeStripeProduct {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prodSeq++
	p := &fakeStripeProduct{
		ID:          fmt.Sprintf("prod_seed_%d", c.prodSeq),
		Name:        name,
		Description: desc,
		Active:      active,
		Metadata:    metadata,
	}
	c.products[p.ID] = p
	return p
}

// seedPrice inserts a price as if it already existed in Stripe.
func (c *fakeStripeCatalog) seedPrice(productID string, amount int64, currency, interval string, metadata map[string]string) *fakeStripePrice {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.priceSeq++
	p := &fakeStripePrice{
		ID:         fmt.Sprintf("price_seed_%d", c.priceSeq),
		Product:    productID,
		UnitAmount: amount,
		Currency:   currency,
		Interval:   interval,
		Active:     true,
		Metadata:   metadata,
	}
	c.prices[p.ID] = p
	return p
}

func (c *fakeStripeCatalog) getProduct(id string) *fakeStripeProduct {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.products[id]
}

func (c *fakeStripeCatalog) getPrice(id string) *fakeStripePrice {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prices[id]
}

func productJSON(p *fakeStripeProduct) map[string]any {
	return map[string]any{
		"id":          p.ID,
		"object":      "product",
		"name":        p.Name,
		"description": p.Description,
		"active":      p.Active,
		"metadata":    p.Metadata,
	}
}

func priceJSON(p *fakeStripePrice) map[string]any {
	return map[string]any{
		"id":          p.ID,
		"object":      "price",
		"product":     p.Product,
		"unit_amount": p.UnitAmount,
		"currency":    p.Currency,
		"active":      p.Active,
		"recurring": map[string]any{
			"object":   "recurring",
			"interval": p.Interval,
		},
		"metadata": p.Metadata,
	}
}

// installFakeStripeCatalog points the global stripe backend at a stateful local
// server that simulates the Stripe product/price catalog. Globals (backend +
// key) are restored on cleanup so no other payment test is affected.
func installFakeStripeCatalog(t *testing.T) *fakeStripeCatalog {
	t.Helper()

	cat := &fakeStripeCatalog{
		products: map[string]*fakeStripeProduct{},
		prices:   map[string]*fakeStripePrice{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		cat.mu.Lock()
		defer cat.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimSuffix(r.URL.Path, "/")
		writing := r.Method == http.MethodPost || r.Method == http.MethodDelete
		if writing {
			cat.recordWrite(path)
		}

		switch {
		// ---- Products collection: list (GET) or create (POST) ----
		case path == "/v1/products":
			if r.Method == http.MethodGet {
				data := []map[string]any{}
				for _, p := range cat.products {
					if p.Active { // ImportPlans-style listing filters active=true
						data = append(data, productJSON(p))
					}
				}
				writeList(w, "/v1/products", data)
				return
			}
			cat.prodSeq++
			p := &fakeStripeProduct{
				ID:          fmt.Sprintf("prod_new_%d", cat.prodSeq),
				Name:        r.PostForm.Get("name"),
				Description: r.PostForm.Get("description"),
				Active:      true,
				Metadata:    formMetadata(r.PostForm),
			}
			cat.products[p.ID] = p
			writeJSON(w, productJSON(p))

		// ---- Prices collection: list (GET) or create (POST) ----
		case path == "/v1/prices":
			if r.Method == http.MethodGet {
				wantProduct := r.URL.Query().Get("product")
				data := []map[string]any{}
				for _, p := range cat.prices {
					if wantProduct != "" && p.Product != wantProduct {
						continue
					}
					if p.Active {
						data = append(data, priceJSON(p))
					}
				}
				writeList(w, "/v1/prices", data)
				return
			}
			cat.priceSeq++
			amount, _ := strconv.ParseInt(r.PostForm.Get("unit_amount"), 10, 64)
			p := &fakeStripePrice{
				ID:         fmt.Sprintf("price_new_%d", cat.priceSeq),
				Product:    r.PostForm.Get("product"),
				UnitAmount: amount,
				Currency:   r.PostForm.Get("currency"),
				Interval:   r.PostForm.Get("recurring[interval]"),
				Active:     true,
				Metadata:   formMetadata(r.PostForm),
			}
			cat.prices[p.ID] = p
			writeJSON(w, priceJSON(p))

		// ---- Single product: get (GET) or update/archive (POST) ----
		case strings.HasPrefix(path, "/v1/products/"):
			id := strings.TrimPrefix(path, "/v1/products/")
			p := cat.products[id]
			if p == nil {
				http.Error(w, `{"error":{"message":"no such product"}}`, http.StatusNotFound)
				return
			}
			if r.Method == http.MethodPost {
				if v := r.PostForm.Get("name"); v != "" {
					p.Name = v
				}
				if _, ok := r.PostForm["description"]; ok {
					p.Description = r.PostForm.Get("description")
				}
				if v := r.PostForm.Get("active"); v != "" {
					p.Active = v == "true"
				}
				if m := formMetadata(r.PostForm); len(m) > 0 {
					p.Metadata = m
				}
			}
			writeJSON(w, productJSON(p))

		// ---- Single price: get (GET) or update/archive (POST) ----
		case strings.HasPrefix(path, "/v1/prices/"):
			id := strings.TrimPrefix(path, "/v1/prices/")
			p := cat.prices[id]
			if p == nil {
				http.Error(w, `{"error":{"message":"no such price"}}`, http.StatusNotFound)
				return
			}
			if r.Method == http.MethodPost {
				if v := r.PostForm.Get("active"); v != "" {
					p.Active = v == "true"
				}
			}
			writeJSON(w, priceJSON(p))

		default:
			fmt.Fprint(w, `{}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key

	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_fake_sync"

	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})

	return cat
}

// formMetadata extracts metadata[...] form fields into a plain map.
func formMetadata(form map[string][]string) map[string]string {
	out := map[string]string{}
	for k, v := range form {
		if strings.HasPrefix(k, "metadata[") && strings.HasSuffix(k, "]") && len(v) > 0 {
			key := strings.TrimSuffix(strings.TrimPrefix(k, "metadata["), "]")
			out[key] = v[0]
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, obj map[string]any) {
	b, _ := json.Marshal(obj)
	_, _ = w.Write(b)
}

func writeList(w http.ResponseWriter, url string, data []map[string]any) {
	b, _ := json.Marshal(map[string]any{
		"object":   "list",
		"url":      url,
		"has_more": false,
		"data":     data,
	})
	_, _ = w.Write(b)
}

// -----------------------------------------------------------------------------
// Seed helpers
// -----------------------------------------------------------------------------

// syncPlanNoStripe seeds a paid plan that has never been pushed to Stripe.
func syncPlanNoStripe(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		Name:            name,
		Description:     name + " description",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// syncedPlan seeds a paid plan already linked to a Stripe product+price that the
// fake catalog knows about. stripeAmount is the amount currently on the Stripe
// price; localAmount is the plan's PriceAmount. Passing differing values models
// price drift.
func syncedPlan(t *testing.T, db *gorm.DB, cat *fakeStripeCatalog, name string, localAmount, stripeAmount int64) (*models.SubscriptionPlan, *fakeStripeProduct, *fakeStripePrice) {
	t.Helper()
	plan := &models.SubscriptionPlan{
		Name:            name,
		Description:     name + " description",
		PriceAmount:     localAmount,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	meta := map[string]string{"plan_id": plan.ID.String()}
	prod := cat.seedProduct("stale "+name, "stale description", true, meta)
	price := cat.seedPrice(prod.ID, stripeAmount, "eur", "month", map[string]string{"plan_id": plan.ID.String()})

	plan.StripeProductID = &prod.ID
	plan.StripePriceID = &price.ID
	require.NoError(t, db.Save(plan).Error)
	return plan, prod, price
}

// reloadPlan reads the plan back from the DB (default scope: non-soft-deleted).
func reloadPlan(t *testing.T, db *gorm.DB, id uuid.UUID) *models.SubscriptionPlan {
	t.Helper()
	var p models.SubscriptionPlan
	require.NoError(t, db.First(&p, "id = ?", id).Error)
	return &p
}

// containsSubstr reports whether any element of list contains substr.
func containsSubstr(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Safe mode (Mirror:false)
// -----------------------------------------------------------------------------

// TestSyncToStripe_CreatesMissingProduct: a paid plan without a StripePriceID is
// pushed to Stripe (product+price created) and reported in Created, and the plan
// is repointed with the new Stripe IDs.
func TestSyncToStripe_CreatesMissingProduct(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	plan := syncPlanNoStripe(t, db, "Fresh Plan")

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: false})
	require.NoError(t, err)

	assert.True(t, containsSubstr(result.Created, plan.ID.String()) || containsSubstr(result.Created, "Fresh Plan"),
		"a plan with no StripePriceID must be reported in Created; got %+v", result.Created)

	reloaded := reloadPlan(t, db, plan.ID)
	require.NotNil(t, reloaded.StripeProductID, "plan must be repointed to a new Stripe product")
	require.NotNil(t, reloaded.StripePriceID, "plan must be repointed to a new Stripe price")
	assert.NotNil(t, cat.getProduct(*reloaded.StripeProductID), "the created product must exist in Stripe")
}

// TestSyncToStripe_UpdatesExistingProduct: a synced plan (no price drift) has its
// Stripe product updated unconditionally (name/description/active/metadata) and
// is reported in Updated. No price migration occurs.
func TestSyncToStripe_UpdatesExistingProduct(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	// Local price == Stripe price -> no drift. The Stripe product name is stale
	// ("stale ...") while the plan name is authoritative; a successful update
	// pushes the plan name onto the product.
	plan, prod, _ := syncedPlan(t, db, cat, "Update Plan", 1999, 1999)

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: false})
	require.NoError(t, err)

	assert.True(t, containsSubstr(result.Updated, plan.ID.String()) || containsSubstr(result.Updated, "Update Plan"),
		"a synced plan must be reported in Updated; got %+v", result.Updated)
	assert.Empty(t, result.PriceMigrated, "no price drift means no migration")
	assert.Equal(t, "Update Plan", cat.getProduct(prod.ID).Name,
		"the Stripe product name must be overwritten with the plan name on update")
}

// TestSyncToStripe_MigratesPriceOnDrift: when the local price differs from the
// current Stripe price, a NEW price is created, the plan is repointed to it, the
// OLD price is archived, and the plan is reported in PriceMigrated.
func TestSyncToStripe_MigratesPriceOnDrift(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	// Local plan now costs 2999; Stripe price still says 1999 -> drift.
	plan, _, oldPrice := syncedPlan(t, db, cat, "Drift Plan", 2999, 1999)

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: false})
	require.NoError(t, err)

	assert.True(t, containsSubstr(result.PriceMigrated, plan.ID.String()) || containsSubstr(result.PriceMigrated, "Drift Plan"),
		"a plan with drifted price must be reported in PriceMigrated; got %+v", result.PriceMigrated)

	reloaded := reloadPlan(t, db, plan.ID)
	require.NotNil(t, reloaded.StripePriceID)
	assert.NotEqual(t, oldPrice.ID, *reloaded.StripePriceID,
		"the plan must be repointed to a NEW Stripe price after migration")
	assert.False(t, cat.getPrice(oldPrice.ID).Active,
		"the OLD Stripe price must be archived (active=false) after migration")
	newPrice := cat.getPrice(*reloaded.StripePriceID)
	require.NotNil(t, newPrice, "the new price must exist in Stripe")
	assert.Equal(t, int64(2999), newPrice.UnitAmount, "the new price must carry the local plan amount")
}

// TestSyncToStripe_NoMigrationWhenPriceMatches: when the Stripe price already
// matches the local plan, the idempotency guard prevents any price migration —
// the plan keeps its price ID and PriceMigrated stays empty.
func TestSyncToStripe_NoMigrationWhenPriceMatches(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	plan, _, oldPrice := syncedPlan(t, db, cat, "Stable Plan", 1999, 1999)

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: false})
	require.NoError(t, err)

	assert.Empty(t, result.PriceMigrated, "matching price must NOT be migrated; got %+v", result.PriceMigrated)
	reloaded := reloadPlan(t, db, plan.ID)
	require.NotNil(t, reloaded.StripePriceID)
	assert.Equal(t, oldPrice.ID, *reloaded.StripePriceID, "plan price ID must be unchanged when price matches")
	assert.True(t, cat.getPrice(oldPrice.ID).Active, "the existing price must remain active")
}

// TestSyncToStripe_SkipsFreePlans: a free plan (IsFree()) is skipped entirely and
// never pushed to Stripe.
func TestSyncToStripe_SkipsFreePlans(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	plan := &models.SubscriptionPlan{
		Name:            "Free Plan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: false})
	require.NoError(t, err)

	// A free plan is the ONLY plan in the DB, so skipping it must leave the fake
	// Stripe catalog completely untouched — no product/price creation requests.
	assert.Equal(t, 0, cat.writeCount(),
		"a free plan must trigger NO Stripe writes; observed writes to %v", cat.writePaths)
	assert.False(t, containsSubstr(result.Created, plan.ID.String()), "a free plan must NOT be created in Stripe")
	assert.False(t, containsSubstr(result.Updated, plan.ID.String()), "a free plan must NOT be updated in Stripe")

	reloaded := reloadPlan(t, db, plan.ID)
	assert.Nil(t, reloaded.StripeProductID, "a free plan must not be linked to a Stripe product")
	assert.Nil(t, reloaded.StripePriceID, "a free plan must not be linked to a Stripe price")
}

// -----------------------------------------------------------------------------
// Mirror mode (Mirror:true)
// -----------------------------------------------------------------------------

// TestMirrorSync_ArchivesOrphanWithPlanIdMetadata: an active Stripe product whose
// plan_id metadata matches no live local plan is archived and reported.
func TestMirrorSync_ArchivesOrphanWithPlanIdMetadata(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	orphanPlanID := uuid.NewString()
	orphan := cat.seedProduct("Orphan Product", "no matching plan", true, map[string]string{"plan_id": orphanPlanID})

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: true})
	require.NoError(t, err)

	assert.True(t, containsSubstr(result.Archived, orphan.ID) || containsSubstr(result.Archived, orphanPlanID),
		"an orphan product (plan_id matches no live plan) must be reported in Archived; got %+v", result.Archived)
	assert.False(t, cat.getProduct(orphan.ID).Active, "the orphan product must be archived (active=false)")
}

// TestMirrorSync_SkipsProductsWithoutOcfMetadata: an active Stripe product with NO
// plan_id metadata is left untouched (never archived) and reported in Skipped.
func TestMirrorSync_SkipsProductsWithoutOcfMetadata(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	foreign := cat.seedProduct("Foreign Product", "not managed by OCF", true, map[string]string{})

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: true})
	require.NoError(t, err)

	assert.True(t, cat.getProduct(foreign.ID).Active,
		"a product without plan_id metadata must NEVER be archived by mirror sync")
	assert.True(t, containsSubstr(result.Skipped, foreign.ID) || containsSubstr(result.Skipped, "Foreign Product"),
		"a product without plan_id metadata must be reported in Skipped; got %+v", result.Skipped)
}

// TestMirrorSync_ArchivesSoftDeletedPlanProduct: a product whose plan_id points to
// a plan that has been soft-deleted locally is treated as an orphan (the plan is
// no longer live) and archived. This is the Unscoped case.
func TestMirrorSync_ArchivesSoftDeletedPlanProduct(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	plan, prod, _ := syncedPlan(t, db, cat, "Deleted Plan", 1999, 1999)
	// Soft-delete the plan: its product remains active in Stripe.
	require.NoError(t, db.Delete(&models.SubscriptionPlan{}, "id = ?", plan.ID).Error)

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: true})
	require.NoError(t, err)

	assert.True(t, containsSubstr(result.Archived, prod.ID) || containsSubstr(result.Archived, plan.ID.String()),
		"the product of a soft-deleted plan must be archived; got %+v", result.Archived)
	assert.False(t, cat.getProduct(prod.ID).Active, "the soft-deleted plan's product must be archived (active=false)")
}

// TestMirrorSync_DryRunArchivesNothing: with DryRun, an orphan that WOULD be
// archived is reported but ZERO mutating requests hit Stripe.
func TestMirrorSync_DryRunArchivesNothing(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	svc := services.NewStripeService(db)

	orphanPlanID := uuid.NewString()
	orphan := cat.seedProduct("DryRun Orphan", "would be archived", true, map[string]string{"plan_id": orphanPlanID})

	result, err := svc.SyncPlansToStripe(services.SyncToStripeOptions{Mirror: true, DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, 0, cat.writeCount(),
		"DryRun must perform NO Stripe writes; observed writes to %v", cat.writePaths)
	assert.True(t, cat.getProduct(orphan.ID).Active, "DryRun must not actually archive the orphan product")
	assert.True(t, containsSubstr(result.Archived, orphan.ID) || containsSubstr(result.Archived, orphanPlanID),
		"DryRun must still REPORT what would be archived; got %+v", result.Archived)
}
