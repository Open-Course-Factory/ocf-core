package models

import (
	"errors"

	"gorm.io/gorm"
)

// ErrScenarioArchived is the single refusal reason shared by every path that
// would start a new run of a retired scenario.
var ErrScenarioArchived = errors.New("this scenario has been archived and can no longer be launched")

// NotArchived restricts a query to the scenarios that are still in use.
//
// Archiving retires a scenario whose results must stay readable: the row
// survives, so past sessions, grades and flags keep resolving their title and
// their scenario link, but the scenario is no longer offered, assignable or
// launchable. Sessions already running when the scenario is archived are left
// alone and run to completion.
//
// Every visibility and launch gate goes through this one scope, so the rule
// cannot drift between the learner catalogue, the assignment picker and the
// launch path.
func NotArchived(db *gorm.DB) *gorm.DB {
	return db.Where("scenarios.archived_at IS NULL")
}
