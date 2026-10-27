package organizations_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// OrganizationAiService — the per-organization bring-your-own-key config
// that lets the scenario editor's AI-assisted authoring flow call an
// OpenAI-compatible endpoint directly instead of the teacher copying a
// prompt into their own chat assistant and pasting the answer back
// (src/organizations/services/organizationAiService.go).
// ---------------------------------------------------------------------------

var aiConfigDBSeq int

func newAiConfigDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("FIELD_ENCRYPTION_SECRET", "a-very-secret-key-for-testing-ok")
	aiConfigDBSeq++
	dsn := fmt.Sprintf("file:ai_config_%d?mode=memory&cache=shared", aiConfigDBSeq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.OrganizationAiConfig{}))
	return db
}

func TestOrganizationAiService_GetConfig_UnconfiguredOrgReadsAsDisabled(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)

	out, err := svc.GetConfig(uuid.New())
	require.NoError(t, err)
	assert.False(t, out.Enabled)
	assert.False(t, out.HasAPIKey)
	assert.Empty(t, out.Endpoint)
}

func TestOrganizationAiService_UpdateConfig_CreatesAndReportsHasAPIKey(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key := "sk-test-12345"

	out, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled:  true,
		Endpoint: "https://api.example.com/v1/chat/completions",
		Model:    "gpt-test",
		APIKey:   &key,
	})
	require.NoError(t, err)
	assert.True(t, out.Enabled)
	assert.Equal(t, "https://api.example.com/v1/chat/completions", out.Endpoint)
	assert.Equal(t, "gpt-test", out.Model)
	assert.True(t, out.HasAPIKey)

	// The key itself must never round-trip through the output DTO.
	raw, _ := json.Marshal(out)
	assert.NotContains(t, string(raw), key)

	// And it must be encrypted at rest, not stored in plaintext. Raw+Scan
	// bypasses GORM hooks, so row.APIKey here is exactly the DB column.
	var row struct{ APIKey string }
	require.NoError(t, db.Raw(`SELECT api_key FROM organization_ai_configs WHERE organization_id = ?`, orgID).Scan(&row).Error)
	assert.NotEqual(t, key, row.APIKey)
	assert.Contains(t, row.APIKey, "enc::v1:")
}

func TestOrganizationAiService_UpdateConfig_OmittedAPIKeyKeepsTheStoredOne(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key := "sk-original"

	_, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://a.example.com", Model: "m1", APIKey: &key,
	})
	require.NoError(t, err)

	// Second update changes the model only; APIKey is nil (field omitted).
	out, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: true, Endpoint: "https://a.example.com", Model: "m2", APIKey: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, "m2", out.Model)
	assert.True(t, out.HasAPIKey, "omitting api_key must not clear a previously stored key")

	// The stored ciphertext must still decrypt to the original key — proof
	// the no-op update didn't corrupt it.
	var stored models.OrganizationAiConfig
	require.NoError(t, db.Where("organization_id = ?", orgID).First(&stored).Error)
	assert.Equal(t, "sk-original", stored.APIKey)
}

func TestOrganizationAiService_UpdateConfig_EmptyStringAPIKeyClearsIt(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key := "sk-original"

	_, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{Enabled: true, APIKey: &key})
	require.NoError(t, err)

	empty := ""
	out, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{Enabled: true, APIKey: &empty})
	require.NoError(t, err)
	assert.False(t, out.HasAPIKey, "an explicit empty api_key must clear the stored one")
}

func TestOrganizationAiService_Generate_RefusesWhenNotConfigured(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)

	_, err := svc.Generate(uuid.New(), "write me a scenario")
	assert.ErrorIs(t, err, services.ErrAiNotConfigured)
}

func TestOrganizationAiService_Generate_RefusesWhenDisabledDespiteFullConfig(t *testing.T) {
	db := newAiConfigDB(t)
	svc := services.NewOrganizationAiService(db)
	orgID := uuid.New()
	key := "sk-x"

	_, err := svc.UpdateConfig(orgID, dto.UpdateAiConfigInput{
		Enabled: false, Endpoint: "https://a.example.com", Model: "m", APIKey: &key,
	})
	require.NoError(t, err)

	_, err = svc.Generate(orgID, "hello")
	assert.ErrorIs(t, err, services.ErrAiNotConfigured)
}

func TestOrganizationAiService_Generate_SendsOpenAIShapedRequestAndParsesAnswer(t *testing.T) {
	db := newAiConfigDB(t)
	orgID := uuid.New()
	key := "sk-live"

	var gotAuth, gotBody string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"title\":\"Hi\"}"}}]}`))
	}))
	defer server.Close()

	require.NoError(t, db.Create(&models.OrganizationAiConfig{
		OrganizationID: orgID, Enabled: true, Endpoint: server.URL, Model: "gpt-test", APIKey: key,
	}).Error)

	answer, err := aiTestService(db, server).Generate(orgID, "describe a scenario about SSH")
	require.NoError(t, err)
	assert.Equal(t, `{"title":"Hi"}`, answer)
	assert.Equal(t, "Bearer sk-live", gotAuth)
	assert.Contains(t, gotBody, `"model":"gpt-test"`)
	assert.Contains(t, gotBody, "describe a scenario about SSH")
}

func TestOrganizationAiService_Generate_SurfacesEndpointFailure(t *testing.T) {
	db := newAiConfigDB(t)
	orgID := uuid.New()
	key := "sk-x"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer server.Close()

	require.NoError(t, db.Create(&models.OrganizationAiConfig{
		OrganizationID: orgID, Enabled: true, Endpoint: server.URL, Model: "m", APIKey: key,
	}).Error)

	_, err := aiTestService(db, server).Generate(orgID, "hello")
	require.ErrorIs(t, err, services.ErrAiProvider)
	assert.NotErrorIs(t, err, services.ErrAiNotConfigured, "an endpoint failure is a different problem than a missing configuration")
}
