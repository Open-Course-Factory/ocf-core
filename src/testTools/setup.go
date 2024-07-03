package testtools

import (
	"fmt"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

var groups []casdoorsdk.Group
var roles []casdoorsdk.Role

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

	roleSupervisor := casdoorsdk.Role{Owner: orgName, Name: "supervisor", DisplayName: "Responsables", IsEnabled: true,
		Users: []string{orgName + "/1_sup", orgName + "/2_sup"}}
	roles = append(roles, roleSupervisor)

	roleAdministrator := casdoorsdk.Role{Owner: orgName, Name: "administrator", DisplayName: "Administrateurs", IsEnabled: true,
		Users: []string{orgName + "/1_sup"}}
	roles = append(roles, roleAdministrator)

	for _, role := range roles {
		_, err := casdoorsdk.AddRole(&role)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	//MANDATORY LOAD POLICY
	ok0 := casdoor.Enforcer.LoadPolicy()
	fmt.Println(ok0)

	ok1, errPolicy1 := casdoor.Enforcer.AddPolicy(roleStudent.Name, "/api/v1/courses/*", "GET")
	fmt.Println(ok1)

	ok2, errPolicy2 := casdoor.Enforcer.AddPolicy(roleAdministrator.Name, "/api/v1/*", "(GET|POST|DELETE)")
	fmt.Println(ok2)

	if errPolicy1 != nil {
		fmt.Println(errPolicy1.Error())
	}

	if errPolicy2 != nil {
		fmt.Println(errPolicy1.Error())
	}

}

func SetupUsers() {
	userService := services.NewUserService()
	userService.AddUser(dto.CreateUserInput{UserName: "1_st", DisplayName: "1 Student", Email: "1.student@test.com", Password: "test", FirstName: "Student", LastName: "1", DefaultRole: "student"})
	userService.AddUser(dto.CreateUserInput{UserName: "2_st", DisplayName: "2 Student", Email: "2.student@test.com", Password: "test", FirstName: "Student", LastName: "2", DefaultRole: "student"})
	userService.AddUser(dto.CreateUserInput{UserName: "3_st", DisplayName: "3 Student", Email: "3.student@test.com", Password: "test", FirstName: "Student", LastName: "3", DefaultRole: "student"})
	userService.AddUser(dto.CreateUserInput{UserName: "4_st", DisplayName: "4 Student", Email: "4.student@test.com", Password: "test", FirstName: "Student", LastName: "4", DefaultRole: "student"})

	userService.AddUser(dto.CreateUserInput{UserName: "1_sup", DisplayName: "1 Supervisor", Email: "1.supervisor@test.com", Password: "test", FirstName: "Supervisor", LastName: "1", DefaultRole: "administrator"})
	userService.AddUser(dto.CreateUserInput{UserName: "2_sup", DisplayName: "2 Supervisor", Email: "2.supervisor@test.com", Password: "test", FirstName: "Supervisor", LastName: "2", DefaultRole: "administrator"})
}

func DeleteAllObjects() {
	userService := services.NewUserService()

	casdoor.Enforcer.RemovePolicy("administrator")
	casdoor.Enforcer.RemovePolicy("student")
	casdoor.Enforcer.RemoveGroupingPolicy("*")

	users, _ := casdoorsdk.GetUsers()
	for _, user := range users {
		userService.DeleteUser(user.Id)
		casdoor.Enforcer.RemoveGroupingPolicy(user.Id)
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
