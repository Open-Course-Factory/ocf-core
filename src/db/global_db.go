package sqldb

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	config "soli/formations/src/configuration"
)

// DB is a global variable to hold db connection
var DB *gorm.DB

//const ENV_FILE = TESTS_ROOT + ".env.test"

// InitDBConnection opens a connection to the database
func InitDBConnection(envFile string) {

	err := config.LoadDotEnv(envFile)

	if err != nil {
		log.Default().Printf("err loading: %v", err)
	}

	if os.Getenv("DATABASE") == "postgres" {
		db := os.Getenv("POSTGRES_DB")

		host := os.Getenv("POSTGRES_HOST")
		port := os.Getenv("POSTGRES_PORT")
		user := os.Getenv("POSTGRES_USER")
		passwd := os.Getenv("POSTGRES_PASSWORD")

		connectionString := fmt.Sprintf(
			"host=%s port=%s user=%s dbname=%s password=%s",
			host, port, user, db, passwd,
		)

		var err error
		DB, err = gorm.Open(postgres.Open(connectionString), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
		if err != nil {
			log.Fatalf("❌ Failed to connect to PostgreSQL database: %v\nConnection string (without password): host=%s port=%s user=%s dbname=%s",
				err, host, port, user, db)
		}
		log.Printf("✅ Successfully connected to PostgreSQL database: %s@%s:%s/%s", user, host, port, db)
	} else {
		panic("Unsupported DB")
	}

}
