package controller

import (
	"reflect"
	"strings"

	access "soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
)

// ownerReadScope returns the OwnershipConfig to enforce on this read request, or
// nil when no owner scoping applies — either the entity declares no read-scope
// OwnershipConfig, or the caller is an admin and the config permits admin bypass.
//
// Both generic read handlers (GetEntities and GetEntity) route their scoping
// decision through this single predicate so the "read scope + admin bypass" rule
// lives in one place.
func ownerReadScope(ctx *gin.Context, entityName string) *access.OwnershipConfig {
	config := ems.GlobalEntityRegistrationService.GetOwnershipConfig(entityName)
	if config == nil || !ownershipConfigHasReadOp(config) {
		return nil
	}
	if config.AdminBypass {
		if rolesVal, exists := ctx.Get("userRoles"); exists {
			if roles, ok := rolesVal.([]string); ok && access.IsAdmin(roles) {
				return nil
			}
		}
	}
	return config
}

// ownershipConfigHasReadOp reports whether the config scopes the read operation.
func ownershipConfigHasReadOp(config *access.OwnershipConfig) bool {
	for _, op := range config.Operations {
		if strings.EqualFold(op, "read") {
			return true
		}
	}
	return false
}

// ownerMatchesCaller reports whether the loaded entity's owner field equals the
// calling user's ID. A missing or non-string owner field is treated as a
// mismatch so a misconfigured entity fails closed (deny) rather than leaking.
func ownerMatchesCaller(entity any, ownerField, userID string) bool {
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
	field := v.FieldByName(ownerField)
	if !field.IsValid() || field.Kind() != reflect.String {
		return false
	}
	return field.String() == userID
}
