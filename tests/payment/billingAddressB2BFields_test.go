// tests/payment/billingAddressB2BFields_test.go
//
// Contract tests for issue #383 (facture-compliance track): a B2B buyer must be
// able to persist and edit raison sociale (company_name), SIRET and n° TVA
// intracommunautaire (vat_number) on their BillingAddress so the generated
// facture is a valid B2B invoice.
//
// These pin the field round-trip through the REAL production write path a
// billing address uses: the generic entity converters registered via
// RegisterBillingAddress (DtoToModel / ModelToDto) → GenericService →
// GenericRepository → DB. A wrong json/gorm tag, a missing DB column, or a
// forgotten converter mapping must fail one of these tests.
//
// They compile before the feature exists (the new fields are driven through a
// JSON body and read back as a decoded map / output DTO — never through Go
// struct fields that don't exist yet) and go RED until the fields are added.
package payment_tests

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityServices "soli/formations/src/entityManagement/services"
	paymentDto "soli/formations/src/payment/dto"
	registration "soli/formations/src/payment/entityRegistration"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/models"
)

// setupBillingAddressEntity registers the real BillingAddress entity (its
// production converters + roles) into the global registry so the generic
// service resolves its typed operations, exactly as at app startup. A mock
// Casbin enforcer is required by the registration's access setup.
func setupBillingAddressEntity(t *testing.T) {
	t.Helper()
	casdoor.Enforcer = mocks.NewMockEnforcer()
	if _, ok := ems.GlobalEntityRegistrationService.GetEntityOps("BillingAddress"); !ok {
		registration.RegisterBillingAddress(ems.GlobalEntityRegistrationService)
		t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity("BillingAddress") })
	}
}

// createBillingAddressViaService drives the real generic create path with the
// given JSON body and returns the persisted entity's ID.
func createBillingAddressViaService(t *testing.T, body map[string]any) uuid.UUID {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	var input paymentDto.CreateBillingAddressInput
	require.NoError(t, json.Unmarshal(raw, &input))

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	created, err := svc.CreateEntity(input, "BillingAddress")
	require.NoError(t, err, "generic create should persist a billing address")

	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("BillingAddress")
	require.True(t, ok)
	id, err := ops.ExtractID(created)
	require.NoError(t, err)
	return id
}

// outputDTOForBillingAddress re-fetches the row from the DB and converts it via
// the production ModelToDto converter, decoded as a JSON map for assertions.
func outputDTOForBillingAddress(t *testing.T, id uuid.UUID) map[string]any {
	t.Helper()

	var fresh models.BillingAddress
	require.NoError(t, sharedTestDB.First(&fresh, "id = ?", id).Error)

	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("BillingAddress")
	require.True(t, ok)
	outAny, err := ops.ConvertModelToDto(&fresh)
	require.NoError(t, err)

	raw, err := json.Marshal(outAny)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// TestBillingAddress_CreateWithB2BFields_RoundTripsPersisted drives a create
// carrying company_name / siret / vat_number and asserts all three survive the
// DTO → model → DB → output-DTO round-trip. RED until the fields exist end to
// end (DTO json tags, model gorm columns, and both registration converters).
func TestBillingAddress_CreateWithB2BFields_RoundTripsPersisted(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)

	const (
		wantCompany = "Acme Formation SARL"
		wantSiret   = "12345678901234" // 14 digits
		wantVat     = "FR12345678901"
	)

	id := createBillingAddressViaService(t, map[string]any{
		"line1":        "10 rue de la Paix",
		"city":         "Paris",
		"postal_code":  "75002",
		"country":      "FR",
		"company_name": wantCompany,
		"siret":        wantSiret,
		"vat_number":   wantVat,
	})

	// Persisted-column guard: the DB row must physically carry the SIRET column.
	var row map[string]any
	require.NoError(t, sharedTestDB.Table("billing_addresses").Where("id = ?", id).Take(&row).Error)
	assert.Contains(t, row, "siret", "billing_addresses must have a siret column")

	// Full round-trip via the production output converter.
	out := outputDTOForBillingAddress(t, id)
	assert.Equal(t, wantCompany, out["company_name"], "company_name must round-trip")
	assert.Equal(t, wantSiret, out["siret"], "siret must round-trip")
	assert.Equal(t, wantVat, out["vat_number"], "vat_number must round-trip")
}

