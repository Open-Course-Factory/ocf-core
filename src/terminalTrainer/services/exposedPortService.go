package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"

	paymentModels "soli/formations/src/payment/models"
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

// exposedPortService owns the "publish a session port to a public URL"
// concern: plan/state validation, container IP resolution via tt-backend,
// slug allocation, and the read path Traefik's dynamic-config provider
// polls. Carved out as its own collaborator (mirrors composer/catalog/
// lifecycle/etc.) rather than folded into terminalLifecycleService, since it
// owns a distinct entity (ExposedPort) with its own lifecycle.
type exposedPortService struct {
	proxy      *terminalProxyClient
	repository repositories.TerminalRepository
	db         *gorm.DB
}

func newExposedPortService(proxy *terminalProxyClient, repository repositories.TerminalRepository, db *gorm.DB) *exposedPortService {
	return &exposedPortService{
		proxy:      proxy,
		repository: repository,
		db:         db,
	}
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

	if err := s.checkPlanAllowsExposure(terminal); err != nil {
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
		return nil, fmt.Errorf("failed to resolve container address: %w", err)
	}
	if sessionInfo.IP == "" {
		return nil, fmt.Errorf("backend did not report a container address for this session")
	}

	slug, err := s.allocateSlug()
	if err != nil {
		return nil, err
	}

	exposedPort := &models.ExposedPort{
		TerminalID:    terminal.ID,
		SessionID:     sessionID,
		UserID:        terminal.UserID,
		ContainerPort: containerPort,
		Slug:          slug,
		ContainerIP:   sessionInfo.IP,
		ExpiresAt:     terminal.ExpiresAt,
	}
	if err := s.repository.CreateExposedPort(exposedPort); err != nil {
		return nil, fmt.Errorf("failed to save exposure: %w", err)
	}

	return s.toResponse(exposedPort), nil
}

// ListExposedPorts returns every exposure recorded for a session (active or
// past its terminal's lifetime — the caller-facing list does not filter on
// RunningDisplayScope so a user can see what they created even right after
// their session stopped).
func (s *exposedPortService) ListExposedPorts(sessionID string) ([]dto.ExposedPortResponse, error) {
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
// GET /internal/traefik/dynamic-config.
func (s *exposedPortService) GetActiveExposedPortsForTraefik() ([]models.ExposedPort, error) {
	exposedPorts, err := s.repository.GetActiveExposedPortsForTraefik()
	if err != nil {
		return nil, err
	}
	return *exposedPorts, nil
}

// checkPlanAllowsExposure resolves the plan the session was launched under
// and requires PortExposureEnabled. A terminal predating SubscriptionPlanID,
// or one whose plan no longer resolves, is treated as not entitled — this is
// a newly opt-in capability, so the safe default on ambiguity is "off", not
// "on".
func (s *exposedPortService) checkPlanAllowsExposure(terminal *models.Terminal) error {
	if terminal.SubscriptionPlanID == nil {
		return &PlanDisabledError{}
	}
	var plan paymentModels.SubscriptionPlan
	if err := s.db.First(&plan, "id = ?", *terminal.SubscriptionPlanID).Error; err != nil {
		utils.Warn("checkPlanAllowsExposure: failed to load plan %s for session %s: %v", terminal.SubscriptionPlanID.String(), terminal.SessionID, err)
		return &PlanDisabledError{}
	}
	if !plan.PortExposureEnabled {
		return &PlanDisabledError{}
	}
	return nil
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
		URL:       fmt.Sprintf("%s://%s.%s", exposeScheme(), exposedPort.Slug, exposeDomain()),
		CreatedAt: exposedPort.CreatedAt,
		ExpiresAt: exposedPort.ExpiresAt,
	}
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
// "http": during development there is deliberately no TLS/certificate setup
// on the reference Traefik instance (see traefik/README.md) — flipping this
// to "https" once TLS is configured operator-side is a pure env change, no
// code change needed on either side (see also traefikConfigController.go,
// which only adds a router's TLS block when TRAEFIK_CERT_RESOLVER is set).
func exposeScheme() string {
	if scheme := os.Getenv("EXPOSE_SCHEME"); scheme != "" {
		return scheme
	}
	return "http"
}

// IsExposedPortsFeatureEnabled reports whether the operator configured the
// two env vars the public-port-exposure feature needs (EXPOSE_DOMAIN,
// TRAEFIK_PROVIDER_SECRET). Routes are mounted conditionally on this so the
// feature is entirely absent (404, not just plan-gated) unless explicitly
// configured — see plan doc "option désactivée par défaut".
func IsExposedPortsFeatureEnabled() bool {
	return os.Getenv("EXPOSE_DOMAIN") != "" && os.Getenv("TRAEFIK_PROVIDER_SECRET") != ""
}
