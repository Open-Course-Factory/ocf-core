package models

import (
	"encoding/json"
	"log"
	"os"
)

type Extends struct {
	Theme string
}

func LoadExtends(extendsFilePath string) Extends {
	jsonFile, err := os.ReadFile(extendsFilePath)

	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	var extends Extends
	err = json.Unmarshal(jsonFile, &extends)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}

	return extends
}
