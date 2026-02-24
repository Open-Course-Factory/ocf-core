package terminalController

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"soli/formations/src/auth/casdoor"
	config "soli/formations/src/configuration"
	"time"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type TerminalController interface {
	// Méthodes génériques (héritées)
	AddEntity(ctx *gin.Context)
	EditEntity(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)

	// Méthodes spécialisées Terminal Trainer
	StartSession(ctx *gin.Context)
	ConnectConsole(ctx *gin.Context)
	StopSession(ctx *gin.Context)
	GetUserSessions(ctx *gin.Context)

	// Méthodes de partage de terminaux
	ShareTerminal(ctx *gin.Context)
	RevokeTerminalAccess(ctx *gin.Context)
	GetTerminalShares(ctx *gin.Context)
	GetSharedTerminals(ctx *gin.Context)
	GetSharedTerminalInfo(ctx *gin.Context)

	// Méthodes de masquage de terminaux
	HideTerminal(ctx *gin.Context)
	UnhideTerminal(ctx *gin.Context)

	// Méthodes de synchronisation
	SyncSession(ctx *gin.Context)
	SyncAllSessions(ctx *gin.Context)
	SyncUserSessions(ctx *gin.Context)
	GetSessionStatus(ctx *gin.Context)
	GetSyncStatistics(ctx *gin.Context)

	// Méthodes de configuration
	GetInstanceTypes(ctx *gin.Context)

	// Méthodes de métriques
	GetServerMetrics(ctx *gin.Context)

	// Méthodes de correction des permissions
	FixTerminalHidePermissions(ctx *gin.Context)

	// Bulk operations
	BulkCreateTerminalsForGroup(ctx *gin.Context)

	// Enum service endpoints
	GetEnumStatus(ctx *gin.Context)
	RefreshEnums(ctx *gin.Context)

	// Backend management
	GetBackends(ctx *gin.Context)
	SetDefaultBackend(ctx *gin.Context)

	// Session access validation
	GetAccessStatus(ctx *gin.Context)

	// Command history
	GetSessionHistory(ctx *gin.Context)
	DeleteSessionHistory(ctx *gin.Context)
	DeleteAllUserHistory(ctx *gin.Context)

	// Organization session management
	GetOrganizationTerminalSessions(ctx *gin.Context)

	// Group command history
	GetGroupCommandHistory(ctx *gin.Context)

	// Group command history stats
	GetGroupCommandHistoryStats(ctx *gin.Context)

	// Consent status
	GetConsentStatus(ctx *gin.Context)
}

type terminalController struct {
	controller.GenericController
	terminalTrainerURL string
	apiVersion         string
	terminalType       string
	service            services.TerminalTrainerService
}

func NewTerminalController(db *gorm.DB) TerminalController {
	apiVersion := os.Getenv("TERMINAL_TRAINER_API_VERSION")
	if apiVersion == "" {
		apiVersion = "1.0" // default version
	}

	terminalType := os.Getenv("TERMINAL_TRAINER_TYPE")
	if terminalType == "" {
		terminalType = "" // no prefix by default
	}

	return &terminalController{
		GenericController:  controller.NewGenericController(db, casdoor.Enforcer),
		terminalTrainerURL: os.Getenv("TERMINAL_TRAINER_URL"),
		apiVersion:         apiVersion,
		terminalType:       terminalType,
		service:            services.NewTerminalTrainerService(db),
	}
}

// hasTerminalAccess vérifie si un utilisateur a accès à un terminal avec le niveau requis
func (tc *terminalController) hasTerminalAccess(ctx *gin.Context, terminalID, userID, requiredLevel string) (bool, error) {
	// Vérifier d'abord si l'utilisateur est admin
	userRoles := ctx.GetStringSlice("userRoles")
	for _, role := range userRoles {
		if role == "administrator" {
			return true, nil
		}
	}

	// Utiliser le service pour vérifier l'accès (propriétaire ou partagé)
	return tc.service.HasTerminalAccess(terminalID, userID, requiredLevel)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // No origin header (e.g. non-browser clients)
		}
		return config.IsOriginAllowed(origin)
	},
}

// DeleteEntity override pour gérer les sessions de terminal avec hard delete
func (tc *terminalController) DeleteEntity(ctx *gin.Context) {
	tc.GenericController.DeleteEntity(ctx, false)
}

// Start Terminal Session godoc
//
//	@Summary		Démarrer une session terminal
//	@Description	Démarre une nouvelle session de terminal via Terminal Trainer
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			session	body	dto.CreateTerminalSessionInput	true	"Terminal session input"
//	@Security		Bearer
//	@Success		200	{object}	dto.TerminalSessionResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Terminal trainer error"
//	@Router			/terminals/start-session [post]
func (tc *terminalController) StartSession(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	sessionInput := dto.CreateTerminalSessionInput{}
	if err := ctx.ShouldBindJSON(&sessionInput); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Get subscription plan from middleware context
	planInterface, exists := ctx.Get("subscription_plan")
	if !exists {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Subscription plan not found in context",
		})
		return
	}

	sessionResponse, err := tc.service.StartSessionWithPlan(userId, sessionInput, planInterface)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, sessionResponse)
}

