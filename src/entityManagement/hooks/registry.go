// src/entityManagement/hooks/registry.go
package hooks

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

type hookRegistry struct {
	hooks         map[string]Hook   // hookName -> Hook
	enabled       map[string]bool   // hookName -> enabled
	byEntity      map[string][]Hook // entityName -> []Hook
	mutex         sync.RWMutex
	testMode      bool              // Test mode disables async execution
	globalDisable bool              // Globally disable all hooks (for tests)
	errors        []HookError       // Recent async hook errors (circular buffer)
	errorCallback HookErrorCallback // Optional callback for async errors
	maxErrors     int               // Maximum errors to keep (default 100)
}

func NewHookRegistry() HookRegistry {
	return &hookRegistry{
		hooks:     make(map[string]Hook),
		enabled:   make(map[string]bool),
		byEntity:  make(map[string][]Hook),
		errors:    make([]HookError, 0, 100),
		maxErrors: 100,
	}
}

func (hr *hookRegistry) RegisterHook(hook Hook) error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hookName := hook.GetName()
	entityName := hook.GetEntityName()

	// Vérifier que le hook n'existe pas déjà
	if _, exists := hr.hooks[hookName]; exists {
		return fmt.Errorf("hook '%s' already registered", hookName)
	}

	// Enregistrer le hook
	hr.hooks[hookName] = hook
	hr.enabled[hookName] = hook.IsEnabled()

	// Ajouter à la liste par entité
	if hr.byEntity[entityName] == nil {
		hr.byEntity[entityName] = make([]Hook, 0)
	}
	hr.byEntity[entityName] = append(hr.byEntity[entityName], hook)

	// Trier par priorité
	sort.Slice(hr.byEntity[entityName], func(i, j int) bool {
		return hr.byEntity[entityName][i].GetPriority() < hr.byEntity[entityName][j].GetPriority()
	})

	log.Printf("🔗 Hook '%s' registered for entity '%s'", hookName, entityName)
	return nil
}

func (hr *hookRegistry) UnregisterHook(hookName string) error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hook, exists := hr.hooks[hookName]
	if !exists {
		return fmt.Errorf("hook '%s' not found", hookName)
	}

	entityName := hook.GetEntityName()

	// Supprimer de la map principale
	delete(hr.hooks, hookName)
	delete(hr.enabled, hookName)

	// Supprimer de la liste par entité
	if entityHooks, exists := hr.byEntity[entityName]; exists {
		for i, h := range entityHooks {
			if h.GetName() == hookName {
				hr.byEntity[entityName] = append(entityHooks[:i], entityHooks[i+1:]...)
				break
			}
		}
	}

	log.Printf("🔗 Hook '%s' unregistered", hookName)
	return nil
}

func (hr *hookRegistry) ExecuteHooks(ctx *HookContext) error {
	hr.mutex.RLock()
	disabled := hr.globalDisable
	hr.mutex.RUnlock()

	// Skip all hook execution if globally disabled (test mode)
	if disabled {
		return nil
	}

	hr.mutex.RLock()
	hooks := hr.GetHooks(ctx.EntityName, ctx.HookType)

	// Create a copy of enabled status to avoid holding lock during execution
	enabledStatus := make(map[string]bool)
	for _, hook := range hooks {
		enabledStatus[hook.GetName()] = hr.enabled[hook.GetName()]
	}
	hr.mutex.RUnlock()

	for _, hook := range hooks {
		if !enabledStatus[hook.GetName()] {
			continue
		}

		// Vérifier les conditions si c'est un ConditionalHook
		if conditionalHook, ok := hook.(ConditionalHook); ok {
			if !conditionalHook.ShouldExecute(ctx) {
				continue
			}
		}

		// Exécuter le hook
		if err := hook.Execute(ctx); err != nil {
			log.Printf("❌ Hook '%s' failed: %v", hook.GetName(), err)

			// Record error for async hooks (After* types)
			if ctx.HookType == AfterCreate || ctx.HookType == AfterUpdate || ctx.HookType == AfterDelete {
				hr.recordError(hook.GetName(), ctx.EntityName, ctx.HookType, ctx.EntityID, err)
			}

			// Selon la stratégie d'erreur, on peut continuer ou arrêter
			// Pour l'instant, on continue avec les autres hooks
			continue
		}

		log.Printf("✅ Hook '%s' executed successfully", hook.GetName())
	}

	return nil
}

