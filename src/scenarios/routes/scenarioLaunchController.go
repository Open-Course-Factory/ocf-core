package scenarioController

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"soli/formations/src/auth/errors"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalDto "soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/httperrors"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// scenarioLaunchController handles the scenario launch / session-start
// endpoints: starting a session on an existing terminal (StartScenario), the
// composed-session launch flow (LaunchScenario), the launch preview
// (PreviewScenario), listing launchable scenarios (GetAvailableScenarios), and
// the learner's own session list (GetMySessions). It embeds
// scenarioControllerBase to reach the shared db handle and helpers
// (hasAdminRole, buildScenarioOutput).
type scenarioLaunchController struct {
	scenarioControllerBase
	sessionService  *services.ScenarioSessionService
	terminalService terminalServices.TerminalTrainerService
}

// NewScenarioLaunchController creates a launch controller with its service
// dependencies wired to the given database handle.
func NewScenarioLaunchController(db *gorm.DB) *scenarioLaunchController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	terminalService := terminalServices.NewTerminalTrainerService(db)

	// Stop-on-failure and close-the-build-window, wired in one place so no
	// constructor can ship with half the pair (see WireTerminalCallbacks).
	services.WireTerminalCallbacks(sessionService, terminalService)

	// Subscribe to SIGKILLed console shells so a crash trap can end the run.
	// terminalTrainer publishes the event rather than calling us directly: it
	// is the lower layer (scenarios imports it, not the reverse), so this is
	// the mirror image of the terminal-stop callback above.
	terminalServices.SetConsoleShellKilledObserver(sessionService.EndCrashTrapRun)

	return &scenarioLaunchController{
		scenarioControllerBase: scenarioControllerBase{db: db},
		sessionService:         sessionService,
		terminalService:        terminalService,
	}
}

// StartScenario godoc
// @Summary Start a scenario session
// @Description Start a new scenario session on a terminal for the authenticated user
// @Tags scenario-sessions
// @Accept json
// @Produce json
// @Param body body dto.StartScenarioInput true "Start request"
// @Success 201 {object} dto.ScenarioSessionOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/start [post]
// @Security BearerAuth
func (sc *scenarioLaunchController) StartScenario(ctx *gin.Context) {
	var input dto.StartScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	scenarioID, err := uuid.Parse(input.ScenarioID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	// Validate terminal session ownership
	var terminal terminalModels.Terminal
	if err := sc.db.Where("session_id = ?", input.TerminalSessionID).First(&terminal).Error; err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Terminal session not found",
		})
		return
	}
	if terminal.UserID != userID {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You do not own this terminal session",
		})
		return
	}

	// Check machine compatibility: terminal must match scenario requirements
	var scenario models.Scenario
	if err := sc.db.First(&scenario, "id = ?", scenarioID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found",
		})
		return
	}

	// An archived scenario cannot start a new run, on a fresh terminal or on
	// one the learner already has.
	if sc.rejectIfArchived(ctx, &scenario) {
		return
	}

	// Size check: terminal size must be >= scenario required size
	requiredSize := scenario.InstanceType // stores size like "M", "XL"
	machineSize := terminal.MachineSize   // actual terminal size like "L"
	if requiredSize != "" && machineSize != "" {
		requiredOrder, reqOk := sizeOrder[requiredSize]
		machineOrder, machOk := sizeOrder[machineSize]
		if reqOk && machOk && machineOrder < requiredOrder {
			ctx.JSON(http.StatusConflict, &errors.APIError{
				ErrorCode:    http.StatusConflict,
				ErrorMessage: fmt.Sprintf("This scenario requires a %s machine or larger, but this terminal is %s", requiredSize, machineSize),
			})
			return
		}
	}

	// OS type check: look up terminal's distribution to get its OS type from tt-backend
	if scenario.OsType != "" && terminal.ComposedDistribution != "" {
		distributions, ttErr := sc.terminalService.GetDistributions("")
		if ttErr == nil {
			for _, dist := range distributions {
				if dist.Name == terminal.ComposedDistribution || dist.Prefix == terminal.InstanceType {
					if dist.OsType != "" && dist.OsType != scenario.OsType {
						ctx.JSON(http.StatusConflict, &errors.APIError{
							ErrorCode:    http.StatusConflict,
							ErrorMessage: fmt.Sprintf("This scenario requires a %s-based machine, but this terminal runs %s", scenario.OsType, dist.OsType),
						})
						return
					}
					break
				}
			}
		}
	}

	// Check group-based scenario assignment access (admins and public scenarios bypass)
	if scenario.IsPublic {
		// Public scenarios are available to everyone, skip assignment check
	} else if !sc.hasAdminRole(ctx) {
		var groupIDs []uuid.UUID
		if err := sc.db.Model(&groupModels.GroupMember{}).
			Where("user_id = ? AND is_active = true", userID).
			Pluck("group_id", &groupIDs).Error; err != nil {
			slog.Error("failed to check group membership", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to verify scenario access",
			})
			return
		}

		var count int64
		if len(groupIDs) > 0 {
			if err := sc.db.Model(&models.ScenarioAssignment{}).
				Where("scenario_id = ? AND group_id IN ? AND scope = ? AND is_active = true AND (deadline IS NULL OR deadline > ?) AND (start_date IS NULL OR start_date <= ?)",
					scenarioID, groupIDs, "group", time.Now(), time.Now()).
				Count(&count).Error; err != nil {
				slog.Error("failed to check group scenario assignment", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to verify scenario access",
				})
				return
			}
		}

		if count == 0 {
			// Also check organization-scoped assignments
			var orgIDs []uuid.UUID
			if err := sc.db.Model(&orgModels.OrganizationMember{}).
				Where("user_id = ? AND is_active = true", userID).
				Pluck("organization_id", &orgIDs).Error; err != nil {
				slog.Error("failed to check org membership", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to verify scenario access",
				})
				return
			}
			if len(orgIDs) > 0 {
				if err := sc.db.Model(&models.ScenarioAssignment{}).
					Where("scenario_id = ? AND organization_id IN ? AND scope = ? AND is_active = true AND (deadline IS NULL OR deadline > ?) AND (start_date IS NULL OR start_date <= ?)",
						scenarioID, orgIDs, "org", time.Now(), time.Now()).
					Count(&count).Error; err != nil {
					slog.Error("failed to check org scenario assignment", "err", err)
					ctx.JSON(http.StatusInternalServerError, &errors.APIError{
						ErrorCode:    http.StatusInternalServerError,
						ErrorMessage: "Failed to verify scenario access",
					})
					return
				}
			}
		}

		if count == 0 {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Scenario is not assigned to your group or organization",
			})
			return
		}
	}

	session, err := sc.sessionService.StartScenario(userID, scenarioID, input.TerminalSessionID, input.Locale)
	if err != nil {
		if stderrors.Is(err, services.ErrActiveSessionExists) {
			// A conflict the learner can resolve by resuming, not a fault.
			// Reported with a reason so the client can offer that instead of a
			// retry that would fail identically.
			ctx.JSON(http.StatusConflict, gin.H{
				"error_code":    http.StatusConflict,
				"error_message": "A run of this scenario is already in progress.",
				"reason":        blockReasonSessionExists,
			})
			return
		}
		slog.Error("failed to start scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to start scenario",
		})
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusCreated, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TrainerID:         session.TrainerID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		StartedAt:         session.StartedAt,
	})
}