// Connect Console godoc
//
//	@Summary		Connexion console WebSocket
//	@Description	Établit une connexion WebSocket vers la console du terminal
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"Session ID"
//	@Param			width	query	string	false	"Console width"
//	@Param			height	query	string	false	"Console height"
//	@Security		Bearer
//	@Success		101	{string}	string	"Switching Protocols (WebSocket)"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/terminals/{id}/console [get]
func (tc *terminalController) ConnectConsole(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Session ID is required",
		})
		return
	}

	userId := ctx.GetString("userId")

	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}

	// Vérifier les droits d'accès (read level minimum pour la console)
	hasAccess, accessErr := tc.hasTerminalAccess(ctx, sessionID, userId, models.AccessLevelRead)
	if accessErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to check access",
		})
		return
	}
	if !hasAccess {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied to this session",
		})
		return
	}

	// NEW: Validate session state with API verification (critical operation)
	isValid, reason, err := tc.service.ValidateSessionAccess(sessionID, true) // Force API check
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to validate session status: " + err.Error(),
		})
		return
	}

	if !isValid {
		if reason == "backend_offline" {
			ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
				ErrorCode:    http.StatusServiceUnavailable,
				ErrorMessage: fmt.Sprintf("Session's backend '%s' is currently unavailable", terminal.Backend),
			})
		} else if reason == "expired" {
			ctx.JSON(http.StatusGone, &errors.APIError{
				ErrorCode:    http.StatusGone,
				ErrorMessage: "Terminal session has expired and is no longer accessible",
			})
		} else if reason == "stopped" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Terminal session has been stopped and is no longer accessible",
			})
		} else {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Terminal session is not in an active state: " + reason,
			})
		}
		return
	}

	// Récupérer la clé API appropriée pour la connexion
	var userKey *models.UserTerminalKey
	var keyErr error

	if terminal.UserID == userId {
		// L'utilisateur est le propriétaire, utiliser sa propre clé
		userKey, keyErr = tc.service.GetUserKey(userId)
	} else {
		// L'utilisateur accède à un terminal partagé, utiliser la clé du propriétaire
		userKey, keyErr = tc.service.GetUserKey(terminal.UserID)
	}

	if keyErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Terminal owner's API key not found",
		})
		return
	}

	// Construire l'URL WebSocket du Terminal Trainer
	terminalTrainerWSURL, err := url.Parse(tc.terminalTrainerURL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Invalid terminal trainer URL",
		})
		return
	}

	if terminalTrainerWSURL.Scheme == "https" {
		terminalTrainerWSURL.Scheme = "wss"
	} else {
		terminalTrainerWSURL.Scheme = "ws"
	}
	// Construire le chemin avec version et type d'instance dynamique
	path := fmt.Sprintf("/%s", tc.apiVersion)
	if terminal.InstanceType != "" {
		path += fmt.Sprintf("/%s", terminal.InstanceType)
	} else if tc.terminalType != "" {
		path += fmt.Sprintf("/%s", tc.terminalType)
	}
	path += "/console"
	terminalTrainerWSURL.Path = path

	q := terminalTrainerWSURL.Query()
	q.Set("id", terminal.SessionID)
	if width := ctx.Query("width"); width != "" {
		q.Set("width", width)
	}
	if height := ctx.Query("height"); height != "" {
		q.Set("height", height)
	}
	terminalTrainerWSURL.RawQuery = q.Encode()

	// Upgrade la connexion cliente vers WebSocket
	clientConn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "WebSocket upgrade failed",
		})
		return
	}
	defer clientConn.Close()

	// Se connecter au Terminal Trainer
	headers := make(http.Header)
	headers.Set("X-API-Key", userKey.APIKey)

	terminalConn, _, err := websocket.DefaultDialer.Dial(terminalTrainerWSURL.String(), headers)
	if err != nil {
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
				"Failed to connect to terminal trainer"))
		return
	}
	defer terminalConn.Close()

	// Proxy bidirectionnel
	go func() {
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				break
			}
			if err := terminalConn.WriteMessage(messageType, data); err != nil {
				break
			}
		}
	}()

	for {
		messageType, data, err := terminalConn.ReadMessage()
		if err != nil {
			break
		}
		if err := clientConn.WriteMessage(messageType, data); err != nil {
			break
		}
	}
}

// Stop Session godoc
//
//	@Summary		Arrêter une session terminal
//	@Description	Arrête une session de terminal active et la termine côté Terminal Trainer
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/terminals/{id}/stop [post]
func (tc *terminalController) StopSession(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userId := ctx.GetString("userId")

	terminal, err := tc.service.GetSessionInfo(terminalID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}

	// Vérifier les droits d'accès (admin level requis pour arrêter)
	hasAccess, err := tc.hasTerminalAccess(ctx, terminalID, userId, models.AccessLevelOwner)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to check access",
		})
		return
	}
	if !hasAccess {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied to this session",
		})
		return
	}

	// Arrêter la session (maintenant ça appelle aussi l'API externe)
	if err := tc.service.StopSession(terminal.SessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Session stopped successfully"})
}

