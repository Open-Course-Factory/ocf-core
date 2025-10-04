// Package hooks provides an event system for entity lifecycle management.
//
// Hooks allow you to execute custom logic before or after entity operations
// (create, update, delete). They support priority-based execution, conditional
// execution, and can be enabled/disabled at runtime.
//
// Example usage:
//
//	hook := hooks.NewFunctionHook(
//	    "ValidateEmail",
//	    "User",
//	    hooks.BeforeCreate,
//	    func(ctx *hooks.HookContext) error {
//	        user := ctx.NewEntity.(*User)
//	        if !isValidEmail(user.Email) {
//	            return fmt.Errorf("invalid email")
//	        }
//	        return nil
//	    },
//	)
//	hooks.GlobalHookRegistry.RegisterHook(hook)
package hooks

import (
	"context"
)

// HookType defines the different moments when hooks can be executed during entity lifecycle.
type HookType string

const (
	BeforeCreate HookType = "before_create" // Executed before entity creation (synchronous)
	AfterCreate  HookType = "after_create"  // Executed after entity creation (async in production)
	BeforeUpdate HookType = "before_update" // Executed before entity update (synchronous)
	AfterUpdate  HookType = "after_update"  // Executed after entity update (async in production)
	BeforeDelete HookType = "before_delete" // Executed before entity deletion (synchronous)
	AfterDelete  HookType = "after_delete"  // Executed after entity deletion (async in production)
)

// HookContext contains all information available to a hook during execution.
//
// The context provides access to:
//   - Entity metadata (name, ID)
//   - Entity state (old and new values for updates)
//   - User context (who triggered the operation)
//   - Custom metadata for passing data between hooks
type HookContext struct {
	EntityName string                 `json:"entity_name"`          // Name of the entity being operated on
	HookType   HookType               `json:"hook_type"`            // Type of lifecycle event
	EntityID   interface{}            `json:"entity_id,omitempty"`  // ID of the entity (if available)
	OldEntity  interface{}            `json:"old_entity,omitempty"` // Previous state (for updates)
	NewEntity  interface{}            `json:"new_entity"`           // Current state
	UserID     string                 `json:"user_id,omitempty"`    // ID of user who triggered the operation
	Metadata   map[string]interface{} `json:"metadata,omitempty"`   // Custom metadata shared between hooks
	Context    context.Context        `json:"-"`                    // Go context for cancellation
}

// Hook interface that all hooks must implement.
//
// Hooks are executed in priority order (lower priority number = executed first).
// They can be enabled/disabled at runtime and can be conditional based on entity state.
type Hook interface {
	// GetName returns a unique identifier for this hook
	GetName() string

	// GetEntityName returns the name of the entity this hook applies to
	GetEntityName() string

	// GetHookTypes returns the lifecycle events this hook handles
	GetHookTypes() []HookType

	// Execute runs the hook logic. Return an error to abort the operation (for Before* hooks)
	Execute(ctx *HookContext) error

	// IsEnabled checks if the hook is currently active
	IsEnabled() bool

	// GetPriority returns execution priority (lower = higher priority)
	GetPriority() int
}

// HookRegistry gère l'enregistrement et l'exécution des hooks
type HookRegistry interface {
	// RegisterHook enregistre un hook
	RegisterHook(hook Hook) error

	// UnregisterHook désenregistre un hook
	UnregisterHook(hookName string) error

	// ExecuteHooks exécute tous les hooks pour un type donné
	ExecuteHooks(ctx *HookContext) error

	// GetHooks retourne tous les hooks pour une entité et un type
	GetHooks(entityName string, hookType HookType) []Hook

	// EnableHook active/désactive un hook
	EnableHook(hookName string, enabled bool) error

	// ClearAllHooks removes all registered hooks (useful for testing)
	ClearAllHooks()

	// SetTestMode enables or disables test mode (synchronous execution)
	SetTestMode(enabled bool)

	// DisableAllHooks globally disables all hook execution (for tests)
	DisableAllHooks(disabled bool)

	// IsTestMode returns whether test mode is enabled
	IsTestMode() bool
}

// AsyncHook pour les hooks qui peuvent être exécutés en arrière-plan
type AsyncHook interface {
	Hook
	ExecuteAsync(ctx *HookContext) error
}

// ConditionalHook pour les hooks avec des conditions d'exécution
type ConditionalHook interface {
	Hook
	ShouldExecute(ctx *HookContext) bool
}
