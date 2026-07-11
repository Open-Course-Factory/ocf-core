package entityManagement_tests

// Tests for M5 (deny-when-actor-unknown), ownership-hook spot.
//
// ownershipHook.verifyOwnership compares the persisted owner value against
// ctx.UserID with `if ownerValue != ctx.UserID`. When BOTH are the empty string
// (an entity whose owner column is "" AND an unauthenticated / unknown actor),
// the two match, so the update/delete is ALLOWED — a fail-open. The fix must deny
// whenever ctx.UserID == "".
//
// The RED tests below drive the hook directly against a throwaway ownership-hooked
// entity whose owner field is empty, with an empty actor, and assert the
// user-observable outcome (the returned permission error). They are RED today
// (empty == empty -> nil returned -> operation allowed).
//
// The GREEN guard asserts the fix does NOT over-restrict: a real, non-empty owner
// acting on their own entity must STILL be allowed after the fix.

import (
	"testing"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ownedThing is a throwaway entity with a string owner field. Its GORM table name
// ("owned_things") lines up with what the ownership hook derives from the entity
// name "OwnedThing" via the DB naming strategy, so loadOwnerFromDB can read the
// owner column back during verification.
type ownedThing struct {
	ID     string `gorm:"primaryKey"`
	UserID string
}

// newOwnedThingDB creates a fresh in-memory SQLite DB with the ownedThing table.
func newOwnedThingDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory SQLite DB")

	require.NoError(t, db.AutoMigrate(&ownedThing{}), "failed to migrate ownedThing")

	return db
}

// newOwnedThingHook builds an ownership hook for OwnedThing guarding update+delete,
// with admin bypass enabled (mirrors the real registration shape).
func newOwnedThingHook(db *gorm.DB) hooks.Hook {
	return hooks.NewOwnershipHook(db, "OwnedThing", access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"update", "delete"},
		AdminBypass: true,
	})
}

// TestOwnershipHook_EmptyActorUpdate_Denied — an unknown actor (ctx.UserID == "")
// updating an entity whose owner column is also "" must be DENIED. RED today:
// "" != "" is false, so verifyOwnership returns nil and the update is allowed.
func TestOwnershipHook_EmptyActorUpdate_Denied(t *testing.T) {
	db := newOwnedThingDB(t)
	require.NoError(t, db.Create(&ownedThing{ID: "thing-empty-owner", UserID: ""}).Error)

	hook := newOwnedThingHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OwnedThing",
		HookType:   hooks.BeforeUpdate,
		EntityID:   "thing-empty-owner",
		UserID:     "", // unknown actor
	}

	err := hook.Execute(ctx)
	require.Error(t, err,
		"an unknown actor (empty UserID) updating an entity with an empty owner must be denied; "+
			"the hook must not treat empty==empty as ownership")
}

// TestOwnershipHook_EmptyActorDelete_Denied — same fail-open on the delete path.
// RED today for the same reason.
func TestOwnershipHook_EmptyActorDelete_Denied(t *testing.T) {
	db := newOwnedThingDB(t)
	require.NoError(t, db.Create(&ownedThing{ID: "thing-empty-owner", UserID: ""}).Error)

	hook := newOwnedThingHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OwnedThing",
		HookType:   hooks.BeforeDelete,
		EntityID:   "thing-empty-owner",
		UserID:     "", // unknown actor
	}

	err := hook.Execute(ctx)
	require.Error(t, err,
		"an unknown actor (empty UserID) deleting an entity with an empty owner must be denied")
}

// TestOwnershipHook_NonEmptyOwnerActsOnOwn_Allowed — GREEN guard against
// over-restriction: a real, authenticated owner acting on their OWN entity must
// still be allowed after the deny-on-empty fix. Green today, must STAY green.
func TestOwnershipHook_NonEmptyOwnerActsOnOwn_Allowed(t *testing.T) {
	db := newOwnedThingDB(t)
	require.NoError(t, db.Create(&ownedThing{ID: "thing-owned", UserID: "real-user-123"}).Error)

	hook := newOwnedThingHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OwnedThing",
		HookType:   hooks.BeforeUpdate,
		EntityID:   "thing-owned",
		UserID:     "real-user-123", // the genuine owner
	}

	err := hook.Execute(ctx)
	require.NoError(t, err,
		"the genuine owner acting on their own entity must remain allowed; "+
			"the deny-on-empty fix must not block non-empty matching owners")
}