// Get User Sessions godoc
//
//	@Summary		Sessions utilisateur
//	@Description	Récupère toutes les sessions actives d'un utilisateur ou les sessions partagées avec un groupe
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			include_hidden	query	bool	false	"Include hidden terminals"
//	@Param			group_id		query	string	false	"Filter terminals shared with this group (UUID)"
//	@Security		Bearer
//	@Success		200	{array}		dto.TerminalOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/user-sessions [get]
func (tc *terminalController) GetUserSessions(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	includeHidden := ctx.Query("include_hidden") == "true"
	groupID := ctx.Query("group_id")
	organizationID := ctx.Query("organization_id")

	var terminals *[]models.Terminal
	var err error

	// If group_id is provided, return terminals shared with that group
	if groupID != "" {
		terminals, err = tc.service.GetRepository().GetTerminalSessionsSharedWithGroup(groupID, includeHidden)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: fmt.Sprintf("Invalid group_id: %v", err),
			})
			return
		}
	} else {
		// Default behavior: return user's own terminals
		// Pour les admins, permettre de voir les sessions d'autres utilisateurs
		targetUserID := ctx.Query("user_id")
		if targetUserID != "" {
			userRoles := ctx.GetStringSlice("userRoles")
			isAdmin := false
			for _, role := range userRoles {
				if role == "administrator" {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				ctx.JSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: "Only administrators can view other users' sessions",
				})
				return
			}
			userId = targetUserID
		}

		// If organization_id provided, filter by org
		if organizationID != "" {
			orgUUID, parseErr := uuid.Parse(organizationID)
			if parseErr != nil {
				ctx.JSON(http.StatusBadRequest, &errors.APIError{
					ErrorCode:    http.StatusBadRequest,
					ErrorMessage: fmt.Sprintf("Invalid organization_id: %v", parseErr),
				})
				return
			}
			terminals, err = tc.service.GetRepository().GetTerminalSessionsByUserIDAndOrg(userId, &orgUUID, false)
		} else {
			// Récupérer les sessions de l'utilisateur avec gestion des masquées (toutes les sessions, pas seulement les actives)
			terminals, err = tc.service.GetRepository().GetTerminalSessionsByUserIDWithHidden(userId, false, includeHidden)
		}
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			return
		}
	}

	// Convertir vers DTOs
	var terminalOutputs []dto.TerminalOutput
	for _, terminal := range *terminals {
		terminalOutputs = append(terminalOutputs, dto.TerminalOutput{
			ID:              terminal.ID,
			SessionID:       terminal.SessionID,
			UserID:          terminal.UserID,
			Name:            terminal.Name,
			Status:          terminal.Status,
			ExpiresAt:       terminal.ExpiresAt,
			InstanceType:    terminal.InstanceType,
			MachineSize:     terminal.MachineSize,
			Backend:         terminal.Backend,
			OrganizationID:  terminal.OrganizationID,
			IsHiddenByOwner: terminal.IsHiddenByOwner,
			HiddenByOwnerAt: terminal.HiddenByOwnerAt,
			CreatedAt:       terminal.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, terminalOutputs)
}

// Sync Session godoc
//
//	@Summary		Synchroniser une session
//	@Description	Synchronise l'état d'une session avec l'API Terminal Trainer
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Session ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncSessionResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminals/{id}/sync [post]
func (tc *terminalController) SyncSession(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	userId := ctx.GetString("userId")

	// Vérifier que la session appartient à l'utilisateur
	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}

	// Vérifier les droits d'accès (read level minimum pour sync)
	hasAccess, err := tc.hasTerminalAccess(ctx, sessionID, userId, models.AccessLevelRead)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to check access",
		})
		return
	}
	if !hasAccess {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied to this session",
		})
		return
	}

	// Synchroniser via la méthode complète de synchronisation utilisateur
	syncResponse, err := tc.service.SyncUserSessions(terminal.UserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Sync failed: %v", err),
		})
		return
	}

	// Trouver le résultat pour cette session spécifique
	var sessionResult *dto.SyncSessionResponse
	for _, result := range syncResponse.SessionResults {
		if result.SessionID == sessionID {
			sessionResult = &result
			break
		}
	}

	if sessionResult == nil {
		// Session non trouvée dans les résultats, créer une réponse par défaut
		sessionResult = &dto.SyncSessionResponse{
			SessionID:      sessionID,
			PreviousStatus: terminal.Status,
			CurrentStatus:  terminal.Status,
			Updated:        false,
			LastSyncAt:     time.Now(),
		}
	}

	ctx.JSON(http.StatusOK, sessionResult)
}

// Sync All Sessions godoc
//
//	@Summary		Synchroniser toutes les sessions avec l'API comme source de vérité
//	@Description	Synchronise l'état de toutes les sessions en utilisant l'API Terminal Trainer comme référence
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncAllSessionsResponse
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminals/sync-all [post]
func (tc *terminalController) SyncAllSessions(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	userRoles := ctx.GetStringSlice("userRoles")
	globalSync := ctx.Param("admin")

	// Vérifier si l'utilisateur est admin
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}

	var response *dto.SyncAllSessionsResponse
	var err error

	if globalSync != "" && isAdmin {
		// Admin peut synchroniser toutes les sessions de tous les utilisateurs
		err = tc.service.SyncAllActiveSessions()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: fmt.Sprintf("Global sync failed: %v", err),
			})
			return
		}

		// Pour les admins, créer une réponse générique
		response = &dto.SyncAllSessionsResponse{
			TotalSessions:   -1, // Non compté pour éviter la complexité
			SyncedSessions:  -1,
			UpdatedSessions: -1,
			ErrorCount:      0,
			Errors:          nil,
			SessionResults:  nil,
			LastSyncAt:      time.Now(),
		}
	} else {
		// Utilisateur normal synchronise seulement ses sessions
		response, err = tc.service.SyncUserSessions(userId)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: fmt.Sprintf("User sync failed: %v", err),
			})
			return
		}
	}

	ctx.JSON(http.StatusOK, response)
}

