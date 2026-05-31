package services

import (
	"fmt"
	"strings"
	"time"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	paymentModels "soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// terminalLifecycleService owns the local-session lifecycle concern: the
// state transitions a single terminal row goes through (stop / start /
// delete), access validation against the local row + tt-backend, and the
// local-row read helpers. It was carved out of terminalTrainerService to
// shrink that god object; terminalTrainerService embeds a
// *terminalLifecycleService and delegates the relevant interface methods to
// it.
//
// API session calls go through the shared terminalProxyClient. The stopped
// transition is delegated to terminalSyncService.markSessionStopped (the
// SSOT for that transition — there is exactly one definition, on the sync
// service), so lifecycle holds a reference to the sync service. Local-row
// reads/writes go through the repository; db is used for the plan lookup in
// StartSession, the group-owner access query, and the scenario-session
// auto-abandon updates.
type terminalLifecycleService struct {
	proxy      *terminalProxyClient
	sync       *terminalSyncService
	repository repositories.TerminalRepository
	db         *gorm.DB
}

// newTerminalLifecycleService returns a lifecycle service driving local-row
// transitions through the supplied proxy (tt-backend calls) and sync
// (markSessionStopped SSOT). The proxy, sync, repository and db are shared
// with the facade so all reuse the same HTTP configuration and DB.
func newTerminalLifecycleService(proxy *terminalProxyClient, sync *terminalSyncService, repository repositories.TerminalRepository, db *gorm.DB) *terminalLifecycleService {
	return &terminalLifecycleService{
		proxy:      proxy,
		sync:       sync,
		repository: repository,
		db:         db,
	}
}

func (l *terminalLifecycleService) GetSessionInfo(sessionID string) (*models.Terminal, error) {
	return l.repository.GetTerminalSessionByID(sessionID)
}

func (l *terminalLifecycleService) GetTerminalByUUID(terminalUUID string) (*models.Terminal, error) {
	return l.repository.GetTerminalByUUID(terminalUUID)
}

// GetActiveUserSessions récupère toutes les sessions actives d'un utilisateur
func (l *terminalLifecycleService) GetActiveUserSessions(userID string) (*[]models.Terminal, error) {
	return l.repository.GetTerminalSessionsByUserID(userID, true)
}

// StopSession arrête une session en appelant l'endpoint /stop de tt-backend.
// Le comportement de la ligne locale dépend du PersistenceMode :
//
//   - persistent : tt-backend conserve le disque, la session est résumable.
//     On délègue à markSessionStopped, qui passe state=StateStopped et étend
//     ExpiresAt à la fenêtre de reap réelle (idleUntil renvoyé par /stop, ou
//     fallback plan.MaxSessionDurationMinutes). Tant que la ligne est
//     StateStopped non supprimée, OccupiesSlotScope la compte dans le budget —
//     l'utilisateur garde sa capacité réservée jusqu'à ce que sync (étape 5b)
//     constate la disparition côté tt-backend et marque la ligne deleted.
//
//   - ephemeral (ou non renseigné) : tt-backend détruit le conteneur
//     immédiatement, il n'y a plus rien à garder en ligne. On passe
//     directement state=StateDeleted — la ligne devient une pierre tombale,
//     ExpiresAt et IdleUntil sont laissés tels quels. Pas de passage par
//     StateStopped : ce serait un état fantôme (aucun conteneur à reprendre).
//     OccupiesSlotScope sort la ligne dès la transition deleted.
//
// Aucune métrique de quota n'est mise à jour ici : la capacité terminale
// est exclusivement régie par le moteur de budget CPU/RAM
// (QuotaService.CheckBudget via OccupiesSlotScope), qui lit en direct.
func (l *terminalLifecycleService) StopSession(sessionID string) error {
	utils.Debug("StopSession called for session %s", sessionID)

	terminal, err := l.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	utils.Debug("Session %s current state: %s (persistence_mode=%s)",
		sessionID, terminal.State, terminal.PersistenceMode)

	// 1. Appeler le nouvel endpoint /sessions/{id}/stop de tt-backend.
	idleUntil, err := l.proxy.stopSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey)
	if err != nil {
		// Log mais on continue : la ligne locale doit refléter l'intention
		// même si tt-backend est temporairement injoignable. Pour une session
		// persistante, on garde l'expires_at actuel (best-effort) ; pour une
		// éphémère, on la marque deleted comme si la destruction avait eu lieu.
		utils.Warn("failed to stop session in Terminal Trainer API: %v", err)
	}

	// 2. Brancher sur le mode de persistance.
	if terminal.PersistenceMode == "persistent" {
		// Persistent : markSessionStopped est la SSOT — même chemin que la
		// propagation depuis sync (étape 5a) quand tt-backend signale stop.
		l.sync.markSessionStopped(terminal, idleUntil)
	} else {
		// Ephemeral (ou mode vide / inconnu) : le conteneur n'existe plus
		// côté tt-backend, la ligne locale doit le refléter directement —
		// pas de transition StateStopped intermédiaire. ExpiresAt/IdleUntil
		// sont laissés tels quels : la ligne est une pierre tombale, les
		// filtres aval (OccupiesSlotScope) la sortent dès le state=StateDeleted.
		utils.Debug("Marking ephemeral session %s as deleted (container destroyed by tt-backend)", sessionID)
		terminal.State = models.StateDeleted
	}

	if err := l.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s state: %v", sessionID, err)
		return err
	}

	// 3. Auto-abandon any active scenario sessions linked to this terminal
	result := l.db.Model(&struct{}{}).Table("scenario_sessions").
		Where("terminal_session_id = ? AND status IN ?", sessionID, []string{"active", "provisioning", "in_progress"}).
		Update("status", "abandoned")
	if result.Error != nil {
		utils.Warn("failed to abandon scenario sessions for terminal %s: %v", sessionID, result.Error)
	} else if result.RowsAffected > 0 {
		utils.Debug("Auto-abandoned %d scenario session(s) for stopped terminal %s", result.RowsAffected, sessionID)
	}

	return nil
}

