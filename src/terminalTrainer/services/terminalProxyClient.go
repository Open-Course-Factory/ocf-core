package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"golang.org/x/sync/singleflight"
)

// terminalProxyClient owns the tt-backend HTTP layer: session start/stop/delete
// API calls, session/metrics/distribution/size/backend fetches, and the
// backend-list cache. It was carved out of terminalTrainerService to shrink
// that god object; terminalTrainerService now embeds a *terminalProxyClient and
// delegates the relevant interface methods to it.
type terminalProxyClient struct {
	adminKey     string
	baseURL      string
	apiVersion   string
	terminalType string
	repository   repositories.TerminalRepository

	backendCache     []dto.BackendInfo
	backendCacheTime time.Time
	backendCacheMu   sync.RWMutex
	backendCacheSF   singleflight.Group
}

// newTerminalProxyClient reads the same Terminal Trainer env vars as the
// service constructor and returns a ready proxy client.
func newTerminalProxyClient(repository repositories.TerminalRepository) *terminalProxyClient {
	apiVersion := os.Getenv("TERMINAL_TRAINER_API_VERSION")
	if apiVersion == "" {
		apiVersion = "1.0"
	}
	terminalType := os.Getenv("TERMINAL_TRAINER_TYPE")

	return &terminalProxyClient{
		adminKey:     os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
		baseURL:      os.Getenv("TERMINAL_TRAINER_URL"),
		apiVersion:   apiVersion,
		terminalType: terminalType,
		repository:   repository,
	}
}

// buildAPIPath construit le chemin API avec version et type d'instance optionnel
func (p *terminalProxyClient) buildAPIPath(endpoint string, instanceType string) string {
	path := fmt.Sprintf("/%s", p.apiVersion)

	// Utiliser le type d'instance fourni, sinon celui par défaut du service
	if instanceType != "" {
		path += fmt.Sprintf("/%s", instanceType)
	} else if p.terminalType != "" {
		path += fmt.Sprintf("/%s", p.terminalType)
	}

	path += endpoint
	return path
}

// stopSessionInAPI appelle POST /sessions/{id}/stop sur tt-backend.
// Retourne idle_until si tt-backend en propose un.
func (p *terminalProxyClient) stopSessionInAPI(sessionID, userAPIKey string) (*time.Time, error) {
	url := fmt.Sprintf("%s/%s/sessions/%s/stop", p.baseURL, p.apiVersion, sessionID)

	utils.Debug("stopSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	var resp struct {
		IdleUntil *time.Time `json:"idle_until,omitempty"`
	}
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, nil, &resp, opts); err != nil {
		return nil, err
	}
	return resp.IdleUntil, nil
}

// startSessionInAPI appelle POST /sessions/{id}/start sur tt-backend.
// Retourne le nouveau expires_at (unix seconds) tel que tt-backend l'a recalculé
// au redémarrage de l'instance. Retourne 0 si le champ est absent (instance
// sans expiry fini côté backend) — l'appelant doit alors conserver la valeur
// locale actuelle.
//
// expirySeconds, lorsqu'il est > 0, est envoyé dans le corps JSON comme
// {"expiry": N} pour que tt-backend recalcule instance_expiry sur la base
// de la limite du plan plutôt que sur la valeur par défaut de l'instance.
// Quand expirySeconds == 0, le corps est nil et tt-backend utilise son défaut.
func (p *terminalProxyClient) startSessionInAPI(sessionID, userAPIKey string, expirySeconds int) (int64, error) {
	url := fmt.Sprintf("%s/%s/sessions/%s/start", p.baseURL, p.apiVersion, sessionID)

	utils.Debug("startSessionInAPI - calling %s (expiry=%d)", url, expirySeconds)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	// Build body only when we have a positive expiry to forward. A nil body
	// preserves tt-backend's instance-config default for legacy sessions.
	var body any
	if expirySeconds > 0 {
		body = map[string]any{"expiry": expirySeconds}
	}

	var resp struct {
		ExpiresAt int64 `json:"expires_at,omitempty"`
	}
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, body, &resp, opts); err != nil {
		return 0, err
	}
	return resp.ExpiresAt, nil
}

