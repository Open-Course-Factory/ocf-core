package version

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"soli/formations/src/utils"
)

type VersionController struct{}

func NewVersionController() *VersionController {
	return &VersionController{}
}

// VersionResponse represents the version information
type VersionResponse struct {
	API             string `json:"api" example:"1.2.3"`
	TerminalTrainer string `json:"terminalTrainer" example:"2.0.1"`
}

// GetVersion godoc
// @Summary Get API version
// @Description Returns the current version of the OCF API and Terminal Trainer
// @Tags version
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /version [get]
func (vc *VersionController) GetVersion(c *gin.Context) {
	apiVersion := getAPIVersion()
	terminalTrainerVersion := getTerminalTrainerVersion()

	c.JSON(http.StatusOK, VersionResponse{
		API:             apiVersion,
		TerminalTrainer: terminalTrainerVersion,
	})
}

// getAPIVersion reads API version from VERSION file or falls back to env var
func getAPIVersion() string {
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

// TerminalTrainerVersionResponse represents the response from Terminal Trainer's version endpoint
type TerminalTrainerVersionResponse struct {
	Version string `json:"version"`
}

// getTerminalTrainerVersion fetches Terminal Trainer version from its API endpoint
func getTerminalTrainerVersion() string {
	// Get Terminal Trainer URL from environment
	terminalTrainerURL := os.Getenv("TERMINAL_TRAINER_URL")
	if terminalTrainerURL == "" {
		return "N/A"
	}

	// Build the version endpoint URL
	versionURL := strings.TrimRight(terminalTrainerURL, "/") + "/version"

	// Configure HTTP client with short timeout (we don't want to delay the response)
	opts := utils.HTTPClientOptions{
		Timeout:    2 * time.Second, // Short timeout to avoid blocking
		Headers:    make(map[string]string),
		RetryCount: 0, // No retries for version check
	}

	// Make the HTTP GET request
	resp, err := utils.MakeHTTPRequest("GET", versionURL, nil, opts)
	if err != nil {
		// Return N/A if the request fails (service might be down)
		return "N/A"
	}

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return "N/A"
	}

	// Try to parse the response
	var versionResp TerminalTrainerVersionResponse
	if err := resp.DecodeJSON(&versionResp); err != nil {
		return "N/A"
	}

	// Return the version, or N/A if empty
	if versionResp.Version == "" {
		return "N/A"
	}

	return versionResp.Version
}
