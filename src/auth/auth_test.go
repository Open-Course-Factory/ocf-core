package authController

import (
	_ "embed"
	"soli/formations/src/auth/casdoor"
	sqldb "soli/formations/src/db"
	testtools "soli/formations/src/testTools"

	"log"
	"testing"
)

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	sqldb.DB = sqldb.DB.Debug()

	casdoor.InitCasdoorConnection()
	casdoor.InitCasdoorEnforcer(sqldb.DB)

	testtools.DeleteAllObjects()
	testtools.SetupUsers()
	testtools.SetupGroups()
	testtools.SetupRoles()

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
