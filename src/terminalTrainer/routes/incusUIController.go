package terminalController

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	orgModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// incusCookieAuth is a middleware that extracts a JWT from the "incus_token"
// cookie and sets it as the Authorization header. This allows the iframe
// (which cannot send custom headers) to authenticate via a cookie that the
// /incus-ui/auth endpoint sets on the API domain.
func incusCookieAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			if token, err := c.Cookie("incus_token"); err == nil && token != "" {
				c.Request.Header.Set("Authorization", "Bearer "+token)
			}
		}
		c.Next()
	}
}

// SetIncusAuthCookie is called with a valid JWT (already authenticated by
// AuthManagement). It sets an HttpOnly cookie on the API domain so that
// subsequent iframe requests can authenticate without custom headers.
func SetIncusAuthCookie(ctx *gin.Context) {
	token := strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer ")
	if token == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	ctx.SetSameSite(http.SameSiteStrictMode)
	ctx.SetCookie("incus_token", token, 3600, "/api/v1/incus-ui", "", false, true)
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// sensitiveHeaders lists headers that must NOT be forwarded to tt-backend.
// The proxy sets its own X-API-Key; the user's Authorization and Cookie
// headers are stripped to prevent credential leakage.
var sensitiveHeaders = map[string]struct{}{
	"Authorization": {},
	"Cookie":        {},
	"X-Api-Key":     {},
}

// IncusUIController handles proxying requests to tt-backend's Incus UI endpoint.
// Access is restricted to system admins (any backend) and org owners/managers
// (only their org's AllowedBackends).
type IncusUIController struct {
	db                *gorm.DB
	proxyBaseURL      string
	adminKey          string
	protectedBackends map[string]bool
	transport         http.RoundTripper
}

// NewIncusUIController creates a new IncusUIController.
// protectedBackends is the set of backend IDs that only admins can access
// via the Incus UI proxy (e.g. the system default backend).
func NewIncusUIController(db *gorm.DB, proxyBaseURL string, protectedBackends map[string]bool, adminKey ...string) *IncusUIController {
	key := ""
	if len(adminKey) > 0 {
		key = adminKey[0]
	}
	return &IncusUIController{
		db:                db,
		proxyBaseURL:      proxyBaseURL,
		adminKey:          key,
		protectedBackends: protectedBackends,
		transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

// ProxyIncusUI handles proxying Incus UI requests to tt-backend.
// It checks authorization, sanitizes the path, then reverse-proxies the
// request (including WebSocket upgrades) via httputil.ReverseProxy.
func (c *IncusUIController) ProxyIncusUI(ctx *gin.Context) {
	backendID := ctx.Param("backendId")

	// Reserved backendId: set auth cookie and return.
	// The frontend calls this via axios (which sends the JWT header),
	// and the response sets an HttpOnly cookie on the API domain so
	// subsequent iframe requests can authenticate.
	if backendID == "_auth" {
		SetIncusAuthCookie(ctx)
		return
	}

	userID := ctx.GetString("userId")
	userRolesRaw, _ := ctx.Get("userRoles")
	userRoles, _ := userRolesRaw.([]string)

	if !c.IsUserAuthorizedForBackend(userID, userRoles, backendID) {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error_code":    http.StatusForbidden,
			"error_message": "Access forbidden: you are not authorized to access this backend",
		})
		return
	}

	// Sanitize the remaining path to prevent path traversal attacks.
	// Preserve trailing slash (path.Clean strips it, causing redirect loops).
	remainingPath := ctx.Param("path")
	remainingPath = strings.TrimPrefix(remainingPath, "/")
	hadTrailingSlash := strings.HasSuffix(remainingPath, "/")
	remainingPath = path.Clean(remainingPath)
	if hadTrailingSlash && remainingPath != "." {
		remainingPath += "/"
	}
	if remainingPath == "." {
		remainingPath = ""
	}

	if strings.Contains(remainingPath, "..") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error_code":    http.StatusBadRequest,
			"error_message": "Invalid path: path traversal is not allowed",
		})
		return
	}

	// Parse the base URL for the reverse proxy target
	targetBase, err := url.Parse(strings.TrimRight(c.proxyBaseURL, "/"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error_code":    http.StatusInternalServerError,
			"error_message": "Invalid proxy base URL configuration",
		})
		return
	}

	// Build the target path and the proxy prefix for redirect rewriting
	targetPath := "/1.0/admin/incus-ui/" + backendID + "/" + remainingPath
	proxyPrefix := "/api/v1/incus-ui/" + backendID

	adminKey := c.adminKey
	proxy := &httputil.ReverseProxy{
		Transport: c.transport,
		Director: func(req *http.Request) {
			req.URL.Scheme = targetBase.Scheme
			req.URL.Host = targetBase.Host
			req.URL.Path = targetPath
			req.URL.RawPath = targetPath // preserve trailing slash
			req.URL.RawQuery = ctx.Request.URL.RawQuery
			req.Host = targetBase.Host

			// Strip sensitive headers to prevent credential leakage
			for header := range sensitiveHeaders {
				req.Header.Del(header)
			}

			// Set admin API key for tt-backend authentication
			if adminKey != "" {
				req.Header.Set("X-API-Key", adminKey)
			}

			// Prevent upstream from sending compressed responses so we
			// can modify HTML bodies in ModifyResponse without decompressing.
			req.Header.Set("Accept-Encoding", "identity")
		},
		ModifyResponse: func(resp *http.Response) error {
			// Add Content-Security-Policy to prevent clickjacking
			resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self'")

			// Rewrite Location headers so redirects from the upstream
			// point back through our proxy prefix instead of the root.
			if loc := resp.Header.Get("Location"); loc != "" {
				if strings.HasPrefix(loc, "/") {
					resp.Header.Set("Location", proxyPrefix+loc)
				}
			}

			ct := resp.Header.Get("Content-Type")
			isHTML := strings.Contains(ct, "text/html")
			isJS := strings.Contains(ct, "javascript")

			// Only rewrite HTML and JavaScript responses.
			if !isHTML && !isJS {
				return nil
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				resp.Body = io.NopCloser(bytes.NewReader(nil))
				resp.ContentLength = 0
				return nil
			}

			content := string(body)

			// Rewrite absolute /ui/assets/ paths in both HTML and JS.
			// In HTML: asset references in src/href attributes.
			// In JS: Vite's __vite__mapDeps array and other hardcoded paths.
			content = strings.ReplaceAll(content, `"/ui/assets/`, `"`+proxyPrefix+`/ui/assets/`)
			content = strings.ReplaceAll(content, `'/ui/assets/`, `'`+proxyPrefix+`/ui/assets/`)

			if isHTML {
				content = strings.ReplaceAll(content, `"/manifest.json"`, `"`+proxyPrefix+`/manifest.json"`)

				// Inject monkey-patching script before any other scripts
				script := generateMonkeyPatchScript(proxyPrefix)
				content = strings.Replace(content, "<head>", "<head>"+script, 1)
			}

			modified := []byte(content)
			resp.Body = io.NopCloser(bytes.NewReader(modified))
			resp.ContentLength = int64(len(modified))
			resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))

			return nil
		},
	}

	// Unwrap gin's ResponseWriter to expose the underlying http.ResponseWriter.
	// This lets httputil.ReverseProxy perform its own interface checks
	// (Hijacker, Flusher, CloseNotifier) safely, instead of panicking on
	// gin's wrapper when the underlying writer doesn't support them.
	var rw http.ResponseWriter = ctx.Writer
	if unwrapper, ok := rw.(interface{ Unwrap() http.ResponseWriter }); ok {
		rw = unwrapper.Unwrap()
	}

	proxy.ServeHTTP(rw, ctx.Request)
}

