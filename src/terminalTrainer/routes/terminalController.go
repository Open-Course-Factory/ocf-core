package terminalController

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
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
	terminalID := ctx.Param("id")
	if terminalID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Terminal ID is required",
		})
		return
	}

	userId := ctx.GetString("userId")

	// Récupérer la session via l'ID générique (UUID)
	// TODO: Adapter pour récupérer par UUID via le système générique

	// Pour l'instant, on utilise le service direct
	terminal, err := tc.service.GetSessionInfo(terminalID) // Assume terminalID is sessionID for now
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
	proxyURL := fmt.Sprintf("%s/1.0/console", tc.terminalTrainerURL)

	// Créer la requête proxy
	req, err := http.NewRequest("GET", proxyURL, nil)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create proxy request",
		})
		return
	}

	// Ajouter les headers nécessaires
	req.Header.Set("X-API-Key", userKey.APIKey)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	// Copier les query parameters
	q := req.URL.Query()
	q.Set("id", terminal.SessionID) // Utiliser le SessionID du terminal
	if width := ctx.Query("width"); width != "" {
		q.Set("width", width)
	}
	if height := ctx.Query("height"); height != "" {
		q.Set("height", height)
	}
	req.URL.RawQuery = q.Encode()

	// Copier certains headers de la requête originale
	for name, values := range ctx.Request.Header {
		if name == "Upgrade" || name == "Connection" || name == "Sec-WebSocket-Key" ||
			name == "Sec-WebSocket-Version" || name == "Sec-WebSocket-Extensions" {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
	}

	// Faire la requête proxy
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Terminal trainer unavailable",
		})
		return
	}
	defer resp.Body.Close()

	// Copier tous les headers de réponse
	for name, values := range resp.Header {
		for _, value := range values {
			ctx.Header(name, value)
		}
	}

	// Définir le status code
	ctx.Status(resp.StatusCode)

	// Copier le body de la réponse
	if resp.StatusCode < 400 {
		io.Copy(ctx.Writer, resp.Body)
	} else {
		body, _ := io.ReadAll(resp.Body)
		ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
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
