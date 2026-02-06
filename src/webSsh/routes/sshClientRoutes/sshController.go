package controller

import (
	"net/http"
	"soli/formations/src/auth/errors"
	config "soli/formations/src/configuration"
	sqldb "soli/formations/src/db"
	"soli/formations/src/webSsh/models"
	"soli/formations/src/webSsh/services"

	authServices "soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type SshClientController interface {
	ShellWeb(ctx *gin.Context)
}

type sshClientController struct {
	//controller.GenericController
	service services.SshClientService
}

func NewSshClientController() SshClientController {
	return &sshClientController{
		service: services.NewSshClientService(),
	}
}

var (
	upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // No origin header (e.g. non-browser clients)
			}
			return config.IsOriginAllowed(origin)
		},
	}
)

// GetSShConnection godoc
//
//	@Summary		Accès WebSocket SSH
//	@Description	Récupération des accès SSH
//	@Tags			ssh
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		1000 {object}	string "ok"
//
//	@Failure		1002 {object}	errors.APIError	"Protocol Error"
//	@Failure		1011 {object}	errors.APIError	"Internal Server Error"
//	@Failure		1003 {object}	errors.APIError	"Unsupported Data"
//
//	@Router			/ssh [get]
func (s sshClientController) ShellWeb(ctx *gin.Context) {
	var err error

	conn, upgradeErr := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if upgradeErr != nil {
		ctx.JSON(websocket.CloseProtocolError, &errors.APIError{
			ErrorCode:    websocket.CloseProtocolError,
			ErrorMessage: upgradeErr.Error(),
		})
		return
	}

	_, readContent, readErr := conn.ReadMessage()
	if readErr != nil {
		ctx.JSON(websocket.CloseInternalServerErr, &errors.APIError{
			ErrorCode:    websocket.CloseInternalServerErr,
			ErrorMessage: readErr.Error(),
		})
		return
	}

	sshClient, decodeError := s.service.DecodeMsgToSSHClient(string(readContent))
	if decodeError != nil {
		ctx.JSON(websocket.CloseUnsupportedData, &errors.APIError{
			ErrorCode:    websocket.CloseUnsupportedData,
			ErrorMessage: decodeError.Error(),
		})
		return
	}

	userId := ctx.GetString("userId")
	sshkeyService := authServices.NewSshKeyService(sqldb.DB)

	keysDto, errorGettingSshKeys := sshkeyService.GetKeysByUserId(userId)
	if errorGettingSshKeys != nil {
		ctx.JSON(websocket.CloseInternalServerErr, &errors.APIError{
			ErrorCode:    websocket.CloseInternalServerErr,
			ErrorMessage: errorGettingSshKeys.Error(),
		})
	}

	var keys []string

	for _, sshkey := range *keysDto {
		keys = append(keys, sshkey.PrivateKey)
	}

	terminal := models.Terminal{
		Columns: 150,
		Rows:    35,
	}

	var port = 22
	err = sshClient.GenerateClient(sshClient.IpAddress, sshClient.Username, keys, port)
	if err != nil {
		conn.WriteMessage(1, []byte(err.Error()))
		conn.Close()
		ctx.JSON(websocket.CloseInternalServerErr, &errors.APIError{
			ErrorCode:    websocket.CloseInternalServerErr,
			ErrorMessage: err.Error(),
		})
		return
	}
	sshClient.RequestTerminal(terminal)
	sshClient.Connect(conn)
	ctx.JSON(websocket.CloseNormalClosure, "ok")
}
