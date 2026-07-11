package services

import (
	"fmt"
	"strings"
	"time"

	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// terminalSyncService owns the session-reconciliation concern: it treats
// tt-backend's /sessions list as the source of truth and brings the local
// Terminal rows into agreement — creating missing running sessions,
// propagating lifecycle state (including the stopped transition via
// markSessionStopped), and soft-deleting rows whose container has been reaped.
//
// It was carved out of terminalTrainerService to shrink that god object;
// terminalTrainerService embeds a *terminalSyncService and delegates the
// relevant interface methods (SyncUserSessions, SyncAllActiveSessions) to it.
// markSessionStopped lives here too because it is the shared SSOT for the
// stopped-state transition — the facade's StopSession routes through
// tts.sync.markSessionStopped so there is exactly one definition.
//
// API session fetches go through the shared terminalProxyClient. Local-row
// reads/writes go through the repository; db is used for the plan lookup in
// the markSessionStopped fallback.
type terminalSyncService struct {
	proxy      *terminalProxyClient
	repository repositories.TerminalRepository
	db         *gorm.DB
}

// newTerminalSyncService returns a sync service that reconciles local rows
// against tt-backend through the supplied proxy. The proxy and repository are
// shared with the facade so both reuse the same HTTP configuration and DB.
func newTerminalSyncService(proxy *terminalProxyClient, repository repositories.TerminalRepository, db *gorm.DB) *terminalSyncService {
	return &terminalSyncService{
		proxy:      proxy,
		repository: repository,
		db:         db,
	}
}

// markSessionStopped is the SSOT for the stopped-state transition on a
// terminal row. Every code path that wants to mark a terminal stopped —
// user-initiated StopSession, auto-stop detected by SyncUserSessions step
// 5a, any future trigger — MUST call this helper so the row's invariants
// stay consistent:
//
//   - State = StateStopped
//   - ExpiresAt extended to the tt-backend reap deadline. Preference
//     order: (1) idleUntil from tt-backend, (2) time.Now() +
//     plan.MaxSessionDurationMinutes when idleUntil is nil, (3) leave
//     ExpiresAt untouched when neither value is available.
//   - IdleUntil populated when idleUntil is non-nil (for traceability /
//     UI display).
//
// The helper does NOT branch on persistence_mode (D6', supersedes D6):
// "a stop is a stop". The reservation is held until sync confirms
// tt-backend has reaped the container.
//
// Returns true iff any field on the terminal was modified, so the caller
// can decide whether to flush. Idempotent: a row already in the correct
// shape produces no spurious DB writes.
//
// Why .Local() on the assigned ExpiresAt: the JSON reader produces UTC
// time values (Z suffix). SQLite text-compares timestamps
// lexicographically and "Z"-formatted values do not match the "+02:00"
// local serializations written elsewhere in the codebase, so the scope
// would drop the row in tests. PostgreSQL handles the timezone
// implicitly, so production is fine — but matching the convention
// everywhere keeps the SQLite/Postgres behavior aligned.
func (s *terminalSyncService) markSessionStopped(
	terminal *models.Terminal,
	idleUntil *time.Time,
) bool {
	changed := false

	if terminal.State != models.StateStopped {
		terminal.State = models.StateStopped
		changed = true
	}

	if idleUntil != nil {
		idleUntilLocal := idleUntil.Local()
		if terminal.IdleUntil == nil || !terminal.IdleUntil.Equal(idleUntilLocal) {
			terminal.IdleUntil = &idleUntilLocal
			changed = true
		}
		newExpiry := idleUntilLocal
		if !terminal.ExpiresAt.Equal(newExpiry) {
			terminal.ExpiresAt = newExpiry
			changed = true
		}
		return changed
	}

	// Fallback: plan-derived window. Mirrors StartComposedSession + StartSession.
	if terminal.SubscriptionPlanID == nil {
		return changed
	}
	var plan paymentModels.SubscriptionPlan
	if err := s.db.First(&plan, "id = ?", *terminal.SubscriptionPlanID).Error; err != nil {
		utils.Warn("markSessionStopped: failed to load plan %s for terminal %s: %v — leaving ExpiresAt untouched",
			terminal.SubscriptionPlanID.String(), terminal.SessionID, err)
		return changed
	}
	if plan.MaxSessionDurationMinutes <= 0 {
		return changed
	}
	terminal.ExpiresAt = time.Now().Add(time.Duration(plan.MaxSessionDurationMinutes) * time.Minute).Local()
	return true
}

// SyncUserSessions synchronise toutes les sessions d'un utilisateur avec l'API comme source de vérité
func (s *terminalSyncService) SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error) {
	// 1. Récupérer la clé utilisateur
	userKey, err := s.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %w", err)
	}

	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// 2. Récupérer TOUTES les sessions depuis l'API Terminal Trainer pour tous les types d'instances
	apiSessions, err := s.proxy.getAllSessionsFromAllInstanceTypes(userKey.APIKey, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions from Terminal Trainer API: %w", err)
	}

	// 3. Récupérer les sessions locales pour cet utilisateur
	localSessions, err := s.repository.GetTerminalSessionsByUserID(userID, false)
	if err != nil {
		localSessions = &[]models.Terminal{} // Traiter comme liste vide si erreur
	}

	// 4. Créer des maps pour faciliter la comparaison
	localSessionsMap := make(map[string]*models.Terminal)
	for i := range *localSessions {
		session := &(*localSessions)[i]
		localSessionsMap[session.SessionID] = session
	}

	apiSessionsMap := make(map[string]*dto.TerminalTrainerSession)
	for i := range apiSessions.Sessions {
		session := &apiSessions.Sessions[i]
		apiSessionsMap[session.SessionID] = session
	}

	// 5. Synchronisation bidirectionnelle
	sessionResults := make([]dto.SyncSessionResponse, 0, len(apiSessionsMap)+len(localSessionsMap))
	errors := make([]string, 0, 8)
	syncedCount := 0
	updatedCount := 0
	createdCount := 0

	// 5a. Traiter les sessions qui existent côté API (source de vérité)
	for sessionID, apiSession := range apiSessionsMap {
		localSession := localSessionsMap[sessionID]

		// Map SessionStatus from /sessions endpoint to terminal lifecycle state.
		// /sessions returns SessionStatus: 0=active, 1=expired, 2=preallocated, 3+=deleted.
		// We translate to State-space directly: 0 → StateRunning, anything else → StateDeleted.
		// This is different from InstanceCreationStatus (0=started, 1=invalid_terms) used by /start and /info.
		apiStateName := models.StateDeleted
		if apiSession.Status == 0 {
			apiStateName = models.StateRunning
		}

		if localSession == nil {
			// Session existe côté API mais pas côté local
			// Ne recréer que les sessions actives (state=StateRunning), pas les expirées/arrêtées
			if apiStateName == models.StateRunning {
				utils.Debug("SyncUserSessions - Creating missing running session %s (api_status=%d, state=%s)",
					sessionID, apiSession.Status, apiStateName)
				err := s.createMissingLocalSession(userID, userKey, apiSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to create missing session %s: %v", sessionID, err))
				} else {
					sessionResults = append(sessionResults, dto.SyncSessionResponse{
						SessionID:     sessionID,
						PreviousState: "missing",
						CurrentState:  string(apiStateName),
						Updated:       true,
						LastSyncAt:    time.Now(),
					})
					syncedCount++
					updatedCount++
					createdCount++
				}
			} else {
				utils.Debug("SyncUserSessions - Ignoring non-running session %s (api_status=%d, state=%s) from API",
					sessionID, apiSession.Status, apiStateName)
				// Ajouter quand même aux résultats pour le suivi
				sessionResults = append(sessionResults, dto.SyncSessionResponse{
					SessionID:     sessionID,
					PreviousState: "missing",
					CurrentState:  fmt.Sprintf("ignored-%s", apiStateName),
					Updated:       false,
					LastSyncAt:    time.Now(),
				})
				syncedCount++
			}
		} else {
			// Session existe des deux côtés -> synchroniser le state
			previousState := localSession.State
			needsUpdate := false

			// Les états stopped (pause manuelle) et revoked (révocation billing,
			// issue #388) font autorité côté ocf-core : tt-backend continue de
			// lister le conteneur comme actif (TerminateUserTerminals ne met à
			// jour que la DB, il ne détruit pas le conteneur), donc une passe de
			// sync ne doit JAMAIS réécrire ces états vers la vérité API — sinon
			// un utilisateur révoqué récupérerait silencieusement sa session. Un
			// seul prédicat, appliqué partout où l'état API serait propagé sur la
			// ligne locale (mismatch direct ci-dessous ET propagation
			// apiSession.State plus bas).
			localStateIsAuthoritative := localSession.State == models.StateStopped ||
				localSession.State == models.StateRevoked

			utils.Debug("SyncUserSessions - Session %s: local='%s', api_status='%d' (target_state='%s')",
				sessionID, localSession.State, apiSession.Status, apiStateName)

			// Vérifier si le state a changé.
			if localSession.State != apiStateName && !localStateIsAuthoritative {
				utils.Debug("SyncUserSessions - State mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.State, apiStateName)
				localSession.State = apiStateName
				needsUpdate = true
			} else if localStateIsAuthoritative {
				utils.Debug("SyncUserSessions - Session %s is %s locally (authoritative), keeping local state",
					sessionID, localSession.State)
			}

			// Vérifier si la session a expiré selon la date.
			// Si tt-backend la voit encore StateRunning mais ExpiresAt est passé,
			// le sync local doit la marquer comme effacée.
			expiryTime := time.Unix(apiSession.ExpiresAt, 0)
			if time.Now().After(expiryTime) && apiStateName == models.StateRunning {
				utils.Debug("SyncUserSessions - Session %s expired by date, marking as deleted", sessionID)
				localSession.State = models.StateDeleted
				needsUpdate = true
			}

			// Propager persistence_mode depuis tt-backend (source de vérité
			// pour ce champ — il pilote l'affordance Resume côté UI mais ne
			// joue PAS dans la logique budget, cf. D6').
			if apiSession.PersistenceMode != "" && localSession.PersistenceMode != apiSession.PersistenceMode {
				utils.Debug("SyncUserSessions - PersistenceMode mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.PersistenceMode, apiSession.PersistenceMode)
				localSession.PersistenceMode = apiSession.PersistenceMode
				needsUpdate = true
			}

			// Propager l'état lifecycle depuis tt-backend (source de vérité).
			// Sans ça, une session auto-arrêtée côté tt-backend reste affichée
			// comme StateRunning en local et le front continue d'afficher
			// "Session expirée" au lieu d'offrir un bouton Reprendre.
			//
			// Quand l'état cible est StateStopped, on délègue à markSessionStopped
			// (SSOT) — même chemin que la StopSession utilisateur — ce qui
			// fixe state, IdleUntil ET ExpiresAt en une passe. Sans cette
			// extension, expires_at resterait à la deadline de création
			// (déjà dépassée puisque c'est précisément ce qui a déclenché
			// l'auto-stop), et OccupiesSlotScope laisserait tomber la ligne :
			// l'utilisateur perdrait la capacité réservée d'un coup, alors
			// que le conteneur est toujours résumable côté tt-backend.
			//
			// Garde SPÉCIFIQUE à StateRevoked (et non le prédicat
			// localStateIsAuthoritative complet) : la révocation billing laisse
			// le conteneur vivant, donc son reaper idle finit par le rapporter
			// "stopped" — sans cette garde, markSessionStopped ressusciterait la
			// ligne révoquée en stopped (resumable + ré-occupe le budget),
			// rendant sa session à un utilisateur révoqué. StateStopped, lui,
			// DOIT continuer à passer par markSessionStopped pour rafraîchir sa
			// deadline idle (fenêtre de reprise), d'où la garde ciblée.
			if apiSession.State == models.StateStopped && localSession.State != models.StateRevoked {
				var idleUntilPtr *time.Time
				if apiSession.IdleUntil > 0 {
					t := time.Unix(apiSession.IdleUntil, 0).Local()
					idleUntilPtr = &t
				}
				if s.markSessionStopped(localSession, idleUntilPtr) {
					needsUpdate = true
				}
			} else if apiSession.State != "" && localSession.State != apiSession.State && !localStateIsAuthoritative {
				utils.Debug("SyncUserSessions - State mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.State, apiSession.State)
				localSession.State = apiSession.State
				needsUpdate = true
				// Pour les états non-stopped, on continue de propager IdleUntil
				// tel quel — markSessionStopped ne s'applique qu'à la transition
				// vers StateStopped.
				if apiSession.IdleUntil > 0 {
					t := time.Unix(apiSession.IdleUntil, 0).UTC()
					if localSession.IdleUntil == nil || !localSession.IdleUntil.Equal(t) {
						localSession.IdleUntil = &t
					}
				}
			} else if apiSession.IdleUntil > 0 {
				// État inchangé mais idle_until potentiellement mis à jour
				// (ex : tt-backend a recalé la deadline pendant un running).
				t := time.Unix(apiSession.IdleUntil, 0).UTC()
				if localSession.IdleUntil == nil || !localSession.IdleUntil.Equal(t) {
					localSession.IdleUntil = &t
					needsUpdate = true
				}
			}

			if needsUpdate {
				utils.Debug("SyncUserSessions - Updating session %s state to '%s'", sessionID, localSession.State)
				err := s.repository.UpdateTerminalSession(localSession)
				if err != nil {
					utils.Error("SyncUserSessions - Failed to update session %s: %v", sessionID, err)
					errors = append(errors, fmt.Sprintf("Failed to update session %s: %v", sessionID, err))
				} else {
					utils.Debug("SyncUserSessions - Successfully updated session %s", sessionID)
					updatedCount++
				}
			}

			sessionResults = append(sessionResults, dto.SyncSessionResponse{
				SessionID:     sessionID,
				PreviousState: string(previousState),
				CurrentState:  string(localSession.State),
				Updated:       needsUpdate,
				LastSyncAt:    time.Now(),
			})
			syncedCount++
		}
	}

	// 5b. Traiter les sessions qui existent côté local mais pas côté API.
	//
	// tt-backend est la SSOT pour "ce conteneur existe-t-il ?". Si une ligne
	// locale n'apparaît plus dans /sessions (qui inclut déjà les rows expirées
	// via include_expired=true), c'est que le conteneur a été reapé : on doit
	// donc la marquer StateDeleted — peu importe son state local actuel.
	//
	// L'ancien skip sur state=StateStopped laissait des fantômes : une session
	// éphémère stoppée par l'utilisateur (ou auto-stoppée) restait visible
	// sur /terminal-sessions alors qu'aucun conteneur ne pouvait plus être
	// repris. Si le state local est déjà StateDeleted, on n'a rien à faire.
	expiredCount := 0
	for sessionID, localSession := range localSessionsMap {
		if _, exists := apiSessionsMap[sessionID]; !exists {
			if localSession.State != models.StateDeleted {
				previousState := localSession.State
				localSession.State = models.StateDeleted

				err := s.repository.UpdateTerminalSession(localSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to expire orphaned session %s: %v", sessionID, err))
				} else {
					sessionResults = append(sessionResults, dto.SyncSessionResponse{
						SessionID:     sessionID,
						PreviousState: string(previousState),
						CurrentState:  string(localSession.State),
						Updated:       true,
						LastSyncAt:    time.Now(),
					})
					updatedCount++
					expiredCount++
				}
			}
			syncedCount++
		}
	}

	// 6. Construire la réponse
	response := &dto.SyncAllSessionsResponse{
		TotalSessions:   len(apiSessions.Sessions),
		SyncedSessions:  syncedCount,
		UpdatedSessions: updatedCount,
		ErrorCount:      len(errors),
		Errors:          errors,
		SessionResults:  sessionResults,
		LastSyncAt:      time.Now(),
	}

	return response, nil
}

// createMissingLocalSession crée une session locale manquante basée sur les données de l'API
func (s *terminalSyncService) createMissingLocalSession(userID string, userKey *models.UserTerminalKey, apiSession *dto.TerminalTrainerSession) error {
	terminal := BuildTerminalFromAPISession(userID, userKey, apiSession)
	return s.repository.CreateTerminalSessionFromAPI(terminal)
}

// BuildTerminalFromAPISession materialises a Terminal from a tt-backend
// /sessions response. Exported (and side-effect free) so the sync path AND
// the unit tests can exercise the exact same denormalisation logic — the C5
// regression (size_cpu / size_memory_mb left at 0 on synced rows) is gated
// on this single builder.
//
// MachineSize is resolved through catalog.LookupSize (case-insensitive).
// Unknown size keys leave SizeCPU / SizeMemoryMB at 0 — matches the
// defensive pattern used for legacy rows that pre-date the denorm columns.
func BuildTerminalFromAPISession(userID string, userKey *models.UserTerminalKey, apiSession *dto.TerminalTrainerSession) *models.Terminal {
	expiresAt := time.Unix(apiSession.ExpiresAt, 0)

	// Map SessionStatus from /sessions endpoint to terminal lifecycle state.
	// /sessions returns SessionStatus: 0=active, 1=expired. Translate to State-space.
	stateName := models.StateDeleted
	if apiSession.Status == 0 {
		stateName = models.StateRunning
	}

	terminal := &models.Terminal{
		SessionID:         apiSession.SessionID,
		UserID:            userID,
		State:             stateName, // Terminal lifecycle state: StateRunning or StateDeleted
		ExpiresAt:         expiresAt,
		MachineSize:       apiSession.MachineSize, // Taille réelle depuis l'API
		Backend:           apiSession.Backend,
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	// Snapshot the catalog footprint so the budget summing query has a
	// non-zero size_cpu / size_memory_mb for synced rows. Without this the
	// row would be invisible to the budget aggregate even though it
	// occupies real resources on tt-backend (C5).
	if size, found := catalog.LookupSize(apiSession.MachineSize); found {
		terminal.SizeCPU = size.CPU
		terminal.SizeMemoryMB = size.MemoryMB
	} else if strings.TrimSpace(apiSession.MachineSize) != "" {
		// Unknown size key — log and continue with zeroes (matches the
		// legacy-row defensive pattern). Operators can backfill later if
		// the catalog grows to cover this key.
		utils.Warn("createMissingLocalSession: machine_size %q not in catalog; size denorm left at 0", apiSession.MachineSize)
	}

	// Propager le State de tt-backend (source de vérité authoritative) s'il
	// est explicitement fourni — il a la précédence sur notre traduction
	// depuis api_status.
	if apiSession.State != "" {
		terminal.State = apiSession.State
	}
	if apiSession.PersistenceMode != "" {
		terminal.PersistenceMode = apiSession.PersistenceMode
	}
	if apiSession.IdleUntil > 0 {
		t := time.Unix(apiSession.IdleUntil, 0).UTC()
		terminal.IdleUntil = &t
	}

	return terminal
}

// SyncAllActiveSessions - version améliorée qui utilise la nouvelle logique
func (s *terminalSyncService) SyncAllActiveSessions() error {
	// Récupérer tous les utilisateurs ayant des clés actives
	activeKeys, err := s.repository.GetAllActiveUserKeys()
	if err != nil {
		return fmt.Errorf("failed to get active user keys: %w", err)
	}

	var globalErrors []string
	for _, userKey := range *activeKeys {
		_, err := s.SyncUserSessions(userKey.UserID)
		if err != nil {
			globalErrors = append(globalErrors, fmt.Sprintf("User %s: %v", userKey.UserID, err))
		}
	}

	if len(globalErrors) > 0 {
		return fmt.Errorf("sync completed with errors: %v", globalErrors)
	}

	return nil
}
