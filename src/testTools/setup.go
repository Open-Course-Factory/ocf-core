package testtools

import (
	"fmt"
	"soli/formations/src/auth/casdoor"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

var groups []casdoorsdk.Group
var roles []casdoorsdk.Role
var permissions []casdoorsdk.Permission

func SetupPermissions() {

	permissions = append(permissions, casdoorsdk.Permission{Name: "permission_test", Owner: "ocf",
		ResourceType: "Course", Resources: []string{"courses/1", "courses/2"}, Actions: []string{"Read"}, Effect: "Allow",
		State: "", Domains: []string{}, Users: []string{}, Roles: []string{"sdv/student"}, Groups: []string{},
		IsEnabled: true,
	})

	for _, permission := range permissions {
		_, err := casdoorsdk.AddPermission(&permission)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

}

func SetupGroups() {
	groups = append(groups, casdoorsdk.Group{ParentId: "sdv", Name: "classes", DisplayName: "Toutes les classes"})
	groups = append(groups, casdoorsdk.Group{ParentId: "classes", Name: "do_m1", DisplayName: "Dev Ops M1"})
	groups = append(groups, casdoorsdk.Group{ParentId: "classes", Name: "do_m2", DisplayName: "Dev Ops M2"})
	groups = append(groups, casdoorsdk.Group{ParentId: "do_m1", Name: "do_m1-classeA", DisplayName: "Groupe A"})
	groups = append(groups, casdoorsdk.Group{ParentId: "do_m1", Name: "do_m1-classeB", DisplayName: "Groupe B"})
	groups = append(groups, casdoorsdk.Group{ParentId: "do_m2", Name: "do_m2-classeA", DisplayName: "Groupe A"})
	groups = append(groups, casdoorsdk.Group{ParentId: "do_m2", Name: "do_m2-classeB", DisplayName: "Groupe B"})

	for _, group := range groups {
		_, err := casdoorsdk.AddGroup(&group)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

}

func SetupRoles() {
	orgName := "sdv"
	roleStudent := casdoorsdk.Role{Owner: orgName, Name: "student", DisplayName: "Etudiants", IsEnabled: true,
		Users: []string{orgName + "/1_st", orgName + "/2_st", orgName + "/3_st", orgName + "/4_st"}}
	roles = append(roles, roleStudent)

	_, errPolicy1 := casdoor.Enforcer.AddPolicy(roleStudent.Name, "/api/v1/courses/*", "GET")
	if errPolicy1 != nil {
		fmt.Println(errPolicy1.Error())
	}

	roleSupervisor := casdoorsdk.Role{Owner: orgName, Name: "supervisor", DisplayName: "Responsables", IsEnabled: true,
		Users: []string{orgName + "/1_sup", orgName + "/2_sup"}}
	roles = append(roles, roleSupervisor)

	roleAdministrator := casdoorsdk.Role{Owner: orgName, Name: "administrator", DisplayName: "Administrateurs", IsEnabled: true,
		Users: []string{orgName + "/1_sup"}}
	roles = append(roles, roleAdministrator)

	casdoor.Enforcer.AddPolicy(roleAdministrator.Name, "/api/v1/courses/*", "(GET)|(POST)|(DELETE)")

	for _, role := range roles {
		_, err := casdoorsdk.AddRole(&role)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
}

func SetupUsers() {
	createUser("1_st", "1 Student", "1.student@test.com", "test", "Student", "1", "student")
	createUser("2_st", "2 Student", "2.student@test.com", "test", "Student", "2", "student")
	createUser("3_st", "3 Student", "3.student@test.com", "test", "Student", "3", "student")
	createUser("4_st", "4 Student", "4.student@test.com", "test", "Student", "4", "student")

	createUser("1_sup", "1 Supervisor", "1.supervisor@test.com", "test", "Supervisor", "1", "administrator")
	createUser("2_sup", "2 Supervisor", "2.supervisor@test.com", "test", "Supervisor", "2", "administrator")
}

// ToDo: Move in User Service
func createUser(userName string, displayName string, email string, password string, lastName string, firstName string, defaultRole string) {
	user1 := casdoorsdk.User{Name: userName, DisplayName: displayName, Email: email, Password: password,
		LastName: lastName, FirstName: firstName, SignupApplication: "ocf"}

	_, errCreate := casdoorsdk.AddUser(&user1)
	if errCreate != nil {
		fmt.Println(errCreate.Error())
	}

	createdUser, _ := casdoorsdk.GetUserByEmail(email)

	_, errStudent := casdoor.Enforcer.AddGroupingPolicy(defaultRole, createdUser.Id)
	if errStudent != nil {
		fmt.Println(errStudent.Error())
	}
}

func DeleteAllObjects() {
	permissions, _ := casdoorsdk.GetPermissions()
	for _, permission := range permissions {
		casdoorsdk.DeletePermission(permission)
	}

	users, _ := casdoorsdk.GetUsers()
	for _, user := range users {
		casdoorsdk.DeleteUser(user)
	}

	roles, _ := casdoorsdk.GetRoles()
	for _, role := range roles {
		casdoorsdk.DeleteRole(role)
	}

	groups, _ := casdoorsdk.GetGroups()
	for i := len(groups) - 1; i >= 0; i-- {
		_, err := casdoorsdk.DeleteGroup(groups[i])
		if err != nil {
			fmt.Println(err.Error())
		}

	}

	models, _ := casdoorsdk.GetModels()
	for _, model := range models {
		casdoorsdk.DeleteModel(model)
	}
}
