package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roomy(freeGB float64) *dto.ServerMetricsResponse {
	return &dto.ServerMetricsResponse{RAMAvailableGB: freeGB, RAMTotalGB: 64, RAMPercent: 100 * (1 - freeGB/64)}
}

func saturated() *dto.ServerMetricsResponse {
	return &dto.ServerMetricsResponse{RAMAvailableGB: 0.2, RAMTotalGB: 64, RAMPercent: 99.5}
}

func TestChooseHost(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{}

	got, err := services.ChooseHost([]services.HostLoad{{Backend: "a", Metrics: saturated()}, {Backend: "b", Metrics: roomy(30)}}, plan, "S")
	require.NoError(t, err)
	assert.Equal(t, "b", got, "a full host is skipped")

	got, err = services.ChooseHost([]services.HostLoad{{Backend: "a"}}, plan, "S")
	require.NoError(t, err)
	assert.Equal(t, "a", got, "unreadable metrics never block a launch")

	_, err = services.ChooseHost([]services.HostLoad{{Backend: "a", Metrics: saturated()}}, plan, "S")
	assert.ErrorIs(t, err, services.ErrNoBackendCapacity)

	_, err = services.ChooseHost(nil, plan, "S")
	assert.ErrorIs(t, err, services.ErrBackendOffline)
}

// fakeTTWithMetrics serves a backend list and per-backend metrics.
func fakeTTWithMetrics(t *testing.T, backends []dto.BackendInfo, metrics map[string]*dto.ServerMetricsResponse) services.TerminalTrainerService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/backends"):
			_ = json.NewEncoder(w).Encode(backends)
		case strings.HasSuffix(r.URL.Path, "/metrics"):
			m, ok := metrics[r.URL.Query().Get("backend")]
			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(m)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	for k, v := range map[string]string{"TERMINAL_TRAINER_URL": srv.URL, "TERMINAL_TRAINER_ADMIN_KEY": "test-key", "TERMINAL_TRAINER_API_VERSION": "1.0"} {
		orig := os.Getenv(k)
		os.Setenv(k, v)
		t.Cleanup(func() { os.Setenv(k, orig) })
	}
	return services.NewTerminalTrainerService(freshTestDB(t))
}

func TestRankBackends_FreestFirst_OfflineDropped_UnknownLast(t *testing.T) {
	svc := fakeTTWithMetrics(t,
		[]dto.BackendInfo{
			{ID: "small", Connected: true, IsDefault: true},
			{ID: "big", Connected: true},
			{ID: "down", Connected: false},
			{ID: "mute", Connected: true},
		},
		map[string]*dto.ServerMetricsResponse{"small": roomy(4), "big": roomy(40), "down": roomy(60)},
	)

	assert.Equal(t, []string{"big", "small", "mute"}, svc.RankBackends([]string{"small", "big", "down", "mute"}))
}
