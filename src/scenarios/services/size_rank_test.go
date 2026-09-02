package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/payment/catalog"
)

// TestSizeRanks_FollowsTheCatalog verifies that size ordering is read from the
// size catalog rather than restated here.
//
// It was restated, as a hardcoded map, and had already drifted: it ranked an
// "XXL" no catalog has ever contained. A second copy of this order is the
// dangerous kind of duplicate, because nothing fails when the two disagree —
// the budget engine admits a machine that the scenario resolver rejects as
// below a distribution's minimum, and neither says why.
func TestSizeRanks_FollowsTheCatalog(t *testing.T) {
	keys := catalog.CanonicalSizeKeys()
	require.NotEmpty(t, keys, "the catalog always has its cold-start fallback")

	ranks := sizeRanks()
	assert.Len(t, ranks, len(keys), "every catalog size ranks, and nothing else does")

	for i, key := range keys {
		rank, known := sizeRank(ranks, key)
		assert.True(t, known, "catalog size %q must rank", key)
		assert.Equal(t, i, rank, "size %q must keep the catalog's position", key)
	}
}

// TestSizeRanks_UnknownKeyDoesNotRank verifies that a size the catalog does not
// know is reported as unknown rather than silently ranked.
//
// The callers depend on the distinction: an unrecognised size must not be read
// as "smaller than required", or a typo in a scenario would make it unlaunchable
// instead of merely odd.
func TestSizeRanks_UnknownKeyDoesNotRank(t *testing.T) {
	ranks := sizeRanks()

	_, known := sizeRank(ranks, "xxl")
	assert.False(t, known, "XXL is not in the catalog and must not rank")

	_, known = sizeRank(ranks, "")
	assert.False(t, known, "an empty size key ranks nowhere")
}

// TestSizeIsSmallerThan_ComparesOnTheCatalogOrder covers the launch-time guard
// that refuses a terminal too small for the scenario it is asked to run.
func TestSizeIsSmallerThan_ComparesOnTheCatalogOrder(t *testing.T) {
	keys := catalog.CanonicalSizeKeys()
	require.GreaterOrEqual(t, len(keys), 2, "need two sizes to compare")
	smallest, largest := keys[0], keys[len(keys)-1]

	assert.True(t, SizeIsSmallerThan(smallest, largest),
		"the smallest catalog size is smaller than the largest")
	assert.False(t, SizeIsSmallerThan(largest, smallest))
	assert.False(t, SizeIsSmallerThan(smallest, smallest),
		"a size is not smaller than itself")

	// Case is the caller's business, not the comparison's: scenarios store "M"
	// while the catalog stores "m".
	assert.True(t, SizeIsSmallerThan(strings.ToUpper(smallest), strings.ToUpper(largest)),
		"ranking must not depend on how the caller cased the key")

	// An unknown key is not evidence of a machine being too small.
	assert.False(t, SizeIsSmallerThan("nonexistent-size", largest))
	assert.False(t, SizeIsSmallerThan(smallest, "nonexistent-size"))
}
