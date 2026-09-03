package models

import (
	"errors"

	entityManagementModels "soli/formations/src/entityManagement/models"
)

// ErrScenarioArchived is the one learner- and trainer-facing wording for a
// refused launch, bulk start or assignment of an archived scenario. The
// generic archive capability has no product wording of its own, so every
// scenario path that surfaces the refusal must use this error.
var ErrScenarioArchived = errors.New("this scenario has been archived and can no longer be launched")

// NotArchived restricts a hand-written scenario query to the rows still in
// use. It is the framework's scope bound to this table, so it emits the same
// predicate the generic list applies by default.
var NotArchived = entityManagementModels.NotArchived("scenarios")
