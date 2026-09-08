package controller

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"soli/formations/src/auth/errors"
	config "soli/formations/src/configuration"
	sqldb "soli/formations/src/db"
	"soli/formations/src/webSsh/models"

	authServices "soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type SshClientController interface {
	ShellWeb(ctx *gin.Context)
}

type sshClientController struct{}

func NewSshClientController() SshClientController {
	return &sshClientController{}
}

// decodeMsgToSSHClient parses the first websocket frame: a base64-encoded JSON SSHClient.
func decodeMsgToSSHClient(msg string) (models.SSHClient, error) {
	client := models.NewSSHClient()
	decoded, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return client, err
	}
	return client, json.Unmarshal(decoded, &client)
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
		errors.Respond(ctx, websocket.CloseProtocolError, upgradeErr.Error())
		return
	}

	_, readContent, readErr := conn.ReadMessage()
	if readErr != nil {
		errors.Respond(ctx, websocket.CloseInternalServerErr, readErr.Error())
		return
	}

	sshClient, decodeError := decodeMsgToSSHClient(string(readContent))
	if decodeError != nil {
		errors.Respond(ctx, websocket.CloseUnsupportedData, decodeError.Error())
		return
	}

	userId := ctx.GetString("userId")
	sshkeyService := authServices.NewSshKeyService(sqldb.DB)

	keysDto, errorGettingSshKeys := sshkeyService.GetKeysByUserId(userId)
	if errorGettingSshKeys != nil {
		errors.Respond(ctx, websocket.CloseInternalServerErr, errorGettingSshKeys.Error())
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
		errors.Respond(ctx, websocket.CloseInternalServerErr, err.Error())
		return
	}
	sshClient.RequestTerminal(terminal)
	sshClient.Connect(conn)
	ctx.JSON(websocket.CloseNormalClosure, "ok")
}
