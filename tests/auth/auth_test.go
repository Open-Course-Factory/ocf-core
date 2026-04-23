//go:build integration

package auth_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	services "soli/formations/src/auth/services"

	"soli/formations/src/auth/casdoor"
	sqldb "soli/formations/src/db"
	paymentServices "soli/formations/src/payment/services"
)

func TestUserCreation(t *testing.T) {
	userService := services.NewUserService(
		services.NewCasdoorUserClient(),
		paymentServices.NewPaymentDeletionHelper(sqldb.DB),
	)

	users, _ := userService.GetAllUsers()

	assert.Equal(t, 6, len(*users))

	for _, user := range *users {
		casdoor.Enforcer.LoadPolicy()
		test, _ := casdoor.Enforcer.GetRolesForUser(user.Id.String())

		assert.GreaterOrEqual(t, len(test), 1, "User %s should have at least one role", user.Id.String())
	}
}
