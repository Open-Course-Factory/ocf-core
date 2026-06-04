package services

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	authModels "soli/formations/src/auth/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// terminalComposer owns composed-session orchestration: it validates a
// requested distribution/size/feature combination against the caller's
// plan, runs the budget gate, resolves the backend, and persists the
// resulting Terminal row by POSTing to tt-backend's /sessions endpoint.
// It also drives group bulk-creation, which fans the same composed-session
// flow across all active members of a group.
//
// It was carved out of terminalTrainerService to shrink that god object;
// terminalTrainerService holds a *terminalComposer and delegates the
// StartComposedSession and BulkCreateTerminalsForGroup interface methods
// to it.
//
// Collaborators: catalog supplies the session-options used for validation;
// proxy owns the tt-backend HTTP layer (server metrics, console path,
// system default backend); repository persists the local Terminal row and
// reads user keys; quotaService backs the budget gate; enumService maps
// tt-backend status codes to messages; db is read for org-level backend
// and idle-window overrides. createUserKey is the facade's CreateUserKey,
// passed as a callback so bulk creation can auto-provision keys without the
// composer duplicating the key-management concern.
type terminalComposer struct {
	proxy         *terminalProxyClient
	catalog       *terminalCatalogService
	repository    repositories.TerminalRepository
	quotaService  paymentServices.QuotaService
	enumService   TerminalTrainerEnumService
	db            *gorm.DB
	baseURL       string
	apiVersion    string
	createUserKey func(userID, keyName string) error
}

// newTerminalComposer returns a composer wired to the supplied
// collaborators. createUserKey is the facade's CreateUserKey method,
// injected so the bulk flow can auto-provision missing keys without the
// composer owning key management.
func newTerminalComposer(
	proxy *terminalProxyClient,
	catalog *terminalCatalogService,
	repository repositories.TerminalRepository,
	quotaService paymentServices.QuotaService,
	enumService TerminalTrainerEnumService,
	db *gorm.DB,
	baseURL, apiVersion string,
	createUserKey func(userID, keyName string) error,
) *terminalComposer {
	return &terminalComposer{
		proxy:         proxy,
		catalog:       catalog,
		repository:    repository,
		quotaService:  quotaService,
		enumService:   enumService,
		db:            db,
		baseURL:       baseURL,
		apiVersion:    apiVersion,
		createUserKey: createUserKey,
	}
}

// ApplyNameTemplate applies template placeholders to generate terminal names.
// Supported placeholders: {group_name}, {user_email}, {user_id}, {machine_size}.
// The machineSize is always rendered uppercase (e.g. "xs" -> "XS") so admins
// get a consistent display regardless of the catalog key casing. When the
// template is empty and a machineSize is provided, it is appended to the
// default template so the size is visible in auto-generated session names.
func ApplyNameTemplate(template, groupName, userEmail, userID, machineSize string) string {
	if template == "" {
		template = "{group_name} - {user_email}"
		if machineSize != "" {
			template += " - {machine_size}"
		}
	}

	result := template
	result = strings.ReplaceAll(result, "{group_name}", groupName)
	result = strings.ReplaceAll(result, "{user_email}", userEmail)
	result = strings.ReplaceAll(result, "{user_id}", userID)
	result = strings.ReplaceAll(result, "{machine_size}", strings.ToUpper(machineSize))

	return result
}

// ErrBulkInsufficientRAM is returned by BulkCreateTerminalsForGroup when the
// Terminal Trainer server lacks enough RAM to provision terminals for all active
// group members. Callers should map this to HTTP 503.
var ErrBulkInsufficientRAM = errors.New("server at capacity: insufficient RAM for bulk terminal creation")

