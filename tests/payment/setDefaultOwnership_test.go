// tests/payment/setDefaultOwnership_test.go
package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
)

// defaultBillingAddressIDs returns the IDs of the user's addresses flagged as default.
func defaultBillingAddressIDs(t *testing.T, db *gorm.DB, userID string) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, db.Model(&models.BillingAddress{}).
		Where("user_id = ? AND is_default = ?", userID, true).Pluck("id", &ids).Error)
	return ids
}

// defaultPaymentMethodIDs returns the IDs of the user's payment methods flagged as default.
func defaultPaymentMethodIDs(t *testing.T, db *gorm.DB, userID string) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, db.Model(&models.PaymentMethod{}).
		Where("user_id = ? AND is_default = ?", userID, true).Pluck("id", &ids).Error)
	return ids
}

// setupDefaultBillingAddress gives the caller a current default address and a second one.
func setupDefaultBillingAddress(t *testing.T, db *gorm.DB, userID string) (current, other *models.BillingAddress) {
	t.Helper()
	current = createTestBillingAddress(t, userID)
	other = createTestBillingAddress(t, userID)
	require.NoError(t, db.Model(current).Update("is_default", true).Error)
	return current, other
}

func setupDefaultPaymentMethod(t *testing.T, db *gorm.DB, userID string) (current, other *models.PaymentMethod) {
	t.Helper()
	current = createTestPaymentMethod(t, userID)
	other = createTestPaymentMethod(t, userID)
	require.NoError(t, db.Model(current).Update("is_default", true).Error)
	return current, other
}

func TestSetDefaultBillingAddress_ForeignID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")
	foreign := createTestBillingAddress(t, "someone-else")

	err := repo.SetDefaultBillingAddress("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
	assert.Empty(t, defaultBillingAddressIDs(t, db, "someone-else"))
}

func TestSetDefaultBillingAddress_UnknownID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")

	err := repo.SetDefaultBillingAddress("caller", uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_OwnID_MovesTheDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	_, other := setupDefaultBillingAddress(t, db, "caller")

	require.NoError(t, repo.SetDefaultBillingAddress("caller", other.ID))

	assert.Equal(t, []uuid.UUID{other.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_ForeignID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")
	foreign := createTestPaymentMethod(t, "someone-else")

	err := repo.SetDefaultPaymentMethod("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
	assert.Empty(t, defaultPaymentMethodIDs(t, db, "someone-else"))
}

func TestSetDefaultPaymentMethod_UnknownID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")

	err := repo.SetDefaultPaymentMethod("caller", uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_OwnID_MovesTheDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	_, other := setupDefaultPaymentMethod(t, db, "caller")

	require.NoError(t, repo.SetDefaultPaymentMethod("caller", other.ID))

	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}
