package routes

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
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"soli/formations/src/configuration/models"
)

func TestBrandingRejectsUnsafeInputs(t *testing.T) {
	for _, logo := range []string{"https://example.com/logo.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,bm90YW5pbWFnZQ==", "data:image/png;base64," + strings.Repeat("A", 400000)} {
		b := models.DefaultBranding()
		b.Logo = logo
		require.Error(t, validateBranding(&b))
	}
	b := models.DefaultBranding()
	b.PrimaryColor = "red; background:url(https://example.com)"
	require.Error(t, validateBranding(&b))
	b = models.DefaultBranding()
	b.Name = "  "
	require.Error(t, validateBranding(&b))
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2049, 1))))
	b = models.DefaultBranding()
	b.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	require.Error(t, validateBranding(&b))
}

func TestBrandingPersistsAndResetsSingleton(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&models.Branding{}))
	r := gin.New()
	c := brandingController{db: db}
	r.GET("/branding", c.get)
	r.PUT("/branding", c.put)
	get := func() models.Branding {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/branding", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var got models.Branding
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	require.Equal(t, "OCF", get().Name)
	b := models.DefaultBranding()
	b.Name = "Classe Foehn"
	b.PrimaryColor = "#123456"
	var pixels bytes.Buffer
	require.NoError(t, png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	b.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(pixels.Bytes(), []byte("untrusted trailing data")...))
	for _, value := range []models.Branding{b, models.DefaultBranding()} {
		payload, err := json.Marshal(value)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/branding", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		got := get()
		require.Equal(t, value.Name, got.Name)
		require.Equal(t, value.PrimaryColor, got.PrimaryColor)
		if value.Logo != "" {
			require.NotEqual(t, value.Logo, got.Logo, "metadata and trailing payload must be stripped")
		}
	}
	var count int64
	require.NoError(t, db.Model(&models.Branding{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