// BulkCreateTerminalsForGroup creates terminals for all members of a group
func (c *terminalComposer) BulkCreateTerminalsForGroup(
	groupID string,
	requestingUserID string,
	userRoles []string,
	request dto.BulkCreateTerminalsRequest,
	planInterface any,
) (*dto.BulkCreateTerminalsResponse, error) {
	// Parse groupID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %w", err)
	}

	// Get group details
	var group groupModels.ClassGroup
	if err := c.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}

	// Check permissions - only group owner, group admin, or system administrator can bulk create terminals
	canManage := false
	if group.OwnerUserID == requestingUserID {
		canManage = true
	} else {
		// Check if user is a system administrator
		for _, role := range userRoles {
			if authModels.IsSystemAdmin(authModels.RoleName(role)) {
				canManage = true
				break
			}
		}
		// Check if user is an admin of the group
		if !canManage {
			for _, member := range group.Members {
				if member.UserID == requestingUserID && (member.Role == groupModels.GroupMemberRoleManager || member.Role == groupModels.GroupMemberRoleOwner) {
					canManage = true
					break
				}
			}
		}
	}

	if !canManage {
		return nil, fmt.Errorf("only group owner or admin can create bulk terminals")
	}

	// Filter active members only
	activeMembers := make([]groupModels.GroupMember, 0, len(group.Members))
	for _, member := range group.Members {
		if member.IsActive {
			activeMembers = append(activeMembers, member)
		}
	}

	// Pre-flight RAM check: refuse up-front if the server lacks capacity for all terminals.
	// Mirrors the logic in src/payment/middleware/ramCheckMiddleware.go.
	// Fail-open: if metrics are unavailable, skip the check and proceed.
	if len(activeMembers) > 0 {
		metrics, metricsErr := c.proxy.GetServerMetrics(true, "")
		if metricsErr != nil {
			utils.Warn("bulk terminal creation: could not fetch server metrics, skipping RAM check: %v", metricsErr)
		} else {
			plan, _ := planInterface.(*paymentModels.SubscriptionPlan)
			if plan != nil {
				// SSOT: derive per-terminal RAM from the canonical catalog
				// via EstimatePerTerminalRAMGB. Bulk pre-flight uses the
				// catalog's largest size as a worst-case estimate — the
				// real per-terminal cost is decided by the launcher.
				perTerminalRAM := EstimatePerTerminalRAMGB()

				totalRequiredRAM := float64(len(activeMembers)) * perTerminalRAM
				totalRAM := metrics.RAMAvailableGB / (1.0 - metrics.RAMPercent/100.0)
				minReservedRAM := totalRAM * 0.05

				if metrics.RAMPercent >= 99.0 || metrics.RAMAvailableGB-totalRequiredRAM < minReservedRAM {
					utils.Warn("bulk terminal creation blocked: insufficient RAM (%d terminals × %.2f GB = %.2f GB required, %.2f GB available)",
						len(activeMembers), perTerminalRAM, totalRequiredRAM, metrics.RAMAvailableGB)
					return nil, fmt.Errorf("%w: %d terminals need %.2f GB, %.2f GB available",
						ErrBulkInsufficientRAM, len(activeMembers), totalRequiredRAM, metrics.RAMAvailableGB)
				}
			}
		}
	}

	// Initialize response
	response := &dto.BulkCreateTerminalsResponse{
		Success:      true,
		CreatedCount: 0,
		FailedCount:  0,
		TotalMembers: len(activeMembers),
		Terminals:    make([]dto.BulkTerminalCreationResult, 0, len(activeMembers)),
		Errors:       make([]string, 0, len(activeMembers)/4),
	}

	// Get user details from Casdoor for email addresses
	userEmails := make(map[string]string) // userID -> email
	for _, member := range activeMembers {
		user, err := casdoorsdk.GetUserByUserId(member.UserID)
		if err != nil || user == nil {
			utils.Warn("failed to get user details for %s: %v", member.UserID, err)
			userEmails[member.UserID] = member.UserID // Fallback to userID
		} else {
			userEmails[member.UserID] = user.Email
		}
	}

	// Auto-provision terminal keys for members who don't have one
	for _, member := range activeMembers {
		_, err := c.repository.GetUserTerminalKeyByUserID(member.UserID, true)
		if err != nil {
			keyName := "auto-" + userEmails[member.UserID]
			if createErr := c.createUserKey(member.UserID, keyName); createErr != nil {
				utils.Warn("failed to auto-provision terminal key for user %s: %v", member.UserID, createErr)
			}
		}
	}

	// Resolve the machine size once: an explicit size from the request wins,
	// otherwise we fall back to the historical default of "S" so existing
	// admin tooling keeps the same behaviour.
	bulkSize := request.Size
	if bulkSize == "" {
		bulkSize = "S"
	}

	// Create terminals for each member
	for _, member := range activeMembers {
		userEmail := userEmails[member.UserID]

		// Generate terminal name using template
		terminalName := ApplyNameTemplate(request.NameTemplate, group.DisplayName, userEmail, member.UserID, bulkSize)

		// Create composed session input for this user
		composedInput := dto.CreateComposedSessionInput{
			Distribution:     request.InstanceType, // InstanceType now maps to distribution name
			Size:             bulkSize,
			Terms:            request.Terms,
			Name:             terminalName,
			Expiry:           request.Expiry,
			Backend:          request.Backend,
			OrganizationID:   request.OrganizationID,
			RecordingEnabled: request.RecordingEnabled,
			ExternalRef:      request.ExternalRef,
			Hostname:         request.Hostname,
		}

		// Try to create terminal via composed session
		sessionResp, err := c.StartComposedSession(member.UserID, composedInput, planInterface)

		result := dto.BulkTerminalCreationResult{
			UserID:    member.UserID,
			UserEmail: userEmail,
			Name:      terminalName,
			Success:   err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			response.FailedCount++
			response.Errors = append(response.Errors, fmt.Sprintf("Failed for user %s (%s): %v", userEmail, member.UserID, err))
		} else {
			result.SessionID = &sessionResp.SessionID
			// Get the terminal record to get the UUID
			terminal, terr := c.repository.GetTerminalSessionByID(sessionResp.SessionID)
			if terr == nil {
				terminalID := terminal.ID.String()
				result.TerminalID = &terminalID
			}
			response.CreatedCount++
		}

		response.Terminals = append(response.Terminals, result)
	}

	// If all failed, mark as not successful
	if response.FailedCount > 0 && response.CreatedCount == 0 {
		response.Success = false
	}

	return response, nil
}

