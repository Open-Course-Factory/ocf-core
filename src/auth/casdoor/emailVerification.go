package casdoor

import (
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// MarkEmailVerified is the one definition of "this address is verified" on a
// Casdoor account: the native flag the login flow reads, plus the timestamp
// kept in Properties because Casdoor has no field for it. The verification
// flow and the organization import both go through here, so an account
// vouched for by its organization is indistinguishable from one that clicked
// the link.
func MarkEmailVerified(user *casdoorsdk.User, at time.Time) {
	user.EmailVerified = true
	if user.Properties == nil {
		user.Properties = make(map[string]string)
	}
	user.Properties["email_verified_at"] = at.Format(time.RFC3339)
}

// EmailVerifiedColumns names the columns MarkEmailVerified touches, for
// UpdateUserForColumns. Casdoor's default whitelist carries `properties` but
// not `email_verified`: written without this list the flag silently never
// lands, which is how production ended up full of accounts carrying an
// email_verified_at stamp next to a false flag.
func EmailVerifiedColumns() []string {
	return []string{"email_verified", "properties"}
}