// GetMySessions godoc
// @Summary Get my scenario sessions
// @Description Get all scenario sessions for the authenticated user
// @Tags scenario-sessions
// @Produce json
// @Success 200 {array} dto.MySessionResponse
// @Failure 401 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/my [get]
// @Security BearerAuth
func (sc *scenarioLaunchController) GetMySessions(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Unauthorized",
		})
		return
	}

	sessions, err := sc.sessionService.GetMySessions(userID)
	if err != nil {
		slog.Error("failed to get my sessions", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get sessions",
		})
		return
	}

	ctx.JSON(http.StatusOK, sessions)
}

// @Summary Get available scenarios
// @Description Get scenarios assigned to the current user's groups or organizations. Admins see all scenarios.
// @Tags scenarios
// @Produce json
// @Success 200 {array} map[string]any
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/available [get]
// @Security BearerAuth
func (sc *scenarioLaunchController) GetAvailableScenarios(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Read org context from middleware (set by InjectOrgContext)
	var orgID *uuid.UUID
	if orgCtx, exists := ctx.Get("org_context_id"); exists {
		if orgStr, ok := orgCtx.(string); ok && orgStr != "" {
			if parsed, parseErr := uuid.Parse(orgStr); parseErr == nil {
				orgID = &parsed
			}
		}
	}

	var scenarios []models.Scenario

	if sc.hasAdminRole(ctx) {
		if err := sc.db.Scopes(models.NotArchived).Preload("CompatibleInstanceTypes").Find(&scenarios).Error; err != nil {
			slog.Error("failed to fetch all scenarios", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to fetch scenarios",
			})
			return
		}
	} else {
		// Scope scenario list to the current org context:
		// - With org context: only show that org's assignments + that org's groups
		// - Without org context (personal): only personal groups (no org) + public scenarios
		var groupIDs []uuid.UUID
		var conditions []string
		var args []interface{}

		if orgID != nil {
			// Get groups belonging to this org that the user is a member of
			if err := sc.db.Model(&groupModels.GroupMember{}).
				Joins("JOIN class_groups cg ON cg.id = group_members.group_id").
				Where("group_members.user_id = ? AND group_members.is_active = true AND cg.organization_id = ?", userID, *orgID).
				Pluck("group_members.group_id", &groupIDs).Error; err != nil {
				slog.Error("failed to get user group memberships for org", "err", err)
			}

			if len(groupIDs) > 0 {
				conditions = append(conditions, "(sa.scope = 'group' AND sa.group_id IN ?)")
				args = append(args, groupIDs)
			}
			conditions = append(conditions, "(sa.scope = 'org' AND sa.organization_id = ?)")
			args = append(args, *orgID)
		} else {
			// Personal context: only groups without an org
			if err := sc.db.Model(&groupModels.GroupMember{}).
				Joins("JOIN class_groups cg ON cg.id = group_members.group_id").
				Where("group_members.user_id = ? AND group_members.is_active = true AND cg.organization_id IS NULL", userID).
				Pluck("group_members.group_id", &groupIDs).Error; err != nil {
				slog.Error("failed to get user personal group memberships", "err", err)
			}

			if len(groupIDs) > 0 {
				conditions = append(conditions, "(sa.scope = 'group' AND sa.group_id IN ?)")
				args = append(args, groupIDs)
			}
		}

		if len(conditions) > 0 {
			now := time.Now()
			combined := strings.Join(conditions, " OR ")
			query := sc.db.Distinct().
				Scopes(models.NotArchived).
				Preload("CompatibleInstanceTypes").
				Joins("JOIN scenario_assignments sa ON sa.scenario_id = scenarios.id").
				Where("sa.is_active = true AND (sa.deadline IS NULL OR sa.deadline > ?) AND (sa.start_date IS NULL OR sa.start_date <= ?)", now, now).
				Where(combined, args...)

			if err := query.Find(&scenarios).Error; err != nil {
				slog.Error("failed to fetch available scenarios", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to fetch scenarios",
				})
				return
			}
		}

		// Also include public scenarios
		var publicScenarios []models.Scenario
		sc.db.Scopes(models.NotArchived).Preload("CompatibleInstanceTypes").Where("is_public = ?", true).Find(&publicScenarios)
		// Merge, avoiding duplicates
		existingIDs := make(map[uuid.UUID]bool)
		for _, s := range scenarios {
			existingIDs[s.ID] = true
		}
		for _, s := range publicScenarios {
			if !existingIDs[s.ID] {
				scenarios = append(scenarios, s)
			}
		}
	}

	// For admins: determine which scenarios they'd see as a regular user
	// so we can flag admin-only visibility
	assignedScenarioIDs := make(map[uuid.UUID]bool)
	isAdmin := sc.hasAdminRole(ctx)
	if isAdmin {
		var groupIDs []uuid.UUID
		sc.db.Model(&groupModels.GroupMember{}).
			Where("user_id = ? AND is_active = true", userID).
			Pluck("group_id", &groupIDs)
		var orgIDs []uuid.UUID
		sc.db.Model(&orgModels.OrganizationMember{}).
			Where("user_id = ? AND is_active = true", userID).
			Pluck("organization_id", &orgIDs)

		var assignedIDs []uuid.UUID
		now := time.Now()
		if len(groupIDs) > 0 || len(orgIDs) > 0 {
			var conditions []string
			var scopeArgs []interface{}
			if len(groupIDs) > 0 {
				conditions = append(conditions, "(scope = 'group' AND group_id IN ?)")
				scopeArgs = append(scopeArgs, groupIDs)
			}
			if len(orgIDs) > 0 {
				conditions = append(conditions, "(scope = 'org' AND organization_id IN ?)")
				scopeArgs = append(scopeArgs, orgIDs)
			}
			combined := strings.Join(conditions, " OR ")
			sc.db.Model(&models.ScenarioAssignment{}).
				Where("is_active = ? AND (deadline IS NULL OR deadline > ?) AND (start_date IS NULL OR start_date <= ?)", true, now, now).
				Where(combined, scopeArgs...).
				Pluck("scenario_id", &assignedIDs)
		}
		for _, id := range assignedIDs {
			assignedScenarioIDs[id] = true
		}
	}

	// Get user's effective plan so the budget gate can check remaining
	// CPU/RAM against the scenario's required size.
	effectivePlanService := paymentServices.NewEffectivePlanService(sc.db)
	var resolvedPlan *paymentModels.SubscriptionPlan
	// The pool the plan draws on, which is not necessarily the org in the request
	// — see #457. Used below so a scenario marked launchable here is one the
	// launch gate will actually accept.
	var budgetScope *uuid.UUID
	planResult, planErr := effectivePlanService.GetUserEffectivePlan(userID, orgID)
	if planErr == nil && planResult != nil && planResult.Plan != nil {
		resolvedPlan = planResult.Plan
		budgetScope = planResult.ScopeOrganizationID
	}

	budgetGateActive := resolvedPlan != nil
	var quotaSvc paymentServices.QuotaService
	if budgetGateActive {
		quotaSvc = paymentServices.NewQuotaService(sc.db, effectivePlanService)
	}

	// A run the learner already has is the other thing that stops a launch, and
	// it is evaluated by the same rule the launch path applies — see
	// ScenarioSessionService.GetResumableSessions. Reported here so the card can
	// offer Resume rather than a Launch that would come back 409.
	scenarioIDs := make([]uuid.UUID, 0, len(scenarios))
	for _, s := range scenarios {
		scenarioIDs = append(scenarioIDs, s.ID)
	}
	resumableSessions, resumableErr := sc.sessionService.GetResumableSessions(userID, scenarioIDs)
	if resumableErr != nil {
		// Not fatal: without this the card falls back to offering a launch, and
		// the launch itself still refuses with 409. Losing the whole listing
		// would be the worse trade.
		slog.Warn("could not load existing scenario sessions", "err", resumableErr)
	}

	// Convert to enriched output with launchability info
	output := make([]dto.AvailableScenarioOutput, 0, len(scenarios))
	for _, s := range scenarios {
		item := dto.AvailableScenarioOutput{
			ID:            s.ID,
			Name:          s.Name,
			Title:         s.Title,
			Description:   s.Description,
			Difficulty:    s.Difficulty,
			EstimatedTimeMinutes: s.EstimatedTimeMinutes,
			InstanceType:  s.InstanceType,
			OsType:        s.OsType,
			IsPublic:      s.IsPublic,
		}

		// A scenario offered in more than one language says so here, so the card
		// can present the choice without asking a second endpoint.
		//
		// Guarded on the declaration the row already carries: resolving coverage
		// costs a handful of queries per scenario, and this runs once per card in
		// the catalogue. Single-language scenarios — which is all of them today —
		// pay nothing.
		if s.Locales != "" {
			if locales, err := services.LaunchableLocales(sc.db, s.ID); err != nil {
				slog.Warn("failed to resolve available languages", "scenario_id", s.ID, "err", err)
			} else {
				item.AvailableLocales = locales
			}
			// The card's own words in each language, so the picker beside them
			// can change what it is describing without another request.
			if text, err := services.ScenarioTextByLocale(sc.db, s); err != nil {
				slog.Warn("failed to resolve card text", "scenario_id", s.ID, "err", err)
			} else {
				item.LocalisedText = text
			}
		}

		// Convert compatible instance types
		if len(s.CompatibleInstanceTypes) > 0 {
			types := make([]dto.ScenarioInstanceTypeOutput, 0, len(s.CompatibleInstanceTypes))
			for _, t := range s.CompatibleInstanceTypes {
				types = append(types, dto.ScenarioInstanceTypeOutput{
					ID:           t.ID,
					ScenarioID:   t.ScenarioID,
					InstanceType: t.InstanceType,
					OsType:       t.OsType,
					Priority:     t.Priority,
					CreatedAt:    t.CreatedAt,
					UpdatedAt:    t.UpdatedAt,
				})
			}
			item.CompatibleInstanceTypes = types
		}

		// Populate required features
		if rf, rfErr := s.GetRequiredFeatures(); rfErr != nil {
			slog.Warn("invalid required_features for scenario", "scenario", s.Name, "err", rfErr)
		} else {
			item.RequiredFeatures = rf
		}

		// Determine launchability by checking distribution compatibility.
		provisioning, resolveErr := resolveScenarioProvisioning(sc.db, sc.terminalService, s, orgID, "")
		item.Launchable = resolveErr == nil && provisioning.CreatesTerminal()
		item.ResolvedDistribution = provisioning.Distribution
		item.ResolvedSize = provisioning.Size
		if resolveErr != nil {
			// Separate reasons because they need different fixes: a missing
			// declared image is "install it on this backend", while
			// no_distribution is "nothing here fits this scenario at all".
			item.BlockReason = blockReasonNoDistribution
			if stderrors.Is(resolveErr, errDeclaredImageUnavailable) {
				item.BlockReason = blockReasonDeclaredImageUnavail
			}
		}

		// Budget gate: the scenario's required size must fit in the user's
		// remaining CPU/RAM budget.
		if budgetGateActive && item.Launchable && provisioning.Size != "" {
			fits, fitErr := quotaSvc.RemainingBudgetFits(userID, budgetScope, resolvedPlan, provisioning.Size)
			if fitErr != nil {
				slog.Warn("budget fit check failed for scenario", "scenario", s.Name, "err", fitErr)
			} else if !fits {
				item.Launchable = false
				item.BlockReason = blockReasonForBudgetMiss(quotaSvc, resolvedPlan, provisioning.Size, s.Name)
			}
		}

		if existing := resumableSessions[s.ID]; existing != nil {
			item.Launchable = false
			item.BlockReason = blockReasonSessionExists
			item.ActiveSessionID = existing.ID.String()
			if existing.TerminalSessionID != nil {
				item.ActiveTerminalSessionID = *existing.TerminalSessionID
			}
		}

		// Flag scenarios visible only due to admin status
		if isAdmin && !assignedScenarioIDs[s.ID] && !s.IsPublic {
			item.AdminOnly = true
		}

		output = append(output, item)
	}
	ctx.JSON(http.StatusOK, output)
}

