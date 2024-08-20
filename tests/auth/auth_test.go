package auth_tests

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"

	test_tools "soli/formations/tests/testTools"

	services "soli/formations/src/auth/services"

	"soli/formations/src/auth/casdoor"
)

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	test_tools.SetupTestDatabase()
	test_tools.SetupCasdoor()

	test_tools.DeleteAllObjects()
	test_tools.SetupUsers()
	test_tools.SetupGroups()
	test_tools.SetupRoles()

	return func(tb testing.TB) {
		log.Println("teardown test")
		test_tools.DeleteAllObjects()
	}
}

func TestUserCreation(t *testing.T) {
	teardownTest := SetupFunctionnalTests(t)

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