// deleteSessionInAPI appelle DELETE /sessions/{id} sur tt-backend.
func (p *terminalProxyClient) deleteSessionInAPI(sessionID, userAPIKey string) error {
	url := fmt.Sprintf("%s/%s/sessions/%s", p.baseURL, p.apiVersion, sessionID)

	utils.Debug("deleteSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	_, err := utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	return err
}

// GetAllSessionsFromAPI récupère toutes les sessions depuis l'API Terminal Trainer
func (p *terminalProxyClient) GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error) {
	// Utiliser le type d'instance par défaut configuré pour récupérer toutes les sessions
	path := p.buildAPIPath("/sessions", "")
	url := fmt.Sprintf("%s%s?include_expired=true&limit=1000", p.baseURL, path)

	var sessionsResp dto.TerminalTrainerSessionsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionsResp, opts)
	if err != nil {
		return nil, err
	}

	return &sessionsResp, nil
}

// GetSessionInfoFromAPI récupère les infos d'une session directement depuis l'API
func (p *terminalProxyClient) GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error) {
	// Récupérer la session locale pour obtenir la clé API
	terminal, err := p.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found locally: %w", err)
	}

	// Construire le chemin avec version et type d'instance dynamique
	path := p.buildAPIPath("/info", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", p.baseURL, path, sessionID)

	var sessionInfo dto.TerminalTrainerSessionInfo
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionInfo, opts)
	if err != nil {
		// Check for 404 specifically
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("session not found on Terminal Trainer")
		}
		return nil, err
	}

	return &sessionInfo, nil
}

// getAllSessionsFromAllInstanceTypes récupère les sessions de tous les types d'instances utilisés par l'utilisateur
func (p *terminalProxyClient) getAllSessionsFromAllInstanceTypes(userAPIKey, userID string) (*dto.TerminalTrainerSessionsResponse, error) {
	// 1. Récupérer toutes les sessions locales de l'utilisateur pour connaître les types d'instances utilisés
	localSessions, err := p.repository.GetTerminalSessionsByUserID(userID, false)
	if err != nil {
		localSessions = &[]models.Terminal{} // Traiter comme liste vide si erreur
	}

	// 2. Créer un set des types d'instances utilisés (incluant le type par défaut)
	instanceTypesUsed := make(map[string]bool)
	instanceTypesUsed[""] = true // Toujours inclure le type par défaut

	for _, session := range *localSessions {
		if session.InstanceType != "" {
			instanceTypesUsed[session.InstanceType] = true
		}
	}

	// 3. Récupérer les sessions depuis chaque type d'instance utilisé
	allSessions := make([]dto.TerminalTrainerSession, 0, len(instanceTypesUsed)*10)
	totalCount := 0

	for instanceType := range instanceTypesUsed {
		apiResponse, err := p.getSessionsFromInstanceType(userAPIKey, instanceType)
		if err != nil {
			// Log l'erreur mais continuer avec les autres types d'instances
			utils.Warn("failed to get sessions from instance type '%s': %v", instanceType, err)
			continue
		}
		allSessions = append(allSessions, apiResponse.Sessions...)
		totalCount += apiResponse.Count
	}

	// 4. Retourner une réponse combinée
	return &dto.TerminalTrainerSessionsResponse{
		Sessions:       allSessions,
		Count:          totalCount,
		APIKeyID:       0, // Valeur par défaut car on combine plusieurs réponses
		IncludeExpired: true,
		Limit:          1000,
	}, nil
}

// getSessionsFromInstanceType récupère les sessions d'un type d'instance spécifique
func (p *terminalProxyClient) getSessionsFromInstanceType(userAPIKey, instanceType string) (*dto.TerminalTrainerSessionsResponse, error) {
	path := p.buildAPIPath("/sessions", instanceType)
	url := fmt.Sprintf("%s%s?include_expired=true&limit=1000", p.baseURL, path)

	var sessionsResp dto.TerminalTrainerSessionsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionsResp, opts)
	if err != nil {
		return nil, err
	}

	return &sessionsResp, nil
}

// GetServerMetrics récupère les métriques du serveur Terminal Trainer
func (p *terminalProxyClient) GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error) {
	// Skip if Terminal Trainer is not configured
	if p.baseURL == "" {
		return nil, fmt.Errorf("terminal trainer not configured")
	}

	// Construire l'URL des métriques
	path := fmt.Sprintf("/%s/metrics", p.apiVersion)
	url := fmt.Sprintf("%s%s", p.baseURL, path)

	// Ajouter les paramètres
	params := []string{}
	if nocache {
		params = append(params, "nocache=true")
	}
	if backend != "" {
		params = append(params, fmt.Sprintf("backend=%s", backend))
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	// Exécuter la requête (pas besoin d'authentification selon les specs)
	var metrics dto.ServerMetricsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithTimeout(10*time.Second))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &metrics, opts)
	if err != nil {
		return nil, err
	}

	metrics.Backend = backend
	return &metrics, nil
}