// sizeOrder maps size labels to numeric order for comparison.
// A larger number means a more powerful machine.
var sizeOrder = map[string]int{
	"XS": 1, "S": 2, "M": 3, "L": 4, "XL": 5, "XXL": 6,
}

// resolveSizeOrFallback returns a valid size key, falling back when the
// requested size is not in the catalog. Returns the canonical key from the
// catalog and a bool indicating whether a fallback was applied.
//
// Resolution order:
//  1. Requested matches a catalog entry (case-insensitive) → use canonical key
//  2. Distribution default matches a catalog entry → use it
//  3. Smallest size in catalog (lowest SortOrder)
//  4. Catalog unavailable (nil/empty) → pass requested through unchanged
//     (preserves prior behavior when the catalog fetch fails)
func resolveSizeOrFallback(requested string, dist terminalDto.TTDistribution, sizes []terminalDto.TTSize) (string, bool) {
	for _, s := range sizes {
		if strings.EqualFold(s.Key, requested) {
			return s.Key, false
		}
	}
	if dist.DefaultSizeKey != "" {
		for _, s := range sizes {
			if strings.EqualFold(s.Key, dist.DefaultSizeKey) {
				return s.Key, true
			}
		}
	}
	if len(sizes) > 0 {
		smallest := sizes[0]
		for _, s := range sizes[1:] {
			if s.SortOrder < smallest.SortOrder {
				smallest = s
			}
		}
		return smallest.Key, true
	}
	return requested, false
}

