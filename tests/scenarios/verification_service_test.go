package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestVerificationService creates a VerificationService pointing at a mock server
func newTestVerificationService(serverURL string) *services.VerificationService {
	return services.NewVerificationServiceWithConfig(serverURL, "test-api-key")
}

func TestVerificationService_ExecInContainer_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/1.0/exec", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "session-123", body["session_id"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"exit_code": 0,
			"stdout":    "command output",
			"stderr":    "",
		})
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	exitCode, stdout, stderr, err := svc.ExecInContainer("session-123", []string{"echo", "hello"}, 5)

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "command output", stdout)
	assert.Empty(t, stderr)
}

func TestVerificationService_ExecInContainer_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"exit_code": 1,
			"stdout":    "",
			"stderr":    "command not found",
		})
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	exitCode, stdout, stderr, err := svc.ExecInContainer("session-123", []string{"bad-command"}, 5)

	require.NoError(t, err)
	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "command not found", stderr)
}

func TestVerificationService_PushFile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/1.0/file-push", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "session-123", body["session_id"])
		assert.Equal(t, "/tmp/test.sh", body["target_path"])
		assert.Equal(t, "#!/bin/bash\necho hello", body["content"])
		assert.Equal(t, "0755", body["mode"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	err := svc.PushFile("session-123", "/tmp/test.sh", "#!/bin/bash\necho hello", "0755")
	require.NoError(t, err)
}

func TestVerificationService_VerifyStep_Pass(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		assert.Equal(t, "/1.0/exec", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		// Verify the command passes the script inline via sh -c
		commands := body["command"].([]any)
		assert.Equal(t, "/bin/sh", commands[0])
		assert.Equal(t, "-c", commands[1])
		assert.Equal(t, "#!/bin/bash\nexit 0", commands[2])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"exit_code": 0,
			"stdout":    "verification passed",
			"stderr":    "",
		})
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	step := &models.ScenarioStep{
		Order:        1,
		VerifyScript: "#!/bin/bash\nexit 0",
	}

	passed, output, err := svc.VerifyStep("session-123", step)

	require.NoError(t, err)
	assert.True(t, passed)
	assert.Equal(t, "verification passed", output)
	assert.Equal(t, 1, callCount, "should make exactly 1 call: exec only (no file-push)")
}

func TestVerificationService_VerifyStep_Fail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/1.0/exec", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		// Verify the command passes the script inline via sh -c
		commands := body["command"].([]any)
		assert.Equal(t, "/bin/sh", commands[0])
		assert.Equal(t, "-c", commands[1])
		assert.Equal(t, "#!/bin/bash\ntest -f /etc/config && exit 0 || exit 1", commands[2])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"exit_code": 1,
			"stdout":    "expected file not found",
			"stderr":    "",
		})
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	step := &models.ScenarioStep{
		Order:        2,
		VerifyScript: "#!/bin/bash\ntest -f /etc/config && exit 0 || exit 1",
	}

	passed, output, err := svc.VerifyStep("session-123", step)

	require.NoError(t, err)
	assert.False(t, passed)
	assert.Equal(t, "expected file not found", output)
}

func TestVerificationService_VerifyStep_NoScript(t *testing.T) {
	svc := services.NewVerificationServiceWithConfig("http://localhost:9999", "test-key")

	step := &models.ScenarioStep{
		Order:        1,
		VerifyScript: "",
	}

	passed, output, err := svc.VerifyStep("session-123", step)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no verify script")
	assert.False(t, passed)
	assert.Empty(t, output)
}

func TestVerificationService_ExecInContainer_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	exitCode, stdout, stderr, err := svc.ExecInContainer("session-123", []string{"echo"}, 5)

	assert.Error(t, err)
	assert.Equal(t, -1, exitCode)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestVerificationService_PushFile_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	svc := newTestVerificationService(server.URL)

	err := svc.PushFile("session-123", "/tmp/test.sh", "content", "0755")
	assert.Error(t, err)
}
