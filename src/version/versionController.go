package version

import (
	"net/http"
	"os"

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
	version := os.Getenv("OCF_VERSION")
	if version == "" {
		version = "unknown"
	}

	c.JSON(http.StatusOK, gin.H{
		"version": version,
	})
}
