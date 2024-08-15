package controller

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/webSsh/models"
	"soli/formations/src/webSsh/services"

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
			return true
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

	terminal := models.Terminal{
		Columns: 150,
		Rows:    35,
	}

	var port = 22
	err = sshClient.GenerateClient(sshClient.IpAddress, sshClient.Username, sshClient.Password, port)
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
