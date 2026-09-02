package services

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/catalog"
	"soli/formations/src/scenarios/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// ScenarioProvisioningService answers one question: how must this scenario's
// container be built?
//
// It is the single owner of that answer, and it has to be, because the answer
// is made of four things that must agree — distribution, size, features and
// build-time features. Bulk-start once assembled its own, taking the
// distribution a teacher picked and inventing the rest: size hardcoded, no
// features, and no build features, so a scenario whose setup installs packages
// got a container with no network and died at step 0 for a whole class.
//
// The resolution reads the scenario's declaration against a backend's live
// catalog, which is why it needs the terminal service, and against the
// organization's backend configuration, which is why it needs the database.
// Controllers hold neither: they ask this.
type ScenarioProvisioningService struct {
	db              *gorm.DB
	terminalService ttServices.TerminalTrainerService
}

func NewScenarioProvisioningService(db *gorm.DB, terminalService ttServices.TerminalTrainerService) *ScenarioProvisioningService {
	return &ScenarioProvisioningService{db: db, terminalService: terminalService}
}

// ScenarioForProvisioning loads a scenario in the shape a resolution needs:
// with the images it declares, which is the first thing Resolve consults.
//
// Separate from Resolve on purpose. Callers have work to do between the two —
// bulk-start refuses a trainer whose subscription is past due, and that refusal
// must not wait on a backend catalog, or an outage would answer a billing
// question with 503. So they load, decide, then resolve.
func (s *ScenarioProvisioningService) ScenarioForProvisioning(scenarioID uuid.UUID) (models.Scenario, error) {
	var scenario models.Scenario
	err := s.db.Preload("CompatibleInstanceTypes").First(&scenario, "id = ?", scenarioID).Error
	return scenario, err
}

// Resolve picks the backend, distribution, size and features for a scenario
// already in hand.
//
// orgID is the organization context the run belongs to, whose configured
// backends are tried before the system default. preferredBackend, when set,
// is tried before either: which host to build on is the caller's call, while
// what to build on it is the scenario's.
func (s *ScenarioProvisioningService) Resolve(
	scenario models.Scenario,
	orgID *uuid.UUID,
	preferredBackend string,
) (ScenarioProvisioning, error) {
	// Determine candidate backends. A caller that already knows which backend it
	// wants — the teacher picking one for a whole class — gets it tried first;
	// the org's own backends follow, then the system default.
	var candidateBackends []string
	if preferredBackend != "" {
		candidateBackends = append(candidateBackends, preferredBackend)
	}
	if orgID != nil {
		var org orgModels.Organization
		if err := s.db.First(&org, "id = ?", *orgID).Error; err == nil {
			if org.DefaultBackend != "" && org.DefaultBackend != preferredBackend {
				candidateBackends = append(candidateBackends, org.DefaultBackend)
			}
			for _, b := range org.AllowedBackends {
				if b != org.DefaultBackend && b != preferredBackend {
					candidateBackends = append(candidateBackends, b)
				}
			}
		}
	}
	if len(candidateBackends) == 0 {
		candidateBackends = []string{""} // system default
	}

	// Fetch the size catalog once (cached 60s in the service) so resolveDistribution
	// can apply launch-time fallback for scenarios with unknown InstanceType values
	// (typos, stale imports, keys from another tt-backend instance). On fetch
	// failure we pass nil — resolveDistribution then preserves prior behavior
	// and tt-backend's validateComposition() remains the final authority.
	sizes, sizesErr := s.terminalService.GetCatalogSizes()
	if sizesErr != nil {
		slog.Warn("failed to fetch sizes catalog, scenario size fallback disabled", "err", sizesErr)
		sizes = nil
	}

	// Try each candidate backend. A backend whose catalog cannot be read is a
	// different failure from one that simply has nothing suitable — the first is
	// an outage, the second a scenario nobody can run here — and callers answer
	// them with different status codes, so the two are kept apart.
	var lastErr error
	reachedNoCatalog := true
	for _, b := range candidateBackends {
		distributions, distErr := s.terminalService.GetDistributions(b)
		if distErr != nil {
			lastErr = distErr
			continue
		}
		reachedNoCatalog = false
		resolvedDist, resolvedSize, resolvedFeatures, resolveErr := resolveDistribution(scenario, distributions, sizes)
		if resolveErr != nil {
			lastErr = resolveErr
			continue
		}
		return ScenarioProvisioning{
			Backend:       b,
			Distribution:  resolvedDist,
			Size:          resolvedSize,
			Features:      resolvedFeatures,
			BuildFeatures: scenarioBuildFeatures(scenario),
		}, nil
	}

	if reachedNoCatalog {
		return ScenarioProvisioning{}, fmt.Errorf("%w: %v", ErrBackendCatalogUnavailable, lastErr)
	}
	return ScenarioProvisioning{}, fmt.Errorf("no compatible distribution on any backend: %w", lastErr)
}

// ErrBackendCatalogUnavailable marks a resolution that failed because no
// candidate backend would say what it offers, rather than because nothing it
// offers fits. It is the difference between "the terminal service is down" and
// "this scenario cannot run here", which are not the same thing to tell a user.
var ErrBackendCatalogUnavailable = stderrors.New("backend catalog unavailable")

