// src/entityManagement/hooks/registry.go
package hooks

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

type hookRegistry struct {
	hooks    map[string]Hook   // hookName -> Hook
	enabled  map[string]bool   // hookName -> enabled
	byEntity map[string][]Hook // entityName -> []Hook
	mutex    sync.RWMutex
}

func NewHookRegistry() HookRegistry {
	return &hookRegistry{
		hooks:    make(map[string]Hook),
		enabled:  make(map[string]bool),
		byEntity: make(map[string][]Hook),
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
	defer hr.mutex.RUnlock()

	hooks := hr.GetHooks(ctx.EntityName, ctx.HookType)

	for _, hook := range hooks {
		if !hr.enabled[hook.GetName()] {
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

// Instance globale du registre de hooks
var GlobalHookRegistry = NewHookRegistry()
