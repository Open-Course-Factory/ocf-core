package casdoor

import (
	"fmt"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

// JwtTokenParser is the package-level seam used to parse JWT tokens.
// Tests may override this to inject stubbed claims without requiring a
// live Casdoor configuration.
var JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
	return casdoorsdk.ParseJwtToken(token)
}

// extractBearerToken reads the raw bearer token from the request's
// Authorization header. For WebSocket upgrade requests, it also accepts
// a `token` query parameter (browsers cannot set custom headers on
// WebSocket handshakes).
//
// Returns an empty string with a descriptive error when the token is
// missing or empty after stripping the "Bearer " prefix.
func extractBearerToken(ctx *gin.Context) (string, error) {
	token := ctx.Request.Header.Get("Authorization")

	// Allow query parameter auth ONLY for WebSocket upgrade requests.
	isWebSocketUpgrade := strings.ToLower(ctx.Request.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(ctx.Request.Header.Get("Connection")), "upgrade")

	if token == "" && isWebSocketUpgrade {
		token = ctx.Query("token")
		if token == "" {
			return "", fmt.Errorf("missing Authorization header or token query parameter for WebSocket connection")
		}
	} else if token == "" {
		return "", fmt.Errorf("missing Authorization header - tokens in query parameters are not allowed for non-WebSocket requests")
	}

	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	} else if strings.HasPrefix(token, "bearer ") {
		token = strings.TrimPrefix(token, "bearer ")
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
