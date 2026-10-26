package configuration_tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/configuration/dto"
	"soli/formations/src/configuration/models"
	"soli/formations/src/configuration/routes/brandingRoutes"
	"soli/formations/src/configuration/services"
)

type brandingTestServer struct {
	router *gin.Engine
	db     *gorm.DB
}

// newBrandingTestServer mounts the real branding routes, declarations and
// Layer 2 middleware. The X-Test-Roles header stands in for the JWT that
// AuthManagement would decode.
func newBrandingTestServer(t *testing.T) brandingTestServer {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Branding{}))

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	brandingRoutes.RegisterBrandingPermissions(mocks.NewMockEnforcer())
	access.RegisterBuiltinEnforcers(nil, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	identity := func(c *gin.Context) {
		if roles := c.GetHeader("X-Test-Roles"); roles != "" {
			c.Set("userId", "user-1")
			c.Set("userRoles", []string{roles})
		}
		c.Next()
	}
	api := router.Group("/api/v1", identity, access.Layer2Enforcement())
	controller := brandingRoutes.NewBrandingController(services.NewBrandingService(db))
	brandingRoutes.MountBrandingRoutes(api, controller, func(c *gin.Context) { c.Next() })
	return brandingTestServer{router: router, db: db}
}

func (s brandingTestServer) do(method, path, role string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("X-Test-Roles", role)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func pngDataURI(t *testing.T, width, height int) string {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, height))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}

func brandingInput(name string, logo *string) dto.UpdateBrandingInput {
	d := models.DefaultBranding()
	return dto.UpdateBrandingInput{Name: name, PrimaryColor: d.PrimaryColor, SecondaryColor: d.SecondaryColor, Logo: logo}
}

func TestBrandingUpdate_Member_IsForbiddenAndLeavesDefaults(t *testing.T) {
	s := newBrandingTestServer(t)

	w := s.do("PUT", "/api/v1/branding", "member", brandingInput("Hijacked", nil), nil)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, s.db.Model(&models.Branding{}).Count(&count).Error)
	assert.Zero(t, count, "a refused update must not write the row")
	assert.Equal(t, "OCF", services.NewBrandingService(s.db).InstanceName())
}

func TestBrandingUpdate_Administrator_ServesLogoFromCacheableEndpoint(t *testing.T) {
	s := newBrandingTestServer(t)
	logo := pngDataURI(t, 4, 2)

	w := s.do("PUT", "/api/v1/branding", "administrator", brandingInput("Classe Foehn", &logo), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = s.do("GET", "/api/v1/branding", "", nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "base64", "the logo must not be inlined")
	var got models.Branding
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Classe Foehn", got.Name)
	assert.Equal(t, 4, got.LogoWidth)
	assert.Equal(t, 2, got.LogoHeight)
	require.NotEmpty(t, got.LogoVersion)
	assert.Equal(t, "Classe Foehn", services.NewBrandingService(s.db).InstanceName())

	w = s.do("GET", "/api/v1/branding/logo", "", nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, `"`+got.LogoVersion+`"`, w.Header().Get("ETag"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age")
	_, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
	assert.NoError(t, err, "the logo endpoint must serve the image bytes")

	w = s.do("GET", "/api/v1/branding/logo", "", nil, map[string]string{"If-None-Match": `"` + got.LogoVersion + `"`})
	assert.Equal(t, http.StatusNotModified, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestBrandingUpdate_OmittedLogoKeepsIt_EmptyLogoRemovesIt(t *testing.T) {
	s := newBrandingTestServer(t)
	logo := pngDataURI(t, 2, 2)
	require.Equal(t, http.StatusOK, s.do("PUT", "/api/v1/branding", "administrator", brandingInput("A", &logo), nil).Code)

	require.Equal(t, http.StatusOK, s.do("PUT", "/api/v1/branding", "administrator", brandingInput("B", nil), nil).Code)
	assert.Equal(t, http.StatusOK, s.do("GET", "/api/v1/branding/logo", "", nil, nil).Code, "omitting logo keeps it")

	empty := ""
	require.Equal(t, http.StatusOK, s.do("PUT", "/api/v1/branding", "administrator", brandingInput("C", &empty), nil).Code)
	assert.Equal(t, http.StatusNotFound, s.do("GET", "/api/v1/branding/logo", "", nil, nil).Code)

	var count int64
	require.NoError(t, s.db.Model(&models.Branding{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "branding is a singleton")
}

func TestBrandingUpdate_NormalizesLogo_DropsTrailingPayload(t *testing.T) {
	s := newBrandingTestServer(t)
	clean := pngDataURI(t, 2, 2)
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(clean, "data:image/png;base64,"))
	tainted := "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(raw, []byte("untrusted trailing data")...))

	require.Equal(t, http.StatusOK, s.do("PUT", "/api/v1/branding", "administrator", brandingInput("A", &tainted), nil).Code)

	w := s.do("GET", "/api/v1/branding/logo", "", nil, nil)
	assert.NotContains(t, w.Body.String(), "untrusted trailing data")
}

func TestBrandingUpdate_RejectsUnsafeOrUnreadableInputs(t *testing.T) {
	tooWide := pngDataURI(t, 2049, 1)
	cases := map[string]dto.UpdateBrandingInput{}
	for name, logo := range map[string]string{
		"remote url":   "https://example.com/logo.png",
		"svg":          "data:image/svg+xml;base64,PHN2Zz4=",
		"not an image": "data:image/png;base64,bm90YW5pbWFnZQ==",
		"too large":    "data:image/png;base64," + strings.Repeat("A", 400000),
		"too wide":     tooWide,
	} {
		cases["logo "+name] = brandingInput("A", &logo)
	}
	cases["blank name"] = brandingInput("  ", nil)
	cssInjection := brandingInput("A", nil)
	cssInjection.PrimaryColor = "red; background:url(https://example.com)"
	cases["css injection"] = cssInjection
	// White button labels on yellow are unreadable; black vanishes on the dark theme.
	for _, color := range []string{"#ffff00", "#000000"} {
		lowContrast := brandingInput("A", nil)
		lowContrast.PrimaryColor = color
		cases["low contrast "+color] = lowContrast
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			s := newBrandingTestServer(t)
			assert.Equal(t, http.StatusBadRequest, s.do("PUT", "/api/v1/branding", "administrator", input, nil).Code)
		})
	}
}

func TestBrandingUpdate_DefaultPaletteIsAccepted(t *testing.T) {
	_, err := services.NewBrandingService(newBrandingTestServer(t).db).UpdateBranding(brandingInput("OCF", nil))
	assert.NoError(t, err)
}
