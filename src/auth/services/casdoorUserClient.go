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

	authModels "soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"soli/formations/src/utils"
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
	// AddUser creates the account. Reported through the interface so callers
	// that create accounts (the bulk import) can be tested on what they send.
	AddUser(user *casdoorsdk.User) error
	// SetPassword goes through Casdoor's dedicated call, the only one that
	// hashes; the password column of an update is stored raw.
	SetPassword(user *casdoorsdk.User, newPassword string) error
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
	// Every block and unblock (admin portal, organization offboarding) goes
	// through here, so this is the one place the mirror the auth middleware
	// reads can be kept in step with Casdoor.
	if err := authModels.SetAccountBlocked(sqldb.DB, userID, forbidden); err != nil {
		return fmt.Errorf("account %s is forbidden=%t in Casdoor but the local block was not recorded: %w", userID, forbidden, err)
	}
	if forbidden {
		// Best-effort: the mirror already refuses the account's JWTs; this
		// also kills the refresh tokens Casdoor holds for it.
		if err := revokeCasdoorTokens(user); err != nil {
			utils.Warn("could not revoke Casdoor tokens of blocked account %s: %v", userID, err)
		}
	}
	return nil
}

// revokeCasdoorTokens is swappable so tests never reach Casdoor's token API.
var revokeCasdoorTokens = deleteCasdoorTokensOf

// SwapCasdoorTokenRevoker replaces the token-revocation seam for a test and
// returns a function restoring it.
func SwapCasdoorTokenRevoker(revoke func(user *casdoorsdk.User) error) func() {
	prev := revokeCasdoorTokens
	revokeCasdoorTokens = revoke
	return func() { revokeCasdoorTokens = prev }
}

const casdoorTokenPageSize = 100

// deleteCasdoorTokensOf deletes every OAuth token Casdoor issued to user. All
// pages are listed before any deletion, so deleting cannot shift a page.
func deleteCasdoorTokensOf(user *casdoorsdk.User) error {
	var owned []*casdoorsdk.Token
	for page := 1; ; page++ {
		tokens, total, err := casdoorsdk.GetPaginationTokens(page, casdoorTokenPageSize,
			map[string]string{"field": "user", "value": user.Name})
		if err != nil {
			return fmt.Errorf("failed to list tokens: %w", err)
		}
		for _, token := range tokens {
			// The filter matches the user name alone; names are unique only
			// within an organization.
			if token.User == user.Name && token.Organization == user.Owner {
				owned = append(owned, token)
			}
		}
		if len(tokens) < casdoorTokenPageSize || page*casdoorTokenPageSize >= total {
			break
		}
	}
	for _, token := range owned {
		if _, err := casdoorsdk.DeleteToken(token); err != nil {
			return fmt.Errorf("failed to delete token %s: %w", token.Name, err)
		}
	}
	return nil
}

func (c *defaultCasdoorUserClient) AddUser(user *casdoorsdk.User) error {
	_, err := casdoorsdk.AddUser(user)
	return err
}

func (c *defaultCasdoorUserClient) SetPassword(user *casdoorsdk.User, newPassword string) error {
	ok, err := casdoorsdk.SetPassword(user.Owner, user.Name, "", newPassword)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("password update rejected by casdoor for user %s", user.Name)
	}
	return nil
}
