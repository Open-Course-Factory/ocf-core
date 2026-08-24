package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// An unset setting must withhold nothing. The subtlety is that
// strings.Split("", ",") returns []string{""} rather than an empty slice, so a
// naive parse would build a set containing "" and start comparing against it.
func TestParseDistributionNames_EmptySettingWithholdsNothing(t *testing.T) {
	for _, stored := range []string{"", "   ", ",", " , , "} {
		parsed := services.ParseDistributionNames(strings.Split(stored, ","))
		assert.Empty(t, parsed, "stored value %q must withhold nothing", stored)
	}
}

func TestParseDistributionNames_TolerantOfSpacingAndTrailingCommas(t *testing.T) {
	// This value is typed by a person into a text field in the admin panel.
	parsed := services.ParseDistributionNames(strings.Split(" alpine-xs , Challenge-Deb ,", ","))

	assert.Equal(t, map[string]bool{"alpine-xs": true, "challenge-deb": true}, parsed)
}

// ============================================================
// Withholding an image from the menu must not uninstall it
//
// The scenario launcher resolves the images a scenario declares against the
// distribution list, and refuses to substitute one the author never approved.
// So if the curated list were the one it saw, adding `challenge-deb` to the
// setting would not merely hide it — it would make linux-rogue-lite-v2
// unlaunchable, because that scenario names it and nothing else it could fall
// back to.
//
// Hence two reads. GetDistributions stays complete and is what resolves names;
// GetOfferedDistributions is the curated one and belongs to presentation.
// ============================================================

func TestGetDistributions_StaysCompleteForNameResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "distributions") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"Debian"},{"name":"challenge-deb"}]`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	// Build the service first: newCatalogTestService calls freshTestDB, which
	// truncates the shared tables — seeding the setting before it would wipe it.
	svc := newCatalogTestService(t, srv.URL)
	require.NoError(t, sharedTestDB.Create(&configModels.Feature{
		Key:   services.UnlistedDistributionsKey,
		Name:  "unlisted",
		Value: "challenge-deb",
	}).Error)

	offered, err := svc.GetOfferedDistributions("")
	require.NoError(t, err)
	assert.Equal(t, []string{"Debian"}, namesOf(offered),
		"the picker must not offer a withheld image")

	all, err := svc.GetDistributions("")
	require.NoError(t, err)
	assert.Contains(t, namesOf(all), "challenge-deb",
		"the launcher resolves declared images against this list — withholding one "+
			"from the menu must not make the scenario that requires it unlaunchable")
}

// ============================================================
// Saving from the admin screen
//
// The screen sends the whole set of withheld names, so what the boxes say is
// what is stored. Joining happens next to the splitting, so a caller cannot
// store a value the reader will not understand.
// ============================================================

func TestSetWithheldDistributions_RoundTripsThroughTheSetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "distributions") {
			_, _ = w.Write([]byte(`[{"name":"Debian"},{"name":"Alpine"},{"name":"challenge-deb"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	svc := newCatalogTestService(t, srv.URL)

	require.NoError(t, svc.SetWithheldDistributions([]string{"challenge-deb"}))

	offered, err := svc.GetOfferedDistributions("")
	require.NoError(t, err)
	assert.Equal(t, []string{"Debian", "Alpine"}, namesOf(offered))

	// Unticking it again must bring it back — the screen is the only writer, so
	// a set that cannot be emptied would strand an image out of sight for good.
	require.NoError(t, svc.SetWithheldDistributions(nil))

	offered, err = svc.GetOfferedDistributions("")
	require.NoError(t, err)
	assert.Equal(t, []string{"Debian", "Alpine", "challenge-deb"}, namesOf(offered))
}

func TestSetWithheldDistributions_DropsBlanksAndDuplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "distributions") {
			_, _ = w.Write([]byte(`[{"name":"Debian"},{"name":"challenge-deb"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	svc := newCatalogTestService(t, srv.URL)

	require.NoError(t, svc.SetWithheldDistributions(
		[]string{" challenge-deb ", "", "Challenge-Deb", "   "}))

	offered, err := svc.GetOfferedDistributions("")
	require.NoError(t, err)
	assert.Equal(t, []string{"Debian"}, namesOf(offered),
		"whitespace and repeats must not change what is withheld")
}