func (hr *hookRegistry) GetHooks(entityName string, hookType HookType) []Hook {
	entityHooks, exists := hr.byEntity[entityName]
	if !exists {
		return nil
	}

	var result []Hook
	for _, hook := range entityHooks {
		for _, supportedType := range hook.GetHookTypes() {
			if supportedType == hookType {
				result = append(result, hook)
				break
			}
		}
	}

	return result
}

func (hr *hookRegistry) EnableHook(hookName string, enabled bool) error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if _, exists := hr.hooks[hookName]; !exists {
		return fmt.Errorf("hook '%s' not found", hookName)
	}

	hr.enabled[hookName] = enabled
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	log.Printf("🔗 Hook '%s' %s", hookName, status)

	return nil
}

// ClearAllHooks removes all registered hooks (useful for testing)
func (hr *hookRegistry) ClearAllHooks() {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.hooks = make(map[string]Hook)
	hr.enabled = make(map[string]bool)
	hr.byEntity = make(map[string][]Hook)
	log.Println("🔗 All hooks cleared")
}

// SetTestMode enables or disables test mode (synchronous execution)
func (hr *hookRegistry) SetTestMode(enabled bool) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.testMode = enabled
	if enabled {
		log.Println("🔗 Hook test mode enabled (synchronous execution)")
	} else {
		log.Println("🔗 Hook test mode disabled (async execution)")
	}
}

// DisableAllHooks globally disables all hook execution (for tests)
func (hr *hookRegistry) DisableAllHooks(disabled bool) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.globalDisable = disabled
	if disabled {
		log.Println("🔗 All hooks globally disabled")
	} else {
		log.Println("🔗 All hooks globally enabled")
	}
}

// IsTestMode returns whether test mode is enabled
func (hr *hookRegistry) IsTestMode() bool {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()
	return hr.testMode
}

// GetRecentErrors returns recent async hook errors
func (hr *hookRegistry) GetRecentErrors(maxErrors int) []HookError {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()

	if maxErrors <= 0 || maxErrors > len(hr.errors) {
		// Return all errors
		result := make([]HookError, len(hr.errors))
		copy(result, hr.errors)
		return result
	}

	// Return last N errors
	start := len(hr.errors) - maxErrors
	result := make([]HookError, maxErrors)
	copy(result, hr.errors[start:])
	return result
}

// ClearErrors clears the error history
func (hr *hookRegistry) ClearErrors() {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.errors = make([]HookError, 0, hr.maxErrors)
	log.Println("🔗 Hook errors cleared")
}

// SetErrorCallback sets a callback to be invoked when async hooks fail
func (hr *hookRegistry) SetErrorCallback(callback HookErrorCallback) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.errorCallback = callback
	if callback != nil {
		log.Println("🔗 Hook error callback registered")
	} else {
		log.Println("🔗 Hook error callback removed")
	}
}

// recordError records an async hook error (internal method)
func (hr *hookRegistry) recordError(hookName, entityName string, hookType HookType, entityID any, err error) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hookError := HookError{
		HookName:   hookName,
		EntityName: entityName,
		HookType:   hookType,
		Error:      err.Error(),
		Timestamp:  time.Now().Unix(),
		EntityID:   entityID,
	}

	// Circular buffer: if at max capacity, remove oldest
	if len(hr.errors) >= hr.maxErrors {
		hr.errors = hr.errors[1:]
	}
	hr.errors = append(hr.errors, hookError)

	// Call error callback if set (outside mutex to avoid deadlock)
	callback := hr.errorCallback
	if callback != nil {
		go callback(&hookError)
	}
}

// Instance globale du registre de hooks
var GlobalHookRegistry = NewHookRegistry()
