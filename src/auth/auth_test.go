package authController

import (
	_ "embed"
	"soli/formations/src/auth/casdoor"
	testtools "soli/formations/src/testTools"

	"log"
	"testing"
)

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")
	casdoor.InitCasdoorConnection()

	testtools.DeleteAllObjects()
	testtools.SetupUsers()
	testtools.SetupGroups()
	testtools.SetupRoles()
	testtools.SetupPermissions()

	return func(tb testing.TB) {
		log.Println("teardown test")
		// for _, user := range users {
		// 	casdoorsdk.DeleteUser(&user)
		// }
	}
}

func TestUserCreation(t *testing.T) {
	teardownTest := SetupFunctionnalTests(t)
	defer teardownTest(t)

}
