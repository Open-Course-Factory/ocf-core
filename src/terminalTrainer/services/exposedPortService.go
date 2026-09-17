package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"time"

	configRepositories "soli/formations/src/configuration/repositories"
	paymentModels "soli/formations/src/payment/models"
	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// maxExposedPortsPerSession caps the number of simultaneous public
// exposures a single terminal session may hold, independent of any plan
// field — a simple abuse guard for this MVP rather than a billable limit.
const maxExposedPortsPerSession = 3

// slugAlphabet excludes visually-ambiguous characters (0/O, 1/l/I) since the
// slug is meant to be typed/read as a URL.
const slugAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

const slugLength = 10

// PortExposureFeatureKey is the platform feature flag (admin → Platform
// settings) that turns the whole capability on. It is declared in
// terminalTrainer/moduleConfig.go, disabled by default, and sits above the
// plan and scenario gates: off means no create, no list, and an empty
// Traefik config — every published route dies within one poll.
const PortExposureFeatureKey = "port_exposure"

// exposedPortService owns the "publish a session port to a public URL"
// concern: plan/state validation, container IP resolution via tt-backend,
// slug allocation, and the read path Traefik's dynamic-config provider
// polls. Carved out as its own collaborator (mirrors composer/catalog/
// lifecycle/etc.) rather than folded into terminalLifecycleService, since it
// owns a distinct entity (ExposedPort) with its own lifecycle.
type exposedPortService struct {
	proxy      *terminalProxyClient
	repository repositories.TerminalRepository
	features   configRepositories.FeatureRepository
	db         *gorm.DB
}

func newExposedPortService(proxy *terminalProxyClient, repository repositories.TerminalRepository, db *gorm.DB) *exposedPortService {
	return &exposedPortService{
		proxy:      proxy,
		repository: repository,
		features:   configRepositories.NewFeatureRepository(db),
		db:         db,
	}
}

// FeatureDisabledError is returned while the platform feature flag is off.
// Same 403 mapping as the plan and scenario gates: the frontend hides the
// chip on it.
type FeatureDisabledError struct{}

func (e *FeatureDisabledError) Error() string {
	return "public port exposure is not enabled on this platform"
}

// PlanDisabledError is returned when the resolved plan does not have
// PortExposureEnabled set. Kept distinct from a generic error so the
// controller can map it to a 403 with a clear reason, the same way
// BudgetRejection lets StartComposedSession's caller distinguish a plan
// gate from an infrastructure failure.
type PlanDisabledError struct{}

func (e *PlanDisabledError) Error() string {
	return "the current plan does not allow exposing session ports publicly"
}

// NoNetworkError is returned when the container has no network interface:
// it was started without the "network" feature, so there is no address a
// public route could reach. Its own type so the frontend can word the fix.
type NoNetworkError struct{}

func (e *NoNetworkError) Error() string {
	return "this session has no network interface; start a session with the network feature to expose a port"
}

// ScenarioDisallowsError is returned when the session is running a scenario
// whose PortExposureAllowed is off. Same 403 mapping as PlanDisabledError;
// kept distinct so the message tells the learner which gate said no.
type ScenarioDisallowsError struct{}

func (e *ScenarioDisallowsError) Error() string {
	return "this scenario does not allow exposing session ports publicly"
}

// CreateExposedPort publishes containerPort of the given session to a new
// public URL. Ownership of the session is assumed already verified by the
// caller (RequireTerminalAccess on the route) — this only re-derives what it
// needs from the terminal row itself, it does not re-check userID against
// terminal.UserID, matching the trust boundary every other
// terminalTrainerService method nested under /:id already relies on.
func (s *exposedPortService) CreateExposedPort(sessionID string, containerPort int) (*dto.ExposedPortResponse, error) {
	if containerPort < 1024 || containerPort > 65535 {
		return nil, fmt.Errorf("port must be between 1024 and 65535")
	}

	terminal, err := s.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if !terminal.IsLive() {
		return nil, fmt.Errorf("session is not running")
	}

	if err := s.checkExposureAllowed(terminal); err != nil {
		return nil, err
	}
	plan, err := s.exposurePlan(terminal)
	if err != nil {
		return nil, err
	}

	count, err := s.repository.CountExposedPortsBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count existing exposures: %w", err)
	}
	if count >= maxExposedPortsPerSession {
		return nil, fmt.Errorf("session already has the maximum of %d exposed ports", maxExposedPortsPerSession)
	}

	sessionInfo, err := s.proxy.GetSessionInfoFromAPI(sessionID)
	if err != nil {
		utils.Warn("CreateExposedPort: /info failed for session %s: %v", sessionID, err)
		return nil, fmt.Errorf("failed to resolve container address: %w", err)
	}
	if sessionInfo.IP == "" {
		// The ocf-base profile is NIC-less: only a session started with the
		// "network" feature has an interface, hence an address to route to.
		return nil, &NoNetworkError{}
	}

	slug, err := s.allocateSlug()
	if err != nil {
		return nil, err
	}

	backend := terminal.Backend
	if backend == "" {
		backend = sessionInfo.Backend
	}

	exposedPort := &models.ExposedPort{
		TerminalID:    terminal.ID,
		SessionID:     sessionID,
		UserID:        terminal.UserID,
		Backend:       backend,
		ContainerPort: containerPort,
		Slug:          slug,
		ContainerIP:   sessionInfo.IP,
		ExpiresAt:     exposureExpiry(terminal.ExpiresAt, plan.PortExposureTTLMinutes),
	}
	if err := s.repository.CreateExposedPort(exposedPort); err != nil {
		return nil, fmt.Errorf("failed to save exposure: %w", err)
	}

	return s.toResponse(exposedPort), nil
}

