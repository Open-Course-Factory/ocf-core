package services

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	ttDto "soli/formations/src/terminalTrainer/dto"
	terminalModels "soli/formations/src/terminalTrainer/models"
	ttServices "soli/formations/src/terminalTrainer/services"
)

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

	// AutoPlaced is true when nobody chose Backend: the resolution placed it.
	// A learner whose own policy does not allow it may then be placed
	// elsewhere; a host someone chose is kept, or refused.
	AutoPlaced bool

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

// localDistribution names what a local machine boots, for the terminal row:
// the learner's tab picks the image itself.
const localDistribution = "debian"

// localMachineDefaultSize is the size of a run in the learner's browser
// when its scenario names none the catalog knows.
const localMachineDefaultSize = "s"

// LocalProvisioning is the machine a run gets in the learner's browser: the
// scenario's size, which sets the machine's memory and vCPUs
// (LocalMachineMemoryMiB, LocalMachineCPUs) but reserves no budget (plan
// rule 8), and nothing else — no host to place it on, no network, no build features.
func LocalProvisioning(scenario models.Scenario) ScenarioProvisioning {
	size := strings.ToLower(scenario.InstanceType)
	if _, known := catalog.LookupSize(size); !known {
		size = localMachineDefaultSize
	}
	return ScenarioProvisioning{Backend: terminalModels.LocalBackend, Distribution: localDistribution, Size: size}
}

// IsLocal reports whether this provisioning is a machine in the learner's browser.
func (p ScenarioProvisioning) IsLocal() bool {
	return p.Backend == terminalModels.LocalBackend
}

// CreatesTerminal reports whether this provisioning asks for a container.
func (p ScenarioProvisioning) CreatesTerminal() bool {
	return p.Distribution != ""
}

// ComposedSessionInput is the container request of a scenario run, shared by
// the learner's own launch and the class bulk-start so the two cannot diverge
// (the console uid and the internet cut once reached only the solo launch).
// The caller may still set what only it knows: Expiry, or another Backend.
func (p ScenarioProvisioning) ComposedSessionInput(db *gorm.DB, userID string, scenario *models.Scenario, terms string, orgID *uuid.UUID, plan *paymentModels.SubscriptionPlan, namePrefix string) (ttDto.CreateComposedSessionInput, error) {
	// A trainer's internet cut reaches the scenario through the classes and
	// organizations that assign it, whatever the run's organization context.
	networkCut, err := terminalModels.ScenarioNetworkCut(db, userID, scenario.ID)
	if err != nil {
		return ttDto.CreateComposedSessionInput{}, fmt.Errorf("check network cuts: %w", err)
	}
	input := ttDto.CreateComposedSessionInput{
		Distribution:  p.Distribution,
		Size:          p.Size,
		Features:      p.Features,
		BuildFeatures: p.BuildFeatures,
		Terms:         terms,
		Name:          namePrefix + scenario.Title,
		Hostname:      scenario.Hostname,
		Backend:       p.Backend,
		// Recording is always on (RGPD Art. 6.1.f — legitimate interest).
		RecordingEnabled: 1,
		SessionUser:      scenario.SessionUser,
		NetworkDisabled:  networkCut,
		PersistenceMode:  ttServices.ResolveScenarioPersistenceMode(plan),
	}
	if orgID != nil {
		input.OrganizationID = orgID.String()
	}
	return input, nil
}