// resolvePlanExpirySeconds is the SSOT for converting a plan's
// MaxSessionDurationMinutes into the `expiry` seconds value posted to
// tt-backend. Both the create path (StartComposedSession) and the resume
// path (StartSession) must derive expiry through this helper so the plan
// cap is honored uniformly.
//
// Returns 0 when the plan is nil or has no positive duration cap — callers
// should then OMIT the expiry field on the wire so tt-backend can fall
// back to its instance config default.
func resolvePlanExpirySeconds(plan *paymentModels.SubscriptionPlan) int {
	if plan == nil || plan.MaxSessionDurationMinutes <= 0 {
		return 0
	}
	return plan.MaxSessionDurationMinutes * 60
}

// StartSession reprend une session précédemment arrêtée via l'endpoint
// /sessions/{id}/start de tt-backend. Le disque est restauré côté backend.
//
// La réponse de tt-backend porte le nouveau expires_at (unix seconds) calculé
// côté backend après réinitialisation du timer d'auto-stop. On le mire dans
// terminal.ExpiresAt pour que le front voie immédiatement la nouvelle échéance
// (sinon il affiche "Session expirée" sur une session qui vient d'être reprise,
// jusqu'à la prochaine synchro). Si tt-backend ne renvoie pas d'expires_at
// (instance sans expiry fini), on conserve la valeur actuelle.
//
// L'expiry envoyé à tt-backend dérive de la SubscriptionPlan référencée par
// le terminal (terminal.SubscriptionPlanID). Sans cela, le resume retombait
// sur la valeur par défaut de tt-backend (~1h), bypassant la limite du plan
// (bug observé avec MaxSessionDurationMinutes=1: création 60s OK, reprise 1h).
// Si SubscriptionPlanID est nil (ancien terminal créé avant que le champ
// existe), on n'envoie pas d'expiry et tt-backend utilise son défaut.
func (l *terminalLifecycleService) StartSession(sessionID string) error {
	utils.Debug("StartSession called for session %s", sessionID)

	terminal, err := l.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Resolve the plan-derived expiry cap. When the terminal predates the
	// SubscriptionPlanID column (legacy row), we leave expirySeconds=0 so
	// the call sends no expiry field and tt-backend uses its default.
	expirySeconds := 0
	if terminal.SubscriptionPlanID != nil {
		var plan paymentModels.SubscriptionPlan
		if err := l.db.First(&plan, "id = ?", *terminal.SubscriptionPlanID).Error; err == nil {
			expirySeconds = resolvePlanExpirySeconds(&plan)
		} else {
			utils.Warn("StartSession: failed to load plan %s for terminal %s: %v — falling back to tt-backend default expiry",
				terminal.SubscriptionPlanID.String(), sessionID, err)
		}
	}

	expiresAtUnix, err := l.proxy.startSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey, expirySeconds)
	if err != nil {
		return fmt.Errorf("failed to start session in Terminal Trainer API: %w", err)
	}

	terminal.State = models.StateRunning
	terminal.LastStartedAt = time.Now()
	terminal.IdleUntil = nil
	if expiresAtUnix > 0 {
		terminal.ExpiresAt = time.Unix(expiresAtUnix, 0)
	}
	if err := l.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s after start: %v", sessionID, err)
		return err
	}

	return nil
}

