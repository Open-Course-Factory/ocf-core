package sqldb

import (
	"fmt"
	"log"
	"os"
	"soli/formations/src/courses/models"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// DB is a global variable to hold db connection
var DB *gorm.DB
var DBType string

const DB_FILE = "./db-file.db"

// InitDBConnection opens a connection to the database
func InitDBConnection() {
	var err error

	// load
	err = godotenv.Load()
	DBType = os.Getenv("DATABASE")

	if err != nil {
		log.Default().Printf("err loading: %v", err)
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
		DB, err = gorm.Open(sqlite.Open(DB_FILE))
	}
	if err != nil {
		panic(err)
	}

}

func AutoMigrate() {
	DB.AutoMigrate()

	DB.AutoMigrate(&models.Page{})
	DB.AutoMigrate(&models.Section{})
	DB.AutoMigrate(&models.Chapter{})
	DB.AutoMigrate(&models.Course{})
}