// Sync User Sessions godoc
//
//	@Summary		Synchronisation complète d'un utilisateur
//	@Description	Synchronise toutes les sessions d'un utilisateur en utilisant l'API comme source de vérité
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query	string	false	"User ID (admin only)"
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncAllSessionsResponse
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminals/sync-user [post]
func (tc *terminalController) SyncUserSessions(ctx *gin.Context) {
	currentUserId := ctx.GetString("userId")
	targetUserId := ctx.Query("user_id")

	// Si pas de user_id spécifié, utiliser l'utilisateur actuel
	if targetUserId == "" {
		targetUserId = currentUserId
	}

	// Vérifier les permissions si ce n'est pas l'utilisateur lui-même
	if targetUserId != currentUserId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Only administrators can sync other users' sessions",
			})
			return
		}
	}

	// Effectuer la synchronisation complète
	response, err := tc.service.SyncUserSessions(targetUserId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("User sync failed: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// Get Session Status godoc
//
//	@Summary		Obtenir le statut détaillé d'une session
//	@Description	Compare le statut local et celui de l'API Terminal Trainer avec informations étendues
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Session ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.ExtendedSessionStatusResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		500	{object}	errors.APIError	"Status check error"
//	@Router			/terminals/{id}/status [get]
func (tc *terminalController) GetSessionStatus(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	userId := ctx.GetString("userId")

	// Récupérer la session locale
	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}

	// Vérifier les droits d'accès
	if terminal.UserID != userId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied to this session",
			})
			return
		}
	}

	// Récupérer TOUTES les sessions depuis l'API pour ce user
	userKey, err := tc.service.GetUserKey(terminal.UserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get user API key",
		})
		return
	}

	apiSessions, err := tc.service.GetAllSessionsFromAPI(userKey.APIKey)

	response := dto.ExtendedSessionStatusResponse{
		SessionID:       sessionID,
		Status:          terminal.Status,
		ExpiresAt:       terminal.ExpiresAt,
		LastChecked:     time.Now(),
		LocalStatus:     terminal.Status,
		ExistsInAPI:     false,
		ExistsLocally:   true,
		SyncRecommended: false,
	}

	if err != nil {
		response.APIStatus = "api_error"
		response.APIError = err.Error()
		response.SyncRecommended = true
	} else {
		// Chercher la session dans la réponse API
		var foundInAPI *dto.TerminalTrainerSession
		for _, apiSession := range apiSessions.Sessions {
			if apiSession.SessionID == sessionID {
				foundInAPI = &apiSession
				break
			}
		}

		if foundInAPI != nil {
			// Convert numeric status to semantic name
			enumService := tc.service.GetEnumService()
			apiStatusName := enumService.GetEnumName("session_status", int(foundInAPI.Status))

			response.ExistsInAPI = true
			response.APIStatus = apiStatusName
			response.APIExpiresAt = time.Unix(foundInAPI.ExpiresAt, 0)
			response.SyncRecommended = terminal.Status != apiStatusName
			response.StatusMatch = terminal.Status == apiStatusName
		} else {
			response.APIStatus = "not_found"
			response.SyncRecommended = true
			response.StatusMatch = false
		}
	}

	ctx.JSON(http.StatusOK, response)
}

// Get Sync Statistics godoc
//
//	@Summary		Obtenir des statistiques de synchronisation
//	@Description	Retourne des statistiques sur les sessions et la synchronisation
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query	string	false	"User ID (admin only)"
//	@Security		Bearer
//	@Success		200	{object}	map[string]any
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Error getting statistics"
//	@Router			/terminals/sync-stats [get]
func (tc *terminalController) GetSyncStatistics(ctx *gin.Context) {
	currentUserId := ctx.GetString("userId")
	targetUserId := ctx.Query("user_id")
	userRoles := ctx.GetStringSlice("userRoles")

	// Vérifier les permissions
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}

	// Si pas admin et demande stats d'un autre user, refuser
	if targetUserId != "" && targetUserId != currentUserId && !isAdmin {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied",
		})
		return
	}

	// Si pas admin, forcer à ses propres stats
	if !isAdmin {
		targetUserId = currentUserId
	}

	// Récupérer les statistiques
	stats, err := tc.service.GetRepository().GetSyncStatistics(targetUserId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to get statistics: %v", err),
		})
		return
	}

	response := map[string]any{
		"user_id":      targetUserId,
		"statistics":   stats,
		"generated_at": time.Now(),
	}

	ctx.JSON(http.StatusOK, response)
}

// Get Instance Types godoc
//
//	@Summary		Récupérer les types d'instances disponibles
//	@Description	Récupère la liste des types d'instances/préfixes disponibles depuis Terminal Trainer. Optionnellement filtré par backend.
//	@Tags			terminals
//	@Security		Bearer
//	@Accept			json
//	@Produce		json
//	@Param			backend	query		string	false	"Backend ID to filter instance types for"
//	@Success		200	{array}		dto.InstanceType
//	@Failure		500	{object}	errors.APIError	"Erreur interne du serveur"
//	@Router			/terminals/instance-types [get]
func (tc *terminalController) GetInstanceTypes(ctx *gin.Context) {
	backend := ctx.Query("backend")
	instanceTypes, err := tc.service.GetInstanceTypes(backend)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to get instance types: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, instanceTypes)
}

// Get Server Metrics godoc
//
//	@Summary		Récupérer les métriques du serveur Terminal Trainer
//	@Description	Récupère les métriques de CPU, RAM et disponibilité du serveur Terminal Trainer
//	@Tags			terminals
//	@Security		Bearer
//	@Accept			json
//	@Produce		json
//	@Param			nocache	query		bool	false	"Bypass cache for real-time data"
//	@Success		200		{object}	dto.ServerMetricsResponse
//	@Failure		500		{object}	errors.APIError	"Erreur interne du serveur"
//	@Router			/terminals/metrics [get]
func (tc *terminalController) GetServerMetrics(ctx *gin.Context) {
	nocache := ctx.Query("nocache") == "true" || ctx.Query("nocache") == "1"
	backend := ctx.Query("backend")

	metrics, err := tc.service.GetServerMetrics(nocache, backend)
	if err != nil {
		utils.Debug("GetServerMetrics failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get server metrics",
		})
		return
	}

	ctx.JSON(http.StatusOK, metrics)
}

// Terminal sharing endpoints implementation

