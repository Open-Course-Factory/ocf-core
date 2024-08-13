package controller

import (
	"fmt"
	"net/http"
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
//	@Summary		Accès SSH
//	@Description	Récupération des accès SSH
//	@Tags			ssh
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	string
//
//	@Failure		404	{object}	error	"SSH inexistant"
//
//	@Router			/ssh [get]
func (s sshClientController) ShellWeb(c *gin.Context) {
	var err error

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println(err)
	}
	_, readContent, err := conn.ReadMessage()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ readContent: %v\n", string(readContent))

	sshClient, err := s.service.DecodeMsgToSSHClient(string(readContent))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ sshClient: %v\n", sshClient)

	terminal := models.Terminal{
		Columns: 150,
		Rows:    35,
	}

	var port = 22
	err = sshClient.GenerateClient(sshClient.IpAddress, sshClient.Username, sshClient.Password, port)
	if err != nil {
		conn.WriteMessage(1, []byte(err.Error()))
		conn.Close()
		return
	}
	sshClient.RequestTerminal(terminal)
	sshClient.Connect(conn)
}
