package casdoor

import (
	"fmt"
	"log"
	"os"
	"soli/formations/src/auth/interfaces"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/persist"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"

	config "soli/formations/src/configuration"
)

var JwtPublicKey string

var Enforcer interfaces.EnforcerInterface

func InitCasdoorConnection(basePath string, fileName string) {
	err := config.LoadDotEnv(basePath + fileName)
	casdoorEndPoint := os.Getenv("CASDOOR_ENDPOINT")
	casdoorClientId := os.Getenv("CASDOOR_CLIENT_ID")
	casdoorClientsecret := os.Getenv("CASDOOR_CLIENT_SECRET")
	casdoorOrganizationName := os.Getenv("CASDOOR_ORGANIZATION_NAME")
	casdoorApplicationName := os.Getenv("CASDOOR_APPLICATION_NAME")

	if err != nil {
		log.Default().Printf("err loading: %v", err)
	}

	b, err := os.ReadFile(basePath + "token_jwt_key.pem")
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
	enforcer, err := NewEnforcer(basePath+"src/configuration/keymatch_model.conf", adapter)
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

// NewEnforcer builds the enforcer the application uses, wrapped behind
// EnforcerInterface. With no adapter, policies live in memory only.
//
// It is a SyncedEnforcer: requests enforce concurrently while member hooks and
// role sync change policies, and every policy change resets casbin's matcher
// cache. Unsynchronised, a reader caught by that reset ends the process
// ("sync: unlock of unlocked mutex").
func NewEnforcer(modelPath string, adapter ...persist.Adapter) (interfaces.EnforcerInterface, error) {
	params := []any{modelPath}
	for _, a := range adapter {
		params = append(params, a)
	}
	enforcer, err := casbin.NewSyncedEnforcer(params...)
	if err != nil {
		return nil, err
	}
	return NewEnforcerWrapper(enforcer), nil
}

// SetEnforcer permet d'injecter un enforcer pour les tests
func SetEnforcer(enforcer interfaces.EnforcerInterface) {
	Enforcer = enforcer
}