// Share Terminal godoc
//
//	@Summary		Partager une session terminal avec un autre utilisateur
//	@Description	Partage l'accès d'une session terminal avec un autre utilisateur avec un niveau d'accès spécifique
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string						true	"Session ID"
//	@Param			request	body	dto.ShareTerminalRequest	true	"Share request"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string	"Terminal shared successfully"
//	@Failure		400	{object}	errors.APIError		"Bad request"
//	@Failure		403	{object}	errors.APIError		"Access denied"
//	@Failure		404	{object}	errors.APIError		"Terminal not found"
//	@Failure		500	{object}	errors.APIError		"Internal server error"
//	@Router			/terminals/{id}/share [post]
func (tc *terminalController) ShareTerminal(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userId := ctx.GetString("userId")

	var request dto.ShareTerminalRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Validate that at least one recipient is specified
	hasUser := request.SharedWithUserID != nil && *request.SharedWithUserID != ""
	hasGroup := request.SharedWithGroupID != nil

	if !hasUser && !hasGroup {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Must specify either shared_with_user_id or shared_with_group_id",
		})
		return
	}

	// Support both user and group sharing
	var sharedWithUserID string
	if request.SharedWithUserID != nil {
		sharedWithUserID = *request.SharedWithUserID
	}

	err := tc.service.ShareTerminal(terminalID, userId, sharedWithUserID, request.AccessLevel, request.ExpiresAt)
	if err != nil {
		if err.Error() == "terminal not found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if err.Error() == "only the terminal owner can share the terminal" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Terminal shared successfully"})
}

// Revoke Terminal Access godoc
//
//	@Summary		Révoquer l'accès d'un utilisateur à une session terminal
//	@Description	Révoque l'accès d'un utilisateur spécifique à une session terminal
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"Session ID"
//	@Param			user_id	path	string	true	"User ID to revoke access from"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string	"Access revoked successfully"
//	@Failure		400	{object}	errors.APIError		"Bad request"
//	@Failure		403	{object}	errors.APIError		"Access denied"
//	@Failure		404	{object}	errors.APIError		"Terminal or share not found"
//	@Failure		500	{object}	errors.APIError		"Internal server error"
//	@Router			/terminals/{id}/share/{user_id} [delete]
func (tc *terminalController) RevokeTerminalAccess(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	sharedWithUserID := ctx.Param("user_id")
	userId := ctx.GetString("userId")

	err := tc.service.RevokeTerminalAccess(terminalID, sharedWithUserID, userId)
	if err != nil {
		if err.Error() == "terminal not found" || err.Error() == "no active share found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if err.Error() == "only the terminal owner can revoke the terminal" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Access revoked successfully"})
}

