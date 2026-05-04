package impersonationRoutes

import (
	"soli/formations/src/auth/casdoor"
)

// CasdoorValidatorAdapter adapts a *casdoor.CasdoorUserValidator to the
// local UserValidator interface. It exists so that the controller package
// can keep its own DTO type (TargetUser) without forcing the casdoor
// package to import this package — that direction would create an
// import cycle (impersonationRoutes → src/auth → casdoor → impersonationRoutes).
type CasdoorValidatorAdapter struct {
	Inner *casdoor.CasdoorUserValidator
}

// NewCasdoorValidatorAdapter wraps a Casdoor validator so it satisfies
// the local UserValidator interface (which returns *TargetUser).
func NewCasdoorValidatorAdapter(inner *casdoor.CasdoorUserValidator) *CasdoorValidatorAdapter {
	return &CasdoorValidatorAdapter{Inner: inner}
}

// UserExists delegates straight to the underlying Casdoor validator.
func (a *CasdoorValidatorAdapter) UserExists(userID string) (bool, error) {
	return a.Inner.UserExists(userID)
}

// GetUser delegates to the Casdoor validator and converts its
// casdoor.UserProfile into the controller's TargetUser DTO.
func (a *CasdoorValidatorAdapter) GetUser(userID string) (*TargetUser, error) {
	p, err := a.Inner.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	return &TargetUser{
		ID:          p.ID,
		Username:    p.Username,
		DisplayName: p.DisplayName,
		Email:       p.Email,
		Avatar:      p.Avatar,
	}, nil
}
