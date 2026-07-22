// tests/payment/subscriptionPlanConverterRoundTrip_test.go
//
// SECURITY/CORRECTNESS RED tests: the SubscriptionPlan converters silently drop
// fields, so plan capabilities set in the DB are invisible through the API.
//
// Live-confirmed losses (this is the classic hand-written-converter field drift):
//   1. ModelToDto (subscriptionPlanRegistration.go) never copies DefaultBackend
//      or AllowedBackends into the output — even though SubscriptionPlanOutput
//      HAS both fields (subscriptionDto.go:90-91). Result: the generic GET
//      returns "" / null for backend routing that IS set in the DB. This is why
//      prod plans appear to have empty backend fields.
//   2. SessionSupervisionEnabled exists on the model
//      (subscriptionPlan.go:44) but is ENTIRELY ABSENT from
//      SubscriptionPlanOutput — the supervision capability can never be observed
//      through the API.
//   3. Write path: DtoToModel (the create converter) never copies DefaultBackend
//      or AllowedBackends FROM the create DTO into the model, even though
//      CreateSubscriptionPlanInput carries them (subscriptionDto.go:27-28). And
//      SessionSupervisionEnabled is absent from BOTH input DTOs, so it cannot be
//      set through the API at all.
//
// The definitive guard below (a) is REFLECTION-DRIVEN: it assigns a distinctive
// non-zero sentinel to every field declared directly on the model, persists,
// reads it back through the REAL generic GET route (admin caller so the
// catalog-visibility scoping added in f0bcaef does not interfere), and asserts
// every such field arrives non-zero in the JSON. A FUTURE model field added
// without converter wiring therefore fails this test BY CONSTRUCTION. A short,
// explicitly-justified exclusion set covers fields deliberately kept out of the
// public plan representation.
//
// These tests drive the REAL router / REAL registration and assert
// USER-OBSERVABLE state (decoded JSON, persisted model), never a mock call.
// They reuse registerSubscriptionPlanForScoping + subscriptionPlanScopingRouter
// from subscriptionPlanCatalogScoping_test.go (same package).
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outputContractExclusions are json keys of model fields deliberately NOT part
// of the public SubscriptionPlan output contract. Each MUST be justified — an
// unjustified addition here is how field loss sneaks back in. NOTE: these were
// surfaced by the reflection sweep during investigation; they are excluded so
// the sweep is greenable once the three real losses are wired. Whether any of
// them SHOULD be exposed is a product decision (reported to main), not a test
// concern.
var outputContractExclusions = map[string]string{
	"stripe_created":         "internal Stripe-sync bookkeeping flag; not a plan attribute",
	"creation_error":         "internal Stripe-sync error surface (omitempty); not a plan attribute",
	"addon_network_price_id": "internal Stripe Price ID for add-on billing; not currently exposed",
	"addon_storage_price_id": "internal Stripe Price ID for add-on billing; not currently exposed",
	"addon_terminal_price_id": "internal Stripe Price ID for add-on billing; not currently exposed",
}

// setSentinel assigns a distinctive non-zero value to a settable reflect.Value,
// recursing through pointers, slices, and nested structs so every leaf becomes
// non-zero. Used to build a SubscriptionPlan with no zero-valued business field.
func setSentinel(v reflect.Value, name string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("sentinel_" + name)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(7)
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
		setSentinel(v.Elem(), name)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		setSentinel(s.Index(0), name)
		v.Set(s)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			setSentinel(v.Field(i), name+"_"+f.Name)
		}
	}
}

// buildFullySeededPlan returns a SubscriptionPlan whose every DIRECTLY-declared
// field carries a non-zero sentinel. The embedded BaseModel is skipped by the
// caller's field walk; its ID is set explicitly so the row persists.
func buildFullySeededPlan(t *testing.T) *models.SubscriptionPlan {
	t.Helper()
	p := &models.SubscriptionPlan{}
	pv := reflect.ValueOf(p).Elem()
	for i := 0; i < pv.NumField(); i++ {
		f := pv.Type().Field(i)
		if f.Anonymous { // skip embedded BaseModel (id/created_at/updated_at/bookkeeping)
			continue
		}
		if f.PkgPath != "" {
			continue
		}
		setSentinel(pv.Field(i), f.Name)
	}
	p.BaseModel = entityManagementModels.BaseModel{ID: uuid.New()}
	return p
}

