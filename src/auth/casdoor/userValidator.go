package casdoor

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorUserValidator implements impersonationRoutes.UserValidator by
// querying Casdoor for the existence of a user.
type CasdoorUserValidator struct{}

// NewCasdoorUserValidator returns a Casdoor-backed user validator.
func NewCasdoorUserValidator() *CasdoorUserValidator {
	return &CasdoorUserValidator{}
}

// UserExists reports whether the given user id is known to Casdoor. A
// transport / API error is propagated so the caller can distinguish "the
// user does not exist" (returns false, nil) from "we could not find out"
// (returns false, err).
func (v *CasdoorUserValidator) UserExists(userID string) (bool, error) {
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return false, err
	}
	return user != nil, nil
}
