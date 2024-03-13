package authController

import (
	_ "embed"
	"log"
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/joho/godotenv"
)

// /go:embed token_jwt_key.pem
var JwtPublicKey string

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