// TestBillingAddress_UpdateB2BFields_Persisted patches an existing address's
// SIRET through the real generic update path and reads it back. RED until the
// gorm column exists to receive the map-keyed update.
func TestBillingAddress_UpdateB2BFields_Persisted(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)

	id := createBillingAddressViaService(t, map[string]any{
		"line1":       "10 rue de la Paix",
		"city":        "Paris",
		"postal_code": "75002",
		"country":     "FR",
	})

	const newSiret = "98765432109876"

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	err := svc.EditEntity(id, "BillingAddress", &models.BillingAddress{}, map[string]any{
		"siret": newSiret,
	})
	require.NoError(t, err, "generic update should persist the siret patch")

	out := outputDTOForBillingAddress(t, id)
	assert.Equal(t, newSiret, out["siret"], "patched siret must be persisted and read back")
}

// TestBillingAddress_CreateWithoutB2BFields_StaysValid guards the B2C path: a
// buyer who supplies no company/SIRET/VAT still creates a valid billing
// address. GREEN today and must stay GREEN after the fields are added.
func TestBillingAddress_CreateWithoutB2BFields_StaysValid(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)

	id := createBillingAddressViaService(t, map[string]any{
		"line1":       "5 avenue des Champs",
		"city":        "Lyon",
		"postal_code": "69001",
		"country":     "FR",
	})

	out := outputDTOForBillingAddress(t, id)
	assert.Equal(t, "5 avenue des Champs", out["line1"], "base B2C fields must persist")
}

// ============================================================================
// B2B field validation (issue #383)
//
// Format validation cannot live in gin `binding` tags: the generic entity
// create/update path binds JSON into an `any`, so the struct validator never
// runs (platform-wide, tracked as #390). The enforcement mechanism is a
// BeforeCreate/BeforeUpdate hook registered in InitPaymentHooks (like the
// ownership hooks). These tests drive the real generic service with the real
// payment hooks registered, so they go RED today (no B2B validation hook) and
// GREEN once the dev adds and wires that hook into InitPaymentHooks.
//
// Contract: siret = exactly 14 digits when present; vat_number = free-form,
// max 20 chars; company_name = max 255 chars. All optional (empty = B2C, valid).
// ============================================================================

// withPaymentHooksRegistered installs the real payment hooks (validation +
// ownership) into an isolated hook registry for the duration of the test,
// restoring the global registry afterwards. Registering via the production
// InitPaymentHooks entry point means the test picks up the B2B validation hook
// exactly when the dev wires it there — no reference to an unwritten symbol.
func withPaymentHooksRegistered(t *testing.T) {
	t.Helper()
	orig := hooks.GlobalHookRegistry
	hooks.GlobalHookRegistry = hooks.NewHookRegistry()
	t.Cleanup(func() { hooks.GlobalHookRegistry = orig })
	paymentHooks.InitPaymentHooks(sharedTestDB)
}