// Get Terminal Shares godoc
//
//	@Summary		Obtenir les partages d'une session terminal
//	@Description	Récupère la liste des utilisateurs ayant accès à une session terminal
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.TerminalShareOutput	"List of terminal shares"
//	@Failure		400	{object}	errors.APIError			"Bad request"
//	@Failure		403	{object}	errors.APIError			"Access denied"
//	@Failure		404	{object}	errors.APIError			"Terminal not found"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminals/{id}/shares [get]
func (tc *terminalController) GetTerminalShares(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userId := ctx.GetString("userId")

	shares, err := tc.service.GetTerminalShares(terminalID, userId)
	if err != nil {
		if err.Error() == "terminal not found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if err.Error() == "only terminal owner can view shares" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTOs
	var shareOutputs []dto.TerminalShareOutput
	for _, share := range *shares {
		shareOutputs = append(shareOutputs, dto.TerminalShareOutput{
			ID:                  share.ID,
			TerminalID:          share.TerminalID,
			SharedWithUserID:    share.SharedWithUserID,
			SharedByUserID:      share.SharedByUserID,
			AccessLevel:         share.AccessLevel,
			ExpiresAt:           share.ExpiresAt,
			IsActive:            share.IsActive,
			IsHiddenByRecipient: share.IsHiddenByRecipient,
			HiddenAt:            share.HiddenAt,
			CreatedAt:           share.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, shareOutputs)
}

// Get Shared Terminals godoc
//
//	@Summary		Obtenir les sessions terminals partagées avec l'utilisateur
//	@Description	Récupère toutes les sessions terminals auxquelles l'utilisateur a accès via des partages
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			include_hidden	query	bool	false	"Include hidden terminals"
//	@Security		Bearer
//	@Success		200	{array}		dto.SharedTerminalInfo	"List of shared terminals"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminals/shared-with-me [get]
func (tc *terminalController) GetSharedTerminals(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	includeHidden := ctx.Query("include_hidden") == "true"

	terminals, err := tc.service.GetSharedTerminalsWithHidden(userId, includeHidden)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTOs avec informations de partage
	var sharedTerminalInfos []dto.SharedTerminalInfo
	for _, terminal := range *terminals {
		info, err := tc.service.GetSharedTerminalInfo(terminal.SessionID, userId)
		if err != nil {
			// Skip terminals we can't get info for, but don't fail the whole request
			continue
		}
		sharedTerminalInfos = append(sharedTerminalInfos, *info)
	}

	ctx.JSON(http.StatusOK, sharedTerminalInfos)
}

// Get Shared Terminal Info godoc
//
//	@Summary		Obtenir les informations détaillées d'une session terminal partagée
//	@Description	Récupère les informations détaillées d'une session terminal avec les détails de partage
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SharedTerminalInfo	"Detailed terminal information"
//	@Failure		400	{object}	errors.APIError			"Bad request"
//	@Failure		403	{object}	errors.APIError			"Access denied"
//	@Failure		404	{object}	errors.APIError			"Terminal not found"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminals/{id}/info [get]
func (tc *terminalController) GetSharedTerminalInfo(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userId := ctx.GetString("userId")

	info, err := tc.service.GetSharedTerminalInfo(terminalID, userId)
	if err != nil {
		if err.Error() == "terminal not found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if err.Error() == "access denied" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, info)
}

// Hide Terminal godoc
//
//	@Summary		Masquer une session terminal
//	@Description	Masque une session terminal pour l'utilisateur courant (propriétaire ou destinataire d'un partage)
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string	"Terminal hidden successfully"
//	@Failure		400	{object}	errors.APIError		"Cannot hide active terminal"
//	@Failure		403	{object}	errors.APIError		"Access denied"
//	@Failure		404	{object}	errors.APIError		"Terminal not found"
//	@Failure		500	{object}	errors.APIError		"Internal server error"
//	@Router			/terminals/{id}/hide [post]
func (tc *terminalController) HideTerminal(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userID := ctx.GetString("userId")

	err := tc.service.HideTerminal(terminalID, userID)
	if err != nil {
		switch err.Error() {
		case "terminal not found":
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		case "cannot hide active terminals":
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: err.Error(),
			})
			return
		case "access denied":
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		default:
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			return
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Terminal hidden successfully"})
}

// Unhide Terminal godoc
//
//	@Summary		Afficher à nouveau une session terminal
//	@Description	Affiche à nouveau une session terminal masquée pour l'utilisateur courant
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string	"Terminal unhidden successfully"
//	@Failure		403	{object}	errors.APIError		"Access denied"
//	@Failure		404	{object}	errors.APIError		"Terminal not found"
//	@Failure		500	{object}	errors.APIError		"Internal server error"
//	@Router			/terminals/{id}/hide [delete]
func (tc *terminalController) UnhideTerminal(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userID := ctx.GetString("userId")

	err := tc.service.UnhideTerminal(terminalID, userID)
	if err != nil {
		switch err.Error() {
		case "terminal not found":
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		case "access denied":
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		default:
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			return
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Terminal unhidden successfully"})
}

// Fix Terminal Hide Permissions godoc
//
//	@Summary		Corriger les permissions de masquage des terminaux
//	@Description	Corrige les permissions Casbin pour permettre à l'utilisateur de masquer ses terminaux et ceux partagés avec lui
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query	string	false	"User ID (admin only, sinon utilise l'utilisateur courant)"
//	@Security		Bearer
//	@Success		200	{object}	dto.FixPermissionsResponse	"Permissions corrigées avec succès"
//	@Failure		403	{object}	errors.APIError				"Accès refusé"
//	@Failure		500	{object}	errors.APIError				"Erreur interne du serveur"
//	@Router			/terminals/fix-hide-permissions [post]
func (tc *terminalController) FixTerminalHidePermissions(ctx *gin.Context) {
	currentUserID := ctx.GetString("userId")
	targetUserID := ctx.Query("user_id")
	userRoles := ctx.GetStringSlice("userRoles")

	// Si pas de user_id spécifié, utiliser l'utilisateur actuel
	if targetUserID == "" {
		targetUserID = currentUserID
	}

	// Vérifier les permissions si ce n'est pas l'utilisateur lui-même
	if targetUserID != currentUserID {
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Only administrators can fix permissions for other users",
			})
			return
		}
	}

	// Appeler le service pour corriger les permissions
	response, err := tc.service.FixTerminalHidePermissions(targetUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to fix permissions: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// Bulk Create Terminals For Group godoc
//
//	@Summary		Créer des terminaux pour tous les membres d'un groupe
//	@Description	Crée des sessions de terminal pour tous les membres actifs d'un groupe en une seule requête
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			groupId	path	string							true	"Group ID"
//	@Param			request	body	dto.BulkCreateTerminalsRequest	true	"Bulk creation request"
//	@Security		Bearer
//	@Success		200	{object}	dto.BulkCreateTerminalsResponse	"Bulk creation results"
//	@Failure		400	{object}	errors.APIError					"Bad request"
//	@Failure		403	{object}	errors.APIError					"Access denied"
//	@Failure		404	{object}	errors.APIError					"Group not found"
//	@Failure		500	{object}	errors.APIError					"Internal server error"
//	@Router			/class-groups/{groupId}/bulk-create-terminals [post]
func (tc *terminalController) BulkCreateTerminalsForGroup(ctx *gin.Context) {
	groupID := ctx.Param("groupId")
	requestingUserID := ctx.GetString("userId")

	var request dto.BulkCreateTerminalsRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Get subscription plan from middleware context
	planInterface, exists := ctx.Get("subscription_plan")
	if !exists {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Active subscription required to bulk create terminals",
		})
		return
	}

	// Get user roles for system admin check
	userRoles := ctx.GetStringSlice("userRoles")

	// Call service to create terminals
	response, err := tc.service.BulkCreateTerminalsForGroup(groupID, requestingUserID, userRoles, request, planInterface)
	if err != nil {
		// Determine appropriate status code based on error
		statusCode := http.StatusInternalServerError
		if err.Error() == "group not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "only group owner or admin can create bulk terminals" {
			statusCode = http.StatusForbidden
		} else if len(err.Error()) >= len("invalid group ID") && err.Error()[:len("invalid group ID")] == "invalid group ID" {
			statusCode = http.StatusBadRequest
		}

		ctx.JSON(statusCode, &errors.APIError{
			ErrorCode:    statusCode,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// GetEnumStatus returns the current status of the enum service (admin only)
func (tc *terminalController) GetEnumStatus(ctx *gin.Context) {
	enumService := tc.service.GetEnumService()
	status := enumService.GetStatus()

	ctx.JSON(http.StatusOK, status)
}

// RefreshEnums forces a refresh of enums from the Terminal Trainer API (admin only)
func (tc *terminalController) RefreshEnums(ctx *gin.Context) {
	enumService := tc.service.GetEnumService()

	err := enumService.RefreshEnums()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to refresh enums: %v", err),
		})
		return
	}

	// Return updated status
	status := enumService.GetStatus()
	ctx.JSON(http.StatusOK, status)
}

// Get Backends godoc
//
//	@Summary		Get available backends
//	@Description	Returns available Terminal Trainer backends, optionally filtered by organization
//	@Tags			terminals
//	@Security		Bearer
//	@Accept			json
//	@Produce		json
//	@Param			organization_id	query		string	false	"Organization ID to filter backends"
//	@Success		200				{array}		dto.BackendInfo
//	@Failure		400				{object}	errors.APIError	"Bad request"
//	@Failure		500				{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/backends [get]
func (tc *terminalController) GetBackends(ctx *gin.Context) {
	organizationID := ctx.Query("organization_id")

	var backends []dto.BackendInfo
	var err error

	if organizationID != "" {
		orgUUID, parseErr := uuid.Parse(organizationID)
		if parseErr != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid organization_id",
			})
			return
		}
		backends, err = tc.service.GetBackendsForOrganization(orgUUID)
	} else {
		// Unfiltered backend list requires admin role
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Admin access required to list all backends. Use ?organization_id= to filter.",
			})
			return
		}
		backends, err = tc.service.GetBackends()
	}

	if err != nil {
		utils.Debug("GetBackends failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get backends",
		})
		return
	}

	ctx.JSON(http.StatusOK, backends)
}

