package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func getDSN() string {
	DB_HOST := os.Getenv("POSTGRES_HOST")
	DB_USER := os.Getenv("POSTGRES_USER")
	DB_PASSWORD := os.Getenv("POSTGRES_PASSWORD")
	DB_NAME := os.Getenv("POSTGRES_DB")

	return "host=" + DB_HOST + " user=" + DB_USER + " password=" + DB_PASSWORD + " dbname=" + DB_NAME + " sslmode=disable"
}
func TestMainDatabaseConnection(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("Erreur lors du chargement du fichier .env : %v", err)
	}

	db, err := sql.Open("postgres", getDSN())
	if err != nil {
		t.Fatalf("Erreur lors de l'ouverture de la base de données : %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("La base de données principale n'est pas accessible : %v", err)
	}

}

func TestTableExists(t *testing.T) {
	db, err := sql.Open("postgres", getDSN())
	if err != nil {
		t.Fatalf("Erreur lors de la connexion à la base de données : %v", err)
	}
	defer db.Close()

	query := "SELECT to_regclass('public.usernames')"
	var tableName sql.NullString
	err = db.QueryRow(query).Scan(&tableName)
	if err != nil {
		t.Fatalf("Erreur lors de la vérification de la table : %v", err)
	}

	if !tableName.Valid {
		t.Fatalf("La table 'usernames' n'existe pas dans la base de données")
	}

}
