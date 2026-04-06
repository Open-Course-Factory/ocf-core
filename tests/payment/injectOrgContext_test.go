package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	paymentMiddleware "soli/formations/src/payment/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestInjectOrgContext_FromQueryParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedOrgID string
	router := gin.New()
	router.GET("/test", paymentMiddleware.InjectOrgContext(), func(ctx *gin.Context) {
		val, exists := ctx.Get("org_context_id")
		if exists {
			capturedOrgID = val.(string)
		}
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?organization_id=org-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "org-123", capturedOrgID)
}

func TestInjectOrgContext_FromJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedOrgID string
	router := gin.New()
	router.POST("/test", paymentMiddleware.InjectOrgContext(), func(ctx *gin.Context) {
		val, exists := ctx.Get("org_context_id")
		if exists {
			capturedOrgID = val.(string)
		}
		ctx.Status(http.StatusOK)
	})

	body := `{"organization_id": "org-456", "other_field": "value"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "org-456", capturedOrgID)
}

func TestInjectOrgContext_NoOrgID_NotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgIDSet := false
	router := gin.New()
	router.GET("/test", paymentMiddleware.InjectOrgContext(), func(ctx *gin.Context) {
		_, orgIDSet = ctx.Get("org_context_id")
		ctx.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, orgIDSet, "org_context_id should not be set when no org ID provided")
}

func TestInjectOrgContext_QueryParamTakesPrecedenceOverBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedOrgID string
	router := gin.New()
	router.POST("/test", paymentMiddleware.InjectOrgContext(), func(ctx *gin.Context) {
		val, exists := ctx.Get("org_context_id")
		if exists {
			capturedOrgID = val.(string)
		}
		ctx.Status(http.StatusOK)
	})

	body := `{"organization_id": "body-org"}`
	req := httptest.NewRequest("POST", "/test?organization_id=query-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "query-org", capturedOrgID, "query param should take precedence over body")
}

func TestInjectOrgContext_EmptyQueryParam_FallsBackToBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedOrgID string
	router := gin.New()
	router.POST("/test", paymentMiddleware.InjectOrgContext(), func(ctx *gin.Context) {
		val, exists := ctx.Get("org_context_id")
		if exists {
			capturedOrgID = val.(string)
		}
		ctx.Status(http.StatusOK)
	})

	body := `{"organization_id": "body-org"}`
	req := httptest.NewRequest("POST", "/test?organization_id=", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "body-org", capturedOrgID, "empty query param should fall back to body")
}