// modelJSONKey returns the first component of a struct field's json tag, or ""
// when the field is json-ignored / untagged.
func modelJSONKey(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	for i := 0; i < len(tag); i++ {
		if tag[i] == ',' {
			return tag[:i]
		}
	}
	return tag
}

// jsonValueIsNonZero reports whether a decoded JSON value is a meaningful
// non-zero value (not null, "", 0, false, [], or {}).
func jsonValueIsNonZero(raw json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch tv := v.(type) {
	case nil:
		return false
	case string:
		return tv != ""
	case float64:
		return tv != 0
	case bool:
		return tv
	case []any:
		return len(tv) > 0
	case map[string]any:
		return len(tv) > 0
	default:
		return true
	}
}

// getPlanByIdAsAdmin fetches a plan through the real generic GET-by-id route as
// a platform admin and returns the decoded top-level JSON object.
func getPlanByIdAsAdmin(t *testing.T, r *gin.Engine, id string) map[string]json.RawMessage {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/subscription-plans/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "admin GET by id should succeed; body=%s", w.Body.String())
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

// --- (a) The definitive reflection-driven round-trip guard -------------------

func TestSubscriptionPlanConverter_AllModelFieldsRoundTrip(t *testing.T) {
	registerSubscriptionPlanForScoping(t)
	db := freshTestDB(t)

	plan := buildFullySeededPlan(t)
	require.NoError(t, db.Create(plan).Error)

	r := subscriptionPlanScopingRouter(db, "admin-1", []string{"administrator"})
	out := getPlanByIdAsAdmin(t, r, plan.ID.String())

	// Walk every field declared directly on the model. Each such field, unless
	// explicitly excluded, MUST appear non-zero in the output — otherwise a
	// converter dropped it.
	pt := reflect.TypeOf(models.SubscriptionPlan{})
	for i := 0; i < pt.NumField(); i++ {
		f := pt.Field(i)
		if f.Anonymous || f.PkgPath != "" {
			continue
		}
		key := modelJSONKey(f)
		if key == "" {
			continue
		}
		if reason, excluded := outputContractExclusions[key]; excluded {
			t.Logf("excluded field %q from round-trip contract: %s", key, reason)
			continue
		}
		t.Run(key, func(t *testing.T) {
			raw, present := out[key]
			require.True(t, present,
				"model field %q (json %q) is missing from the API output — the output DTO/converter drops it",
				f.Name, key)
			assert.True(t, jsonValueIsNonZero(raw),
				"model field %q (json %q) round-trips as zero (%s) — the converter did not copy the persisted value",
				f.Name, key, string(raw))
		})
	}
}

// --- (b) Targeted reds for the three known losses ---------------------------

func TestSubscriptionPlanOutput_DefaultBackend_RoundTrips(t *testing.T) {
	registerSubscriptionPlanForScoping(t)
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Backend Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		IsCatalog:       true,
		DefaultBackend:  "incus-eu-1",
	}
	require.NoError(t, db.Create(plan).Error)

	r := subscriptionPlanScopingRouter(db, "admin-1", []string{"administrator"})
	out := getPlanByIdAsAdmin(t, r, plan.ID.String())

	var got string
	require.NoError(t, json.Unmarshal(out["default_backend"], &got))
	assert.Equal(t, "incus-eu-1", got,
		"DefaultBackend set in the DB must survive ModelToDto into the API output")
}

func TestSubscriptionPlanOutput_AllowedBackends_RoundTrips(t *testing.T) {
	registerSubscriptionPlanForScoping(t)
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Allowed Backends Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		IsCatalog:       true,
		AllowedBackends: []string{"incus-eu-1", "incus-eu-2"},
	}
	require.NoError(t, db.Create(plan).Error)

	r := subscriptionPlanScopingRouter(db, "admin-1", []string{"administrator"})
	out := getPlanByIdAsAdmin(t, r, plan.ID.String())

	var got []string
	require.NoError(t, json.Unmarshal(out["allowed_backends"], &got))
	assert.Equal(t, []string{"incus-eu-1", "incus-eu-2"}, got,
		"AllowedBackends set in the DB must survive ModelToDto into the API output")
}