// applySizeFallback resolves the requested size against the catalog and logs
// a warning when a fallback is applied, so the two return sites in
// resolveDistribution stay terse and consistent.
func applySizeFallback(scenario models.Scenario, requested string, dist terminalDto.TTDistribution, sizes []terminalDto.TTSize) string {
	final, fellBack := resolveSizeOrFallback(requested, dist, sizes)
	if fellBack {
		slog.Warn("scenario size fallback",
			"scenario_id", scenario.ID,
			"requested", requested,
			"resolved", final,
		)
	}
	return final
}

// resolveDistribution finds a compatible distribution for a scenario.
// Returns the distribution name, the size key, and the features map.
//
// The `sizes` parameter is the live tt-backend size catalog used to validate
// the scenario's stored InstanceType and apply launch-time fallback when it
// is unknown (typo, stale import, key from another tt-backend instance). When
// `sizes` is nil/empty (catalog fetch failed), the requested size is passed
// through unchanged — tt-backend's `validateComposition()` remains the final
// authority and will reject truly invalid sizes.
// errDeclaredImageUnavailable marks a scenario that named the images it needs
// when none of them exist on the chosen backend.
//
// It is deliberately distinct from the generic "nothing matched" failure: the
// two are different operator problems. This one means "install the missing
// distribution on this backend"; the generic one means "no image anywhere fits
// this scenario's os_type, size and features".
var errDeclaredImageUnavailable = stderrors.New("declared instance type unavailable")

// Block reasons are a contract with the launcher, which switches its copy on
// them. Named here so the endpoint that emits one and the endpoint that refuses
// a launch for the same cause cannot drift apart in spelling.
const (
	blockReasonNoDistribution       = "no_distribution"
	blockReasonDeclaredImageUnavail = "declared_image_unavailable"
	blockReasonBudgetExhausted      = "budget_exhausted"
	blockReasonSizeOverPlan         = "size_over_plan"
	blockReasonSessionExists        = "session_exists"
)

// blockReasonForBudgetMiss names WHY the machine does not fit. A size larger
// than the plan's own maximum never fits, however much the learner stops, so
// reporting it as an exhausted budget sent them hunting for sessions they did
// not have — the launcher told a learner with nothing running that their
// "budget is fully used by your current sessions".
//
// The plan-capacity question belongs to the quota service, which owns the
// arithmetic; this only chooses which of its two answers to name. A failure to
// decide keeps the conservative, older reason.
func blockReasonForBudgetMiss(quotaSvc paymentServices.QuotaService, plan *paymentModels.SubscriptionPlan, sizeKey string, scenarioName string) string {
	allowed, err := quotaSvc.PlanAllowsSize(plan, sizeKey)
	if err != nil {
		slog.Warn("plan capacity check failed for scenario", "scenario", scenarioName, "err", err)
		return blockReasonBudgetExhausted
	}
	if !allowed {
		return blockReasonSizeOverPlan
	}
	return blockReasonBudgetExhausted
}

