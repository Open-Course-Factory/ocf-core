package auth_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	test_tools "soli/formations/tests/testTools"

	services "soli/formations/src/auth/services"

	"soli/formations/src/auth/casdoor"
)

func TestUserCreation(t *testing.T) {
	teardownTest := test_tools.SetupFunctionnalTests(t)

	userService := services.NewUserService()

	users, _ := userService.GetAllUsers()

	assert.Equal(t, 6, len(*users))

	for _, user := range *users {
		casdoor.Enforcer.LoadPolicy()
		test, _ := casdoor.Enforcer.GetFilteredGroupingPolicy(0, user.Id.String())

		assert.GreaterOrEqual(t, 1, len(test))
	}

	defer teardownTest(t)

}