// validateBackendForContext resolves the backend using a multi-level chain:
// 1. If orgID != nil and the org has backend config → delegate to validateBackendForOrg
// 2. Otherwise apply the plan-level rules (plan default / AllowedBackends)
// 3. Final fallback: only the system default is allowed
func (c *terminalComposer) validateBackendForContext(orgID *uuid.UUID, plan *paymentModels.SubscriptionPlan, requestedBackend string) (string, error) {
	if orgID != nil {
		var org orgModels.Organization
		if err := c.db.First(&org, "id = ?", *orgID).Error; err != nil {
			return "", fmt.Errorf("failed to get organization: %w", err)
		}

		// If the org has its own backend config, delegate to org rules
		if len(org.AllowedBackends) > 0 || org.DefaultBackend != "" {
			return c.validateBackendForOrg(orgID, requestedBackend)
		}
	}

	// No org backend config (or no org / personal org) → apply plan-level rules
	if plan != nil {
		// No backend requested → use plan default, fallback to system default
		if requestedBackend == "" {
			if plan.DefaultBackend != "" {
				return plan.DefaultBackend, nil
			}
			return c.proxy.getSystemDefault(), nil
		}

		// Backend requested → check against plan's AllowedBackends
		if len(plan.AllowedBackends) > 0 {
			for _, allowed := range plan.AllowedBackends {
				if allowed == requestedBackend {
					return requestedBackend, nil
				}
			}
			// Requested backend not in allowed list — fall back to plan default
			// (the user likely didn't explicitly choose; the frontend auto-selected from a stale list)
			if plan.DefaultBackend != "" {
				return plan.DefaultBackend, nil
			}
			return "", fmt.Errorf("backend '%s' is not allowed by your subscription plan. Allowed backends: %v",
				requestedBackend, plan.AllowedBackends)
		}

		// Plan has no AllowedBackends restriction → use plan default or system default
		if plan.DefaultBackend != "" {
			return plan.DefaultBackend, nil
		}
	}

	// Final fallback: no org config, no plan config — only system default is allowed
	systemDefault := c.proxy.getSystemDefault()
	if requestedBackend == "" || requestedBackend == systemDefault {
		return systemDefault, nil
	}
	return "", fmt.Errorf("backend '%s' is not allowed: no backend restrictions configured, only system default is available", requestedBackend)
}

