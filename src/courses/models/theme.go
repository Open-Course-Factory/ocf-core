package models

import (
	config "soli/formations/src/configuration"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Theme struct {
	entityManagementModels.BaseModel
	Name             string
	Repository       string
	RepositoryBranch string
	Size             string
}

func (t Theme) IsThemeExtended(themes ...string) (bool, string) {
	res := false
	from := ""

	extendsFilePath := config.THEMES_ROOT + "/" + t.Name + "/extends.json"
	if fileExists(extendsFilePath) {
		extends := LoadExtends(extendsFilePath)
		from = extends.Theme
		res = true
	}

	return res, from
}
