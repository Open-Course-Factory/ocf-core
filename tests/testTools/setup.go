package test_tools

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	authDto "soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"
	labsDto "soli/formations/src/labs/dto"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"

	_ "embed"

	sqldb "soli/formations/src/db"

	authModels "soli/formations/src/auth/models"
	courseModels "soli/formations/src/courses/models"
	baseModels "soli/formations/src/entityManagement/models"
	labsModels "soli/formations/src/labs/models"

	courseServices "soli/formations/src/courses/services"
	genericServices "soli/formations/src/entityManagement/services"
)

var groups []casdoorsdk.Group
var roles []casdoorsdk.Role

func SetupTestDatabase() {

	sqldb.InitDBConnection("../.env.test")
	sqldb.DB = sqldb.DB.Debug()
	sqldb.DB.AutoMigrate()
	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})
	sqldb.DB.AutoMigrate(&courseModels.Session{})

	sqldb.DB.AutoMigrate(&authModels.Sshkey{})
	sqldb.DB.AutoMigrate(&authModels.Username{})

	sqldb.DB.AutoMigrate(&labsModels.Machine{})

}

func SetupCasdoor() {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"
	casdoor.InitCasdoorConnection(basePath + ".env")
	casdoor.InitCasdoorEnforcer(sqldb.DB, basePath)
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

	casdoor.Enforcer.AddPolicy(roleStudent.Name, "/api/v1/courses/*", "GET")
	casdoor.Enforcer.AddPolicy(roleStudent.Name, "/api/v1/usernames/", "(GET|POST)")
	casdoor.Enforcer.AddPolicy(roleAdministrator.Name, "/api/v1/*", "(GET|PATCH|POST|DELETE)")

}

func SetupUsers() {
	userService := services.NewUserService()
	_, err := userService.AddUser(dto.CreateUserInput{UserName: "1_st", DisplayName: "1 Student", Email: "1.student@test.com", Password: "test", FirstName: "Student", LastName: "1", DefaultRole: "student"})
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = userService.AddUser(dto.CreateUserInput{UserName: "2_st", DisplayName: "2 Student", Email: "2.student@test.com", Password: "test", FirstName: "Student", LastName: "2", DefaultRole: "student"})
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = userService.AddUser(dto.CreateUserInput{UserName: "3_st", DisplayName: "3 Student", Email: "3.student@test.com", Password: "test", FirstName: "Student", LastName: "3", DefaultRole: "student"})
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = userService.AddUser(dto.CreateUserInput{UserName: "4_st", DisplayName: "4 Student", Email: "4.student@test.com", Password: "test", FirstName: "Student", LastName: "4", DefaultRole: "student"})
	if err != nil {
		log.Fatal(err.Error())
	}

	_, err = userService.AddUser(dto.CreateUserInput{UserName: "1_sup", DisplayName: "1 Supervisor", Email: "1.supervisor@test.com", Password: "test", FirstName: "Supervisor", LastName: "1", DefaultRole: "administrator"})
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = userService.AddUser(dto.CreateUserInput{UserName: "2_sup", DisplayName: "2 Supervisor", Email: "2.supervisor@test.com", Password: "test", FirstName: "Supervisor", LastName: "2", DefaultRole: "administrator"})
	if err != nil {
		log.Fatal(err.Error())
	}
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

	cs := courseServices.NewCourseService(sqldb.DB)
	courses, _ := cs.GetCourses()
	for _, cours := range courses {
		uuid, err := uuid.Parse(cours.CourseID_str)
		if err != nil {
			fmt.Println(err.Error())
		}
		cs.DeleteCourse(uuid)
	}

	gs := genericServices.NewGenericService(sqldb.DB)
	usernamesPages, _ := gs.GetEntities(authModels.Username{})
	usernamesDtoArray, _ := gs.GetDtoArrayFromEntitiesPages(usernamesPages, authModels.Username{}, "Username")
	for _, usernameDto := range usernamesDtoArray {
		id := gs.ExtractUuidFromReflectEntity(usernameDto)
		usernameDto := usernameDto.(*authDto.UsernameOutput)
		usernameToDelete := &authModels.Username{
			BaseModel: baseModels.BaseModel{
				ID: id,
			},
			Username: usernameDto.Username,
		}

		gs.DeleteEntity(id, usernameToDelete)
	}

	machinesPages, _ := gs.GetEntities(labsModels.Machine{})
	machinesDtoArray, _ := gs.GetDtoArrayFromEntitiesPages(machinesPages, labsModels.Machine{}, "Machine")
	for _, machineDto := range machinesDtoArray {
		id := gs.ExtractUuidFromReflectEntity(machineDto)
		machineDto := machineDto.(*labsDto.MachineOutput)
		machineToDelete := &labsModels.Machine{
			BaseModel: baseModels.BaseModel{
				ID: id,
			},
			Name: machineDto.Name,
		}

		gs.DeleteEntity(id, machineToDelete)
	}

}

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	SetupTestDatabase()
	SetupCasdoor()

	DeleteAllObjects()
	SetupUsers()
	SetupGroups()
	SetupRoles()

	return func(tb testing.TB) {
		log.Println("teardown test")
		DeleteAllObjects()
	}
}
