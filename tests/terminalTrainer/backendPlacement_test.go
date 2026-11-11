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
	return fakeTTWithCatalogs(t, backends, metrics, nil, nil)
}

// fakeTTWithCatalogs also serves each backend's distributions, when catalogs
// names it, and records which backend's catalog was asked for last.
func fakeTTWithCatalogs(t *testing.T, backends []dto.BackendInfo, metrics map[string]*dto.ServerMetricsResponse, catalogs map[string][]dto.TTDistribution, lastCatalog *string) services.TerminalTrainerService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/distributions") && catalogs != nil:
			backend := r.URL.Query().Get("backend")
			if lastCatalog != nil {
				*lastCatalog = backend
			}
			_ = json.NewEncoder(w).Encode(catalogs[backend])
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

func TestPlacementCapacity(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{}

	got := services.PlacementCapacity([]services.HostLoad{{Backend: "a", Metrics: saturated()}, {Backend: "b", Metrics: roomy(30)}}, plan, "S")
	assert.Equal(t, services.CapacityStatusOK, got.Status, "the verdict is the chosen host's, not the first one's")

	got = services.PlacementCapacity([]services.HostLoad{{Backend: "a", Metrics: saturated()}}, plan, "S")
	assert.Equal(t, services.CapacityStatusCritical, got.Status)

	got = services.PlacementCapacity([]services.HostLoad{{Backend: "a"}}, plan, "S")
	assert.Equal(t, services.CapacityStatusUnknown, got.Status)

	got = services.PlacementCapacity(nil, plan, "S")
	assert.Equal(t, services.CapacityStatusUnknown, got.Status)
}

func TestLaunchCapacity_AnswersForTheHostALaunchWouldUse(t *testing.T) {
	svc := fakeTTWithMetrics(t,
		[]dto.BackendInfo{{ID: "full", Connected: true, IsDefault: true}, {ID: "free", Connected: true}},
		map[string]*dto.ServerMetricsResponse{"full": saturated(), "free": roomy(30)},
	)
	plan := &paymentModels.SubscriptionPlan{}

	assert.Equal(t, services.CapacityStatusOK, svc.LaunchCapacity([]string{"full", "free"}, "", plan, "S").Status,
		"a full default host must not hide the free one a launch would land on")
	assert.Equal(t, services.CapacityStatusCritical, svc.LaunchCapacity([]string{"full"}, "", plan, "S").Status)
}

func TestHostsFit_CountsWholeSessionsAcrossHosts(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{}
	// 64 GB hosts keep a 3.2 GB reserve; an M session needs 1 GB.
	twoEach := []services.HostLoad{{Backend: "a", Metrics: roomy(5.5)}, {Backend: "b", Metrics: roomy(5.5)}}

	assert.True(t, services.HostsFit(twoEach, plan, "M", 4), "two M sessions fit on each host")
	assert.False(t, services.HostsFit(twoEach, plan, "M", 5), "the half-session left on each host does not add up to a fifth")
	assert.False(t, services.HostsFit([]services.HostLoad{{Backend: "a", Metrics: saturated()}}, plan, "XS", 1))
	assert.True(t, services.HostsFit([]services.HostLoad{{Backend: "a", Metrics: saturated()}, {Backend: "b"}}, plan, "M", 50),
		"unreadable metrics never block")
	assert.True(t, services.HostsFit(nil, plan, "M", 3), "no host online: each member reports it")
}

// startOnTwoHosts makes an automatic launch over two allowed hosts. The fake
// serves no catalog, so the launch stops right after placing the session: the
// catalog request it then makes names the host it chose.
func startOnTwoHosts(t *testing.T, first, second *dto.ServerMetricsResponse) error {
	t.Helper()
	svc := fakeTTWithMetrics(t,
		[]dto.BackendInfo{{ID: "first", Connected: true, IsDefault: true}, {ID: "second", Connected: true}},
		map[string]*dto.ServerMetricsResponse{"first": first, "second": second},
	)
	plan := &paymentModels.SubscriptionPlan{AllowedBackends: []string{"first", "second"}, MaxSessionDurationMinutes: 60}
	_, err := svc.StartComposedSession("placement-user", dto.CreateComposedSessionInput{Distribution: "debian", Size: "S", Terms: "accepted"}, plan)
	require.Error(t, err)
	return err
}

func TestStartComposedSession_Auto_TakesTheFreestHost(t *testing.T) {
	err := startOnTwoHosts(t, roomy(4), roomy(40))
	assert.Contains(t, err.Error(), "backend=second", "the default has less room than the other host")
}

func TestStartComposedSession_Auto_SkipsAHostWithoutRoom(t *testing.T) {
	// Most free memory, but saturated all the same: the capacity rule refuses
	// any launch at 99 % used.
	full := &dto.ServerMetricsResponse{RAMAvailableGB: 50, RAMTotalGB: 64, RAMPercent: 99.5}
	err := startOnTwoHosts(t, full, roomy(10))
	assert.Contains(t, err.Error(), "backend=second")
}

// An automatic launch only considers the hosts that offer the distribution:
// the roomier host lacks it, so the session goes to the other one.
func TestStartComposedSession_Auto_OnlyHostsOfferingTheDistribution(t *testing.T) {
	var lastCatalog string
	svc := fakeTTWithCatalogs(t,
		[]dto.BackendInfo{{ID: "first", Connected: true, IsDefault: true}, {ID: "second", Connected: true}},
		map[string]*dto.ServerMetricsResponse{"first": roomy(4), "second": roomy(40)},
		map[string][]dto.TTDistribution{
			"first":  {{Name: "debian", Prefix: "deb"}},
			"second": {{Name: "ubuntu", Prefix: "ubu"}},
		},
		&lastCatalog,
	)
	plan := &paymentModels.SubscriptionPlan{AllowedBackends: []string{"first", "second"}, MaxSessionDurationMinutes: 60}

	_, err := svc.StartComposedSession("placement-user", dto.CreateComposedSessionInput{Distribution: "debian", Size: "S", Terms: "accepted"}, plan)

	require.Error(t, err, "the fake serves no sizes, so the launch stops after reading the chosen host's catalog")
	assert.NotContains(t, err.Error(), "distribution 'debian' not found")
	assert.Equal(t, "first", lastCatalog, "the launch reads the catalog of the host it chose")
}

// The capacity indicator answers for the same hosts: with no host offering the
// distribution, it cannot vouch for any.
func TestLaunchCapacity_OnlyHostsOfferingTheDistribution(t *testing.T) {
	svc := fakeTTWithCatalogs(t,
		[]dto.BackendInfo{{ID: "first", Connected: true, IsDefault: true}, {ID: "second", Connected: true}},
		map[string]*dto.ServerMetricsResponse{"first": saturated(), "second": roomy(40)},
		map[string][]dto.TTDistribution{"first": {{Name: "debian"}}, "second": {{Name: "ubuntu"}}},
		nil,
	)
	plan := &paymentModels.SubscriptionPlan{}

	assert.Equal(t, services.CapacityStatusCritical, svc.LaunchCapacity([]string{"first", "second"}, "debian", plan, "S").Status,
		"only the full host offers debian")
	assert.Equal(t, services.CapacityStatusOK, svc.LaunchCapacity([]string{"first", "second"}, "", plan, "S").Status,
		"no distribution named: every allowed host counts")
}
