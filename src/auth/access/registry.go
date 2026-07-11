package access

import (
	"sort"
	"sync"

	"soli/formations/src/auth/interfaces"
)

// RouteRegistry collects route permission declarations from all modules.
// Each module calls Register() during its permission setup.
var RouteRegistry = &routeRegistry{
	routes:  make(map[string][]RoutePermission),
	byRoute: make(map[string]RoutePermission),
}

type routeRegistry struct {
	mu       sync.RWMutex
	routes   map[string][]RoutePermission // category -> routes
	byRoute  map[string]RoutePermission   // "METHOD:path" -> RoutePermission
	entities []EntityCRUDPermissions       // registered entity CRUD permissions
}

// Register adds route permissions to the registry under the given category.
func (r *routeRegistry) Register(category string, routes ...RoutePermission) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range routes {
		routes[i].Category = category
		key := routes[i].Method + ":" + routes[i].Path
		r.byRoute[key] = routes[i]
	}
	r.routes[category] = append(r.routes[category], routes...)
}

// RegisterEntity adds an entity's CRUD permission declarations to the registry.
func (r *routeRegistry) RegisterEntity(config EntityCRUDPermissions) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entities = append(r.entities, config)
}

// GetReference returns the full permission reference, sorted by category.
func (r *routeRegistry) GetReference() PermissionReference {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Sort categories alphabetically
	categoryNames := make([]string, 0, len(r.routes))
	for name := range r.routes {
		categoryNames = append(categoryNames, name)
	}
	sort.Strings(categoryNames)

	categories := make([]PermissionCategory, 0, len(categoryNames))
	for _, name := range categoryNames {
		categories = append(categories, PermissionCategory{
			Name:   name,
			Routes: r.routes[name],
		})
	}

	entities := make([]EntityCRUDPermissions, len(r.entities))
	copy(entities, r.entities)

	return PermissionReference{
		Categories: categories,
		Entities:   entities,
	}
}

// Reset clears the registry (for testing).
func (r *routeRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = make(map[string][]RoutePermission)
	r.byRoute = make(map[string]RoutePermission)
	r.entities = nil
}

// RegisterEnforced declares routes ONCE: it registers each RoutePermission in
// the RouteRegistry (Layer 2) and derives its Layer 1 Casbin policy via
// ReconcilePolicy(role, path, method). This is the single source of truth for a
// route's authorization — the previous pattern hand-maintained a parallel Casbin
// list that drifted (see the incus-ui path bug). Optional per-route escape hatches:
//   - CasbinPath overrides the Layer 1 policy path (keyMatch2 wants /* where the
//     registry exact-match wants /*path).
//   - NoGateway declares the route in the registry but registers NO Casbin policy
//     (route mounted without AuthManagement).
func RegisterEnforced(enforcer interfaces.EnforcerInterface, category string, perms ...RoutePermission) {
	RouteRegistry.Register(category, perms...)
	for _, p := range perms {
		if p.NoGateway {
			continue
		}
		path := p.Path
		if p.CasbinPath != "" {
			path = p.CasbinPath
		}
		ReconcilePolicy(enforcer, p.Role, path, p.Method)
	}
}

// Lookup returns the RoutePermission for a given HTTP method and path.
// The second return value is false if no matching route is registered.
func (r *routeRegistry) Lookup(method, path string) (RoutePermission, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := method + ":" + path
	perm, found := r.byRoute[key]
	return perm, found
}
