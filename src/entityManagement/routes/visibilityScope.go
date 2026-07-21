package controller

import (
	"reflect"

	access "soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
)

// visibilityScope returns the VisibilityScopeConfig to enforce on this read
// request, or nil when none applies — either the entity declares no scope, or the
// caller is an admin (admins see every row).
//
// Both generic read handlers (GetEntities and GetEntity) route their
// visibility decision through this single predicate so the "hide flagged-off rows
// from non-admins" rule lives in one place. Unlike ownerReadScope this does NOT
// key on the caller's identity: a missing userId (unauthenticated caller) still
// sees the visible rows, so a boolean-flagged entity can serve a public catalog.
func visibilityScope(ctx *gin.Context, entityName string) *access.VisibilityScopeConfig {
	config := ems.GlobalEntityRegistrationService.GetVisibilityScope(entityName)
	if config == nil {
		return nil
	}
	if rolesVal, exists := ctx.Get("userRoles"); exists {
		if roles, ok := rolesVal.([]string); ok && access.IsAdmin(roles) {
			return nil
		}
	}
	return config
}

// entityBoolFieldIsTrue reports whether the loaded entity's named bool field is
// true. A missing or non-bool field is treated as false so a misconfigured entity
// fails closed (hidden) rather than leaking — same fail-closed contract as
// ownerMatchesCaller.
func entityBoolFieldIsTrue(entity any, field string) bool {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName(field)
	if !f.IsValid() || f.Kind() != reflect.Bool {
		return false
	}
	return f.Bool()
}