// IsUserAuthorizedForBackend checks whether a user (identified by userID and
// system roles) is allowed to access the given backendID.
//
// Authorization rules:
//   - System administrators can access any backend.
//   - Org owners/managers can access backends listed in their org's AllowedBackends.
//   - Everyone else is denied.
func (c *IncusUIController) IsUserAuthorizedForBackend(userID string, userRoles []string, backendID string) bool {
	// System administrators can access any backend
	for _, role := range userRoles {
		if role == "administrator" {
			return true
		}
	}

	// Non-admins are blocked from protected backends (e.g. system default)
	if c.protectedBackends[backendID] {
		return false
	}

	// Query organization memberships where user is an owner or manager
	var members []orgModels.OrganizationMember
	err := c.db.
		Where("user_id = ? AND role IN ? AND is_active = ?", userID, []string{string(orgModels.OrgRoleOwner), string(orgModels.OrgRoleManager)}, true).
		Find(&members).Error
	if err != nil || len(members) == 0 {
		return false
	}

	// For each membership, load the organization and check AllowedBackends
	for _, member := range members {
		var org orgModels.Organization
		err := c.db.First(&org, "id = ?", member.OrganizationID).Error
		if err != nil {
			continue
		}

		if !org.IncusUIEnabled {
			continue
		}

		for _, allowed := range org.AllowedBackends {
			if allowed == backendID {
				return true
			}
		}
	}

	return false
}

