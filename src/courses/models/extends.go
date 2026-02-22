package models

import (
	"encoding/json"
	"fmt"
	"os"
)

type Extends struct {
	Theme string
}

func LoadExtends(extendsFilePath string) (Extends, error) {
	jsonFile, err := os.ReadFile(extendsFilePath)

	if err != nil {
		return Extends{}, fmt.Errorf("error reading extends file %s: %w", extendsFilePath, err)
	}

	var extends Extends
	err = json.Unmarshal(jsonFile, &extends)
	if err != nil {
		return Extends{}, fmt.Errorf("error unmarshaling extends file %s: %w", extendsFilePath, err)
	}

	return extends, nil
}
