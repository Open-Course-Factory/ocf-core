package hooks

import (
	"fmt"
	"reflect"
	"strings"

	casbinUtils "soli/formations/src/auth/casbin"

	"gorm.io/gorm"
)

// ownershipHook is a generic Hook that enforces entity ownership checks
// using reflection, driven by OwnershipConfig.
type ownershipHook struct {
	db         *gorm.DB
	entityName string
	config     casbinUtils.OwnershipConfig
	hookTypes  []HookType
}

// NewOwnershipHook creates a generic ownership hook for any entity.
// The hook uses reflection to get/set the ownership field specified in config.
func NewOwnershipHook(db *gorm.DB, entityName string, config casbinUtils.OwnershipConfig) Hook {
	hookTypes := operationsToHookTypes(config.Operations)

	return &ownershipHook{
		db:         db,
		entityName: entityName,
		config:     config,
		hookTypes:  hookTypes,
	}
}

func (h *ownershipHook) GetName() string {
	return h.entityName + "OwnershipHook"
}

func (h *ownershipHook) GetEntityName() string {
	return h.entityName
}

func (h *ownershipHook) GetHookTypes() []HookType {
	return h.hookTypes
}

func (h *ownershipHook) IsEnabled() bool {
	return true
}

func (h *ownershipHook) GetPriority() int {
	return 10
}

func (h *ownershipHook) Execute(ctx *HookContext) error {
	switch ctx.HookType {
	case BeforeCreate:
		return h.handleBeforeCreate(ctx)
	case BeforeUpdate:
		return h.handleBeforeUpdate(ctx)
	case BeforeDelete:
		return h.handleBeforeDelete(ctx)
	}
	return nil
}

func (h *ownershipHook) handleBeforeCreate(ctx *HookContext) error {
	if ctx.NewEntity == nil {
		return fmt.Errorf("NewEntity is nil for %s ownership hook", h.entityName)
	}

	// Admin bypass: admin can create for another user
	if h.config.AdminBypass && ctx.IsAdmin() {
		return nil
	}

	// Force the ownership field to the authenticated user's ID
	v := reflect.ValueOf(ctx.NewEntity).Elem()
	field := v.FieldByName(h.config.OwnerField)
	if !field.IsValid() || !field.CanSet() {
		return nil
	}
	if field.Kind() != reflect.String {
		return fmt.Errorf("ownership field %s on %s is not a string type", h.config.OwnerField, h.entityName)
	}
	field.SetString(ctx.UserID)

	return nil
}

func (h *ownershipHook) handleBeforeUpdate(ctx *HookContext) error {
	return h.verifyOwnership(ctx)
}

func (h *ownershipHook) handleBeforeDelete(ctx *HookContext) error {
	return h.verifyOwnership(ctx)
}

// verifyOwnership checks that the authenticated user owns the entity.
// Used by both handleBeforeUpdate and handleBeforeDelete.
func (h *ownershipHook) verifyOwnership(ctx *HookContext) error {
	if h.config.AdminBypass && ctx.IsAdmin() {
		return nil
	}
	ownerValue, err := h.loadOwnerFromDB(ctx.EntityID)
	if err != nil {
		return err
	}
	if ownerValue != ctx.UserID {
		return fmt.Errorf("permission denied: you do not own this %s", h.entityName)
	}
	return nil
}

// loadOwnerFromDB loads the ownership field value from the database for the given entity ID.
func (h *ownershipHook) loadOwnerFromDB(entityID any) (string, error) {
	if entityID == nil || entityID == "" {
		return "", fmt.Errorf("entity ID is empty for %s ownership check", h.entityName)
	}

	tableName := h.db.Config.NamingStrategy.TableName(h.entityName)
	column := h.db.Config.NamingStrategy.ColumnName("", h.config.OwnerField)

	var ownerValue string
	result := h.db.Table(tableName).
		Where("id = ?", entityID).
		Select(column).
		Scan(&ownerValue)

	if result.Error != nil {
		return "", fmt.Errorf("failed to load %s for ownership check: %w", h.entityName, result.Error)
	}
	if result.RowsAffected == 0 {
		return "", fmt.Errorf("%s not found (id=%v)", h.entityName, entityID)
	}

	return ownerValue, nil
}

// operationsToHookTypes converts operation strings to HookType values.
func operationsToHookTypes(operations []string) []HookType {
	var types []HookType
	for _, op := range operations {
		switch strings.ToLower(op) {
		case "create":
			types = append(types, BeforeCreate)
		case "update":
			types = append(types, BeforeUpdate)
		case "delete":
			types = append(types, BeforeDelete)
		}
	}
	return types
}