// ErrDeclaredImageUnavailable marks a scenario that named the images it needs
// when none of them exist on the chosen backend.
//
// It is deliberately distinct from the generic "nothing matched" failure: the
// two are different operator problems. This one means "install the missing
// distribution on this backend"; the generic one means "no image anywhere fits
// this scenario's os_type, size and features".
var ErrDeclaredImageUnavailable = stderrors.New("declared instance type unavailable")

// resolveDistribution finds a compatible distribution for a scenario.
// Returns the distribution name, the size key, and the features map.
//
// The `sizes` parameter is the live tt-backend size catalog used to validate
// the scenario's stored InstanceType and apply launch-time fallback when it
// is unknown (typo, stale import, key from another tt-backend instance). When
// `sizes` is nil/empty (catalog fetch failed), the requested size is passed
// through unchanged — tt-backend's `validateComposition()` remains the final
// authority and will reject truly invalid sizes.
func resolveDistribution(scenario models.Scenario, distributions []terminalDto.TTDistribution, sizes []terminalDto.TTSize) (distName string, size string, features map[string]bool, err error) {
	requiredFeatures, featErr := scenario.GetRequiredFeatures()
	if featErr != nil {
		return "", "", nil, fmt.Errorf("invalid scenario configuration: %w", featErr)
	}
	requiredSize := scenario.InstanceType // This is actually a SIZE like "M"

	// Priority path: if CompatibleInstanceTypes is populated, try matching by name first
	if len(scenario.CompatibleInstanceTypes) > 0 {
		sorted := SortInstanceTypesByPriority(scenario.CompatibleInstanceTypes)

		for _, cit := range sorted {
			for _, dist := range distributions {
				if strings.EqualFold(cit.InstanceType, dist.Name) {
					// Match found — use scenario size if set, otherwise distribution default
					resolvedSize := requiredSize
					if resolvedSize == "" {
						resolvedSize = dist.DefaultSizeKey
					}
					featuresMap, featMapErr := scenarioFeatures(scenario)
					if featMapErr != nil {
						return "", "", nil, featMapErr
					}
					return dist.Name, applySizeFallback(scenario, resolvedSize, dist, sizes), featuresMap, nil
				}
			}
		}
		// Nothing the scenario named is available here. Naming images is a
		// statement of requirement, so substituting one the author never
		// approved is refused: that silent substitution is how a challenge ran
		// on the generic Debian base for months, in production, unnoticed.
		//
		// A scenario naming several images has already said which substitutions
		// are acceptable — they are simply lower-priority entries in this list,
		// and one of them would have matched above.
		wanted := make([]string, 0, len(sorted))
		for _, cit := range sorted {
			wanted = append(wanted, cit.InstanceType)
		}
		return "", "", nil, fmt.Errorf("%w: scenario requires one of [%s]",
			ErrDeclaredImageUnavailable, strings.Join(wanted, ", "))
	}

	for _, dist := range distributions {
		// Match OS type
		if scenario.OsType != "" && dist.OsType != scenario.OsType {
			continue
		}
		// Check distribution supports all required features
		if !distributionSupportsFeatures(dist, requiredFeatures) {
			continue
		}
		// Check distribution's min_size_key allows the requested size
		if requiredSize != "" && dist.MinSizeKey != "" {
			ranks := sizeRanks()
			reqOrder, reqOk := sizeRank(ranks, requiredSize)
			minOrder, minOk := sizeRank(ranks, dist.MinSizeKey)
			if reqOk && minOk && reqOrder < minOrder {
				continue // requested size is smaller than distribution's minimum
			}
		}
		// Found a compatible distribution
		size = requiredSize
		if size == "" && dist.DefaultSizeKey != "" {
			size = dist.DefaultSizeKey
		}
		featuresMap, featMapErr := scenarioFeatures(scenario)
		if featMapErr != nil {
			return "", "", nil, featMapErr
		}
		return dist.Name, applySizeFallback(scenario, size, dist, sizes), featuresMap, nil
	}
	return "", "", nil, fmt.Errorf("no compatible distribution found for scenario (os_type=%s, size=%s)", scenario.OsType, requiredSize)
}

// distributionSupportsFeatures checks if a distribution supports all required features
func distributionSupportsFeatures(dist terminalDto.TTDistribution, required []string) bool {
	if len(required) == 0 {
		return true
	}
	supported := make(map[string]bool, len(dist.SupportedFeatures))
	for _, f := range dist.SupportedFeatures {
		supported[f] = true
	}
	for _, req := range required {
		if !supported[req] {
			return false
		}
	}
	return true
}

// sizeRanks maps each size key to its place in the catalog's increasing
// footprint order.
//
// The order is not restated here. src/payment/catalog is the single source of
// truth for sizes: it is hydrated at startup from tt-backend's /sizes and
// reports any disagreement with its own cold-start fallback as drift, so the
// budget engine, the session-options endpoint and this all rank sizes the same
// way and cannot come apart when a size is added or reordered.
//
// The hardcoded map this replaces had already drifted — it ranked an "XXL"
// that no catalog has ever contained.
func sizeRanks() map[string]int {
	keys := catalog.CanonicalSizeKeys()
	ranks := make(map[string]int, len(keys))
	for i, key := range keys {
		ranks[key] = i
	}
	return ranks
}