// generateMonkeyPatchScript returns a <script> tag that monkey-patches
// fetch, XMLHttpRequest.open, and WebSocket to rewrite absolute URLs
// through the given proxy prefix. This ensures the Incus UI SPA's API
// calls and WebSocket connections are routed through the reverse proxy.
func generateMonkeyPatchScript(proxyPrefix string) string {
	return fmt.Sprintf(`<script>(function(){var p=%s;var O=location.origin;function nm(u,b){return u===b||u.startsWith(b+"/")||u.startsWith(b+"?")}function r(u){if(typeof u!=="string")return u;if(nm(u,"/1.0"))return p+u;if(nm(u,"/ui"))return p+u;return u}function rFull(u){if(typeof u!=="string")return u;try{var o=new URL(u);if(o.origin===O){var pn=o.pathname;if(nm(pn,"/1.0")||nm(pn,"/ui")){o.pathname=p+pn;return o.toString()}}}catch(e){}return r(u)}var of=window.fetch;window.fetch=function(i,n){if(typeof i==="string"){i=rFull(i)}else if(i instanceof Request){i=new Request(rFull(i.url),i)}return of.call(this,i,n)};var ox=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){arguments[1]=rFull(u);return ox.apply(this,arguments)};var OWS=window.WebSocket;window.WebSocket=function(u,pr){if(typeof u==="string"){try{var o=new URL(u,O);var pn=o.pathname;if(o.host===location.host&&(nm(pn,"/1.0")||nm(pn,"/ui"))){o.pathname=p+o.pathname;u=o.toString()}}catch(e){u=r(u)}}return pr!==undefined?new OWS(u,pr):new OWS(u)};window.WebSocket.prototype=OWS.prototype;window.WebSocket.CONNECTING=OWS.CONNECTING;window.WebSocket.OPEN=OWS.OPEN;window.WebSocket.CLOSING=OWS.CLOSING;window.WebSocket.CLOSED=OWS.CLOSED;var lD=Object.getOwnPropertyDescriptor(HTMLLinkElement.prototype,"href");if(lD&&lD.set){Object.defineProperty(HTMLLinkElement.prototype,"href",{set:function(v){return lD.set.call(this,rFull(v))},get:lD.get,enumerable:lD.enumerable,configurable:true})}var sD=Object.getOwnPropertyDescriptor(HTMLScriptElement.prototype,"src");if(sD&&sD.set){Object.defineProperty(HTMLScriptElement.prototype,"src",{set:function(v){return sD.set.call(this,rFull(v))},get:sD.get,enumerable:sD.enumerable,configurable:true})}if(location.pathname.startsWith(p+"/")){history.replaceState(null,"",location.pathname.substring(p.length)+location.search+location.hash)}})()</script>`, fmt.Sprintf("%q", proxyPrefix))
}