// GetDistributions fetches available distributions from tt-backend
func (p *terminalProxyClient) GetDistributions(backend string) ([]dto.TTDistribution, error) {
	url := fmt.Sprintf("%s/%s/distributions", p.baseURL, p.apiVersion)
	if backend != "" {
		url += fmt.Sprintf("?backend=%s", backend)
	}

	var distributions []dto.TTDistribution
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(p.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &distributions, opts)
	if err != nil {
		return nil, err
	}
	return distributions, nil
}

// FetchRawSizes performs a one-shot, uncached HTTP GET against
// tt-backend's /sizes endpoint. Used by the startup catalog hydration;
// the deadline carried by ctx is honored via a per-call client timeout
// (the lower of ctx's remaining time and the default). Returns the raw
// tt-backend Size records — CPUMcpu stamping is the catalog's job once
// it has been hydrated.
func (p *terminalProxyClient) FetchRawSizes(ctx context.Context) ([]dto.TTSize, error) {
	url := fmt.Sprintf("%s/%s/sizes", p.baseURL, p.apiVersion)
	var sizes []dto.TTSize
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(p.adminKey))

	// The utils HTTP layer has no context plumbing; translate the
	// context deadline into a client-side timeout so the hydration call
	// cannot block startup indefinitely.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, ctx.Err()
		}
		if remaining < opts.Timeout {
			opts.Timeout = remaining
		}
	}

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sizes, opts)
	if err != nil {
		return nil, err
	}
	return sizes, nil
}

// GetBackends retrieves all available backends from Terminal Trainer
func (p *terminalProxyClient) GetBackends() ([]dto.BackendInfo, error) {
	if p.baseURL == "" {
		return nil, fmt.Errorf("terminal trainer not configured")
	}

	url := fmt.Sprintf("%s/%s/backends", p.baseURL, p.apiVersion)

	var backends []dto.BackendInfo
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(p.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &backends, opts)
	if err != nil {
		return nil, err
	}

	for i := range backends {
		// Default Name to ID if upstream doesn't provide one
		if backends[i].Name == "" {
			backends[i].Name = backends[i].ID
		}
	}

	return backends, nil
}

// getBackendsCached returns cached backends or refreshes if older than 30s.
// Uses singleflight to coalesce concurrent cache misses into a single upstream call.
func (p *terminalProxyClient) getBackendsCached() ([]dto.BackendInfo, error) {
	p.backendCacheMu.RLock()
	if p.backendCache != nil && time.Since(p.backendCacheTime) < 30*time.Second {
		cached := make([]dto.BackendInfo, len(p.backendCache))
		copy(cached, p.backendCache)
		p.backendCacheMu.RUnlock()
		return cached, nil
	}
	p.backendCacheMu.RUnlock()

	// Use singleflight to prevent cache stampede: concurrent callers that find
	// stale cache will share a single upstream GetBackends() call.
	v, err, _ := p.backendCacheSF.Do("backends", func() (interface{}, error) {
		backends, err := p.GetBackends()
		if err != nil {
			return nil, err
		}

		p.backendCacheMu.Lock()
		p.backendCache = backends
		p.backendCacheTime = time.Now()
		p.backendCacheMu.Unlock()

		return backends, nil
	})
	if err != nil {
		return nil, err
	}

	return v.([]dto.BackendInfo), nil
}

// getSystemDefault returns the backend ID marked as default by tt-backend.
// Returns empty string if no backend is marked as default.
func (p *terminalProxyClient) getSystemDefault() string {
	backends, err := p.getBackendsCached()
	if err != nil || len(backends) == 0 {
		return ""
	}
	for _, b := range backends {
		if b.IsDefault {
			return b.ID
		}
	}
	return ""
}