// Set Default Backend godoc
//
//	@Summary		Set system default backend
//	@Description	Sets the system-wide default backend (admin only)
//	@Tags			terminals
//	@Security		Bearer
//	@Accept			json
//	@Produce		json
//	@Param			backendId	path		string	true	"Backend ID"
//	@Success		200			{object}	dto.BackendInfo
//	@Failure		400			{object}	errors.APIError	"Backend is offline"
//	@Failure		403			{object}	errors.APIError	"Admin access required"
//	@Failure		404			{object}	errors.APIError	"Backend not found"
//	@Failure		500			{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/backends/{backendId}/set-default [patch]
func (tc *terminalController) SetDefaultBackend(ctx *gin.Context) {
	userRoles := ctx.GetStringSlice("userRoles")
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	backendID := ctx.Param("backendId")
	backend, err := tc.service.SetSystemDefaultBackend(backendID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if strings.Contains(err.Error(), "offline") {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, backend)
}

// Get Access Status godoc
//
//	@Summary		Check console accessibility
//	@Description	Check if a terminal session is accessible for console operations
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]any	"Access status information"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Terminal not found"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/{id}/access-status [get]
func (tc *terminalController) GetAccessStatus(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	userId := ctx.GetString("userId")

	// Check if user has access
	hasAccess, err := tc.service.HasTerminalAccess(sessionID, userId, models.AccessLevelRead)
	if err != nil {
		if err.Error() == "terminal not found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Terminal not found",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to check access: %v", err),
		})
		return
	}

	// Validate session state with API verification
	// If API call fails, fall back to local validation
	isValid, reason, validationErr := tc.service.ValidateSessionAccess(sessionID, true)
	if validationErr != nil {
		// API unavailable, try local validation
		isValid, reason, _ = tc.service.ValidateSessionAccess(sessionID, false)
	}

	// Determine denial reason
	var denialReason string
	if !hasAccess {
		denialReason = "no_permission"
	} else if !isValid {
		denialReason = reason
	}

	response := gin.H{
		"session_id":         sessionID,
		"has_permission":     hasAccess,
		"can_access_console": isValid && hasAccess,
		"session_status":     reason,
	}

	if denialReason != "" {
		response["denial_reason"] = denialReason
	}

	if validationErr != nil {
		response["validation_warning"] = validationErr.Error()
	}

	ctx.JSON(http.StatusOK, response)
}

// isSessionOwnerOrAdmin checks whether the current user owns the session, is an admin,
// or is an owner/manager of the organization associated with the terminal session.
func (tc *terminalController) isSessionOwnerOrAdmin(ctx *gin.Context, terminal *models.Terminal) bool {
	userId := ctx.GetString("userId")
	userRoles := ctx.GetStringSlice("userRoles")
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}
	return tc.service.IsUserAuthorizedForSession(userId, terminal, isAdmin)
}

// GetSessionHistory returns command history for a terminal session
//
//	@Summary		Get command history for a terminal session
//	@Description	Retrieves the command history recorded during a terminal session. Works for active, stopped, and expired sessions.
//	@Tags			terminal
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"Terminal session ID"
//	@Param			since	query	integer	false	"Unix timestamp to filter commands since"
//	@Param			format	query	string	false	"Response format: json or csv"
//	@Param			limit	query	integer	false	"Maximum number of commands to return"
//	@Param			offset	query	integer	false	"Number of commands to skip"
//	@Security		BearerAuth
//	@Success		200	{object}	dto.CommandHistoryResponse
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/{id}/history [get]
func (tc *terminalController) GetSessionHistory(ctx *gin.Context) {
	sessionID := ctx.Param("id")

	// Command history is accessible to session owner, admin, or any user with shared access to the terminal.
	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}
	if !tc.isSessionOwnerOrAdmin(ctx, terminal) {
		// Check if the user has shared access (any level, including read)
		userId := ctx.GetString("userId")
		hasSharedAccess, accessErr := tc.service.HasTerminalAccess(sessionID, userId, models.AccessLevelRead)
		if accessErr != nil || !hasSharedAccess {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Only session owner, admin, or shared users can access command history",
			})
			return
		}
	}

	var since *int64
	if sinceStr := ctx.Query("since"); sinceStr != "" {
		if sinceVal, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			if sinceVal < 0 {
				sinceVal = 0
			}
			since = &sinceVal
		}
	}
	format := ctx.Query("format")

	const maxHistoryLimit = 1000
	var limit, offset int
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	body, contentType, err := tc.service.GetSessionCommandHistory(sessionID, since, format, limit, offset)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Session not found",
			})
			return
		}
		utils.Debug("GetSessionHistory failed for session %s: %v", sessionID, err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get command history",
		})
		return
	}

	ctx.Data(http.StatusOK, contentType, body)
}