// DeleteSession supprime définitivement une session via DELETE /sessions/{id}
// de tt-backend, marque la ligne locale comme StateDeleted et abandonne tout
// scenario session lié.
//
// Aucune métrique de quota n'est touchée : la capacité terminale est
// exclusivement régie par le moteur de budget CPU/RAM
// (QuotaService.CheckBudget via OccupiesSlotScope). La ligne locale
// passée à state=StateDeleted sort automatiquement de la portée (qui sert
// à la fois au compte de slots et au budget, cf. D6') sans écriture
// supplémentaire.
func (l *terminalLifecycleService) DeleteSession(sessionID string) error {
	utils.Debug("DeleteSession called for session %s", sessionID)

	terminal, err := l.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := l.proxy.deleteSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey); err != nil {
		// Log mais on continue : la session locale doit être marquée StateDeleted
		// même si tt-backend est injoignable, sinon l'utilisateur reste bloqué
		// avec un slot consommé.
		utils.Warn("failed to delete session in Terminal Trainer API: %v", err)
	}

	terminal.State = models.StateDeleted
	if err := l.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s after delete: %v", sessionID, err)
		return err
	}

	// Auto-abandon any active scenario sessions linked to this terminal
	result := l.db.Model(&struct{}{}).Table("scenario_sessions").
		Where("terminal_session_id = ? AND status IN ?", sessionID, []string{"active", "provisioning", "in_progress"}).
		Update("status", "abandoned")
	if result.Error != nil {
		utils.Warn("failed to abandon scenario sessions for terminal %s: %v", sessionID, result.Error)
	} else if result.RowsAffected > 0 {
		utils.Debug("Auto-abandoned %d scenario session(s) for deleted terminal %s", result.RowsAffected, sessionID)
	}

	return nil
}

// HasTerminalAccess checks if a user has access to a terminal.
// Only terminal owners and group owners of the owner's group have access.
func (l *terminalLifecycleService) HasTerminalAccess(terminalIDOrSessionID, userID string) (bool, error) {
	// Try to get terminal by UUID first (most common case from API)
	terminal, err := l.repository.GetTerminalByUUID(terminalIDOrSessionID)
	if err != nil {
		// If UUID lookup fails, try SessionID lookup
		terminal, err = l.repository.GetTerminalSessionBySessionID(terminalIDOrSessionID)
		if err != nil {
			return false, fmt.Errorf("failed to get terminal: %w", err)
		}
		if terminal == nil {
			return false, fmt.Errorf("terminal not found")
		}
	}

	// The terminal owner always has access
	if terminal.UserID == userID {
		return true, nil
	}

	// Check if requesting user is a group owner with the terminal owner as member
	isGroupOwner, err := l.checkGroupOwnerAccess(terminal.UserID, userID)
	if err == nil && isGroupOwner {
		return true, nil
	}

	return false, nil
}

