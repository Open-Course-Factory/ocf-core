package brandingRoutes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	"soli/formations/src/configuration/dto"
	"soli/formations/src/configuration/services"
)

type BrandingController struct {
	service *services.BrandingService
}

func NewBrandingController(service *services.BrandingService) *BrandingController {
	return &BrandingController{service: service}
}

// GetBranding godoc
// @Summary Get the instance identity (name, colors, logo version)
// @Tags Branding
// @Produce json
// @Success 200 {object} models.Branding
// @Router /api/v1/branding [get]
func (c *BrandingController) GetBranding(ctx *gin.Context) {
	b, err := c.service.GetBranding()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load branding"})
		return
	}
	// The identity changes rarely but must show at once after a save; the
	// payload is small since the logo moved to its own endpoint.
	ctx.Header("Cache-Control", "no-cache")
	ctx.JSON(http.StatusOK, b)
}

// GetLogo godoc
// @Summary Get the instance logo image
// @Description Cacheable: send If-None-Match with the ETag (the logo_version) to get a 304.
// @Tags Branding
// @Produce png,jpeg
// @Success 200 {file} binary
// @Success 304
// @Failure 404 {object} map[string]string
// @Router /api/v1/branding/logo [get]
func (c *BrandingController) GetLogo(ctx *gin.Context) {
	logo, version, err := c.service.GetLogo()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load logo"})
		return
	}
	if logo == "" {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "No logo configured"})
		return
	}
	etag := `"` + version + `"`
	// Clients version the URL with logo_version, so a day of freshness is
	// safe; the ETag makes the revalidation after that a 304.
	ctx.Header("Cache-Control", "public, max-age=86400")
	ctx.Header("ETag", etag)
	if ctx.GetHeader("If-None-Match") == etag {
		ctx.Status(http.StatusNotModified)
		return
	}
	format, raw, err := services.DecodeLogoDataURI(logo)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Stored logo is corrupt"})
		return
	}
	ctx.Data(http.StatusOK, "image/"+format, raw)
}

// UpdateBranding godoc
// @Summary Replace the instance identity (administrator only)
// @Tags Branding
// @Security Bearer
// @Accept json
// @Produce json
// @Param branding body dto.UpdateBrandingInput true "Identity; omit logo to keep it, send \"\" to remove it"
// @Success 200 {object} models.Branding
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /api/v1/branding [put]
func (c *BrandingController) UpdateBranding(ctx *gin.Context) {
	if middleware.RefuseIfImpersonated(ctx, "Updating instance branding") {
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 400*1024)
	var input dto.UpdateBrandingInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid branding payload (maximum 400 KiB)"})
		return
	}
	b, err := c.service.UpdateBranding(input)
	if errors.Is(err, services.ErrInvalidBranding) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save branding"})
		return
	}
	ctx.JSON(http.StatusOK, b)
}