// ListExposedPorts returns every exposure recorded for a session (active or
// past its terminal's lifetime — the caller-facing list does not filter on
// RunningDisplayScope so a user can see what they created even right after
// their session stopped). It runs the same plan/scenario gate as
// CreateExposedPort so the frontend can hide the panel on its 403 instead of
// re-deriving the rule from plan fields.
func (s *exposedPortService) ListExposedPorts(sessionID string) ([]dto.ExposedPortResponse, error) {
	terminal, err := s.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if err := s.checkExposureAllowed(terminal); err != nil {
		return nil, err
	}

	exposedPorts, err := s.repository.GetExposedPortsBySessionID(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ExposedPortResponse, 0, len(*exposedPorts))
	for i := range *exposedPorts {
		out = append(out, *s.toResponse(&(*exposedPorts)[i]))
	}
	return out, nil
}

// DeleteExposedPort removes one exposure. sessionID scopes the lookup so a
// caller who owns terminal A cannot delete an exposure that belongs to
// terminal B by guessing its id — RequireTerminalAccess already proved
// ownership of sessionID, so this is the second half of that guarantee.
func (s *exposedPortService) DeleteExposedPort(sessionID string, exposedPortID uuid.UUID) error {
	exposedPort, err := s.repository.GetExposedPortByID(exposedPortID)
	if err != nil {
		return fmt.Errorf("exposed port not found: %w", err)
	}
	if exposedPort.SessionID != sessionID {
		return fmt.Errorf("exposed port not found")
	}
	return s.repository.DeleteExposedPort(exposedPortID)
}

// GetActiveExposedPortsForTraefik is the read path polled by
// GET /internal/traefik/dynamic-config. With the feature flag off it
// publishes nothing, which is the operator's kill switch.
func (s *exposedPortService) GetActiveExposedPortsForTraefik() ([]models.ExposedPort, error) {
	if !s.features.IsFeatureEnabled(PortExposureFeatureKey) {
		return nil, nil
	}
	exposedPorts, err := s.repository.GetActiveExposedPortsForTraefik()
	if err != nil {
		return nil, err
	}
	return *exposedPorts, nil
}

// checkExposureAllowed is the one gate for both the create and the list
// path: the plan must carry PortExposureEnabled, and if the terminal is
// running a scenario, that scenario must allow exposure too.
func (s *exposedPortService) checkExposureAllowed(terminal *models.Terminal) error {
	if !s.features.IsFeatureEnabled(PortExposureFeatureKey) {
		return &FeatureDisabledError{}
	}
	if err := s.checkPlanAllowsExposure(terminal); err != nil {
		return err
	}
	return s.checkScenarioAllowsExposure(terminal.SessionID)
}

// checkScenarioAllowsExposure looks for an open scenario run on this
// terminal. A plain terminal has none and passes; a run whose scenario keeps
// PortExposureAllowed off is refused. The open-run definition belongs to the
// scenarios package (OpenSessionStatuses), as in terminalLifecycleService.
func (s *exposedPortService) checkScenarioAllowsExposure(sessionID string) error {
	var disallowed int64
	err := s.db.Table("scenario_sessions").
		Joins("JOIN scenarios ON scenarios.id = scenario_sessions.scenario_id").
		Where("scenario_sessions.terminal_session_id = ? AND scenario_sessions.status IN ? AND scenarios.port_exposure_allowed = ?",
			sessionID, scenarioModels.OpenSessionStatuses, false).
		Count(&disallowed).Error
	if err != nil {
		return fmt.Errorf("failed to check scenario exposure policy: %w", err)
	}
	if disallowed > 0 {
		return &ScenarioDisallowsError{}
	}
	return nil
}

// checkPlanAllowsExposure resolves the plan the session was launched under
// and requires PortExposureEnabled. A terminal predating SubscriptionPlanID,
// or one whose plan no longer resolves, is treated as not entitled — this is
// a newly opt-in capability, so the safe default on ambiguity is "off", not
// "on".
func (s *exposedPortService) checkPlanAllowsExposure(terminal *models.Terminal) error {
	_, err := s.exposurePlan(terminal)
	return err
}