// createBillingAddressExpectingRejection drives the real generic create path
// with the given B2B body and asserts it is rejected and nothing is persisted.
func createBillingAddressExpectingRejection(t *testing.T, body map[string]any) {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)
	var input paymentDto.CreateBillingAddressInput
	require.NoError(t, json.Unmarshal(raw, &input))

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err = svc.CreateEntityWithUser(input, "BillingAddress", "b2b-user")
	require.Error(t, err, "invalid B2B billing field must be rejected")

	var count int64
	require.NoError(t, sharedTestDB.Model(&models.BillingAddress{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "rejected billing address must not be persisted")
}

// TestBillingAddress_InvalidSiret_Rejected pins the headline contract: a
// 13-digit SIRET is rejected at create time. RED until the validation hook exists.
func TestBillingAddress_InvalidSiret_Rejected(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	createBillingAddressExpectingRejection(t, map[string]any{
		"line1":       "1 rue de Rivoli",
		"city":        "Paris",
		"postal_code": "75001",
		"country":     "FR",
		"siret":       "1234567890123", // 13 digits — must be exactly 14
	})
}

// TestBillingAddress_InvalidB2BFields_Rejected covers the remaining format rules
// in one table: non-numeric SIRET, over-long VAT number, over-long company name.
func TestBillingAddress_InvalidB2BFields_Rejected(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	base := func() map[string]any {
		return map[string]any{
			"line1": "1 rue de Rivoli", "city": "Paris", "postal_code": "75001", "country": "FR",
		}
	}

	cases := []struct {
		name  string
		field string
		value string
	}{
		{"siret not numeric", "siret", "1234567890123X"},            // 14 chars but non-numeric
		{"vat too long", "vat_number", "FR123456789012345678901"},   // > 20 chars
		{"company name too long", "company_name", stringOfLen(256)}, // > 255 chars
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_ = freshTestDB(t)
			body := base()
			body[tc.field] = tc.value
			createBillingAddressExpectingRejection(t, body)
		})
	}
}

// TestBillingAddress_ValidB2BFields_Accepted is the green guard: well-formed B2B
// fields must NOT be rejected by the validation hook. Green today (no hook) and
// must stay green after the hook lands — guards against an over-strict rule.
func TestBillingAddress_ValidB2BFields_Accepted(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	raw, _ := json.Marshal(map[string]any{
		"line1": "1 rue de Rivoli", "city": "Paris", "postal_code": "75001", "country": "FR",
		"company_name": "Acme Formation SARL",
		"siret":        "12345678901234", // 14 digits
		"vat_number":   "FR12345678901",
	})
	var input paymentDto.CreateBillingAddressInput
	require.NoError(t, json.Unmarshal(raw, &input))

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err := svc.CreateEntityWithUser(input, "BillingAddress", "b2b-user")
	require.NoError(t, err, "well-formed B2B fields must be accepted")
}

// TestBillingAddress_EmptyB2BFields_AcceptedUnderValidation guards the B2C path
// specifically under the validation hook: a buyer supplying none of the optional
// fields must still pass validation. Green today and after the hook lands.
func TestBillingAddress_EmptyB2BFields_AcceptedUnderValidation(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	raw, _ := json.Marshal(map[string]any{
		"line1": "5 avenue des Champs", "city": "Lyon", "postal_code": "69001", "country": "FR",
	})
	var input paymentDto.CreateBillingAddressInput
	require.NoError(t, json.Unmarshal(raw, &input))

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err := svc.CreateEntityWithUser(input, "BillingAddress", "b2c-user")
	require.NoError(t, err, "empty optional B2B fields must pass validation (B2C)")
}

// TestBillingAddress_InvalidSiret_UpdateRejected pins that the format rule also
// guards the update path: patching an existing address with a 13-digit SIRET is
// rejected. The validation hook must inspect the BeforeUpdate patch (a map).
// RED until the BeforeUpdate validation hook exists.
func TestBillingAddress_InvalidSiret_UpdateRejected(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	id := createBillingAddressViaService(t, map[string]any{
		"line1": "1 rue de Rivoli", "city": "Paris", "postal_code": "75001", "country": "FR",
	})

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	err := svc.EditEntity(id, "BillingAddress", &models.BillingAddress{}, map[string]any{
		"siret": "1234567890123", // 13 digits
	})
	require.Error(t, err, "a 13-digit siret patch must be rejected on update")
	// Must be rejected by the BeforeUpdate validation hook, not merely because
	// the column is missing today — otherwise this passes for the wrong reason
	// and would silently break once the column exists but the hook is absent.
	assert.NotContains(t, err.Error(), "no such column",
		"rejection must come from validation, not a missing DB column")
}

// stringOfLen returns a string of exactly n ASCII letters.
func stringOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