// validateBackendForOrg validates and resolves the backend for an organization
func (c *terminalComposer) validateBackendForOrg(orgID *uuid.UUID, requestedBackend string) (string, error) {
	if orgID == nil {
		return requestedBackend, nil // No org context, allow any backend
	}

	var org orgModels.Organization
	if err := c.db.First(&org, "id = ?", *orgID).Error; err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	// Resolve org's effective default: org default → system default → ""
	effectiveDefault := org.DefaultBackend
	if effectiveDefault == "" {
		effectiveDefault = c.proxy.getSystemDefault()
	}

	// If no backend requested, use the effective default
	if requestedBackend == "" {
		return effectiveDefault, nil
	}

	// If AllowedBackends is empty, only the default backend is allowed
	// If no default is configured either, allow any backend (no restrictions)
	if len(org.AllowedBackends) == 0 {
		if effectiveDefault == "" {
			return requestedBackend, nil
		}
		if requestedBackend == effectiveDefault {
			return requestedBackend, nil
		}
		return "", fmt.Errorf("backend '%s' is not allowed for your organization (no backends configured, default only)",
			requestedBackend)
	}

	// Check if requested backend is in allowed list
	for _, allowed := range org.AllowedBackends {
		if allowed == requestedBackend {
			return requestedBackend, nil
		}
	}

	return "", fmt.Errorf("backend '%s' is not allowed for your organization. Allowed backends: %v",
		requestedBackend, org.AllowedBackends)
}

