package terminalTrainer_tests

import (
	"testing"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
)

// ============================================================
// What a person may be offered
//
// tt-backend answers with every instance config it can run. Deciding which of
// those are offered is a product question, and this service is where the size
// and feature catalogs already have their product rules applied — GetDistributions
// was the one read that proxied straight through, which is how `challenge-deb`
// (the Rogue-Lite base image) and `alpine-xs` (no size metadata at all) ended
// up in production's picker beside Debian and Ubuntu.
//
// The rule cannot be derived from scenario_instance_types: that column records
// which images a scenario is *compatible with*, and production lists `debian`
// there next to `challenge-deb`, so deriving it would hide the catalogue's own
// default distribution.
// ============================================================

func distributions(names ...string) []dto.TTDistribution {
	out := make([]dto.TTDistribution, 0, len(names))
	for _, n := range names {
		out = append(out, dto.TTDistribution{Name: n})
	}
	return out
}

func namesOf(ds []dto.TTDistribution) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}

func TestFilterListedDistributions_WithholdsTheInternalOnes(t *testing.T) {
	unlisted := map[string]bool{"alpine-xs": true, "challenge-deb": true}

	listed := services.FilterListedDistributions(
		distributions("Alpine", "Debian", "Ubuntu", "alpine-xs", "challenge-deb"),
		unlisted,
	)

	assert.Equal(t, []string{"Alpine", "Debian", "Ubuntu"}, namesOf(listed))
}

func TestFilterListedDistributions_KeepsTheRealDistributions(t *testing.T) {
	// The regression that matters: Debian is named by a scenario in production,
	// so any rule sourced from scenario compatibility would drop it — and it is
	// the composer's default pick.
	listed := services.FilterListedDistributions(
		distributions("Debian"),
		map[string]bool{"challenge-deb": true},
	)

	assert.Equal(t, []string{"Debian"}, namesOf(listed))
}

func TestFilterListedDistributions_MatchesRegardlessOfCase(t *testing.T) {
	// tt-backend's catalog and the exclusion list are maintained by different
	// hands; `Challenge-Deb` must not slip through on capitalisation alone.
	listed := services.FilterListedDistributions(
		distributions("Challenge-Deb", "Debian"),
		map[string]bool{"challenge-deb": true},
	)

	assert.Equal(t, []string{"Debian"}, namesOf(listed))
}

func TestFilterListedDistributions_EmptyExclusionListKeepsEverything(t *testing.T) {
	// An operator clearing TERMINAL_UNLISTED_DISTRIBUTIONS opts out entirely.
	// Silently dropping everything on empty input is the failure mode to avoid.
	all := distributions("Alpine", "Debian", "challenge-deb")

	listed := services.FilterListedDistributions(all, map[string]bool{})

	assert.Equal(t, []string{"Alpine", "Debian", "challenge-deb"}, namesOf(listed))
}

func TestFilterListedDistributions_NoDistributionsIsNotACrash(t *testing.T) {
	listed := services.FilterListedDistributions(nil, map[string]bool{"alpine-xs": true})
	assert.Empty(t, listed)
}
