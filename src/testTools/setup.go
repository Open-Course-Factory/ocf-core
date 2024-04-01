package testtools

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

var users []casdoorsdk.User
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
	roles = append(roles, casdoorsdk.Role{Owner: orgName, Name: "student", DisplayName: "Etudiants", IsEnabled: true,
		Users: []string{orgName + "/1_st", orgName + "/2_st", orgName + "/3_st", orgName + "/4_st"}})
	roles = append(roles, casdoorsdk.Role{Owner: orgName, Name: "supervisor", DisplayName: "Responsables", IsEnabled: true,
		Users: []string{orgName + "/1_sup", orgName + "/2_sup"}})
	roles = append(roles, casdoorsdk.Role{Owner: orgName, Name: "administrator", DisplayName: "Administrateurs", IsEnabled: true,
		Users: []string{orgName + "/1_sup"}})

	for _, role := range roles {
		_, err := casdoorsdk.AddRole(&role)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
}

func SetupUsers() {
	users = append(users, casdoorsdk.User{Name: "1_st", DisplayName: "1 Student", Email: "1.student@test.com", Password: "test",
		LastName: "Student", FirstName: "1", SignupApplication: "ocf"})
	users = append(users, casdoorsdk.User{Name: "2_st", DisplayName: "2 Student", Email: "2.student@test.com", Password: "test",
		LastName: "Student", FirstName: "2", SignupApplication: "ocf"})
	users = append(users, casdoorsdk.User{Name: "3_st", DisplayName: "3 Student", Email: "3.student@test.com", Password: "test",
		LastName: "Student", FirstName: "3", SignupApplication: "ocf"})
	users = append(users, casdoorsdk.User{Name: "4_st", DisplayName: "4 Student", Email: "4.student@test.com", Password: "test",
		LastName: "Student", FirstName: "4", SignupApplication: "ocf"})
	users = append(users, casdoorsdk.User{Name: "1_sup", DisplayName: "1 Supervisor", Email: "1.supervisor@test.com", Password: "test",
		LastName: "Supervisor", FirstName: "1", SignupApplication: "ocf"})
	users = append(users, casdoorsdk.User{Name: "2_sup", DisplayName: "2 Supervisor", Email: "2.supervisor@test.com", Password: "test",
		LastName: "Supervisor", FirstName: "2", SignupApplication: "ocf"})

	for _, user := range users {
		_, err := casdoorsdk.AddUser(&user)
		if err != nil {
			fmt.Println(err.Error())
		}
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
