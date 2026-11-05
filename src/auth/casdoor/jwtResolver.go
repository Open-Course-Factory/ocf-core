package casdoor

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// JwtTokenParser is the package-level seam used to parse JWT tokens.
// Tests may override this to inject stubbed claims without requiring a
// live Casdoor configuration.
var JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
	return casdoorsdk.ParseJwtToken(token)
}

// WebSocketAuthProtocol is the subprotocol a browser offers next to its access
// token, as new WebSocket(url, [WebSocketAuthProtocol, token]). Browsers cannot
// set any other header on a WebSocket.
const WebSocketAuthProtocol = "ocf.bearer"

// MoveWebSocketTokenToHeader keeps access tokens out of logs and proxied
// requests, so it must run before anything that logs or forwards the request.
//
// A WebSocket upgrade may authenticate with a token in its subprotocols, or in
// ?token= for a front that predates them. On any other request the token is
// removed and ignored.
func MoveWebSocketTokenToHeader() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		req := ctx.Request
		protocolToken := removeProtocolToken(req)
		queryToken := removeQueryToken(req)
		token := cmp.Or(protocolToken, queryToken)
		if token != "" && req.Header.Get("Authorization") == "" && isWebSocketUpgrade(req) {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		ctx.Next()
	}
}

// removeProtocolToken returns the token offered next to WebSocketAuthProtocol
// and leaves only that protocol's name in the header, for the upgrader to echo.
func removeProtocolToken(req *http.Request) string {
	offered := websocket.Subprotocols(req)
	if !slices.Contains(offered, WebSocketAuthProtocol) {
		return ""
	}
	req.Header.Set("Sec-WebSocket-Protocol", WebSocketAuthProtocol)

	for _, protocol := range offered {
		if protocol != WebSocketAuthProtocol {
			return protocol
		}
	}
	return ""
}

// removeQueryToken removes the token from the request URL and returns it. A
// query that fails to parse is rewritten too, because the token may be in the
// part that failed, as in ?width=100;token=x.
func removeQueryToken(req *http.Request) string {
	query, parseErr := url.ParseQuery(req.URL.RawQuery)
	if !query.Has("token") && parseErr == nil {
		return ""
	}

	token := query.Get("token")
	query.Del("token")
	req.URL.RawQuery = query.Encode()
	req.RequestURI = req.URL.RequestURI() // gin's recovery dump prints this field, not the URL
	return token
}

func isWebSocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

// extractBearerToken reads the raw bearer token from the request's
// Authorization header, where MoveWebSocketTokenToHeader also puts a WebSocket's
// token. It returns an error when the token is missing or empty after
// stripping the "Bearer " prefix.
func extractBearerToken(ctx *gin.Context) (string, error) {
	token := ctx.Request.Header.Get("Authorization")
	if token == "" {
		return "", fmt.Errorf("missing Authorization header")
	}

	if after, ok := strings.CutPrefix(token, "Bearer "); ok {
		token = after
	} else if after, ok := strings.CutPrefix(token, "bearer "); ok {
		token = after
	}

	if token == "" {
		return "", fmt.Errorf("missing or invalid authorization token")
	}

	return token, nil
}

// ParseUserIDFromRequest extracts the JWT from the request, parses it,
// and returns the authenticated user's ID and the token's JTI claim (used
// for blacklist lookups). This is the low-level helper that preserves the
// legacy return shape expected by AuthManagement.
func ParseUserIDFromRequest(ctx *gin.Context) (userID string, tokenJTI string, err error) {
	token, err := extractBearerToken(ctx)
	if err != nil {
		return "", "", err
	}

	claims, err := JwtTokenParser(token)
	if err != nil {
		return "", "", err
	}
	if claims == nil {
		return "", "", fmt.Errorf("parsed JWT claims were nil")
	}

	return claims.Id, claims.ID, nil
}

// ResolveUserFromRequest extracts the JWT, parses it, and resolves the
// Casbin roles attached to the authenticated user. It is a pure lookup
// helper — it does NOT write to the gin context. Callers decide what to
// do with the resolved values.
//
// Used by both AuthManagement (platform RBAC gateway) and Layer2Enforcement
// (business-logic authorization) so a single code path drives JWT parsing,
// avoiding the "Layer 2 silently bypassed when userId is absent" bug.
func ResolveUserFromRequest(ctx *gin.Context) (userID string, roles []string, err error) {
	userID, _, err = ParseUserIDFromRequest(ctx)
	if err != nil {
		return "", nil, err
	}

	if Enforcer == nil {
		// Enforcer not initialised (typical in unit tests that don't set one up).
		// Return an empty role list so callers can still evaluate non-role-based
		// rules; admin-bypass paths will simply not trigger.
		return userID, nil, nil
	}

	roles, err = Enforcer.GetRolesForUser(userID)
	if err != nil {
		return "", nil, err
	}

	return userID, roles, nil
}
