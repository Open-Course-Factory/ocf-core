package services

import (
	"errors"
	"slices"
	"sort"
	"time"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
)

var (
	// ErrNoBackendCapacity: every candidate host lacks the RAM for the session.
	ErrNoBackendCapacity = errors.New("no backend has room for this session")
	// ErrBackendOffline: no candidate host is connected.
	ErrBackendOffline = errors.New("no backend is online")
)

// HostLoad is a candidate backend and its RAM metrics; Metrics is nil when
// they could not be read.
type HostLoad struct {
	Backend string
	Metrics *dto.ServerMetricsResponse
}

// ChooseHost returns the first host with room for a session of size. A host
// whose metrics could not be read is taken as it comes: unreadable metrics
// have never blocked a launch.
func ChooseHost(loads []HostLoad, plan *paymentModels.SubscriptionPlan, size string) (string, error) {
	host, _, err := place(loads, plan, size)
	return host, err
}

// PlacementCapacity is the capacity verdict for the host ChooseHost would pick
// or, when none has room, for the freest one.
func PlacementCapacity(loads []HostLoad, plan *paymentModels.SubscriptionPlan, size string) CapacityResult {
	_, verdict, _ := place(loads, plan, size)
	return verdict
}

func place(loads []HostLoad, plan *paymentModels.SubscriptionPlan, size string) (string, CapacityResult, error) {
	if len(loads) == 0 {
		return "", CapacityResult{Status: CapacityStatusUnknown, Reason: "metrics_unavailable"}, ErrBackendOffline
	}
	for _, h := range loads {
		if verdict := EvaluateLaunchCapacity(plan, size, h.Metrics); verdict.Status != CapacityStatusCritical {
			return h.Backend, verdict, nil
		}
	}
	return "", EvaluateLaunchCapacity(plan, size, loads[0].Metrics), ErrNoBackendCapacity
}

// HostsFit reports whether loads together have room for n sessions of size,
// each host counted in whole sessions. A host whose metrics could not be read
// makes the answer yes, as does having no host online: unreadable metrics
// have never blocked a launch, and each launch reports an offline host itself.
func HostsFit(loads []HostLoad, plan *paymentModels.SubscriptionPlan, size string, n int) bool {
	if len(loads) == 0 {
		return true
	}
	fits := 0
	for _, h := range loads {
		if h.Metrics == nil {
			return true
		}
		fits += sessionsThatFit(plan, size, h.Metrics)
	}
	return fits >= n
}

// hostLoads returns the connected candidates, most free RAM first; those whose
// metrics cannot be read follow in their given order. "" stands for
// tt-backend's own default and is always kept, as is every candidate when
// tt-backend cannot list its backends: connectivity is then unknown, and the
// metrics still decide.
func (p *terminalProxyClient) hostLoads(candidates []string, readMetrics func(backend string) (*dto.ServerMetricsResponse, error)) []HostLoad {
	backends, listErr := p.getBackendsCached()
	online := func(id string) bool {
		return id == "" || listErr != nil ||
			slices.ContainsFunc(backends, func(b dto.BackendInfo) bool { return b.ID == id && b.Connected })
	}

	// ponytail: reads metrics one host at a time, fine for the handful of
	// hosts an org has; read them concurrently if that ever grows.
	loads := make([]HostLoad, 0, len(candidates))
	for _, id := range candidates {
		if !online(id) {
			continue
		}
		metrics, err := readMetrics(id)
		if err != nil {
			metrics = nil
		}
		loads = append(loads, HostLoad{Backend: id, Metrics: metrics})
	}
	sort.SliceStable(loads, func(i, j int) bool { return freeRAMGB(loads[i]) > freeRAMGB(loads[j]) })
	return loads
}

func freeRAMGB(h HostLoad) float64 {
	if h.Metrics == nil {
		return -1
	}
	return h.Metrics.RAMAvailableGB
}

// pickHost settles where a session of size starts: this is the launch's
// capacity check.
func (p *terminalProxyClient) pickHost(candidates []string, plan *paymentModels.SubscriptionPlan, size string) (string, error) {
	return ChooseHost(p.currentHostLoads(candidates), plan, size)
}

// LaunchCapacity answers, without starting anything, whether a session of
// distribution and size would find room among candidates: the same hosts and
// the same reading the launch decides on.
func (p *terminalProxyClient) LaunchCapacity(candidates []string, distribution string, plan *paymentModels.SubscriptionPlan, size string) CapacityResult {
	return PlacementCapacity(p.currentHostLoads(p.offering(candidates, distribution)), plan, size)
}

// offering keeps the candidates whose catalog lists distribution, as
// tt-backend configures it per server: a host that does not offer it would
// refuse the session. One candidate, or no distribution named, is left as is,
// and so is a host whose catalog cannot be read: the launch then reports that
// host's own answer. Whether the host could actually build it (template
// prepared, image present) is not something the catalog says.
func (p *terminalProxyClient) offering(candidates []string, distribution string) []string {
	if len(candidates) < 2 || distribution == "" {
		return candidates
	}
	kept := make([]string, 0, len(candidates))
	for _, b := range candidates {
		distributions, err := p.GetDistributions(b)
		if err != nil || findDistribution(distributions, distribution) != nil {
			kept = append(kept, b)
		}
	}
	// Offered nowhere: leave the hosts be, so the launch says the
	// distribution was not found rather than that no server is available.
	if len(kept) == 0 {
		return candidates
	}
	return kept
}

// currentHostLoads reads each host through tt-backend's own short metrics
// cache (8 s) rather than querying Incus every time: a class started ten
// learners at a time would otherwise query each host once per learner. The
// figures can be that old; fresh ones would not see the containers still
// booting either.
func (p *terminalProxyClient) currentHostLoads(candidates []string) []HostLoad {
	return p.hostLoads(candidates, func(b string) (*dto.ServerMetricsResponse, error) {
		return p.GetServerMetrics(false, b)
	})
}

// rankingMetricsTTL bounds how stale the RAM figures used to order hosts may
// be. Ordering runs once per card in the scenario catalogue; the launch itself
// re-reads them in pickHost.
const rankingMetricsTTL = 15 * time.Second

type timedMetrics struct {
	metrics *dto.ServerMetricsResponse
	at      time.Time
}

// RankBackends orders candidates for placement: connected hosts only, most
// free RAM first.
func (p *terminalProxyClient) RankBackends(candidates []string) []string {
	loads := p.hostLoads(candidates, p.recentMetrics)
	ranked := make([]string, len(loads))
	for i, h := range loads {
		ranked[i] = h.Backend
	}
	return ranked
}

// recentMetrics reads a host's metrics for ranking, at most once per TTL. A
// failed read is remembered as nil too, so a hung host costs one timeout per
// TTL rather than one per catalogue card.
func (p *terminalProxyClient) recentMetrics(backend string) (*dto.ServerMetricsResponse, error) {
	p.rankingMu.Lock()
	cached, ok := p.rankingMetrics[backend]
	p.rankingMu.Unlock()
	if ok && time.Since(cached.at) < rankingMetricsTTL {
		return cached.metrics, nil
	}
	metrics, err := p.GetServerMetrics(false, backend)
	if err != nil {
		metrics = nil
	}
	p.rankingMu.Lock()
	if p.rankingMetrics == nil {
		p.rankingMetrics = make(map[string]timedMetrics)
	}
	p.rankingMetrics[backend] = timedMetrics{metrics: metrics, at: time.Now()}
	p.rankingMu.Unlock()
	return metrics, nil
}
