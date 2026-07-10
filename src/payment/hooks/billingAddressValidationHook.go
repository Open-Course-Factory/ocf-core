package paymentHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// BillingAddressValidationHook enforces the B2B facturation field formats
// (issue #383). The rules cannot live in gin `binding` tags: the generic entity
// create/update path binds JSON into an `any`, so the struct validator never
// runs (platform-wide, tracked as #390). Enforcement therefore happens here.
//
// Contract: all three fields are optional (empty = B2C, valid). When present:
//   - siret: exactly 14 digits (numeric)
//   - vat_number: free-form, max 20 chars (NO forced FR prefix)
//   - company_name: max 255 chars
type BillingAddressValidationHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewBillingAddressValidationHook(db *gorm.DB) hooks.Hook {
	return &BillingAddressValidationHook{
		db:       db,
		enabled:  true,
		priority: 5, // Runs before the ownership hook (default priority)
	}
}

func (h *BillingAddressValidationHook) GetName() string {
	return "billing_address_validation"
}

func (h *BillingAddressValidationHook) GetEntityName() string {
	return "BillingAddress"
}

func (h *BillingAddressValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{
		hooks.BeforeCreate,
		hooks.BeforeUpdate,
	}
}

func (h *BillingAddressValidationHook) IsEnabled() bool {
	return h.enabled
}

func (h *BillingAddressValidationHook) GetPriority() int {
	return h.priority
}

// Execute reads the B2B fields from whichever shape the generic service supplies:
// a converted *models.BillingAddress on BeforeCreate, or the raw patch map on
// BeforeUpdate (keys are the JSON/mapstructure field names). A field absent from
// the update patch is simply not validated (partial update).
func (h *BillingAddressValidationHook) Execute(ctx *hooks.HookContext) error {
	var companyName, siret, vatNumber string
	var hasCompany, hasSiret, hasVat bool

	switch v := ctx.NewEntity.(type) {
	case *models.BillingAddress:
		companyName, hasCompany = v.CompanyName, true
		siret, hasSiret = v.Siret, true
		vatNumber, hasVat = v.VatNumber, true
	case map[string]any:
		companyName, hasCompany = stringField(v, "company_name")
		siret, hasSiret = stringField(v, "siret")
		vatNumber, hasVat = stringField(v, "vat_number")
	default:
		return nil // Not a recognized type, skip validation
	}

	// Validation failures are returned as structured EntityErrors so the generic
	// controllers surface them as 400 client errors (WrapHookError preserves the
	// status), not a generic ENT007/500 hook failure.
	if hasCompany && len(companyName) > 255 {
		return entityErrors.NewValidationError("company_name", "must be at most 255 characters")
	}

	if hasSiret && siret != "" {
		if len(siret) != 14 || !isAllDigits(siret) {
			return entityErrors.NewValidationError("siret", "must be exactly 14 digits")
		}
	}

	if hasVat && len(vatNumber) > 20 {
		return entityErrors.NewValidationError("vat_number", "must be at most 20 characters")
	}

	return nil
}

func (h *BillingAddressValidationHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.enabled
}

// stringField extracts a string value from an update patch map, reporting whether
// the key was present at all so absent keys skip validation on partial updates.
// The patch map's values may be *string (the generic PATCH path decodes the
// pointer-field Edit DTO via mapstructure, leaving pointers) or plain string
// (service-layer callers), so both are handled; a nil pointer is treated as absent.
func stringField(m map[string]any, key string) (string, bool) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, true
	case *string:
		if v == nil {
			return "", false
		}
		return *v, true
	default:
		return "", false
	}
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
