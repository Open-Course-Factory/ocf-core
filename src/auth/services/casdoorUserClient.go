// src/auth/services/casdoorUserClient.go
//
// CasdoorUserClient is a thin seam over the package-level casdoorsdk functions
// that userService uses during DeleteUser. Introducing this interface lets the
// deletion orchestration be unit-tested without standing up a real Casdoor
// instance (see tests/auth/userDeletion_test.go).
package services

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorUserClient wraps the Casdoor SDK calls needed to look up and delete
// a user. Production code uses defaultCasdoorUserClient which forwards to the
// casdoorsdk package functions of the same names.
type CasdoorUserClient interface {
	GetUserByUserId(userID string) (*casdoorsdk.User, error)
	DeleteUser(user *casdoorsdk.User) (bool, error)
}

// defaultCasdoorUserClient is the production implementation that delegates to
// the casdoorsdk package-level functions.
type defaultCasdoorUserClient struct{}

// NewCasdoorUserClient returns a CasdoorUserClient wired to the real
// casdoorsdk package functions. Use this in production constructors.
func NewCasdoorUserClient() CasdoorUserClient {
	return &defaultCasdoorUserClient{}
}

func (c *defaultCasdoorUserClient) GetUserByUserId(userID string) (*casdoorsdk.User, error) {
	return casdoorsdk.GetUserByUserId(userID)
}

func (c *defaultCasdoorUserClient) DeleteUser(user *casdoorsdk.User) (bool, error) {
	return casdoorsdk.DeleteUser(user)
}
