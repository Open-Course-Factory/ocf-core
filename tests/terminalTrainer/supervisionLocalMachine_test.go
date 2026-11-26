package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	auditModels "soli/formations/src/audit/models"
	terminalController "soli/formations/src/terminalTrainer/routes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// A machine in the learner's browser has no console on any server: nobody can
// watch it, and supervising it is refused plainly, before anything is audited
// or dialled.
func TestSuperviseSession_LocalMachineCannotBeWatched(t *testing.T) {
	db := freshTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "an-admin")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	router.GET("/terminals/:id/supervise", terminalController.NewTerminalController(db).SuperviseSession)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/terminals/local-0000/supervise", nil))

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "browser")
	var audited int64
	db.Model(&auditModels.AuditLog{}).Count(&audited)
	assert.Zero(t, audited, "no supervision is recorded")
}
