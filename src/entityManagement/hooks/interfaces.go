// src/entityManagement/hooks/interfaces.go
package hooks

import (
	"context"
)

// HookType définit les différents moments où les hooks peuvent être exécutés
type HookType string

const (
	BeforeCreate HookType = "before_create"
	AfterCreate  HookType = "after_create"
	BeforeUpdate HookType = "before_update"
	AfterUpdate  HookType = "after_update"
	BeforeDelete HookType = "before_delete"
	AfterDelete  HookType = "after_delete"
)

// HookContext contient toutes les informations disponibles pour un hook
type HookContext struct {
	EntityName string                 `json:"entity_name"`
	HookType   HookType               `json:"hook_type"`
	EntityID   interface{}            `json:"entity_id,omitempty"`
	OldEntity  interface{}            `json:"old_entity,omitempty"` // Pour les updates
	NewEntity  interface{}            `json:"new_entity"`           // L'entité actuelle
	UserID     string                 `json:"user_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Context    context.Context        `json:"-"`
}

// Hook interface que tous les hooks doivent implémenter
type Hook interface {
	// GetName retourne un nom unique pour le hook
	GetName() string

	// GetEntityName retourne le nom de l'entité concernée
	GetEntityName() string

	// GetHookTypes retourne les types de hooks supportés
	GetHookTypes() []HookType

	// Execute exécute la logique du hook
	Execute(ctx *HookContext) error

	// IsEnabled vérifie si le hook est activé
	IsEnabled() bool

	// GetPriority retourne la priorité d'exécution (plus bas = plus prioritaire)
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