// StartComposedSession validates inputs against the plan and starts a composed session
func (c *terminalComposer) StartComposedSession(userID string, input dto.CreateComposedSessionInput, planInterface any) (*dto.TerminalSessionResponse, error) {
	plan, ok := planInterface.(*paymentModels.SubscriptionPlan)
	if !ok {
		return nil, fmt.Errorf("invalid subscription plan type")
	}

	// Resolve effective persistence mode (free tier hard-fails; empty defaults to ephemeral).
	// Done up-front so we never hit tt-backend with a request the plan forbids.
	effectiveMode, persistErr := resolvePersistenceMode(input.PersistenceMode, plan)
	if persistErr != nil {
		return nil, persistErr
	}
	input.PersistenceMode = effectiveMode

	// Compute session options to validate the request
	options, err := c.catalog.GetSessionOptions(plan, input.Distribution, input.Backend)
	if err != nil {
		return nil, err
	}

	// Store the distribution prefix for console URL and InstanceType
	input.DistributionPrefix = options.Distribution.Prefix

	// Validate requested size
	requestedSizeNorm := NormalizeSizeKey(input.Size)
	sizeAllowed := false
	for _, s := range options.AllowedSizes {
		if NormalizeSizeKey(s.Key) == requestedSizeNorm {
			if !s.Allowed {
				return nil, fmt.Errorf("size '%s' is not allowed: %s", input.Size, s.Reason)
			}
			sizeAllowed = true
			break
		}
	}
	if !sizeAllowed {
		return nil, fmt.Errorf("size '%s' not found in catalog", input.Size)
	}

	// Validate requested features
	if input.Features != nil {
		featureAllowedMap := make(map[string]*dto.SessionOptionFeature, len(options.AllowedFeatures))
		for i := range options.AllowedFeatures {
			featureAllowedMap[options.AllowedFeatures[i].Key] = &options.AllowedFeatures[i]
		}
		for featureKey, enabled := range input.Features {
			if !enabled {
				continue
			}
			opt, exists := featureAllowedMap[featureKey]
			if !exists {
				return nil, fmt.Errorf("feature '%s' not found in catalog", featureKey)
			}
			if !opt.Allowed {
				return nil, fmt.Errorf("feature '%s' is not allowed: %s", featureKey, opt.Reason)
			}
		}
	}

	// Custom startup packages install at container boot via cloud-init, which
	// needs egress — so they are only allowed when network is enabled for this
	// session: the plan must permit network AND the request must enable the
	// network feature. Reject before touching tt-backend so no container boots
	// with a package install that would silently fail.
	if len(input.Packages) > 0 && !(plan.NetworkAccessEnabled && input.Features["network"]) {
		return nil, fmt.Errorf("custom packages require network access, which is not enabled for this session")
	}

	// Validate backend
	var orgID *uuid.UUID
	if input.OrganizationID != "" {
		parsed, err := uuid.Parse(input.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("invalid organization_id: %w", err)
		}
		orgID = &parsed
	}

	// Snapshot the size catalog footprint onto the input so the persisted
	// Terminal row carries SizeCPU / SizeMemoryMB even in count-mode. The
	// budget sum query reads from these columns, so leaving them at zero
	// would let post-rollout count-mode rows escape budget accounting if
	// the deployment later flips to budget mode mid-session. Mirrors the
	// snapshot step the TerminalBudgetHook performs on the generic path.
	if cataSize, found := catalog.LookupSize(input.Size); found {
		input.SizeCPU = cataSize.CPU
		input.SizeMemoryMB = cataSize.MemoryMB
	}

	// Budget enforcement (MR-CORE-6).
	//
	// The TerminalBudgetHook (MR-CORE-5) is a no-op on this path because
	// StartComposedSession bypasses the generic Create flow — terminals
	// are persisted directly via repository.CreateTerminalSession in
	// startComposedSession() below. We therefore call CheckBudget
	// explicitly here.
	//
	// Budget enforcement: reject overspend with a structured budget error.
	if budgetErr := c.enforceBudget(userID, orgID, plan, input.SizeCPU, input.SizeMemoryMB); budgetErr != nil {
		return nil, budgetErr
	}

	validatedBackend, err := c.validateBackendForContext(orgID, plan, input.Backend)
	if err != nil {
		return nil, err
	}
	input.Backend = validatedBackend

	// Resolve effective idle window from the org override (if any). nil means
	// "let tt-backend fall back to its global default".
	input.IdleWindowSeconds = c.resolveIdleWindowSeconds(orgID, input.PersistenceMode)

	// Enforce max session duration. resolvePlanExpirySeconds is the SSOT
	// for this conversion — the resume path (StartSession) reads from it too,
	// so a single plan-cap rule covers both create and resume.
	maxDurationSeconds := resolvePlanExpirySeconds(plan)
	if maxDurationSeconds > 0 && (input.Expiry == 0 || input.Expiry > maxDurationSeconds) {
		input.Expiry = maxDurationSeconds
	}

	// Set plan-derived fields
	input.HistoryRetentionDays = plan.CommandHistoryRetentionDays
	input.SubscriptionPlanID = &plan.ID

	return c.startComposedSession(userID, input)
}

// enforceBudget runs the budget gate before the Terminal is persisted.
// Returns a *BudgetRejection on overspend and nil otherwise. Thin
// wrapper over EnforceBudget — exposes the configured QuotaService to
// the package-private composed-session path.
func (c *terminalComposer) enforceBudget(
	userID string,
	orgID *uuid.UUID,
	plan *paymentModels.SubscriptionPlan,
	requestedCPU, requestedMemMB int,
) error {
	return EnforceBudget(c.quotaService, userID, orgID, plan, requestedCPU, requestedMemMB)
}

// resolveIdleWindowSeconds returns the org-level idle window override for the
// requested persistence mode, or nil if the org has no override (in which case
// tt-backend falls back to its globally-configured default).
func (c *terminalComposer) resolveIdleWindowSeconds(orgID *uuid.UUID, mode string) *int {
	if orgID == nil {
		return nil
	}
	var org orgModels.Organization
	if err := c.db.First(&org, "id = ?", *orgID).Error; err != nil {
		return nil
	}
	return computeIdleWindowSeconds(&org, mode)
}

