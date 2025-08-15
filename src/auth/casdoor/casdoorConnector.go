package casdoor

import (
	"fmt"
	"log"
	"os"
	"soli/formations/src/auth/interfaces"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

var JwtPublicKey string

var Enforcer interfaces.EnforcerInterface

func InitCasdoorConnection(basePath string) {
	err := godotenv.Load(basePath)
	casdoorEndPoint := os.Getenv("CASDOOR_ENDPOINT")
	casdoorClientId := os.Getenv("CASDOOR_CLIENT_ID")
	casdoorClientsecret := os.Getenv("CASDOOR_CLIENT_SECRET")
	casdoorOrganizationName := os.Getenv("CASDOOR_ORGANIZATION_NAME")
	casdoorApplicationName := os.Getenv("CASDOOR_APPLICATION_NAME")

	if err != nil {
		log.Default().Printf("err loading: %v", err)
	}

	b, err := os.ReadFile("./token_jwt_key.pem")
	if err != nil {
		panic(fmt.Sprintf("Certificate token_jwt_key.pem not readable: %v", err))
	}

	JwtPublicKey := string(b)

	casdoorsdk.InitConfig(casdoorEndPoint, casdoorClientId, casdoorClientsecret, JwtPublicKey, casdoorOrganizationName, casdoorApplicationName)
}

func InitCasdoorEnforcer(db *gorm.DB, basePath string) {
	// Initialize  casbin adapter
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize casbin adapter: %v", err))
	}

	// Load model configuration file and policy store adapter
	enforcer, err := casbin.NewEnforcer(basePath+"src/configuration/keymatch_model.conf", adapter)
	if err != nil {
		panic(fmt.Sprintf("failed to create casbin enforcer: %v", err))
	}

	// Load policy from Database
	errEnforcer := enforcer.LoadPolicy()
	if errEnforcer != nil {
		panic(fmt.Sprintf("Failed to load policy from DB: %v", errEnforcer))
	}

	// Utiliser le wrapper au lieu d'assigner directement
	Enforcer = NewEnforcerWrapper(enforcer)
}

// SetEnforcer permet d'injecter un enforcer pour les tests
func SetEnforcer(enforcer interfaces.EnforcerInterface) {
	Enforcer = enforcer
}