// exposurePlan loads the plan a terminal runs under and refuses exposure
// unless it carries PortExposureEnabled. A terminal predating
// SubscriptionPlanID, or whose plan cannot be loaded, is refused too.
func (s *exposedPortService) exposurePlan(terminal *models.Terminal) (*paymentModels.SubscriptionPlan, error) {
	if terminal.SubscriptionPlanID == nil {
		return nil, &PlanDisabledError{}
	}
	var plan paymentModels.SubscriptionPlan
	if err := s.db.First(&plan, "id = ?", *terminal.SubscriptionPlanID).Error; err != nil {
		utils.Warn("exposurePlan: failed to load plan %s for session %s: %v", terminal.SubscriptionPlanID.String(), terminal.SessionID, err)
		return nil, &PlanDisabledError{}
	}
	if !plan.PortExposureEnabled {
		return nil, &PlanDisabledError{}
	}
	return &plan, nil
}

// allocateSlug generates a random DNS-label-safe slug and retries on the
// (astronomically unlikely, but not impossible) unique-index collision.
func (s *exposedPortService) allocateSlug() (string, error) {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		slug, err := randomSlug()
		if err != nil {
			return "", fmt.Errorf("failed to generate slug: %w", err)
		}
		var count int64
		if err := s.db.Model(&models.ExposedPort{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
			return "", fmt.Errorf("failed to check slug uniqueness: %w", err)
		}
		if count == 0 {
			return slug, nil
		}
	}
	return "", fmt.Errorf("failed to allocate a unique slug after %d attempts", maxAttempts)
}

func randomSlug() (string, error) {
	b := make([]byte, slugLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(slugAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = slugAlphabet[n.Int64()]
	}
	return string(b), nil
}

func (s *exposedPortService) toResponse(exposedPort *models.ExposedPort) *dto.ExposedPortResponse {
	return &dto.ExposedPortResponse{
		ID:        exposedPort.ID,
		Port:      exposedPort.ContainerPort,
		Slug:      exposedPort.Slug,
		URL:       ExposedPortURL(exposedPort.Slug),
		CreatedAt: exposedPort.CreatedAt,
		ExpiresAt: exposedPort.ExpiresAt,
	}
}

// exposureExpiry bounds an exposure to the plan's TTL after now, or to the
// end of the session if that comes first. A public URL exists to look at
// one's own work during the training, not to hand out links: the short
// lifetime is the feature's policy, not a technical limit.
func exposureExpiry(sessionExpiry time.Time, ttlMinutes int) time.Time {
	ttl := defaultExposeTTL
	if ttlMinutes > 0 {
		ttl = time.Duration(ttlMinutes) * time.Minute
	}
	expiry := time.Now().Add(ttl)
	if sessionExpiry.Before(expiry) {
		return sessionExpiry
	}
	return expiry
}

// defaultExposeTTL applies to a plan whose PortExposureTTLMinutes is unset.
const defaultExposeTTL = time.Hour

// ExposedPortURL is the one place the public URL of an exposure is minted
// from its slug; the admin listing uses it too.
func ExposedPortURL(slug string) string {
	return fmt.Sprintf("%s://%s.%s", exposeScheme(), slug, exposeDomain())
}

// exposeDomain reads the domain under which exposed-port URLs are minted
// (<scheme>://<slug>.<EXPOSE_DOMAIN>). Empty when the operator has not opted
// into the feature — IsExposedPortsFeatureEnabled gates route mounting on
// this being non-empty, so a non-empty response here is expected by the
// time toResponse runs.
func exposeDomain() string {
	return os.Getenv("EXPOSE_DOMAIN")
}

// exposeScheme reads the scheme minted into exposed-port URLs. Defaults to
// "http" for a bare dev Traefik; "https" once the operator's Traefik carries
// a certificate — traefikConfigController then also marks every router TLS.
func exposeScheme() string {
	if scheme := os.Getenv("EXPOSE_SCHEME"); scheme != "" {
		return scheme
	}
	return "http"
}

// ExposeTLSEnabled reports whether minted URLs are https, i.e. whether the
// Traefik routers must terminate TLS. One env var drives both.
func ExposeTLSEnabled() bool {
	return exposeScheme() == "https"
}

// IsExposedPortsFeatureEnabled reports whether the operator configured the
// two env vars the public-port-exposure feature needs (EXPOSE_DOMAIN,
// TRAEFIK_PROVIDER_SECRET). Routes are mounted conditionally on this so the
// feature is entirely absent (404, not just plan-gated) unless explicitly
// configured — see plan doc "option désactivée par défaut".
func IsExposedPortsFeatureEnabled() bool {
	return os.Getenv("EXPOSE_DOMAIN") != "" && os.Getenv("TRAEFIK_PROVIDER_SECRET") != ""
}
