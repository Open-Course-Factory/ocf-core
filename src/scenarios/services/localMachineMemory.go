package services

import (
	"soli/formations/src/payment/catalog"
	"soli/formations/src/scenarios/models"
)

// Memory of a machine in the learner's browser, in MiB. Below 1 GiB a Debian
// lab does not run; above 2 GiB the machine crowds a learner's 4 to 8 GB
// laptop.
const (
	localMachineMinMemoryMiB     = 1024
	localMachineDefaultMemoryMiB = 2048
)

// LocalMachineMemoryMiB is the memory a run of scenario gets on a machine in
// the learner's browser: its size's, from the size catalog, within 1 to 2
// GiB; 2 GiB when it asks for no size the catalog knows.
func LocalMachineMemoryMiB(scenario models.Scenario) int {
	size, ok := catalog.LookupSize(scenario.InstanceType)
	if !ok {
		return localMachineDefaultMemoryMiB
	}
	return min(max(size.MemoryMB, localMachineMinMemoryMiB), localMachineDefaultMemoryMiB)
}
