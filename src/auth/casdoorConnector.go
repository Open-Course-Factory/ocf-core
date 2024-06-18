package authController

import (
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

//go:embed token_jwt_key.pem
var JwtPublicKey string

var Enforcer *casbin.Enforcer

func InitCasdoorConnection() {
	err := godotenv.Load()
	casdoorEndPoint := os.Getenv("CASDOOR_ENDPOINT")
	casdoorClientId := os.Getenv("CASDOOR_CLIENT_ID")
	casdoorClientsecret := os.Getenv("CASDOOR_CLIENT_SECRET")
	casdoorOrganizationName := os.Getenv("CASDOOR_ORGANIZATION_NAME")
	casdoorApplicationName := os.Getenv("CASDOOR_APPLICATION_NAME")

	if err != nil {
		log.Default().Printf("err loading: %v", err)
	}

	casdoorsdk.InitConfig(casdoorEndPoint, casdoorClientId, casdoorClientsecret, JwtPublicKey, casdoorOrganizationName, casdoorApplicationName)
}

func InitCasdoorEnforcer(db *gorm.DB) {
	// Initialize  casbin adapter
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize casbin adapter: %v", err))
	}

	// Load model configuration file and policy store adapter
	enforcer, err := casbin.NewEnforcer("config/rbac_model.conf", adapter)
	if err != nil {
		panic(fmt.Sprintf("failed to create casbin enforcer: %v", err))
	}

	// Load policy from Database
	errEnforcer := enforcer.LoadPolicy()
	if errEnforcer != nil {
		panic(fmt.Sprintf("Failed to load policy from DB: %v", errEnforcer))
	}

	Enforcer = enforcer
}