func resolveDistribution(scenario models.Scenario, distributions []terminalDto.TTDistribution, sizes []terminalDto.TTSize) (distName string, size string, features map[string]bool, err error) {
	requiredFeatures, featErr := scenario.GetRequiredFeatures()
	if featErr != nil {
		return "", "", nil, fmt.Errorf("invalid scenario configuration: %w", featErr)
	}
	requiredSize := scenario.InstanceType // This is actually a SIZE like "M"

	// Priority path: if CompatibleInstanceTypes is populated, try matching by name first
	if len(scenario.CompatibleInstanceTypes) > 0 {
		sorted := services.SortInstanceTypesByPriority(scenario.CompatibleInstanceTypes)

		for _, cit := range sorted {
			for _, dist := range distributions {
				if strings.EqualFold(cit.InstanceType, dist.Name) {
					// Match found — use scenario size if set, otherwise distribution default
					resolvedSize := requiredSize
					if resolvedSize == "" {
						resolvedSize = dist.DefaultSizeKey
					}
					featuresMap, featMapErr := scenarioFeatures(scenario)
					if featMapErr != nil {
						return "", "", nil, featMapErr
					}
					return dist.Name, applySizeFallback(scenario, resolvedSize, dist, sizes), featuresMap, nil
				}
			}
		}
		// Nothing the scenario named is available here. Naming images is a
		// statement of requirement, so substituting one the author never
		// approved is refused: that silent substitution is how a challenge ran
		// on the generic Debian base for months, in production, unnoticed.
		//
		// A scenario naming several images has already said which substitutions
		// are acceptable — they are simply lower-priority entries in this list,
		// and one of them would have matched above.
		wanted := make([]string, 0, len(sorted))
		for _, cit := range sorted {
			wanted = append(wanted, cit.InstanceType)
		}
		return "", "", nil, fmt.Errorf("%w: scenario requires one of [%s]",
			errDeclaredImageUnavailable, strings.Join(wanted, ", "))
	}

	for _, dist := range distributions {
		// Match OS type
		if scenario.OsType != "" && dist.OsType != scenario.OsType {
			continue
		}
		// Check distribution supports all required features
		if !distributionSupportsFeatures(dist, requiredFeatures) {
			continue
		}
		// Check distribution's min_size_key allows the requested size
		if requiredSize != "" && dist.MinSizeKey != "" {
			reqOrder, reqOk := sizeOrder[strings.ToUpper(requiredSize)]
			minOrder, minOk := sizeOrder[strings.ToUpper(dist.MinSizeKey)]
			if reqOk && minOk && reqOrder < minOrder {
				continue // requested size is smaller than distribution's minimum
			}
		}
		// Found a compatible distribution
		size = requiredSize
		if size == "" && dist.DefaultSizeKey != "" {
			size = dist.DefaultSizeKey
		}
		featuresMap, featMapErr := scenarioFeatures(scenario)
		if featMapErr != nil {
			return "", "", nil, featMapErr
		}
		return dist.Name, applySizeFallback(scenario, size, dist, sizes), featuresMap, nil
	}
	return "", "", nil, fmt.Errorf("no compatible distribution found for scenario (os_type=%s, size=%s)", scenario.OsType, requiredSize)
}

// distributionSupportsFeatures checks if a distribution supports all required features
func distributionSupportsFeatures(dist terminalDto.TTDistribution, required []string) bool {
	if len(required) == 0 {
		return true
	}
	supported := make(map[string]bool, len(dist.SupportedFeatures))
	for _, f := range dist.SupportedFeatures {
		supported[f] = true
	}
	for _, req := range required {
		if !supported[req] {
			return false
		}
	}
	return true
}

// resolveScenarioBackendAndDistribution determines the best backend and distribution
// for a scenario, taking into account the user's organization context. It tries
// org-allowed backends first (defaultBackend has priority), then falls back to the
// system default backend.
func resolveScenarioProvisioning(
	db *gorm.DB,
	terminalService terminalServices.TerminalTrainerService,
	scenario models.Scenario,
	orgID *uuid.UUID,
	preferredBackend string,
) (services.ScenarioProvisioning, error) {
	// Determine candidate backends. A caller that already knows which backend it
	// wants — the teacher picking one for a whole class — gets it tried first;
	// the org's own backends follow, then the system default.
	var candidateBackends []string
	if preferredBackend != "" {
		candidateBackends = append(candidateBackends, preferredBackend)
	}
	if orgID != nil {
		var org orgModels.Organization
		if err := db.First(&org, "id = ?", *orgID).Error; err == nil {
			if org.DefaultBackend != "" && org.DefaultBackend != preferredBackend {
				candidateBackends = append(candidateBackends, org.DefaultBackend)
			}
			for _, b := range org.AllowedBackends {
				if b != org.DefaultBackend && b != preferredBackend {
					candidateBackends = append(candidateBackends, b)
				}
			}
		}
	}
	if len(candidateBackends) == 0 {
		candidateBackends = []string{""} // system default
	}

	// Fetch the size catalog once (cached 60s in the service) so resolveDistribution
	// can apply launch-time fallback for scenarios with unknown InstanceType values
	// (typos, stale imports, keys from another tt-backend instance). On fetch
	// failure we pass nil — resolveDistribution then preserves prior behavior
	// and tt-backend's validateComposition() remains the final authority.
	sizes, sizesErr := terminalService.GetCatalogSizes()
	if sizesErr != nil {
		slog.Warn("failed to fetch sizes catalog, scenario size fallback disabled", "err", sizesErr)
		sizes = nil
	}

	// Try each candidate backend
	var lastErr error
	for _, b := range candidateBackends {
		distributions, distErr := terminalService.GetDistributions(b)
		if distErr != nil {
			lastErr = distErr
			continue
		}
		resolvedDist, resolvedSize, resolvedFeatures, resolveErr := resolveDistribution(scenario, distributions, sizes)
		if resolveErr != nil {
			lastErr = resolveErr
			continue
		}
		return services.ScenarioProvisioning{
			Backend:       b,
			Distribution:  resolvedDist,
			Size:          resolvedSize,
			Features:      resolvedFeatures,
			BuildFeatures: scenarioBuildFeatures(scenario),
		}, nil
	}

	return services.ScenarioProvisioning{}, fmt.Errorf("no compatible distribution on any backend: %v", lastErr)
}

