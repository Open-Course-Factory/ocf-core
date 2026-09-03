// src/auth/services/casdoorUserClient.go
//
// CasdoorUserClient is a thin seam over the package-level casdoorsdk functions
// that userService uses during DeleteUser, the import service uses to update
// existing accounts, and the offboarding service uses to forbid/unforbid an
// account. Introducing this interface lets the orchestration be unit-tested
// without standing up a real Casdoor instance (see tests/auth/userDeletion_test.go
// and tests/organizations/memberOffboarding_test.go).
package services

import (
	"errors"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// ErrCasdoorWriteNotPersisted means Casdoor answered the update with
// affected=false and no error: nothing changed on the identity side, so the
// caller must not proceed as if it had.
var ErrCasdoorWriteNotPersisted = errors.New("identity provider did not persist the update")

// CasdoorUserClient wraps the Casdoor SDK calls needed to look up, update,
// forbid and delete a user. Production code uses defaultCasdoorUserClient which
// forwards to the casdoorsdk package functions of the same names.
type CasdoorUserClient interface {
	GetUserByUserId(userID string) (*casdoorsdk.User, error)
	GetUserByEmail(email string) (*casdoorsdk.User, error)
	DeleteUser(user *casdoorsdk.User) (bool, error)
	// UpdateUserForColumns writes only the named DB columns. Always prefer it
	// to a bare UpdateUser, whose default whitelist silently drops columns such
	// as email_verified and is_forbidden.
	UpdateUserForColumns(user *casdoorsdk.User, columns []string) (bool, error)
	// SetForbidden blocks (or unblocks) sign-in for the account. It fails when
	// Casdoor reports the change was not persisted.
	SetForbidden(userID string, forbidden bool) error
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
	return readCasdoorUser(userID)
}

func (c *defaultCasdoorUserClient) GetUserByEmail(email string) (*casdoorsdk.User, error) {
	return casdoorsdk.GetUserByEmail(email)
}

func (c *defaultCasdoorUserClient) DeleteUser(user *casdoorsdk.User) (bool, error) {
	return casdoorsdk.DeleteUser(user)
}

func (c *defaultCasdoorUserClient) UpdateUserForColumns(user *casdoorsdk.User, columns []string) (bool, error) {
	return writeCasdoorUserColumns(user, columns)
}

// SetForbidden writes the is_forbidden column alone. The column name is the
// Casdoor DB column (xorm snake_case of User.IsForbidden), verified against
// casdoor-go-sdk v1.44.0 / Casdoor v2.138.0.
func (c *defaultCasdoorUserClient) SetForbidden(userID string, forbidden bool) error {
	user, err := c.GetUserByUserId(userID)
	if err != nil {
		return fmt.Errorf("failed to load account %s: %w", userID, err)
	}
	if user == nil {
		return fmt.Errorf("%w: %s", ErrUserNotFound, userID)
	}
	user.IsForbidden = forbidden
	affected, err := c.UpdateUserForColumns(user, []string{"is_forbidden"})
	if err != nil {
		return fmt.Errorf("failed to update account %s: %w", userID, err)
	}
	if !affected {
		return fmt.Errorf("%w (is_forbidden=%t for %s)", ErrCasdoorWriteNotPersisted, forbidden, userID)
	}
	return nil
}
