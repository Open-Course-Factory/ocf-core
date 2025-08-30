package terminalController

import (
	"net/http"
	"net/url"
	"os"

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
}

type terminalController struct {
	controller.GenericController
	terminalTrainerURL string
	service            services.TerminalTrainerService
}

func NewTerminalController(db *gorm.DB) TerminalController {
	return &terminalController{
		GenericController:  controller.NewGenericController(db),
		terminalTrainerURL: os.Getenv("TERMINAL_TRAINER_URL"),
		service:            services.NewTerminalTrainerService(db),
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Permettre toutes les origines pour le développement
		// En production, filtrer selon vos besoins
		return true
	},
}

// DeleteEntity override pour gérer les sessions de terminal avec hard delete
func (tc *terminalController) DeleteEntity(ctx *gin.Context) {
	tc.GenericController.DeleteEntity(ctx, false) // hard delete pour les sessions
}

// Start Terminal Session godoc
//
//	@Summary		Démarrer une session terminal
//	@Description	Démarre une nouvelle session de terminal via Terminal Trainer
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			session	body	dto.CreateTerminalSessionInput	true	"Terminal session input"
//
//	@Security		Bearer
//
//	@Success		200	{object}	dto.TerminalSessionResponse
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Terminal trainer error"
//	@Router			/terminals/start-session [post]
func (tc *terminalController) StartSession(ctx *gin.Context) {
	userId := ctx.GetString("userId") // Fourni par le middleware auth

	// Valider les paramètres
	sessionInput := dto.CreateTerminalSessionInput{}
	if err := ctx.ShouldBindJSON(&sessionInput); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Démarrer la session via le service
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
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"Terminal ID"
//	@Param			width	query	string	false	"Console width"
//	@Param			height	query	string	false	"Console height"
//
//	@Security		Bearer
//
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

	// Pour l'instant, on utilise le service direct
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

	// Récupérer la clé API de l'utilisateur
	userKey, err := tc.service.GetUserKey(terminal.UserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "User key not found",
		})
		return
	}

	// Construire l'URL du Terminal Trainer avec tous les paramètres
	// Construire l'URL WebSocket du Terminal Trainer
	terminalTrainerWSURL, err := url.Parse(tc.terminalTrainerURL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Invalid terminal trainer URL",
		})
		return
	}

	// Changer le schéma pour WebSocket
	if terminalTrainerWSURL.Scheme == "https" {
		terminalTrainerWSURL.Scheme = "wss"
	} else {
		terminalTrainerWSURL.Scheme = "ws"
	}
	terminalTrainerWSURL.Path = "/1.0/console"

	// Ajouter les query parameters
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
//	@Description	Arrête une session de terminal active
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Terminal ID"
//
//	@Security		Bearer
//
//	@Success		200	{object}	string
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/terminals/{id}/stop [post]
func (tc *terminalController) StopSession(ctx *gin.Context) {
	terminalID := ctx.Param("id")
	userId := ctx.GetString("userId")

	// TODO: Récupérer via le système générique avec UUID
	terminal, err := tc.service.GetSessionInfo(terminalID) // Assume terminalID is sessionID
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

	// Arrêter la session
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
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{array}		dto.TerminalOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/user-sessions [get]
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
			ID:        terminal.ID,
			SessionID: terminal.SessionID,
			UserID:    terminal.UserID,
			Status:    terminal.Status,
			ExpiresAt: terminal.ExpiresAt,
			CreatedAt: terminal.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, terminalOutputs)
}