// checkGroupOwnerAccess checks if requestingUserID is the owner of any active group
// that terminalOwnerUserID is an active member of.
func (l *terminalLifecycleService) checkGroupOwnerAccess(terminalOwnerUserID, requestingUserID string) (bool, error) {
	var count int64
	err := l.db.Table("class_groups").
		Joins("JOIN group_members ON class_groups.id = group_members.group_id").
		Where("class_groups.owner_user_id = ?", requestingUserID).
		Where("group_members.user_id = ?", terminalOwnerUserID).
		Where("group_members.is_active = ?", true).
		Where("class_groups.is_active = ?", true).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ValidateSessionAccess checks if a session is accessible for console operations
// Returns: (isValid bool, reason string, error)
//   - isValid: true if session can be accessed, false otherwise
//   - reason: a denial reason string (empty when isValid). Either a terminal
//     state name (e.g. "stopped", or any TerminalState surfaced by the default
//     switch arm) or a non-state sentinel: "expired" (deleted / empty / expired
//     ephemeral) or "backend_offline". Kept as string, not TerminalState,
//     because the sentinels are not lifecycle states.
//   - error: any error encountered during validation
func (l *terminalLifecycleService) ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error) {
	// 1. Get session from local database
	terminal, err := l.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return false, "", fmt.Errorf("session not found: %w", err)
	}

	// 2. Check local lifecycle state.
	//
	// State is the canonical SSOT, populated by SyncUserSessions from
	// tt-backend's authoritative session.state. The legacy parallel Status
	// field has been removed — every reader and writer now agrees on State.
	switch terminal.State {
	case models.StateRunning:
		// continue to backend + expiration checks below
	case models.StateStopped:
		return false, string(models.StateStopped), nil
	case models.StateDeleted:
		// Preserve the existing wire format the FE maps to "Session ended".
		return false, "expired", nil
	case "":
		// Defensive fallback for rows that pre-date State being populated.
		// Treat as deleted — without State we cannot prove the session is
		// alive, so we conservatively deny access (matches what the legacy
		// Status="expired" path used to return).
		return false, "expired", nil
	default:
		// Surface other lifecycle states (paused, hibernating, resuming, ...)
		// to the caller; the middleware will 403 with the state name.
		return false, string(terminal.State), nil
	}

	// 3. Check backend online status
	if terminal.Backend != "" {
		online, err := l.proxy.IsBackendOnline(terminal.Backend)
		if err != nil {
			utils.Warn("failed to check backend status: %v", err)
		} else if !online {
			return false, "backend_offline", nil
		}
	}

	// 4. Check expiration time.
	//
	// For State=StateRunning sessions, ExpiresAt in the past means the sync
	// hasn't caught up yet — tt-backend's auto-stop will land on the next
	// poll. The handling diverges by PersistenceMode:
	//
	//   - persistent: the session is resumable. Report StateStopped so the
	//     lifecycle middleware's allowStopped branch lets Resume / Delete
	//     pass through during tt-backend's graceful auto-stop window.
	//     Otherwise the user gets a 410 when clicking Resume on a session
	//     that still exists and is about to (or has just) auto-stopped.
	//
	//   - ephemeral (default): the container is being destroyed; "expired"
	//     is correct — there is nothing to resume.
	if time.Now().After(terminal.ExpiresAt) {
		if terminal.PersistenceMode == "persistent" {
			return false, string(models.StateStopped), nil
		}
		return false, "expired", nil
	}

	// 4. Optional API verification (for critical operations)
	if checkAPI {
		apiInfo, err := l.proxy.GetSessionInfoFromAPI(sessionID)
		if err != nil {
			// Handle 404 = session doesn't exist in Terminal Trainer
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				terminal.State = models.StateDeleted
				updateErr := l.repository.UpdateTerminalSession(terminal)
				if updateErr != nil {
					utils.Warn("failed to update session %s state after API 404: %v", sessionID, updateErr)
				}
				return false, "expired", nil
			}
			// For other API errors, return error but don't block access
			// This allows fail-open behavior when API is unavailable
			utils.Warn("API validation failed for session %s: %v", sessionID, err)
			return false, "", fmt.Errorf("failed to validate session with API: %w", err)
		}

		// Map InstanceCreationStatus from /info endpoint to terminal lifecycle state.
		// /info returns InstanceCreationStatus: 0=started (instance running), 6=expired (instance gone).
		// These are different from SessionStatus (0=active, 1=expired) used by /sessions.
		apiStateName := models.StateDeleted
		if apiInfo.Status == 0 {
			apiStateName = models.StateRunning
		}

		// Sync state if mismatch detected
		if apiStateName != terminal.State {
			previousState := terminal.State
			terminal.State = apiStateName
			err := l.repository.UpdateTerminalSession(terminal)
			if err != nil {
				utils.Warn("failed to sync session %s state from '%s' to '%s': %v",
					sessionID, previousState, apiStateName, err)
			}
			if terminal.State == models.StateRunning {
				return true, "active", nil
			}
			return false, "expired", nil
		}
	}

	return true, "active", nil
}
