package version

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type VersionController struct{}

func NewVersionController() *VersionController {
	return &VersionController{}
}

// GetVersion godoc
// @Summary Get API version
// @Description Returns the current version of the OCF API
// @Tags version
// @Produce json
// @Success 200 {object} map[string]string
// @Router /version [get]
func (vc *VersionController) GetVersion(c *gin.Context) {
	version := getVersion()

	c.JSON(http.StatusOK, gin.H{
		"version": version,
	})
}

// getVersion reads version from VERSION file or falls back to env var
func getVersion() string {
	// Try to read from VERSION file first
	if data, err := os.ReadFile("VERSION"); err == nil {
		return strings.TrimSpace(string(data))
	}

	// Fallback to environment variable
	if version := os.Getenv("OCF_VERSION"); version != "" {
		return version
	}

	return "unknown"
}
