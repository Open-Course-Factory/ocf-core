package sqldb

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// DB is a global variable to hold db connection
var DB *gorm.DB
var DBType string

// ConnectDB opens a connection to the database
func ConnectDB() {
	var err error

	// load
	err = godotenv.Load()
	DBType = os.Getenv("DATABASE")

	if err != nil {
		log.Fatalf("err loading: %v", err)
	}

	if DBType == "postgres" {
		connectionString := fmt.Sprintf("host=%s user=%s dbname=%s password=%s",
			os.Getenv("POSTGRES_HOST"),
			os.Getenv("POSTGRES_USER"),
			os.Getenv("POSTGRES_DB"),
			os.Getenv("POSTGRES_PASSWORD"))

		DB, err = gorm.Open(postgres.Open(connectionString), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
	} else if DBType == "sqlite" {
		DB, err = gorm.Open(sqlite.Open("./db-file.db"))
	}
	if err != nil {
		panic(err)
	}

}
