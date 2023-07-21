package controller

import (
	"soli/formations/src/auth/models"
	"strings"

	pluralize "github.com/gertd/go-pluralize"
)

func GetEntityModelInterface(entityName string) interface{} {
	var result interface{}
	switch entityName {
	case "Role":
		result = models.Role{}
	case "User":
		result = models.User{}
	case "Group":
		result = models.Group{}
	case "Organisation":
		result = models.Organisation{}
	case "Permission":
		result = models.Permission{}
	}
	return result
}

func GetEntityNameFromPath(path string) string {

	// Trim any trailing slashes
	path = strings.TrimRight(path, "/")

	// Split the path into segments
	segments := strings.Split(path, "/")

	// Take resource name segment
	segment := segments[3]

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}
