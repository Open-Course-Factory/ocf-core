package organizations_tests

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	organizationRoutes "soli/formations/src/organizations/routes"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Debug logging on, so a key leaking through a debug line would show up in
// the captured logs below. It only makes this package's logs more verbose.
func init() { os.Setenv("ENVIRONMENT", "test") }

const aiTestKey = "sk-do-not-leak-0123456789"

// storeAiConfig writes a config row directly, bypassing UpdateConfig's
// endpoint validation — the way a row saved before that validation existed,
// or a hostname that later resolves elsewhere, reaches Generate.
func storeAiConfig(t *testing.T, db *gorm.DB, orgID uuid.UUID, endpoint string) {
	t.Helper()
	require.NoError(t, db.Create(&models.OrganizationAiConfig{
		OrganizationID: orgID, Enabled: true, Endpoint: endpoint, Model: "m", APIKey: aiTestKey,
	}).Error)
}

// aiTestService talks to an httptest TLS server through the production
// client settings (no redirects, response cap), only allowing loopback and
// trusting the test certificate.
func aiTestService(db *gorm.DB, server *httptest.Server) services.OrganizationAiService {
	pool := server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	client := services.NewAiProviderClient(func(netip.Addr) bool { return true }, pool)
	return services.NewOrganizationAiServiceWithClient(db, client)
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// --- Fix 1: SSRF ----------------------------------------------------------

func TestOrganizationAiService_UpdateConfig_RejectsUnsafeEndpoints(t *testing.T) {
	cases := []string{
		"http://api.example.com/v1/chat/completions",
		"ftp://api.example.com",
		"https://",
		"https://127.0.0.1/v1",
		"https://localhost/v1",
		"https://10.0.0.5/v1",
		"https://192.168.1.10/v1",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/v1",
		"https://[fd00::1]/v1",
		"https://user:secret@api.example.com/v1",
	}
	for _, endpoint := range cases {
		t.Run(endpoint, func(t *testing.T) {
			db := newAiConfigDB(t)
			key := aiTestKey
			_, err := services.NewOrganizationAiService(db).UpdateConfig(uuid.New(), dto.UpdateAiConfigInput{
				Enabled: true, Endpoint: endpoint, Model: "m", APIKey: &key,
			})
			assert.ErrorIs(t, err, services.ErrAiEndpointInvalid)

			var count int64
			db.Model(&models.OrganizationAiConfig{}).Count(&count)
			assert.Zero(t, count, "a refused endpoint must not be stored")
		})
	}
}

func TestOrganizationAiService_UpdateConfig_AcceptsPublicHTTPSEndpoint(t *testing.T) {
	db := newAiConfigDB(t)
	key := aiTestKey
	out, err := services.NewOrganizationAiService(db).UpdateConfig(uuid.New(), dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://api.example.com/v1/chat/completions", Model: "m", APIKey: &key,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/chat/completions", out.Endpoint)
}

// The dialer, not the save-time check, is what stops DNS rebinding: a stored
// endpoint whose address is private is refused when the connection is made.
func TestOrganizationAiService_Generate_RefusesPrivateAddressAtConnectTime(t *testing.T) {
	db := newAiConfigDB(t)
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL)

	_, err := services.NewOrganizationAiService(db).Generate(orgID, "hello")

	assert.ErrorIs(t, err, services.ErrAiProvider)
	assert.Zero(t, hits.Load(), "the loopback server must never be reached")
}

// A row saved before validation existed may hold a plain http endpoint; the
// key must not be sent over it.
func TestOrganizationAiService_Generate_RefusesStoredPlainHTTPEndpoint(t *testing.T) {
	db := newAiConfigDB(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer tlsServer.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL)

	_, err := aiTestService(db, tlsServer).Generate(orgID, "hello")

	assert.ErrorIs(t, err, services.ErrAiProvider)
	assert.Zero(t, hits.Load(), "the key must not travel over plain http")
}

func TestOrganizationAiService_Generate_DoesNotFollowRedirects(t *testing.T) {
	db := newAiConfigDB(t)
	var redirectedHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/internal", http.StatusFound)
	})
	mux.HandleFunc("/internal", func(w http.ResponseWriter, r *http.Request) {
		redirectedHits.Add(1)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL+"/v1")

	_, err := aiTestService(db, server).Generate(orgID, "hello")

	assert.ErrorIs(t, err, services.ErrAiProvider)
	assert.Zero(t, redirectedHits.Load(), "a redirect must not be followed")
}

// --- Fix 2: changing the endpoint needs a new key ------------------------

func TestOrganizationAiService_UpdateConfig_EndpointChangeWithoutNewKeyIsRefused(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key := aiTestKey
	_, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://api.example.com/v1", Model: "m", APIKey: &key,
	})
	require.NoError(t, err)

	_, err = svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://attacker.example.net/v1", Model: "m", APIKey: nil,
	})
	assert.ErrorIs(t, err, services.ErrAiKeyRequired)

	out, err := svc.GetConfig(orgID)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1", out.Endpoint, "the stored endpoint must stay as it was")
}