// invalidateBackendCache clears the cached backends so the next read
// fetches fresh data from tt-backend.
func (p *terminalProxyClient) invalidateBackendCache() {
	p.backendCacheMu.Lock()
	p.backendCache = nil
	p.backendCacheTime = time.Time{}
	p.backendCacheMu.Unlock()
	// Cancel any in-flight singleflight request that could repopulate the
	// cache with stale data.
	p.backendCacheSF.Forget("backends")
}

// IsBackendOnline checks if a specific backend is connected.
// An empty backendName means "use system default", which is assumed online
// since tt-backend routes empty backend to its own default.
func (p *terminalProxyClient) IsBackendOnline(backendName string) (bool, error) {
	if backendName == "" {
		return true, nil
	}

	backends, err := p.getBackendsCached()
	if err != nil {
		return false, err
	}

	for _, b := range backends {
		if b.ID == backendName {
			return b.Connected, nil
		}
	}

	// Backend not found in list - assume offline
	return false, nil
}

// SetSystemDefaultBackend sets the system-wide default backend by calling
// tt-backend's admin API to mark the backend as default.
func (p *terminalProxyClient) SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error) {
	// Verify backend exists and is connected
	backends, err := p.getBackendsCached()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch backends: %w", err)
	}

	var target *dto.BackendInfo
	for i := range backends {
		if backends[i].ID == backendID {
			target = &backends[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}
	if !target.Connected {
		return nil, fmt.Errorf("backend is offline: %s", backendID)
	}

	// Find the numeric DB ID by listing admin backends
	adminBackends, err := p.getAdminBackends()
	if err != nil {
		return nil, fmt.Errorf("failed to list admin backends: %w", err)
	}

	var adminEntry *adminBackendEntry
	for i := range adminBackends {
		if adminBackends[i].BackendID == backendID {
			adminEntry = &adminBackends[i]
			break
		}
	}
	if adminEntry == nil {
		return nil, fmt.Errorf("backend not found in admin API: %s", backendID)
	}

	// Call PUT /admin/backends/{id} with is_default=true, preserving all existing fields
	isDefault := true
	updateReq := struct {
		Name              string `json:"name"`
		Description       string `json:"description,omitempty"`
		IsDefault         *bool  `json:"is_default"`
		IsActive          bool   `json:"is_active"`
		ServerURL         string `json:"server_url,omitempty"`
		ServerCertificate string `json:"server_certificate,omitempty"`
		ClientCertificate string `json:"client_certificate,omitempty"`
		Project           string `json:"project,omitempty"`
		Target            string `json:"target,omitempty"`
	}{
		Name:              adminEntry.Name,
		Description:       adminEntry.Description,
		IsDefault:         &isDefault,
		IsActive:          adminEntry.IsActive,
		ServerURL:         adminEntry.ServerURL,
		ServerCertificate: adminEntry.ServerCertificate,
		ClientCertificate: adminEntry.ClientCertificate,
		Project:           adminEntry.Project,
		Target:            adminEntry.Target,
	}

	url := fmt.Sprintf("%s/%s/admin/backends/%d", p.baseURL, p.apiVersion, adminEntry.ID)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(p.adminKey))

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, updateReq, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to set default backend on tt-backend: %w", err)
	}

	// Invalidate backend cache so next read picks up the change
	p.invalidateBackendCache()

	target.IsDefault = true
	return target, nil
}

// adminBackendEntry represents a backend from tt-backend's admin API
type adminBackendEntry struct {
	ID                int64  `json:"id"`
	BackendID         string `json:"backend_id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	IsDefault         bool   `json:"is_default"`
	IsActive          bool   `json:"is_active"`
	ServerURL         string `json:"server_url,omitempty"`
	ServerCertificate string `json:"server_certificate,omitempty"`
	ClientCertificate string `json:"client_certificate,omitempty"`
	Project           string `json:"project,omitempty"`
	Target            string `json:"target,omitempty"`
	Connected         bool   `json:"connected"`
}

type adminAPIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func (p *terminalProxyClient) getAdminBackends() ([]adminBackendEntry, error) {
	url := fmt.Sprintf("%s/%s/admin/backends", p.baseURL, p.apiVersion)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(p.adminKey))

	var resp adminAPIResponse
	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &resp, opts)
	if err != nil {
		return nil, err
	}

	var backends []adminBackendEntry
	if err := json.Unmarshal(resp.Data, &backends); err != nil {
		return nil, fmt.Errorf("failed to decode admin backends: %w", err)
	}
	return backends, nil
}
