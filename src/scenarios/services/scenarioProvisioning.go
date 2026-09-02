package services

// ScenarioProvisioning is how a scenario's container must be built: which
// distribution and size it needs, the features it keeps for the whole run, and
// the ones it holds only while it is being provisioned.
//
// It exists so the answer travels as one value. Bulk-start used to take a
// distribution and a backend as two loose strings and fill in the rest itself —
// size hardcoded to "S", features and build features never asked for — which is
// why a class launch produced containers a size too large and with no network
// to install anything from.
//
// The single owner of the answer is the resolver in the routes package, which
// reads the scenario's declaration against a backend's catalog. This type is
// what it returns.
type ScenarioProvisioning struct {
	// Backend is the tt-backend the container is created on; empty means the
	// system default.
	Backend string

	// Distribution is the image name. Empty means "no terminal at all" — a
	// bulk start that only records scenario sessions.
	Distribution string

	// Size is the machine size key ("xs", "s", …), taken from the scenario.
	Size string

	// Features are kept for the whole session.
	Features map[string]bool

	// BuildFeatures are attached to provision the container and taken back the
	// moment setup reports it is done. A scenario whose setup installs packages
	// declares "network" here so the learner's machine is not left online.
	BuildFeatures map[string]bool
}

// CreatesTerminal reports whether this provisioning asks for a container.
func (p ScenarioProvisioning) CreatesTerminal() bool {
	return p.Distribution != ""
}