func TestSubscriptionPlanOutput_SessionSupervisionEnabled_RoundTrips(t *testing.T) {
	registerSubscriptionPlanForScoping(t)
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "Supervision Plan",
		PriceAmount:               1000,
		Currency:                  "eur",
		BillingInterval:           "month",
		IsActive:                  true,
		IsCatalog:                 true,
		SessionSupervisionEnabled: true,
	}
	require.NoError(t, db.Create(plan).Error)

	r := subscriptionPlanScopingRouter(db, "admin-1", []string{"administrator"})
	out := getPlanByIdAsAdmin(t, r, plan.ID.String())

	raw, present := out["session_supervision_enabled"]
	require.True(t, present,
		"session_supervision_enabled is absent from the output contract — the capability is invisible through the API")
	var got bool
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.True(t, got,
		"SessionSupervisionEnabled=true in the DB must be observable in the API output")
}

// --- (c) Write path: the create converter drops backend routing -------------

// getSubscriptionPlanOps returns the registered typed operations, registering
// the entity if needed. The create converter (DtoToModel) is reached via
// ConvertDtoToModel.
func getSubscriptionPlanOps(t *testing.T) entityManagementInterfaces.EntityOperations {
	t.Helper()
	registerSubscriptionPlanForScoping(t)
	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("SubscriptionPlan")
	require.True(t, ok, "SubscriptionPlan must be registered")
	return ops
}

func TestSubscriptionPlanConverter_DtoToModel_PreservesDefaultBackend(t *testing.T) {
	ops := getSubscriptionPlanOps(t)

	in := dto.CreateSubscriptionPlanInput{
		Name:            "Create Backend Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		DefaultBackend:  "incus-eu-1",
	}
	out, err := ops.ConvertDtoToModel(in)
	require.NoError(t, err)
	model, ok := out.(*models.SubscriptionPlan)
	require.True(t, ok, "converter must return *SubscriptionPlan, got %T", out)

	assert.Equal(t, "incus-eu-1", model.DefaultBackend,
		"DtoToModel must copy DefaultBackend from the create DTO — the field is on CreateSubscriptionPlanInput but the converter drops it")
}

func TestSubscriptionPlanConverter_DtoToModel_PreservesAllowedBackends(t *testing.T) {
	ops := getSubscriptionPlanOps(t)

	in := dto.CreateSubscriptionPlanInput{
		Name:            "Create Allowed Backends Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		AllowedBackends: []string{"incus-eu-1", "incus-eu-2"},
	}
	out, err := ops.ConvertDtoToModel(in)
	require.NoError(t, err)
	model, ok := out.(*models.SubscriptionPlan)
	require.True(t, ok, "converter must return *SubscriptionPlan, got %T", out)

	assert.Equal(t, []string{"incus-eu-1", "incus-eu-2"}, model.AllowedBackends,
		"DtoToModel must copy AllowedBackends from the create DTO — the field is on CreateSubscriptionPlanInput but the converter drops it")
}

func TestSubscriptionPlanConverter_DtoToModel_PreservesSessionSupervisionEnabled(t *testing.T) {
	ops := getSubscriptionPlanOps(t)

	in := dto.CreateSubscriptionPlanInput{
		Name:                      "Create Supervision Plan",
		PriceAmount:               1000,
		Currency:                  "eur",
		BillingInterval:           "month",
		SessionSupervisionEnabled: true,
	}
	out, err := ops.ConvertDtoToModel(in)
	require.NoError(t, err)
	model, ok := out.(*models.SubscriptionPlan)
	require.True(t, ok, "converter must return *SubscriptionPlan, got %T", out)

	assert.True(t, model.SessionSupervisionEnabled,
		"DtoToModel must copy SessionSupervisionEnabled from the create DTO so the supervision capability is settable through the API")
}
