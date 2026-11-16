// tests/payment/setDefaultOwnership_test.go
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	paymentController "soli/formations/src/payment/routes"
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

func TestSetDefaultBillingAddress_AlreadyDefault_StaysSingleDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")

	require.NoError(t, repo.SetDefaultBillingAddress("caller", current.ID))

	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_AlreadyDefault_StaysSingleDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")

	require.NoError(t, repo.SetDefaultPaymentMethod("caller", current.ID))

	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_SoftDeletedID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, other := setupDefaultBillingAddress(t, db, "caller")
	require.NoError(t, db.Delete(other).Error)

	err := repo.SetDefaultBillingAddress("caller", other.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_CallerWithoutRows_ReturnsNotFound(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	foreign := createTestBillingAddress(t, "someone-else")

	err := repo.SetDefaultBillingAddress("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Empty(t, defaultBillingAddressIDs(t, db, "someone-else"))
}

// setDefaultRouter mounts the two real set-default handlers with the caller id in the context.
func setDefaultRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userId", userID); c.Next() })
	r.POST("/billing-addresses/:id/set-default", paymentController.NewBillingAddressController(db).SetDefaultBillingAddress)
	r.POST("/payment-methods/:id/set-default", paymentController.NewPaymentMethodController(db).SetDefaultPaymentMethod)
	return r
}

func postSetDefault(r *gin.Engine, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	return w.Code
}

func TestSetDefaultBillingAddressHandler_Answers(t *testing.T) {
	db := freshTestDB(t)
	r := setDefaultRouter(db, "caller")
	current, other := setupDefaultBillingAddress(t, db, "caller")
	foreign := createTestBillingAddress(t, "someone-else")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/billing-addresses/"+foreign.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))

	assert.Equal(t, http.StatusBadRequest, postSetDefault(r, "/billing-addresses/not-a-uuid/set-default"))

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/billing-addresses/"+other.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{other.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_Answers(t *testing.T) {
	db := freshTestDB(t)
	r := setDefaultRouter(db, "caller")
	current, other := setupDefaultPaymentMethod(t, db, "caller")
	foreign := createTestPaymentMethod(t, "someone-else")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/payment-methods/"+foreign.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))

	assert.Equal(t, http.StatusBadRequest, postSetDefault(r, "/payment-methods/not-a-uuid/set-default"))

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}
