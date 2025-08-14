// src/auth/interfaces/casdoorInterface.go
package interfaces

import "github.com/casdoor/casdoor-go-sdk/casdoorsdk"

// CasdoorService interface pour abstraire les appels Casdoor
type CasdoorService interface {
	GetUserByEmail(email string) (*casdoorsdk.User, error)
	GetGroup(groupId string) (*casdoorsdk.Group, error)
}

// casdoorService implémentation réelle
type casdoorService struct{}

func NewCasdoorService() CasdoorService {
	return &casdoorService{}
}

func (c *casdoorService) GetUserByEmail(email string) (*casdoorsdk.User, error) {
	return casdoorsdk.GetUserByEmail(email)
}

func (c *casdoorService) GetGroup(groupId string) (*casdoorsdk.Group, error) {
	return casdoorsdk.GetGroup(groupId)
}