// LaunchScenario godoc
// @Summary Launch a scenario with auto-provisioned terminal
// @Description Creates a terminal session and starts a scenario session in one call
// @Tags scenario-sessions
// @Accept json
// @Produce json
// @Param input body dto.LaunchScenarioInput true "Launch input"
// @Success 200 {object} dto.LaunchScenarioResponse
// @Failure 400 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 409 {object} errors.APIError
// @Failure 503 {object} errors.APIError
// @Router /scenario-sessions/launch [post]
// @Security BearerAuth
func (sc *scenarioLaunchController) LaunchScenario(ctx *gin.Context) {
	// Dunning gate (MANDATORY on every new session-creation route): launching
	// a scenario provisions a terminal. The route carries InjectEffectivePlan,
	// so the gate has its EffectivePlanResult. Without this, a past_due
	// customer could keep starting sessions through scenarios after the
	// terminal routes locked them out.
	if paymentMiddleware.GatePastDueBeyondGrace(ctx) {
		return
	}

	var input dto.LaunchScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid input: " + err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")
	scenarioID, err := uuid.Parse(input.ScenarioID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Load scenario with CompatibleInstanceTypes and Steps
	var scenario models.Scenario
	if err := sc.db.Preload("CompatibleInstanceTypes").Preload("Steps").First(&scenario, scenarioID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found",
		})
		return
	}

	if sc.rejectIfArchived(ctx, &scenario) {
		return
	}

	// Check assignment access: admin OR user has group/org assignment
	if !sc.hasAdminRole(ctx) {
		hasAccess, err := sc.checkScenarioAccess(userID, scenarioID)
		if err != nil {
			slog.Error("failed to check scenario access", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to verify access",
			})
			return
		}
		if !hasAccess {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "No access to this scenario",
			})
			return
		}
	}

	// Read org context from middleware (set by InjectOrgContext)
	var orgID *uuid.UUID
	if orgCtx, exists := ctx.Get("org_context_id"); exists {
		if orgStr, ok := orgCtx.(string); ok && orgStr != "" {
			if parsed, parseErr := uuid.Parse(orgStr); parseErr == nil {
				orgID = &parsed
			}
		}
	}

	// Resolve backend + distribution using org-aware logic
	provisioning, distErr := resolveScenarioProvisioning(sc.db, sc.terminalService, scenario, orgID, "")
	if distErr != nil {
		slog.Error("no compatible distribution for scenario", "scenario", scenario.Name, "err", distErr)
		ctx.JSON(http.StatusConflict, &errors.APIError{
			ErrorCode:    http.StatusConflict,
			ErrorMessage: "No compatible environment available for this scenario",
		})
		return
	}

	// Host RAM capacity check — see scenarioRoutes.go for why this is here
	// instead of in middleware. CheckRAMAvailability drains the request
	// body via ShouldBindBodyWith, which made ShouldBindJSON above return
	// EOF (user-reported 400 "Invalid input: EOF"). Beyond that fix, this
	// check evaluates against the ACTUAL resolved scenario size — not the
	// plan-max fallback the middleware used because scenarios don't carry
	// a size in the request body. Mirrors commit 951b69c (resume path).
	if planVal, exists := ctx.Get("subscription_plan"); exists && planVal != nil {
		if plan, ok := planVal.(*paymentModels.SubscriptionPlan); ok {
			if terminalServices.EnforceLaunchCapacity(ctx, plan, provisioning.Size, sc.terminalService) {
				return
			}
		}
	}

	// Auto-provision terminal key if missing
	_, keyErr := sc.terminalService.GetUserKey(userID)
	if keyErr != nil {
		user, userErr := casdoorsdk.GetUserByUserId(userID)
		keyName := "auto-" + userID
		if userErr == nil && user != nil && user.Email != "" {
			keyName = "auto-" + user.Email
		}
		if createErr := sc.terminalService.CreateUserKey(userID, keyName); createErr != nil {
			slog.Error("failed to create terminal key for user", "userID", userID, "err", createErr)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to provision terminal access",
			})
			return
		}
	}

	// Fetch terms from tt-backend
	terms, termsErr := sc.terminalService.GetTerms()
	if termsErr != nil {
		slog.Error("failed to fetch terminal terms", "err", termsErr)
		ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
			ErrorCode:    http.StatusServiceUnavailable,
			ErrorMessage: "Terminal service unavailable",
		})
		return
	}

	// Read plan from middleware context (set by InjectEffectivePlan + RequirePlan)
	planInterface, exists := ctx.Get("subscription_plan")
	if !exists {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "No active subscription plan",
		})
		return
	}
	plan, ok := planInterface.(*paymentModels.SubscriptionPlan)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Invalid subscription plan",
		})
		return
	}

	// Create terminal session via composed session flow (distribution + size + features)
	composedInput := terminalDto.CreateComposedSessionInput{
		Distribution:     provisioning.Distribution,
		Size:             provisioning.Size,
		Features:         provisioning.Features,
		BuildFeatures:    provisioning.BuildFeatures,
		Terms:            terms,
		Name:             fmt.Sprintf("scenario-%s", scenario.Title),
		Hostname:         scenario.Hostname,
		Backend:          provisioning.Backend,
		RecordingEnabled: 1,
	}
	if orgID != nil {
		composedInput.OrganizationID = orgID.String()
	}
	// Persistence: SSOT lives in ResolveScenarioPersistenceMode (crash_traps →
	// ephemeral; plan-allows-persistence → persistent; else empty default).
	composedInput.PersistenceMode = terminalServices.ResolveScenarioPersistenceMode(scenario.CrashTraps, plan)

	terminalResp, termErr := sc.terminalService.StartComposedSession(userID, composedInput, plan)
	if termErr != nil {
		slog.Error("failed to create terminal session for scenario", "scenario", scenario.Name, "userID", userID, "err", termErr)
		// Budget exhaustion answers the same structured 403 the terminal
		// creation route emits — the launcher renders honest budget copy
		// from it instead of a generic failure.
		if httperrors.WriteBudgetRejection(ctx, termErr, userID) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: termErr.Error(),
		})
		return
	}

	// Create scenario session
	session, startErr := sc.sessionService.StartScenario(userID, scenarioID, terminalResp.SessionID, input.Locale)
	if startErr != nil {
		if stderrors.Is(startErr, services.ErrActiveSessionExists) {
			ctx.JSON(http.StatusConflict, gin.H{
				"error_code":    http.StatusConflict,
				"error_message": "A run of this scenario is already in progress.",
				"reason":        blockReasonSessionExists,
			})
			return
		}
		slog.Error("failed to start scenario session", "userID", userID, "scenarioID", scenarioID, "err", startErr)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to start scenario session. Please try again or contact support.",
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.LaunchScenarioResponse{
		TerminalSessionID: terminalResp.SessionID,
		ScenarioSessionID: session.ID.String(),
		Status:            session.Status,
		ProvisioningPhase: session.ProvisioningPhase,
	})
}