// startComposedSession is the internal method that calls tt-backend's POST /sessions endpoint
func (c *terminalComposer) startComposedSession(userID string, input dto.CreateComposedSessionInput) (*dto.TerminalSessionResponse, error) {
	// Get user key
	userKey, err := c.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %w", err)
	}
	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// Compute terms hash
	hash := sha256.New()
	io.WriteString(hash, input.Terms)
	termsHash := fmt.Sprintf("%x", hash.Sum(nil))

	// Clamp recording_enabled
	if input.RecordingEnabled > 1 {
		input.RecordingEnabled = 1
	}
	if input.RecordingEnabled < 0 {
		input.RecordingEnabled = 0
	}

	// Build POST body for tt-backend
	ttReqBody := map[string]interface{}{
		"distribution":           input.Distribution,
		"size":                   strings.ToLower(input.Size),
		"features":               input.Features,
		"terms":                  termsHash,
		"expiry":                 input.Expiry,
		"hostname":               input.Hostname,
		"packages":               input.Packages,
		"history_retention_days": input.HistoryRetentionDays,
		"recording_enabled":      input.RecordingEnabled,
		"external_ref":           input.ExternalRef,
	}
	if input.Name != "" {
		ttReqBody["name"] = input.Name
	}
	if input.PersistenceMode != "" {
		ttReqBody["persistence_mode"] = input.PersistenceMode
	}
	if input.IdleWindowSeconds != nil {
		ttReqBody["idle_window_seconds"] = *input.IdleWindowSeconds
	}

	// Build URL
	url := fmt.Sprintf("%s/%s/sessions", c.baseURL, c.apiVersion)
	if input.Backend != "" {
		url += fmt.Sprintf("?backend=%s", input.Backend)
	}

	utils.Debug("StartComposedSession - POST %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userKey.APIKey))

	// tt-backend may stream NDJSON, use the same pattern as startSession
	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, ttReqBody, opts)
	if err != nil {
		return nil, err
	}

	var sessionResp dto.TerminalTrainerSessionResponse
	if err := resp.DecodeLastJSON(&sessionResp); err != nil {
		return nil, utils.ExternalAPIError("Terminal Trainer", "decode response", err)
	}

	if sessionResp.Status != 0 {
		errorMsg := c.enumService.FormatError("session_status", int(sessionResp.Status), "Failed to start composed session")
		return nil, fmt.Errorf("%s", errorMsg)
	}

	// Build local terminal record
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)

	var orgID *uuid.UUID
	if input.OrganizationID != "" {
		parsed, err := uuid.Parse(input.OrganizationID)
		if err == nil {
			orgID = &parsed
		}
	}

	// Serialize enabled features as JSON
	composedFeaturesJSON := ""
	if input.Features != nil {
		enabledFeatures := make(map[string]bool)
		for k, v := range input.Features {
			if v {
				enabledFeatures[k] = true
			}
		}
		if len(enabledFeatures) > 0 {
			if b, err := json.Marshal(enabledFeatures); err == nil {
				composedFeaturesJSON = string(b)
			}
		}
	}

	terminal := &models.Terminal{
		SessionID:            sessionResp.SessionID,
		UserID:               userID,
		Name:                 input.Name,
		State:                models.StateRunning,
		PersistenceMode:      input.PersistenceMode,
		ExpiresAt:            expiresAt,
		InstanceType:         input.DistributionPrefix,
		MachineSize:          strings.ToUpper(input.Size),
		Backend:              sessionResp.Backend,
		OrganizationID:       orgID,
		SubscriptionPlanID:   input.SubscriptionPlanID,
		UserTerminalKeyID:    userKey.ID,
		UserTerminalKey:      *userKey,
		ComposedDistribution: input.Distribution,
		ComposedSize:         input.Size,
		ComposedFeatures:     composedFeaturesJSON,
		SizeCPU:              input.SizeCPU,
		SizeMemoryMB:         input.SizeMemoryMB,
	}

	if err := c.repository.CreateTerminalSession(terminal); err != nil {
		return nil, fmt.Errorf("failed to save terminal session: %w", err)
	}

	// Build console URL
	consolePath := c.proxy.buildAPIPath("/console", input.DistributionPrefix)
	response := &dto.TerminalSessionResponse{
		SessionID:  sessionResp.SessionID,
		ExpiresAt:  expiresAt,
		ConsoleURL: fmt.Sprintf("%s%s?id=%s", c.baseURL, consolePath, sessionResp.SessionID),
		Status:     "active",
		Backend:    sessionResp.Backend,
	}

	return response, nil
}
