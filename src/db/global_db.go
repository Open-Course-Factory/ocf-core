package sqldb

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// DB is a global variable to hold db connection
var DB *gorm.DB
var DBType string

var DBFile string

//const ENV_FILE = TESTS_ROOT + ".env.test"

// InitDBConnection opens a connection to the database
func InitDBConnection(envFile string) {

	err := godotenv.Load(envFile)

	DBType = os.Getenv("DATABASE")

	if err != nil {
		log.Default().Printf("err loading: %v", err)
	}

	if DBType == "postgres" {
		db := os.Getenv("POSTGRES_DB")

		host := os.Getenv("POSTGRES_HOST")
		user := os.Getenv("POSTGRES_USER")
		passwd := os.Getenv("POSTGRES_PASSWORD")

		connectionString := fmt.Sprintf(
			"host=%s user=%s dbname=%s password=%s",
			host, user, db, passwd,
		)

		DB, _ = gorm.Open(postgres.Open(connectionString), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
	} else {
		panic("Unsupported DB")
	}

}
