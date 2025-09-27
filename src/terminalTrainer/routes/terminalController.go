package terminalController

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
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

	// Méthodes de synchronisation
	SyncSession(ctx *gin.Context)
	SyncAllSessions(ctx *gin.Context)
	SyncUserSessions(ctx *gin.Context)
	GetSessionStatus(ctx *gin.Context)
	GetSyncStatistics(ctx *gin.Context)

	// Méthodes de configuration
	GetInstanceTypes(ctx *gin.Context)
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
		GenericController:  controller.NewGenericController(db),
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
		return true
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			session	body	dto.CreateTerminalSessionInput	true	"Terminal session input"
//	@Security		Bearer
//	@Success		200	{object}	dto.TerminalSessionResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Terminal trainer error"
//	@Router			/terminal-sessions/start-session [post]
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

	sessionResponse, err := tc.service.StartSession(userId, sessionInput)
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
//	@Tags			terminal-sessions
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
//	@Router			/terminal-sessions/{id}/console [get]
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
	hasAccess, err := tc.hasTerminalAccess(ctx, sessionID, userId, "read")
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

	// Récupérer la clé API de l'utilisateur
	userKey, err := tc.service.GetUserKey(terminal.UserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "User key not found",
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/terminal-sessions/{id}/stop [post]
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
	hasAccess, err := tc.hasTerminalAccess(ctx, terminalID, userId, "admin")
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
//	@Description	Récupère toutes les sessions actives d'un utilisateur
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.TerminalOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminal-sessions/user-sessions [get]
func (tc *terminalController) GetUserSessions(ctx *gin.Context) {
	userId := ctx.GetString("userId")

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

	// Récupérer les sessions actives de l'utilisateur
	terminals, err := tc.service.GetActiveUserSessions(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTOs
	var terminalOutputs []dto.TerminalOutput
	for _, terminal := range *terminals {
		terminalOutputs = append(terminalOutputs, dto.TerminalOutput{
			ID:           terminal.ID,
			SessionID:    terminal.SessionID,
			UserID:       terminal.UserID,
			Status:       terminal.Status,
			ExpiresAt:    terminal.ExpiresAt,
			InstanceType: terminal.InstanceType,
			CreatedAt:    terminal.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, terminalOutputs)
}

// Sync Session godoc
//
//	@Summary		Synchroniser une session
//	@Description	Synchronise l'état d'une session avec l'API Terminal Trainer
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Session ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncSessionResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminal-sessions/{id}/sync [post]
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
	hasAccess, err := tc.hasTerminalAccess(ctx, sessionID, userId, "read")
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncAllSessionsResponse
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminal-sessions/sync-all [post]
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query	string	false	"User ID (admin only)"
//	@Security		Bearer
//	@Success		200	{object}	dto.SyncAllSessionsResponse
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Sync error"
//	@Router			/terminal-sessions/sync-user [post]
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Session ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.ExtendedSessionStatusResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		500	{object}	errors.APIError	"Status check error"
//	@Router			/terminal-sessions/{id}/status [get]
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
			response.ExistsInAPI = true
			response.APIStatus = foundInAPI.Status
			response.APIExpiresAt = time.Unix(foundInAPI.ExpiresAt, 0)
			response.SyncRecommended = terminal.Status != foundInAPI.Status
			response.StatusMatch = terminal.Status == foundInAPI.Status
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			user_id	query	string	false	"User ID (admin only)"
//	@Security		Bearer
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Error getting statistics"
//	@Router			/terminal-sessions/sync-stats [get]
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

	response := map[string]interface{}{
		"user_id":      targetUserId,
		"statistics":   stats,
		"generated_at": time.Now(),
	}

	ctx.JSON(http.StatusOK, response)
}

// Get Instance Types godoc
//
//	@Summary		Récupérer les types d'instances disponibles
//	@Description	Récupère la liste des types d'instances/préfixes disponibles depuis Terminal Trainer
//	@Tags			terminal-sessions
//	@Security		Bearer
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		dto.InstanceType
//	@Failure		500	{object}	errors.APIError	"Erreur interne du serveur"
//	@Router			/terminal-sessions/instance-types [get]
func (tc *terminalController) GetInstanceTypes(ctx *gin.Context) {
	instanceTypes, err := tc.service.GetInstanceTypes()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to get instance types: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, instanceTypes)
}

// Terminal sharing endpoints implementation

// Share Terminal godoc
//
//	@Summary		Partager une session terminal avec un autre utilisateur
//	@Description	Partage l'accès d'une session terminal avec un autre utilisateur avec un niveau d'accès spécifique
//	@Tags			terminal-sessions
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
//	@Router			/terminal-sessions/{id}/share [post]
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

	err := tc.service.ShareTerminal(terminalID, userId, request.SharedWithUserID, request.AccessLevel, request.ExpiresAt)
	if err != nil {
		if err.Error() == "terminal not found" {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: err.Error(),
			})
			return
		}
		if err.Error() == "only terminal owner can share access" {
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
//	@Tags			terminal-sessions
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
//	@Router			/terminal-sessions/{id}/share/{user_id} [delete]
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
		if err.Error() == "only terminal owner can revoke access" {
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.TerminalShareOutput	"List of terminal shares"
//	@Failure		400	{object}	errors.APIError			"Bad request"
//	@Failure		403	{object}	errors.APIError			"Access denied"
//	@Failure		404	{object}	errors.APIError			"Terminal not found"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminal-sessions/{id}/shares [get]
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
			ID:               share.ID,
			TerminalID:       share.TerminalID,
			SharedWithUserID: share.SharedWithUserID,
			SharedByUserID:   share.SharedByUserID,
			AccessLevel:      share.AccessLevel,
			ExpiresAt:        share.ExpiresAt,
			IsActive:         share.IsActive,
			CreatedAt:        share.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, shareOutputs)
}

// Get Shared Terminals godoc
//
//	@Summary		Obtenir les sessions terminals partagées avec l'utilisateur
//	@Description	Récupère toutes les sessions terminals auxquelles l'utilisateur a accès via des partages
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.SharedTerminalInfo	"List of shared terminals"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminal-sessions/shared-with-me [get]
func (tc *terminalController) GetSharedTerminals(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	terminals, err := tc.service.GetSharedTerminals(userId)
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
//	@Tags			terminal-sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SharedTerminalInfo	"Detailed terminal information"
//	@Failure		400	{object}	errors.APIError			"Bad request"
//	@Failure		403	{object}	errors.APIError			"Access denied"
//	@Failure		404	{object}	errors.APIError			"Terminal not found"
//	@Failure		500	{object}	errors.APIError			"Internal server error"
//	@Router			/terminal-sessions/{id}/info [get]
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
