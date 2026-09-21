package routes

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	auth "soli/formations/src/auth"
	"soli/formations/src/auth/middleware"
	"soli/formations/src/configuration/models"
)

const maxLogoBytes = 256 * 1024

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateBranding(b *models.Branding) error {
	b.ID = 1
	b.Name = strings.TrimSpace(b.Name)
	if utf8.RuneCountInString(b.Name) < 1 || utf8.RuneCountInString(b.Name) > 80 {
		return errors.New("name must contain 1 to 80 characters")
	}
	if !colorPattern.MatchString(b.PrimaryColor) || !colorPattern.MatchString(b.SecondaryColor) {
		return errors.New("colors must use #RRGGBB format")
	}
	if b.Logo == "" {
		return nil
	}
	var format, prefix string
	for _, candidate := range []string{"png", "jpeg"} {
		p := "data:image/" + candidate + ";base64,"
		if strings.HasPrefix(b.Logo, p) {
			format, prefix = candidate, p
		}
	}
	if prefix == "" {
		return errors.New("logo must be an embedded PNG or JPEG")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(b.Logo, prefix))
	if err != nil || len(raw) > maxLogoBytes {
		return errors.New("logo must be a valid image of at most 256 KiB")
	}
	cfg, detected, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || detected != format || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 || cfg.Width*cfg.Height > 2097152 {
		return errors.New("invalid logo dimensions or image format")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return errors.New("invalid logo image")
	}
	// Re-encode to keep image pixels only, without metadata or appended content.
	var clean bytes.Buffer
	if format == "png" {
		err = png.Encode(&clean, img)
	} else {
		err = jpeg.Encode(&clean, img, &jpeg.Options{Quality: 90})
	}
	if err != nil || clean.Len() > maxLogoBytes {
		return errors.New("normalized logo exceeds 256 KiB")
	}
	b.Logo = prefix + base64.StdEncoding.EncodeToString(clean.Bytes())
	return nil
}

func BrandingRoutes(router *gin.RouterGroup, db *gorm.DB) {
	c := brandingController{db: db}
	router.GET("/branding", c.get)
	router.PUT("/branding", auth.NewAuthMiddleware(db).AuthManagement(), c.put)
}

type brandingController struct{ db *gorm.DB }

func (c brandingController) get(ctx *gin.Context) {
	b := models.DefaultBranding()
	if err := c.db.First(&b, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to load branding"})
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, b)
}

func (c brandingController) put(ctx *gin.Context) {
	if middleware.RefuseIfImpersonated(ctx, "Updating instance branding") {
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 400*1024)
	var b models.Branding
	if err := ctx.ShouldBindJSON(&b); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid branding payload (maximum 400 KiB)"})
		return
	}
	if err := validateBranding(&b); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := c.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "logo", "primary_color", "secondary_color"})}).Create(&b).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save branding"})
		return
	}
	ctx.JSON(http.StatusOK, b)
}
