package terminalController

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	orgModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	db           *gorm.DB
	proxyBaseURL string
	adminKey     string
	transport    http.RoundTripper
}

// NewIncusUIController creates a new IncusUIController.
func NewIncusUIController(db *gorm.DB, proxyBaseURL string, adminKey ...string) *IncusUIController {
	key := ""
	if len(adminKey) > 0 {
		key = adminKey[0]
	}
	return &IncusUIController{
		db:           db,
		proxyBaseURL: proxyBaseURL,
		adminKey:     key,
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
	userID := ctx.GetString("userId")
	userRolesRaw, _ := ctx.Get("userRoles")
	userRoles, _ := userRolesRaw.([]string)

	backendID := ctx.Param("backendId")

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

		for _, allowed := range org.AllowedBackends {
			if allowed == backendID {
				return true
			}
		}
	}

	return false
}