func TestOrganizationAiService_UpdateConfig_EndpointChangeWithNewKeyIsAccepted(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key, newKey := aiTestKey, "sk-new-key"
	_, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://api.example.com/v1", Model: "m", APIKey: &key,
	})
	require.NoError(t, err)

	out, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://other.example.com/v1", Model: "m", APIKey: &newKey,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://other.example.com/v1", out.Endpoint)
	assert.True(t, out.HasAPIKey)
}

// --- Fix 3: generate is teacher+, body capped ----------------------------

func TestOrganizationAiGenerate_Route_DeclaredForTeacherOrgRole(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	organizationRoutes.RegisterOrganizationPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("POST", "/api/v1/organizations/:id/ai-config/generate")
	require.True(t, found)
	assert.Equal(t, access.OrgRole, perm.Access.Type)
	assert.Equal(t, access.RoleMinimumForClassrooms, perm.Access.MinRole,
		"teachers author scenarios with it; students must not spend the organization's AI key")
	assert.Equal(t, "id", perm.Access.Param)
}

func aiGenerateRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := controller.NewOrganizationController(nil, nil, nil, nil, db)
	r.POST("/organizations/:id/ai-config/generate", ctrl.GenerateWithAi)
	r.PUT("/organizations/:id/ai-config", ctrl.UpdateAiConfig)
	return r
}

func errorMessageOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		ErrorMessage string `json:"error_message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body=%s", w.Body.String())
	return body.ErrorMessage
}

func TestOrganizationAiGenerate_OversizedBody_413(t *testing.T) {
	db := newAiConfigDB(t)
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, "https://api.example.com/v1")

	payload, _ := json.Marshal(dto.GenerateInput{Prompt: strings.Repeat("a", 2<<20)})
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/ai-config/generate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	aiGenerateRouter(db).ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Equal(t, "ai_request_too_large", errorMessageOf(t, w))
}

func TestOrganizationAiGenerate_ErrorCodes(t *testing.T) {
	db := newAiConfigDB(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	configured := uuid.New()
	storeAiConfig(t, db, configured, server.URL)

	cases := []struct {
		name   string
		orgID  uuid.UUID
		status int
		code   string
	}{
		{"not configured", uuid.New(), http.StatusConflict, "ai_not_configured"},
		{"provider unreachable", configured, http.StatusBadGateway, "ai_provider_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(dto.GenerateInput{Prompt: "hello"})
			req := httptest.NewRequest(http.MethodPost, "/organizations/"+tc.orgID.String()+"/ai-config/generate", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			aiGenerateRouter(db).ServeHTTP(w, req)

			assert.Equal(t, tc.status, w.Code)
			assert.Equal(t, tc.code, errorMessageOf(t, w))
		})
	}
}

func TestOrganizationAiUpdate_ErrorCodes(t *testing.T) {
	db := newAiConfigDB(t)
	orgID := uuid.New()
	key := aiTestKey
	_, err := services.NewOrganizationAiService(db).UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://api.example.com/v1", Model: "m", APIKey: &key,
	})
	require.NoError(t, err)

	cases := []struct {
		name string
		body string
		code string
	}{
		{"invalid endpoint", `{"enabled":true,"endpoint":"http://api.example.com","model":"m","api_key":"sk-x"}`, "ai_endpoint_invalid"},
		{"endpoint changed without key", `{"enabled":true,"endpoint":"https://other.example.com","model":"m"}`, "ai_key_required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/organizations/"+orgID.String()+"/ai-config", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			aiGenerateRouter(db).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.code, errorMessageOf(t, w))
		})
	}
}

// --- Fix 4: capped response, key never in errors or logs -----------------

func TestOrganizationAiService_Generate_RefusesOversizedResponse(t *testing.T) {
	db := newAiConfigDB(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"`))
		w.Write(bytes.Repeat([]byte("a"), 3<<20))
		w.Write([]byte(`"}}]}`))
	}))
	defer server.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL)

	_, err := aiTestService(db, server).Generate(orgID, "hello")

	assert.ErrorIs(t, err, services.ErrAiProvider)
}

func TestOrganizationAiService_Generate_KeyNeverInErrorOrLogs(t *testing.T) {
	db := newAiConfigDB(t)
	// A provider that echoes the credential back, as some do in their 401s.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key ` + r.Header.Get("Authorization") + `"}`))
	}))
	defer server.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL)
	logs := captureLogs(t)

	_, err := aiTestService(db, server).Generate(orgID, "hello")

	require.ErrorIs(t, err, services.ErrAiProvider)
	assert.NotContains(t, err.Error(), aiTestKey)
	assert.NotContains(t, logs.String(), aiTestKey)
}

func TestOrganizationAiGenerate_KeyNeverInResponseOrLogs(t *testing.T) {
	db := newAiConfigDB(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	orgID := uuid.New()
	storeAiConfig(t, db, orgID, server.URL)
	logs := captureLogs(t)

	payload, _ := json.Marshal(dto.GenerateInput{Prompt: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/ai-config/generate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	aiGenerateRouter(db).ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
	assert.NotContains(t, w.Body.String(), aiTestKey)
	assert.NotContains(t, logs.String(), aiTestKey)
}
