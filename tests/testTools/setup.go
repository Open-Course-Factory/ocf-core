package test_tools

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"
	coursesDto "soli/formations/src/courses/dto"
	labsDto "soli/formations/src/labs/dto"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	_ "embed"

	sqldb "soli/formations/src/db"

	authModels "soli/formations/src/auth/models"
	courseModels "soli/formations/src/courses/models"
	baseModels "soli/formations/src/entityManagement/models"
	labsModels "soli/formations/src/labs/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	authRegistration "soli/formations/src/auth/entityRegistration"
	coursesRegistration "soli/formations/src/courses/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
	labRegistration "soli/formations/src/labs/entityRegistration"

	genericServices "soli/formations/src/entityManagement/services"
)

var groups []casdoorsdk.Group

func SetupTestDatabase() {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"

	sqldb.InitDBConnection(basePath + ".env.test")
	sqldb.DB = sqldb.DB.Debug()

	// Run migrations FIRST before trying to delete
	sqldb.DB.AutoMigrate()
	sqldb.DB.AutoMigrate(&courseModels.Page{})
	sqldb.DB.AutoMigrate(&courseModels.Section{})
	sqldb.DB.AutoMigrate(&courseModels.Chapter{})
	sqldb.DB.AutoMigrate(&courseModels.Course{})
	sqldb.DB.AutoMigrate(&courseModels.Generation{})
	sqldb.DB.AutoMigrate(&courseModels.Session{})

	sqldb.DB.AutoMigrate(&authModels.SshKey{})

	sqldb.DB.AutoMigrate(&labsModels.Username{})
	sqldb.DB.AutoMigrate(&labsModels.Machine{})
	sqldb.DB.AutoMigrate(&labsModels.Connection{})

	sqldb.DB.AutoMigrate(&terminalModels.UserTerminalKey{})
	sqldb.DB.AutoMigrate(&terminalModels.Terminal{})
	sqldb.DB.AutoMigrate(&terminalModels.TerminalShare{})

	// Now clean up existing test data
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&courseModels.Page{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&courseModels.Section{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&courseModels.Chapter{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&courseModels.Course{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&courseModels.Session{})

	sqldb.DB.Where("1 = 1").Unscoped().Delete(&authModels.SshKey{})

	sqldb.DB.Where("1 = 1").Unscoped().Delete(&labsModels.Connection{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&labsModels.Username{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&labsModels.Machine{})

	sqldb.DB.Where("1 = 1").Unscoped().Delete(&terminalModels.TerminalShare{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&terminalModels.Terminal{})
	sqldb.DB.Where("1 = 1").Unscoped().Delete(&terminalModels.UserTerminalKey{})

}

func SetupCasdoor() {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"
	casdoor.InitCasdoorConnection(basePath+"src/auth/casdoor/", ".env.test")
	casdoor.InitCasdoorEnforcer(sqldb.DB, basePath)
}

// SetupBasicRoles crée les rôles de base nécessaires AVANT la création des utilisateurs
func SetupBasicRoles() {
	orgName := os.Getenv("CASDOOR_ORGANIZATION_NAME")

	// Créer les rôles de base sans utilisateurs ni permissions
	basicRoles := []casdoorsdk.Role{
		{
			Owner:       orgName,
			Name:        "student",
			DisplayName: "Etudiants",
			IsEnabled:   true,
			Users:       []string{}, // Vide pour l'instant
		},
		{
			Owner:       orgName,
			Name:        "supervisor",
			DisplayName: "Responsables",
			IsEnabled:   true,
			Users:       []string{}, // Vide pour l'instant
		},
		{
			Owner:       orgName,
			Name:        "administrator",
			DisplayName: "Administrateurs",
			IsEnabled:   true,
			Users:       []string{}, // Vide pour l'instant
		},
	}

	for _, role := range basicRoles {
		existingRole, err := casdoorsdk.GetRole(role.Name)
		if err != nil || existingRole == nil {
			_, err := casdoorsdk.AddRole(&role)
			if err != nil {
				log.Printf("Erreur lors de la création du rôle de base %s: %v", role.Name, err)
			} else {
				log.Printf("Rôle de base créé: %s", role.Name)
			}
		} else {
			log.Printf("Rôle de base %s existe déjà", role.Name)
		}
	}
}

func SetupGroups() {

	groups = append(groups, casdoorsdk.Group{ParentId: os.Getenv("CASDOOR_ORGANIZATION_NAME"), Name: "classes", DisplayName: "Toutes les classes"})
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

// SetupRoles maintenant met à jour les rôles existants avec les utilisateurs et permissions
func SetupRoles() {
	orgName := os.Getenv("CASDOOR_ORGANIZATION_NAME")

	// Mettre à jour les rôles existants avec les utilisateurs
	roleUpdates := []struct {
		name  string
		users []string
	}{
		{
			name:  "student",
			users: []string{orgName + "/1_st", orgName + "/2_st", orgName + "/3_st", orgName + "/4_st"},
		},
		{
			name:  "supervisor",
			users: []string{orgName + "/1_sup", orgName + "/2_sup"},
		},
		{
			name:  "administrator",
			users: []string{orgName + "/1_sup"},
		},
	}

	// Charger la politique avant de configurer les permissions
	errLoadPolicy := casdoor.Enforcer.LoadPolicy()
	if errLoadPolicy != nil {
		log.Printf("Attention: Échec du chargement de la politique")
	}

	// Mettre à jour chaque rôle avec ses utilisateurs
	for _, update := range roleUpdates {
		existingRole, err := casdoorsdk.GetRole(update.name)
		if err != nil {
			log.Printf("Erreur lors de la récupération du rôle %s: %v", update.name, err)
			continue
		}

		if existingRole != nil {
			existingRole.Users = update.users
			_, err := casdoorsdk.UpdateRole(existingRole)
			if err != nil {
				log.Printf("Erreur lors de la mise à jour du rôle %s: %v", update.name, err)
			} else {
				log.Printf("Rôle mis à jour: %s avec %d utilisateurs", update.name, len(update.users))
			}
		}
	}

	// Ajouter les permissions (politiques)
	casdoor.Enforcer.AddPolicy("student", "/api/v1/courses/*", "GET")
	casdoor.Enforcer.AddPolicy("student", "/api/v1/usernames/", "(GET|POST)")
	casdoor.Enforcer.AddPolicy("administrator", "/api/v1/*", "(GET|PATCH|POST|DELETE)")

	log.Println("Permissions configurées pour les rôles")
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
	// SAFETY CHECK: Verify we're using the test database before deleting anything
	var dbName string
	sqldb.DB.Raw("SELECT current_database()").Scan(&dbName)
	if dbName != "ocf_test" && dbName != "" {
		panic(fmt.Sprintf("❌ DANGER: Attempted to delete data from NON-TEST database '%s'! Tests must use 'ocf_test' database. Check your .env.test file.", dbName))
	}
	fmt.Printf("✅ Safety check passed: Using test database '%s'\n", dbName)

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

	gs := genericServices.NewGenericService(sqldb.DB, nil)

	coursesPages, _, _ := gs.GetEntities(courseModels.Course{}, 1, 100, map[string]interface{}{}, nil)
	coursesDtoArray, _ := gs.GetDtoArrayFromEntitiesPages(coursesPages, courseModels.Course{}, "Course")

	for _, courseDto := range coursesDtoArray {
		id := gs.ExtractUuidFromReflectEntity(courseDto)
		courseDto := courseDto.(*coursesDto.CourseOutput)
		courseToDelete := &courseModels.Course{
			BaseModel: baseModels.BaseModel{
				ID: id,
			},
			Name: courseDto.Name,
		}

		gs.DeleteEntity(id, courseToDelete, true)
	}

	usernamesPages, _, _ := gs.GetEntities(labsModels.Username{}, 1, 100, map[string]interface{}{}, nil)
	usernamesDtoArray, _ := gs.GetDtoArrayFromEntitiesPages(usernamesPages, labsModels.Username{}, "Username")
	for _, usernameDto := range usernamesDtoArray {
		id := gs.ExtractUuidFromReflectEntity(usernameDto)
		usernameDto := usernameDto.(*labsDto.UsernameOutput)
		usernameToDelete := &labsModels.Username{
			BaseModel: baseModels.BaseModel{
				ID: id,
			},
			Username: usernameDto.Username,
		}

		gs.DeleteEntity(id, usernameToDelete, true)
	}

	machinesPages, _, _ := gs.GetEntities(labsModels.Machine{}, 1, 100, map[string]interface{}{}, nil)
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

		gs.DeleteEntity(id, machineToDelete, true)
	}

}

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	SetupTestDatabase()
	SetupCasdoor()

	DeleteAllObjects()

	ems.GlobalEntityRegistrationService.RegisterEntity(coursesRegistration.SessionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(coursesRegistration.PageRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(coursesRegistration.SectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(coursesRegistration.ChapterRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(coursesRegistration.CourseRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.SshKeyRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.UsernameRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(labRegistration.MachineRegistration{})

	SetupBasicRoles() // Créer les rôles de base d'abord
	SetupUsers()      // Les utilisateurs peuvent maintenant être créés avec des rôles existants
	SetupGroups()     // Puis les groupes
	SetupRoles()      // Enfin configurer les permissions et assigner les utilisateurs aux rôles

	return func(tb testing.TB) {
		log.Println("teardown test")
		//DeleteAllObjects()
	}
}
