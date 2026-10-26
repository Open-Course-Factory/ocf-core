package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"soli/formations/src/configuration/dto"
	"soli/formations/src/configuration/models"
)

const maxLogoBytes = 256 * 1024

// minBrandContrast is the WCAG 2 AA ratio for UI components and large text.
// Brand colors carry white button labels and are drawn on both the light and
// the dark theme background, so they must reach it against all three.
// ponytail: 3:1, not the 4.5:1 body-text ratio — OCF's own #007bff is 3.98:1
// on white; raise it once the default palette is reworked.
const minBrandContrast = 3.0

// Theme backgrounds a brand color is drawn on (variables.css --color-bg-primary).
const (
	lightThemeBackground = "#ffffff"
	darkThemeBackground  = "#1a1a1a"
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ErrInvalidBranding marks a rejected update; its message is safe to show.
var ErrInvalidBranding = errors.New("invalid branding")

type BrandingService struct {
	db *gorm.DB
}

func NewBrandingService(db *gorm.DB) *BrandingService {
	return &BrandingService{db: db}
}

// GetBranding returns the stored identity without the logo payload, or the
// defaults when an administrator never saved one.
func (s *BrandingService) GetBranding() (models.Branding, error) {
	b := models.DefaultBranding()
	err := s.db.Omit("logo").First(&b, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.DefaultBranding(), nil
	}
	return b, err
}

// GetLogo returns the stored logo as a data URI, "" when there is none.
func (s *BrandingService) GetLogo() (logo string, version string, err error) {
	var b models.Branding
	err = s.db.Select("logo", "logo_version").First(&b, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", nil
	}
	return b.Logo, b.LogoVersion, err
}

// InstanceName is the display name of this OCF instance, for emails and other
// server-rendered text. It falls back to the default on any read error.
func (s *BrandingService) InstanceName() string {
	b, err := s.GetBranding()
	if err != nil {
		return models.DefaultBranding().Name
	}
	return b.Name
}

// UpdateBranding validates input and replaces the singleton identity.
func (s *BrandingService) UpdateBranding(input dto.UpdateBrandingInput) (models.Branding, error) {
	b := models.Branding{
		ID:             1,
		Name:           strings.TrimSpace(input.Name),
		PrimaryColor:   input.PrimaryColor,
		SecondaryColor: input.SecondaryColor,
	}
	if err := validateBrandName(b.Name); err != nil {
		return models.Branding{}, err
	}
	if err := validateBrandColor(b.PrimaryColor); err != nil {
		return models.Branding{}, err
	}
	if err := validateBrandColor(b.SecondaryColor); err != nil {
		return models.Branding{}, err
	}

	columns := []string{"name", "primary_color", "secondary_color"}
	if input.Logo != nil {
		if err := setNormalizedLogo(&b, *input.Logo); err != nil {
			return models.Branding{}, err
		}
		columns = append(columns, "logo", "logo_version", "logo_width", "logo_height")
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&b).Error
	if err != nil {
		return models.Branding{}, err
	}
	return s.GetBranding()
}

func validateBrandName(name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > 80 {
		return fmt.Errorf("%w: name must contain 1 to 80 characters", ErrInvalidBranding)
	}
	return nil
}

func validateBrandColor(color string) error {
	if !colorPattern.MatchString(color) {
		return fmt.Errorf("%w: colors must use #RRGGBB format", ErrInvalidBranding)
	}
	for _, against := range []string{lightThemeBackground, darkThemeBackground} {
		if contrastRatio(color, against) < minBrandContrast {
			return fmt.Errorf("%w: %s lacks contrast against %s (WCAG AA needs %.0f:1)", ErrInvalidBranding, color, against, minBrandContrast)
		}
	}
	return nil
}

// contrastRatio is the WCAG 2 contrast ratio between two #RRGGBB colors.
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func relativeLuminance(color string) float64 {
	weights := [3]float64{0.2126, 0.7152, 0.0722}
	var luminance float64
	for i, weight := range weights {
		channel, _ := strconv.ParseUint(color[1+2*i:3+2*i], 16, 8)
		c := float64(channel) / 255
		if c <= 0.04045 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		luminance += weight * c
	}
	return luminance
}

// setNormalizedLogo stores dataURI on b after re-encoding it, which keeps the
// pixels only and drops metadata or appended content. "" clears the logo.
func setNormalizedLogo(b *models.Branding, dataURI string) error {
	if dataURI == "" {
		return nil
	}
	format, raw, err := DecodeLogoDataURI(dataURI)
	if err != nil || len(raw) > maxLogoBytes {
		return fmt.Errorf("%w: logo must be an embedded PNG or JPEG of at most 256 KiB", ErrInvalidBranding)
	}
	cfg, detected, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || detected != format || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 || cfg.Width*cfg.Height > 2097152 {
		return fmt.Errorf("%w: invalid logo dimensions or image format", ErrInvalidBranding)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: invalid logo image", ErrInvalidBranding)
	}
	var clean bytes.Buffer
	if format == "png" {
		err = png.Encode(&clean, img)
	} else {
		err = jpeg.Encode(&clean, img, &jpeg.Options{Quality: 90})
	}
	if err != nil || clean.Len() > maxLogoBytes {
		return fmt.Errorf("%w: normalized logo exceeds 256 KiB", ErrInvalidBranding)
	}
	sum := sha256.Sum256(clean.Bytes())
	b.Logo = "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(clean.Bytes())
	b.LogoVersion = hex.EncodeToString(sum[:8])
	b.LogoWidth, b.LogoHeight = cfg.Width, cfg.Height
	return nil
}

// DecodeLogoDataURI splits a logo data URI into its image format ("png" or
// "jpeg") and bytes.
func DecodeLogoDataURI(dataURI string) (format string, raw []byte, err error) {
	for _, candidate := range []string{"png", "jpeg"} {
		prefix := "data:image/" + candidate + ";base64,"
		if encoded, found := strings.CutPrefix(dataURI, prefix); found {
			raw, err = base64.StdEncoding.DecodeString(encoded)
			return candidate, raw, err
		}
	}
	return "", nil, errors.New("logo is not a PNG or JPEG data URI")
}