// sizeRank returns a size's place in that order, and whether the catalog knows
// the key at all. Keys are compared in the catalog's own lowercase form, so a
// scenario storing "M" and a distribution storing "m" rank alike.
func sizeRank(ranks map[string]int, key string) (int, bool) {
	rank, ok := ranks[strings.ToLower(strings.TrimSpace(key))]
	return rank, ok
}

// resolveSizeOrFallback returns a valid size key, falling back when the
// requested size is not in the catalog. Returns the canonical key from the
// catalog and a bool indicating whether a fallback was applied.
//
// Resolution order:
//  1. Requested matches a catalog entry (case-insensitive) → use canonical key
//  2. Distribution default matches a catalog entry → use it
//  3. Smallest size in catalog (lowest SortOrder)
//  4. Catalog unavailable (nil/empty) → pass requested through unchanged
//     (preserves prior behavior when the catalog fetch fails)
func resolveSizeOrFallback(requested string, dist terminalDto.TTDistribution, sizes []terminalDto.TTSize) (string, bool) {
	for _, s := range sizes {
		if strings.EqualFold(s.Key, requested) {
			return s.Key, false
		}
	}
	if dist.DefaultSizeKey != "" {
		for _, s := range sizes {
			if strings.EqualFold(s.Key, dist.DefaultSizeKey) {
				return s.Key, true
			}
		}
	}
	if len(sizes) > 0 {
		smallest := sizes[0]
		for _, s := range sizes[1:] {
			if s.SortOrder < smallest.SortOrder {
				smallest = s
			}
		}
		return smallest.Key, true
	}
	return requested, false
}

// applySizeFallback resolves the requested size against the catalog and logs
// a warning when a fallback is applied, so the two return sites in
// resolveDistribution stay terse and consistent.
func applySizeFallback(scenario models.Scenario, requested string, dist terminalDto.TTDistribution, sizes []terminalDto.TTSize) string {
	final, fellBack := resolveSizeOrFallback(requested, dist, sizes)
	if fellBack {
		slog.Warn("scenario size fallback",
			"scenario_id", scenario.ID,
			"requested", requested,
			"resolved", final,
		)
	}
	return final
}

// scenarioFeatures is everything the scenario's machine has to provide: the
// features the scenario declares, plus the renderer that a configured banner
// cannot be drawn without.
//
// The renderer is derived rather than declared — a trainer fills in an intro
// and an outro, not a capability list — and it is derived here, where the
// feature set is built, so that every caller gets the complete answer. Adding
// it at the call sites instead meant each one had to remember, and the launch
// and the preview are not the only two places that resolve a scenario.
//
// Before this, whether a banner animated depended on the image the scenario
// happened to land on: it worked where somebody had baked a renderer in, and
// printed plain text with no explanation anywhere else.
func scenarioFeatures(scenario models.Scenario) (map[string]bool, error) {
	features, err := scenario.GetFeaturesMap()
	if err != nil {
		return nil, fmt.Errorf("invalid scenario configuration: %w", err)
	}
	if !EffectsEnabled() {
		// A scenario may also name the feature itself. Dropping it here keeps the
		// switch's promise: off means no renderer is installed, whatever asked
		// for it, since with banners silenced nothing would ever use it.
		delete(features, effectsFeatureKey)
		return features, nil
	}
	if !ScenarioUsesEffects(&scenario) {
		return features, nil
	}
	if features == nil {
		features = map[string]bool{}
	}
	features[effectsFeatureKey] = true
	return features, nil
}

// scenarioBuildFeatures reads the features a scenario needs only while its
// container is being provisioned.
//
// A bad declaration is a scenario-authoring error, not a reason to refuse the
// launch: the run proceeds without them and whatever the setup needed them for
// fails visibly during provisioning, where the trainer can see it, rather than
// as an opaque refusal here.
func scenarioBuildFeatures(scenario models.Scenario) map[string]bool {
	features, err := scenario.GetBuildFeaturesMap()
	if err != nil {
		slog.Error("scenario has an unreadable build_features declaration; provisioning without it",
			"scenario", scenario.Name, "err", err)
		return nil
	}
	return features
}

// effectsFeatureKey is tt-backend's catalog key for the banner renderer.
const effectsFeatureKey = "effects"

// SizeIsSmallerThan reports whether machine is a smaller size than required.
//
// False when either key is unknown to the catalog order: an unrecognised size
// is not evidence of a machine being too small, and refusing a launch on that
// basis would turn a typo in a scenario into an unrunnable scenario.
func SizeIsSmallerThan(machine, required string) bool {
	ranks := sizeRanks()
	requiredOrder, reqOk := sizeRank(ranks, required)
	machineOrder, machOk := sizeRank(ranks, machine)
	return reqOk && machOk && machineOrder < requiredOrder
}
