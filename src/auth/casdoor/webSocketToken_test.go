package casdoor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secretJWT = "header.payload.signature"

func plainRequest(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/console"+query, nil)
}

func webSocketRequest(query string) *http.Request {
	req := plainRequest(query)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	return req
}

// serveAndCaptureLogs runs handler behind the same middleware order as main.go
// and returns what the logger and the recovery wrote.
func webSocketRequestWithProtocolToken(token string) *http.Request {
	req := webSocketRequest("?width=100")
	req.Header.Set("Sec-WebSocket-Protocol", WebSocketAuthProtocol+", "+token)
	return req
}

func serveAndCaptureLogs(req *http.Request, handler gin.HandlerFunc) string {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	router := gin.New()
	router.Use(MoveWebSocketTokenToHeader(), gin.LoggerWithWriter(&logs), gin.RecoveryWithWriter(&logs))
	router.GET("/console", handler)
	router.ServeHTTP(httptest.NewRecorder(), req)
	return logs.String()
}

func TestMoveWebSocketTokenToHeader_WebSocket_AuthenticatesWithoutLoggingTheToken(t *testing.T) {
	originalParser := JwtTokenParser
	t.Cleanup(func() { JwtTokenParser = originalParser })
	var parsedToken string
	JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
		parsedToken = token
		return &casdoorsdk.Claims{User: casdoorsdk.User{Id: "user-1"}}, nil
	}

	var userID, tokenSeenByHandler string
	logs := serveAndCaptureLogs(webSocketRequest("?width=100&token="+secretJWT), func(ctx *gin.Context) {
		userID, _, _ = ParseUserIDFromRequest(ctx)
		tokenSeenByHandler = ctx.Query("token")
	})

	assert.Equal(t, "user-1", userID, "a WebSocket can still authenticate with its query token")
	assert.Equal(t, secretJWT, parsedToken)
	assert.Empty(t, tokenSeenByHandler, "no handler or proxy downstream sees the query token")
	assert.NotContains(t, logs, secretJWT)
	assert.Contains(t, logs, "width=100", "the rest of the query stays in the log")
}

func TestMoveWebSocketTokenToHeader_PlainRequest_DoesNotAuthenticateOrLog(t *testing.T) {
	var parseErr error
	logs := serveAndCaptureLogs(plainRequest("?token="+secretJWT), func(ctx *gin.Context) {
		_, _, parseErr = ParseUserIDFromRequest(ctx)
	})

	assert.Error(t, parseErr, "query tokens only authenticate WebSocket upgrades")
	assert.NotContains(t, logs, secretJWT)
}

// A dropped WebSocket reaches gin's recovery as a broken pipe, which dumps the
// request line in every gin mode.
func TestMoveWebSocketTokenToHeader_RecoveryDump_DoesNotContainTheToken(t *testing.T) {
	logs := serveAndCaptureLogs(webSocketRequest("?token="+secretJWT), func(*gin.Context) {
		panic(http.ErrAbortHandler)
	})

	require.Contains(t, logs, "GET /console", "the request dump was written")
	assert.NotContains(t, logs, secretJWT)
}

func TestMoveWebSocketTokenToHeader_MalformedQuery_DoesNotLogTheToken(t *testing.T) {
	logs := serveAndCaptureLogs(plainRequest("?width=100;token="+secretJWT), func(*gin.Context) {})

	assert.NotContains(t, logs, secretJWT)
}

func TestMoveWebSocketTokenToHeader_ExistingHeader_IsKept(t *testing.T) {
	req := webSocketRequest("?token=" + secretJWT)
	req.Header.Set("Authorization", "Bearer from-header")

	var header string
	serveAndCaptureLogs(req, func(ctx *gin.Context) { header = ctx.GetHeader("Authorization") })

	assert.Equal(t, "Bearer from-header", header)
}

func TestMoveWebSocketTokenToHeader_NoToken_LeavesTheRequestAlone(t *testing.T) {
	var rawQuery, header string
	serveAndCaptureLogs(webSocketRequest("?z=1&a=2"), func(ctx *gin.Context) {
		rawQuery = ctx.Request.URL.RawQuery
		header = ctx.GetHeader("Authorization")
	})

	assert.Equal(t, "z=1&a=2", rawQuery)
	assert.Empty(t, header)
}

func TestMoveWebSocketTokenToHeader_ProtocolToken_AuthenticatesAndLeavesOnlyTheProtocolName(t *testing.T) {
	var authorization, protocols string
	logs := serveAndCaptureLogs(webSocketRequestWithProtocolToken(secretJWT), func(ctx *gin.Context) {
		authorization = ctx.GetHeader("Authorization")
		protocols = ctx.GetHeader("Sec-WebSocket-Protocol")
	})

	assert.Equal(t, "Bearer "+secretJWT, authorization)
	assert.Equal(t, WebSocketAuthProtocol, protocols, "the upgrader still echoes the name; nothing downstream sees the token")
	assert.NotContains(t, logs, secretJWT)
}

func TestMoveWebSocketTokenToHeader_ProtocolTokenOnPlainRequest_DoesNotAuthenticate(t *testing.T) {
	req := plainRequest("")
	req.Header.Set("Sec-WebSocket-Protocol", WebSocketAuthProtocol+", "+secretJWT)

	var authorization, protocols string
	serveAndCaptureLogs(req, func(ctx *gin.Context) {
		authorization = ctx.GetHeader("Authorization")
		protocols = ctx.GetHeader("Sec-WebSocket-Protocol")
	})

	assert.Empty(t, authorization)
	assert.Equal(t, WebSocketAuthProtocol, protocols)
}

func TestMoveWebSocketTokenToHeader_ProtocolNameAlone_LeavesTheRequestUnauthenticated(t *testing.T) {
	req := webSocketRequest("")
	req.Header.Set("Sec-WebSocket-Protocol", WebSocketAuthProtocol)

	var authorization string
	serveAndCaptureLogs(req, func(ctx *gin.Context) { authorization = ctx.GetHeader("Authorization") })

	assert.Empty(t, authorization)
}

func TestMoveWebSocketTokenToHeader_OtherProtocols_AreLeftAlone(t *testing.T) {
	req := webSocketRequest("")
	req.Header.Set("Sec-WebSocket-Protocol", "chat, superchat")

	var authorization, protocols string
	serveAndCaptureLogs(req, func(ctx *gin.Context) {
		authorization = ctx.GetHeader("Authorization")
		protocols = ctx.GetHeader("Sec-WebSocket-Protocol")
	})

	assert.Empty(t, authorization)
	assert.Equal(t, "chat, superchat", protocols)
}

func TestMoveWebSocketTokenToHeader_TokenInBothPlaces_RemovesBoth(t *testing.T) {
	req := webSocketRequestWithProtocolToken(secretJWT)
	req.URL.RawQuery = "token=query-token"
	req.RequestURI = req.URL.RequestURI()

	var authorization string
	logs := serveAndCaptureLogs(req, func(ctx *gin.Context) { authorization = ctx.GetHeader("Authorization") })

	assert.Equal(t, "Bearer "+secretJWT, authorization)
	assert.NotContains(t, logs, "query-token")
}
