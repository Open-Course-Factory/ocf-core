// tests/payment/gateway_test.go
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	auditModels "soli/formations/src/audit/models"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	paymentController "soli/formations/src/payment/routes"
	"soli/formations/src/utils"
)

// The two callers installRealGateway knows.
const (
	gatewayMember = "member-through-gateway"
	gatewayAdmin  = "admin-through-gateway"
)

// installRealGateway makes AuthManagement decide for real in this test: Casbin
// holds the production payment policies, gatewayMember holds the member role
// and gatewayAdmin the administrator role. Only the JWT signature check is
// stubbed: the bearer token is the caller's user id.
func installRealGateway(t *testing.T) {
	t.Helper()

	dsn := "file:gateway_" + uuid.NewString() + "?mode=memory&cache=shared&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authModels.TokenBlacklist{}, &authModels.BlockedAccount{}, &auditModels.AuditLog{}))

	prevDB, prevParser, prevEnforcer := sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer
	access.RouteRegistry.Reset()
	t.Cleanup(func() {
		sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer = prevDB, prevParser, prevEnforcer
		access.RouteRegistry.Reset()
	})

	sqldb.DB = db
	casdoor.JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
		return &casdoorsdk.Claims{User: casdoorsdk.User{Id: token}}, nil
	}
	casdoor.InitCasdoorEnforcer(db, crudPermBasePath())
	paymentController.RegisterPaymentPermissions(casdoor.Enforcer)

	opts := utils.DefaultPermissionOptions()
	require.NoError(t, utils.AddGroupingPolicy(casdoor.Enforcer, gatewayMember, string(authModels.Member), opts))
	require.NoError(t, utils.AddGroupingPolicy(casdoor.Enforcer, gatewayAdmin, string(authModels.Administrator), opts))
}

// requestAs sends one JSON request signed in as userID; an empty userID sends
// no token at all.
func requestAs(router *gin.Engine, userID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+userID)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// assertRefusedByGateway fails unless AuthManagement itself refused the caller,
// as opposed to a 403 from a later middleware or from the handler.
func assertRefusedByGateway(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.JSONEq(t, `{"msg":"You are not authorized"}`, w.Body.String())
}
