package terminalController

import (
	"io"
	"net/http"
	"strings"

	orgModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IncusUIController handles proxying requests to tt-backend's Incus UI endpoint.
// Access is restricted to system admins (any backend) and org owners/managers
// (only their org's AllowedBackends).
type IncusUIController struct {
	db           *gorm.DB
	proxyBaseURL string
	adminKey     string
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
	}
}

// ProxyIncusUI handles proxying Incus UI requests to tt-backend.
// It checks authorization, then reverse-proxies the request.
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

	// Build the target URL for tt-backend
	remainingPath := ctx.Param("path")
	remainingPath = strings.TrimPrefix(remainingPath, "/")

	targetPath := strings.TrimRight(c.proxyBaseURL, "/") + "/1.0/admin/incus-ui/" + backendID + "/" + remainingPath

	// Forward the request to tt-backend
	proxyReq, err := http.NewRequestWithContext(ctx.Request.Context(), ctx.Request.Method, targetPath, ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error_code":    http.StatusInternalServerError,
			"error_message": "Failed to create proxy request",
		})
		return
	}

	// Copy headers from the original request
	for key, values := range ctx.Request.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Set admin API key for tt-backend authentication
	if c.adminKey != "" {
		proxyReq.Header.Set("X-API-Key", c.adminKey)
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{
			"error_code":    http.StatusBadGateway,
			"error_message": "Failed to proxy request to backend",
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			ctx.Writer.Header().Add(key, value)
		}
	}

	ctx.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(ctx.Writer, resp.Body)
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
