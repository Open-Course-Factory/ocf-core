package services

import (
	"encoding/json"
	"fmt"
	"sort"
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
	sorted := make([]models.ScenarioInstanceType, len(types))
	copy(sorted, types)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
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

// ResolveStepType decides the step_type stored for an authored step.
//
// It exists because ScenarioStep.StepType carries gorm:"default:'terminal'",
// so a step saved with an empty type is never empty in the database — it is
// the literal string "terminal". Consumers that fall back to has_flag only
// when step_type is blank therefore never got the chance: a flag step was
// stored as a terminal step and rendered the Verify UI with nowhere to enter
// the flag, leaving the scenario uncompletable.
//
// The derivation belongs here, at the import and seed boundary, because this
// is the only place that can see the difference. Once a row is written, "flag
// step that declared nothing" and "terminal step that deliberately declared
// terminal" look identical, and no reader can tell them apart.
//
// An explicit type therefore always wins, including the combination that looks
// contradictory: step_type "terminal" together with has_flag is a legitimate
// authored shape — a terminal step that also drops a flag file via FlagPath —
// and must not be "corrected" to a flag step.
func ResolveStepType(declared string, hasFlag bool) string {
	if declared != "" {
		return declared
	}
	if hasFlag {
		return StepTypeFlag
	}
	return StepTypeTerminal
}

// Canonical step_type values. The strings are part of the stored data and the
// API contract with the frontend, so they must not be renamed.
const (
	StepTypeTerminal = "terminal"
	StepTypeFlag     = "flag"
)

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
