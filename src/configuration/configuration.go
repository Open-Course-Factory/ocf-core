package config

import (
	"encoding/json"
	"log"
	"os"
)

const COURSES_ROOT = "./coursesV2/"
const COURSES_OUTPUT_DIR = "./dist/"

const THEMES_ROOT = "./themes/"
const IMAGES_ROOT = "./images/"

// Part of a Section
type Configuration struct {
	AuthorTrigram    string `json:"author_trigram"`
	AuthorFullname   string `json:"author_fullname"`
	AuthorEmail      string `json:"author_email"`
	SecretJwt        string `mapstructure:"SECRET_JWT"`
	SecretRefreshJwt string `mapstructure:"SECRET_REFRESH_JWT"`
}

func ReadJsonConfigurationFile(jsonConfigurationFilePath string) Configuration {
	jsonFile, err := os.ReadFile(jsonConfigurationFilePath)

	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	var configuration Configuration
	err = json.Unmarshal(jsonFile, &configuration)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}
	return configuration
}
