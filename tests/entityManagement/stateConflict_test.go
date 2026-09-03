// tests/entityManagement/stateConflict_test.go
//
// A Before* hook that refuses because of the entity's current state must reach
// the client as 409 with the reason, not as the generic hook-failure 500.
package entityManagement_tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEntity_BeforeCreateHookStateConflict_Refuses409WithReason(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|POST|PATCH)")
	reason := errors.New("the class is archived")
	registerRecordingHook(t, "refuse-on-state", entityErrors.NewStateConflictError(archivableEntityName, reason), hooks.BeforeCreate)

	body, err := json.Marshal(ArchivableThingInput{Name: "late-comer"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, archivableBasePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var response struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, entityErrors.ErrStateConflict.Code, response.Error.Code)
	assert.Equal(t, reason.Error(), response.Error.Details["reason"])

	var count int64
	require.NoError(t, env.db.Model(&ArchivableThing{}).Count(&count).Error)
	assert.Zero(t, count, "a refused create must not insert the row")
}

func TestNewStateConflictError_WrapsTheDomainReason(t *testing.T) {
	reason := errors.New("archived")
	err := entityErrors.NewStateConflictError("ClassGroup", reason)

	assert.True(t, errors.Is(err, reason), "errors.Is must still find the domain sentinel")
	assert.Equal(t, http.StatusConflict, err.HTTPStatus)
}
