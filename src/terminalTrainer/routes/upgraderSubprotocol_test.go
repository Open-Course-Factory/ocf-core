package terminalController

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
)

// A browser drops a WebSocket when the server does not echo a protocol it
// offered, so this runs a real handshake through the middleware and the upgrader
// the console and the supervision routes share.
func TestConsoleUpgrader_AcceptsTheTokenAsASubprotocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const token = "header.payload.signature"

	var authorization string
	router := gin.New()
	router.Use(casdoor.MoveWebSocketTokenToHeader())
	router.GET("/console", func(ctx *gin.Context) {
		authorization = ctx.GetHeader("Authorization")
		conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err == nil {
			conn.Close()
		}
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	dialer := websocket.Dialer{Subprotocols: []string{casdoor.WebSocketAuthProtocol, token}}
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/console"
	conn, resp, err := dialer.Dial(url, http.Header{})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	assert.Equal(t, casdoor.WebSocketAuthProtocol, conn.Subprotocol())
	assert.NotContains(t, resp.Header.Get("Sec-WebSocket-Protocol"), token, "the token is not sent back")
	assert.Equal(t, "Bearer "+token, authorization)
}
