package services

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"soli/formations/src/scenarios/models"
)

// SortInstanceTypesByPriority returns the declared images in the order the
// author meant them to be tried: priority ascending, lowest first.
//
// It copies rather than sorting in place because callers hold these as a
// preloaded association, and reordering a GORM-loaded slice underneath the
// parent struct has bitten this codebase before. Both the launch resolver and
// the exporter read the order, and they must agree — an export that renumbers
// preferences would re-import as a different scenario.
func SortInstanceTypesByPriority(types []models.ScenarioInstanceType) []models.ScenarioInstanceType {
	sorted := slices.Clone(types)
	slices.SortStableFunc(sorted, func(a, b models.ScenarioInstanceType) int { return cmp.Compare(a.Priority, b.Priority) })
	return sorted
}

// EncodeRequiredFeatures renders authored feature names into the JSON-array
// text that Scenario.RequiredFeatures stores and GetRequiredFeatures parses.
//
// Empty in, empty out — and deliberately so: "" is the column's "requires
// nothing" value, whereas "[]" would parse to an empty slice and read the same
// downstream while making every export noisier. Blank names are dropped so a
// trailing comma in authored content cannot produce a feature called "".
func EncodeRequiredFeatures(names []string) (string, error) {
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return "", fmt.Errorf("failed to encode required_features: %w", err)
	}
	return string(encoded), nil
}

// BuildCompatibleInstanceTypes turns an authored, preference-ordered list of
// distribution names into ScenarioInstanceType rows.
//
// Declaration order is the preference order: resolveDistribution tries these
// by Priority ascending, so position in the authored list becomes Priority.
// Blank names are dropped rather than stored, since an empty InstanceType can
// never match a distribution and would only occupy a priority slot.
//
// Shared by the import and seed paths so an authored scenario means the same
// thing however it reaches the platform.
func BuildCompatibleInstanceTypes(names []string) []models.ScenarioInstanceType {
	types := make([]models.ScenarioInstanceType, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		types = append(types, models.ScenarioInstanceType{
			InstanceType: trimmed,
			Priority:     len(types),
		})
	}
	return types
}
