package filters

import (
	"gorm.io/gorm"
)

// OwnerArrayContainsKey is the sentinel filter key injected by the generic list
// handler for array-owner read-scoped entities (OwnershipConfig.ArrayOwner). The
// double-underscore prefix guarantees it can never collide with a real entity
// field name coming from a query parameter.
const OwnerArrayContainsKey = "__owner_ids_contains"

// OwnerArrayFilter scopes a list query to rows whose BaseModel.OwnerIDs array
// column (owner_ids) CONTAINS the calling user's ID. It is the array twin of the
// scalar owner-scope filter: the scalar path filters `owner_column = userID`,
// this one filters `owner_ids CONTAINS userID`.
type OwnerArrayFilter struct{}

// NewOwnerArrayFilter creates the array-owner read-scope filter strategy.
func NewOwnerArrayFilter() *OwnerArrayFilter {
	return &OwnerArrayFilter{}
}

// Priority matches the membership filter — this is a security scope, not a
// user-supplied field filter, so it runs ahead of the standard field strategies.
func (f *OwnerArrayFilter) Priority() int {
	return 5
}

// Matches handles only the sentinel key injected by the read-scope handler.
func (f *OwnerArrayFilter) Matches(key string, value any) bool {
	return key == OwnerArrayContainsKey
}

// Apply restricts the query to rows whose owner_ids array contains userID.
//
// Postgres uses the array-overlap operator. SQLite (and any non-postgres
// dialect) stores pq.StringArray as the literal `{"<uuid>"}`, so a LIKE on the
// double-quoted element reproduces the containment test. UUIDs contain none of
// the LIKE metacharacters (`%`, `_`) nor a `"`, so no escaping is required and
// no injection is possible through userID here.
func (f *OwnerArrayFilter) Apply(
	query *gorm.DB,
	key string,
	value any,
	tableName string,
) *gorm.DB {
	userID, ok := value.(string)
	if !ok || userID == "" {
		// Fail closed: an unknown or malformed actor sees zero rows rather than
		// the global unscoped set.
		return query.Where("1 = 0")
	}
	const col = "owner_ids" // BaseModel.OwnerIDs array column
	switch query.Dialector.Name() {
	case "postgres":
		return query.Where(col+" && ARRAY[?]", userID)
	default:
		return query.Where(col+" LIKE ?", `%"`+userID+`"%`) // matches {"<uuid>"} on sqlite
	}
}
