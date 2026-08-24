package terminalController

import (
	"net/http"
	"sort"
	"strings"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// DistributionCatalogEntry is one row of the admin catalogue screen: a
// distribution the backend can run, and whether people are offered it.
type DistributionCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	OsType      string `json:"os_type,omitempty"`
	MinSizeKey  string `json:"min_size_key,omitempty"`
	Offered     bool   `json:"offered"`
}

// UpdateDistributionCatalogInput carries the names to withhold from the picker.
//
// The administrator sends the whole set rather than a delta, so the screen and
// the stored value cannot drift apart: whatever the boxes say is what is saved.
type UpdateDistributionCatalogInput struct {
	Withheld []string `json:"withheld"`
}

// GetDistributionCatalog godoc
//
//	@Summary		List every distribution with its picker visibility
//	@Description	Admin view of the terminal catalogue: every distribution the backend can run, and whether it is offered in the session launcher
//	@Tags			terminals
//	@Produce		json
//	@Param			backend	query		string	false	"Backend ID"
//	@Security		Bearer
//	@Success		200	{array}		DistributionCatalogEntry
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/admin/distribution-catalog [get]
func (tc *terminalController) GetDistributionCatalog(ctx *gin.Context) {
	backend := ctx.Query("backend")

	// The complete list on purpose: this screen is where withheld entries are
	// made visible again, so it is the one place that must still see them.
	all, err := tc.service.GetDistributions(backend)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get distributions: " + err.Error(),
		})
		return
	}

	offered, err := tc.service.GetOfferedDistributions(backend)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get offered distributions: " + err.Error(),
		})
		return
	}

	isOffered := make(map[string]bool, len(offered))
	for _, d := range offered {
		isOffered[strings.ToLower(d.Name)] = true
	}

	entries := make([]DistributionCatalogEntry, 0, len(all))
	for _, d := range all {
		entries = append(entries, DistributionCatalogEntry{
			Name:        d.Name,
			Description: d.Description,
			OsType:      d.OsType,
			MinSizeKey:  d.MinSizeKey,
			Offered:     isOffered[strings.ToLower(d.Name)],
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	ctx.JSON(http.StatusOK, entries)
}

// UpdateDistributionCatalog godoc
//
//	@Summary		Set which distributions are offered in the launcher
//	@Description	Replaces the withheld set. Withheld distributions stay launchable by name, so scenarios that declare them are unaffected.
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			input	body	UpdateDistributionCatalogInput	true	"Names to withhold"
//	@Security		Bearer
//	@Success		200	{array}		DistributionCatalogEntry
//	@Failure		400	{object}	errors.APIError	"Invalid request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/terminals/admin/distribution-catalog [put]
func (tc *terminalController) UpdateDistributionCatalog(ctx *gin.Context) {
	var input UpdateDistributionCatalogInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid request body: " + err.Error(),
		})
		return
	}

	if err := tc.service.SetWithheldDistributions(input.Withheld); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save the distribution catalogue: " + err.Error(),
		})
		return
	}

	// Answer with the resulting catalogue so the screen renders what was
	// actually stored rather than what it hoped it stored.
	tc.GetDistributionCatalog(ctx)
}