// scenarioBuildFeatures reads the features a scenario needs only while its
// container is being provisioned.
//
// A bad declaration is a scenario-authoring error, not a reason to refuse the
// launch: the run proceeds without them and whatever the setup needed them for
// fails visibly during provisioning, where the trainer can see it, rather than
// as an opaque refusal here.
func scenarioBuildFeatures(scenario models.Scenario) map[string]bool {
	features, err := scenario.GetBuildFeaturesMap()
	if err != nil {
		slog.Error("scenario has an unreadable build_features declaration; provisioning without it",
			"scenario", scenario.Name, "err", err)
		return nil
	}
	return features
}

// checkScenarioAccess checks if a user has access to a scenario via group or org assignments
func (sc *scenarioLaunchController) checkScenarioAccess(userID string, scenarioID uuid.UUID) (bool, error) {
	// Public scenarios are accessible to everyone
	var scenario models.Scenario
	if err := sc.db.First(&scenario, "id = ?", scenarioID).Error; err == nil && scenario.IsPublic {
		return true, nil
	}

	// Get user's group IDs
	var groupIDs []uuid.UUID
	if err := sc.db.Model(&groupModels.GroupMember{}).
		Where("user_id = ? AND is_active = true", userID).
		Pluck("group_id", &groupIDs).Error; err != nil {
		return false, err
	}

	// Get user's org IDs
	var orgIDs []uuid.UUID
	if err := sc.db.Model(&orgModels.OrganizationMember{}).
		Where("user_id = ? AND is_active = true", userID).
		Pluck("organization_id", &orgIDs).Error; err != nil {
		return false, err
	}

	// Build OR conditions for group and org scopes
	var conditions []string
	var args []interface{}

	args = append(args, scenarioID)

	if len(groupIDs) > 0 {
		conditions = append(conditions, "(scope = 'group' AND group_id IN ?)")
		args = append(args, groupIDs)
	}
	if len(orgIDs) > 0 {
		conditions = append(conditions, "(scope = 'org' AND organization_id IN ?)")
		args = append(args, orgIDs)
	}

	if len(conditions) == 0 {
		return false, nil
	}

	now := time.Now()
	combined := strings.Join(conditions, " OR ")

	var count int64
	if err := sc.db.Model(&models.ScenarioAssignment{}).
		Where("scenario_id = ? AND is_active = true AND (deadline IS NULL OR deadline > ?) AND (start_date IS NULL OR start_date <= ?)", args[0], now, now).
		Where(combined, args[1:]...).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// Only the scenario creator, an org manager, or a platform admin can use this endpoint.
func (sc *scenarioLaunchController) PreviewScenario(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	// Load scenario with CompatibleInstanceTypes and Steps
	var scenario models.Scenario
	if err := sc.db.Preload("CompatibleInstanceTypes").Preload("Steps").First(&scenario, scenarioID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found",
		})
		return
	}

	if sc.rejectIfArchived(ctx, &scenario) {
		return
	}

	// Build preview options
	var previewOpts []services.PreviewOption
	if sc.hasAdminRole(ctx) {
		previewOpts = append(previewOpts, services.WithAdminBypass())
	}
	// Inject org manager check
	previewOpts = append(previewOpts, services.WithOrgManagerCheck(func(uid string, orgID uuid.UUID) bool {
		var count int64
		sc.db.Model(&orgModels.OrganizationMember{}).
			Where("user_id = ? AND organization_id = ? AND is_active = true AND role IN ?", uid, orgID, []string{"manager", "owner"}).
			Count(&count)
		return count > 0
	}))

	// Read optional backend from query param or JSON body
	backend := ctx.Query("backend")
	if backend == "" {
		var body struct {
			Backend string `json:"backend"`
		}
		_ = ctx.ShouldBindJSON(&body)
		backend = body.Backend
	}

	// Get available distributions from tt-backend
	distributions, err := sc.terminalService.GetDistributions(backend)
	if err != nil {
		slog.Error("failed to get distributions from tt-backend", "err", err)
		ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
			ErrorCode:    http.StatusServiceUnavailable,
			ErrorMessage: "Terminal service unavailable",
		})
		return
	}

	// Fetch the size catalog (cached 60s) to enable launch-time size fallback
	// for scenarios with unknown InstanceType values. Non-fatal on failure.
	previewSizes, previewSizesErr := sc.terminalService.GetCatalogSizes()
	if previewSizesErr != nil {
		slog.Warn("failed to fetch sizes catalog, scenario size fallback disabled", "err", previewSizesErr)
		previewSizes = nil
	}

	// Find a compatible distribution for the scenario
	distName, size, features, distErr := resolveDistribution(scenario, distributions, previewSizes)
	if distErr != nil {
		slog.Error("no compatible distribution for scenario preview", "scenario", scenario.Name, "err", distErr)
		ctx.JSON(http.StatusConflict, &errors.APIError{
			ErrorCode:    http.StatusConflict,
			ErrorMessage: "No compatible environment available for this scenario",
		})
		return
	}

	// Auto-provision terminal key if missing
	_, keyErr := sc.terminalService.GetUserKey(userID)
	if keyErr != nil {
		user, userErr := casdoorsdk.GetUserByUserId(userID)
		keyName := "auto-" + userID
		if userErr == nil && user != nil && user.Email != "" {
			keyName = "auto-" + user.Email
		}
		if createErr := sc.terminalService.CreateUserKey(userID, keyName); createErr != nil {
			slog.Error("failed to create terminal key for user", "userID", userID, "err", createErr)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to provision terminal access",
			})
			return
		}
	}

	// Fetch terms from tt-backend
	terms, termsErr := sc.terminalService.GetTerms()
	if termsErr != nil {
		slog.Error("failed to fetch terminal terms", "err", termsErr)
		ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
			ErrorCode:    http.StatusServiceUnavailable,
			ErrorMessage: "Terminal service unavailable",
		})
		return
	}

	// Get user's effective plan for limit enforcement (org-context-aware)
	effectivePlanService := paymentServices.NewEffectivePlanService(sc.db)
	var orgIDForPlan *uuid.UUID
	if orgCtx := ctx.Query("organization_id"); orgCtx != "" {
		if parsed, parseErr := uuid.Parse(orgCtx); parseErr == nil {
			orgIDForPlan = &parsed
		}
	} else if orgFromCtx, exists := ctx.Get("org_context_id"); exists {
		if orgStr, ok := orgFromCtx.(string); ok && orgStr != "" {
			if parsed, parseErr := uuid.Parse(orgStr); parseErr == nil {
				orgIDForPlan = &parsed
			}
		}
	}
	planResult, planErr := effectivePlanService.GetUserEffectivePlan(userID, orgIDForPlan)
	if planErr != nil || planResult == nil || planResult.Plan == nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "No active subscription plan",
		})
		return
	}

	// Dunning gate (MANDATORY on every new session-creation route): preview
	// provisions a real terminal. This route has no InjectEffectivePlan
	// middleware, so gate on the manually-resolved result.
	if paymentMiddleware.GatePastDueBeyondGraceForResult(ctx, planResult) {
		return
	}

	// Terminal launch budget enforcement is performed downstream by
	// StartComposedSession via QuotaService.CheckBudget; no separate slot
	// check is needed here.

	// Create terminal session via composed session flow (distribution + size + features)
	composedInput := terminalDto.CreateComposedSessionInput{
		Distribution:     distName,
		Size:             size,
		Features:         features,
		BuildFeatures:    scenarioBuildFeatures(scenario),
		Terms:            terms,
		Name:             fmt.Sprintf("preview-%s", scenario.Title),
		Hostname:         scenario.Hostname,
		Backend:          backend,
		RecordingEnabled: 1,
	}
	if scenario.OrganizationID != nil {
		composedInput.OrganizationID = scenario.OrganizationID.String()
	}
	// Persistence: SSOT lives in ResolveScenarioPersistenceMode (shared with LaunchScenario).
	composedInput.PersistenceMode = terminalServices.ResolveScenarioPersistenceMode(scenario.CrashTraps, planResult.Plan)

	terminalResp, termErr := sc.terminalService.StartComposedSession(userID, composedInput, planResult.Plan)
	if termErr != nil {
		slog.Error("failed to create terminal session for scenario preview", "scenario", scenario.Name, "userID", userID, "err", termErr)
		if httperrors.WriteBudgetRejection(ctx, termErr, userID) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to start terminal session. Please try again or contact support.",
		})
		return
	}

	// Create preview session (skips assignment check, sets IsPreview)
	session, startErr := sc.sessionService.PreviewScenario(userID, scenarioID, terminalResp.SessionID, previewOpts...)
	if startErr != nil {
		slog.Error("failed to start preview session", "userID", userID, "scenarioID", scenarioID, "err", startErr)
		statusCode := http.StatusInternalServerError
		if strings.Contains(startErr.Error(), "not authorized") {
			statusCode = http.StatusForbidden
		}
		ctx.JSON(statusCode, &errors.APIError{
			ErrorCode:    statusCode,
			ErrorMessage: startErr.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.LaunchScenarioResponse{
		TerminalSessionID: terminalResp.SessionID,
		ScenarioSessionID: session.ID.String(),
		Status:            session.Status,
		ProvisioningPhase: session.ProvisioningPhase,
	})
}

// scenarioFeatures is everything the scenario's machine has to provide: the
// features the scenario declares, plus the renderer that a configured banner
// cannot be drawn without.
//
// The renderer is derived rather than declared — a trainer fills in an intro
// and an outro, not a capability list — and it is derived here, where the
// feature set is built, so that every caller gets the complete answer. Adding
// it at the call sites instead meant each one had to remember, and the launch
// and the preview are not the only two places that resolve a scenario.
//
// Before this, whether a banner animated depended on the image the scenario
// happened to land on: it worked where somebody had baked a renderer in, and
// printed plain text with no explanation anywhere else.
func scenarioFeatures(scenario models.Scenario) (map[string]bool, error) {
	features, err := scenario.GetFeaturesMap()
	if err != nil {
		return nil, fmt.Errorf("invalid scenario configuration: %w", err)
	}
	if !services.EffectsEnabled() {
		// A scenario may also name the feature itself. Dropping it here keeps the
		// switch's promise: off means no renderer is installed, whatever asked
		// for it, since with banners silenced nothing would ever use it.
		delete(features, effectsFeatureKey)
		return features, nil
	}
	if !services.ScenarioUsesEffects(&scenario) {
		return features, nil
	}
	if features == nil {
		features = map[string]bool{}
	}
	features[effectsFeatureKey] = true
	return features, nil
}

// effectsFeatureKey is tt-backend's catalog key for the banner renderer.
const effectsFeatureKey = "effects"
