package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogHydrationSource is an inline HYDRATION_SOURCE so these tests are
// self-contained: the in-process size catalog hydrates from it (no .env
// required), giving CPUMcpu the production XS value of 500 mCPU.
const catalogHydrationSource = `[{"key":"XS","cpu":1,"cpu_allowance":"50%","memory":"256MiB","sort_order":0}]`

// newCatalogTestService wires a TerminalTrainerService against the given stub
// URL. The constructor also spins up the enum service, which fires an async
// GET /1.0/enums against the same base URL — callers' stub handlers must
// therefore route by path and tolerate that extra request rather than
// asserting on every inbound path.
func newCatalogTestService(t *testing.T, baseURL string) services.TerminalTrainerService {
	t.Helper()
	t.Setenv("TERMINAL_TRAINER_URL", baseURL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	t.Setenv("HYDRATION_SOURCE", catalogHydrationSource)
	return services.NewTerminalTrainerService(freshTestDB(t))
}

// TestCatalogGetCatalogSizes_StampsCPUMcpu asserts GetCatalogSizes parses the
// /sizes payload AND stamps CPUMcpu from the hydrated payment catalog. The wire
// payload carries cpu_mcpu=0; the catalog stamps the canonical XS value of
// 500 mCPU (half a vCPU).
func TestCatalogGetCatalogSizes_StampsCPUMcpu(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sizes") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"XS","cpu":1,"memory":"256MiB","cpu_mcpu":0,"sort_order":0}]`))
	}))
	defer srv.Close()

	svc := newCatalogTestService(t, srv.URL)

	sizes, err := svc.GetCatalogSizes()
	require.NoError(t, err)
	require.Len(t, sizes, 1)
	assert.Equal(t, "XS", sizes[0].Key)
	assert.Equal(t, 1, sizes[0].CPU)
	assert.Equal(t, "256MiB", sizes[0].Memory)
	// CPUMcpu is stamped from the OCF catalog (XS = 500 mCPU), not the
	// cpu_mcpu=0 on the wire.
	assert.Equal(t, 500, sizes[0].CPUMcpu)
}

// TestCatalogGetCatalogSizes_Caches asserts the 60s TTL cache: the second call
// is served from cache (no second /sizes hit) and equals the first result.
func TestCatalogGetCatalogSizes_Caches(t *testing.T) {
	var sizesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sizes") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&sizesCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"XS","cpu":1,"memory":"256MiB","sort_order":0}]`))
	}))
	defer srv.Close()

	svc := newCatalogTestService(t, srv.URL)

	first, err := svc.GetCatalogSizes()
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := svc.GetCatalogSizes()
	require.NoError(t, err)

	// Only one upstream /sizes fetch: the second call is served from the cache.
	assert.Equal(t, int32(1), atomic.LoadInt32(&sizesCalls))
	assert.Equal(t, first, second)
}

// TestCatalogGetCatalogFeatures_Parses asserts GetCatalogFeatures parses the
// /features payload.
func TestCatalogGetCatalogFeatures_Parses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/features") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"docker","name":"Docker","description":"Docker engine","min_size_key":"M"}]`))
	}))
	defer srv.Close()

	svc := newCatalogTestService(t, srv.URL)

	features, err := svc.GetCatalogFeatures()
	require.NoError(t, err)
	require.Len(t, features, 1)
	assert.Equal(t, "docker", features[0].Key)
	assert.Equal(t, "Docker", features[0].Name)
	assert.Equal(t, "M", features[0].MinSizeKey)
}
