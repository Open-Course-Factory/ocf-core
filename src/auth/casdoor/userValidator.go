package casdoor

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorUserValidator implements impersonationRoutes.UserValidator by
// querying Casdoor for the existence (and profile) of a user.
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

// GetUser returns a minimal profile for the given user id, or nil if the
// user does not exist. A transport / API error is propagated.
//
// The return type is the controller's own TargetUser DTO. To avoid an
// import cycle (impersonationRoutes → src/auth → src/auth/casdoor →
// impersonationRoutes), this method does NOT import the impersonationRoutes
// package directly — it returns its own value type and the controller
// converts it via UserProfile.ToTargetUser at the call site. The caller in
// impersonationRoutes adapts CasdoorUserValidator into the local interface.
func (v *CasdoorUserValidator) GetUser(userID string) (*UserProfile, error) {
	u, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	return &UserProfile{
		ID:          u.Id,
		Username:    u.Name,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Avatar:      u.Avatar,
	}, nil
}

// UserProfile is the casdoor-side view of a user, returned by GetUser.
// It is intentionally decoupled from any HTTP DTO — the impersonation
// controller wraps this validator with an adapter that converts UserProfile
// into its own TargetUser type.
type UserProfile struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
	Avatar      string
}