// DeleteSessionHistory deletes command history (RGPD right to erasure)
//
//	@Summary		Delete command history for a terminal session
//	@Description	Deletes the command history for a terminal session. Supports RGPD right to erasure. Only the session owner or an admin can delete history.
//	@Tags			terminal
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal session ID"
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]string	"Command history deleted successfully"
//	@Failure		403	{object}	errors.APIError		"Access denied"
//	@Failure		404	{object}	errors.APIError		"Session not found"
//	@Failure		500	{object}	errors.APIError		"Internal server error"
//	@Router			/terminals/{id}/history [delete]
func (tc *terminalController) DeleteSessionHistory(ctx *gin.Context) {
	sessionID := ctx.Param("id")

	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}

	if !tc.isSessionOwnerOrAdmin(ctx, terminal) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Only session owner or admin can delete command history",
		})
		return
	}

	if err := tc.service.DeleteSessionCommandHistory(sessionID); err != nil {
		utils.Debug("DeleteSessionHistory failed for session %s: %v", sessionID, err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to delete command history",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Command history deleted successfully"})
}

// DeleteAllUserHistory deletes all command history for the current user
//
//	@Summary		Delete all user command history
//	@Description	Deletes all recorded command history across all terminal sessions for the current user
//	@Tags			terminal
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]interface{}
//	@Failure		500	{object}	errors.APIError
//	@Router			/terminals/my-history [delete]
func (tc *terminalController) DeleteAllUserHistory(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	userKey, err := tc.service.GetUserKey(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get user API key",
		})
		return
	}

	sessionsCleared, err := tc.service.DeleteAllUserCommandHistory(userKey.APIKey)
	if err != nil {
		utils.Debug("DeleteAllUserHistory failed for user %s: %v", userId, err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to delete command history",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":          "All command history deleted successfully",
		"sessions_cleared": sessionsCleared,
	})
}

// GetOrganizationTerminalSessions lists all terminal sessions for an organization
//
//	@Summary		Get terminal sessions for an organization
//	@Description	Lists all terminal sessions belonging to an organization. Only accessible by organization owners and managers.
//	@Tags			terminal
//	@Accept			json
//	@Produce		json
//	@Param			orgId	path	string	true	"Organization ID"
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/organizations/{orgId}/terminal-sessions [get]
func (tc *terminalController) GetOrganizationTerminalSessions(ctx *gin.Context) {
	orgIDStr := ctx.Param("orgId")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Check if user is org owner/manager or admin
	userId := ctx.GetString("userId")
	userRoles := ctx.GetStringSlice("userRoles")
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}
	if !tc.service.IsUserOrgManagerOrAdmin(userId, orgID, isAdmin) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Only organization owners, managers, or admins can access this resource",
		})
		return
	}

	sessions, err := tc.service.GetOrganizationTerminalSessions(orgID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get organization sessions",
		})
		return
	}

	if sessions == nil {
		empty := make([]models.Terminal, 0)
		sessions = &empty
	}

	ctx.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(*sessions),
	})
}

// GetGroupCommandHistory returns aggregated command history for all members of a group
func (tc *terminalController) GetGroupCommandHistory(ctx *gin.Context) {
	groupID := ctx.Param("groupId")
	userID := ctx.GetString("userId")

	// Parse query params
	var since *int64
	if sinceStr := ctx.Query("since"); sinceStr != "" {
		if sinceVal, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			if sinceVal < 0 {
				sinceVal = 0
			}
			since = &sinceVal
		}
	}
	format := ctx.Query("format")

	const maxHistoryLimit = 1000
	limit := 50 // default
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	offset := 0
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	includeStopped := ctx.Query("include_stopped") == "true"

	body, contentType, err := tc.service.GetGroupCommandHistory(groupID, userID, since, format, limit, offset, includeStopped)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Group not found",
			})
			return
		}
		if strings.Contains(err.Error(), "unauthorized") {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		utils.Debug("GetGroupCommandHistory failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get group command history",
		})
		return
	}

	// For CSV, add Content-Disposition header
	if format == "csv" {
		ctx.Header("Content-Disposition", `attachment; filename="group-history.csv"`)
	}

	ctx.Data(http.StatusOK, contentType, body)
}

// GetGroupCommandHistoryStats returns aggregate command history statistics for all members of a group
func (tc *terminalController) GetGroupCommandHistoryStats(ctx *gin.Context) {
	groupID := ctx.Param("groupId")
	userID := ctx.GetString("userId")
	includeStopped := ctx.Query("include_stopped") == "true"

	body, contentType, err := tc.service.GetGroupCommandHistoryStats(groupID, userID, includeStopped)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Group not found",
			})
			return
		}
		if strings.Contains(err.Error(), "unauthorized") {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		utils.Debug("GetGroupCommandHistoryStats failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get group command history stats",
		})
		return
	}

	ctx.Data(http.StatusOK, contentType, body)
}

// GetConsentStatus returns whether the current user's recording consent is handled
// at the organization or group level (i.e., enrollment contract covers it).
func (tc *terminalController) GetConsentStatus(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	consentHandled, source, err := tc.service.GetUserConsentStatus(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to check consent status",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"consent_handled": consentHandled,
		"source":          source,
	})
}
