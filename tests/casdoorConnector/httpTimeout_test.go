package casdoorConnector_test

// Casdoor calls are bounded in time (#586). The SDK's default client is a bare
// &http.Client{} with no timeout: a Casdoor that stops answering holds a lookup
// slot (and a trainer's request) forever. InitCasdoorConnection installs a
// client with a timeout through casdoorsdk.SetHttpClient.
//
// The SDK exposes no getter for its client, so the bound is observed through
// its effect: with casdoor.HTTPTimeout lowered, a Casdoor that never answers
// makes the SDK call fail after about that long.
//
// Contract (package soli/formations/src/auth/casdoor):
//
//	var HTTPTimeout time.Duration // = 10 * time.Second, read by InitCasdoorConnection

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
)

// TestHTTPTimeout_DefaultIsAtMostTenSeconds pins the production bound.
func TestHTTPTimeout_DefaultIsAtMostTenSeconds(t *testing.T) {
	assert.Greater(t, casdoor.HTTPTimeout, time.Duration(0), "a zero timeout means none")
	assert.LessOrEqual(t, casdoor.HTTPTimeout, 10*time.Second)
}

// TestInitCasdoorConnection_UnresponsiveCasdoor_CallTimesOut: the timeout is
// really installed on the client the SDK uses.
func TestInitCasdoorConnection_UnresponsiveCasdoor_CallTimesOut(t *testing.T) {
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(hang); srv.Close() })

	dir := t.TempDir() + string(filepath.Separator)
	require.NoError(t, os.WriteFile(dir+"token_jwt_key.pem", []byte("dummy"), 0o600))
	t.Setenv("CASDOOR_ENDPOINT", srv.URL)
	t.Setenv("CASDOOR_CLIENT_ID", "dummy")
	t.Setenv("CASDOOR_CLIENT_SECRET", "dummy")
	t.Setenv("CASDOOR_ORGANIZATION_NAME", "dummy")
	t.Setenv("CASDOOR_APPLICATION_NAME", "dummy")

	original := casdoor.HTTPTimeout
	casdoor.HTTPTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		casdoor.HTTPTimeout = original
		casdoorsdk.SetHttpClient(&http.Client{})
		casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
	})
	casdoor.InitCasdoorConnection(dir, "no-such.env")

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := casdoorsdk.GetUserByUserId("anyone")
		done <- err
	}()
	select {
	case err := <-done:
		assert.Error(t, err, "an unanswered Casdoor call fails")
		assert.Less(t, time.Since(start), 2*time.Second, "and fails after about HTTPTimeout")
	case <-time.After(3 * time.Second):
		t.Fatal("the Casdoor call is still waiting after 3 s: the SDK client has no timeout")
	}
}
