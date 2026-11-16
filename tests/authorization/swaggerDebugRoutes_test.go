package authorization_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/initialization"
)

// The swagger debug and stats routes were mounted without authentication and
// exposed every entity's swagger configuration; nothing consumes them.
func TestSwaggerSystem_DebugRoutes_AreNotMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	r := gin.New()
	initialization.SetupCompleteSwaggerSystem(r, db)

	get := func(path string) int {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}

	assert.Equal(t, http.StatusOK, get("/api/v1/swagger/spec"), "the merged spec stays served")
	assert.Equal(t, http.StatusOK, get("/swagger/"), "the documentation UI stays served")
	assert.Equal(t, http.StatusNotFound, get("/api/v1/swagger/debug"))
	assert.Equal(t, http.StatusNotFound, get("/api/v1/swagger/stats"))
}
