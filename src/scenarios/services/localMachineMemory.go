package services

import (
	"soli/formations/src/payment/catalog"
	"soli/formations/src/scenarios/models"
)

// localMachineMinMemoryMiB: below 1 GiB a Debian lab does not run.
const localMachineMinMemoryMiB = 1024

// localMachineSize is the catalog size a run of scenario gets in the
// learner's browser: the one LocalProvisioning gives it.
// Zero if the catalog no longer has it.
func localMachineSize(scenario models.Scenario) catalog.MachineSize {
	size, _ := catalog.LookupSize(LocalProvisioning(scenario).Size)
	return size
}

// LocalMachineMemoryMiB is the memory, in MiB, a run of scenario gets on a
// machine in the learner's browser: its size's, at least 1 GiB. Above 2 GiB
// the page warns that the learner's computer needs that much (the front's rule).
func LocalMachineMemoryMiB(scenario models.Scenario) int {
	return max(localMachineSize(scenario).MemoryMB, localMachineMinMemoryMiB)
}

// LocalMachineCPUs is the size's vCPUs, rounded up: the catalog counts
// millicores, and half a vCPU still needs a whole one in the browser.
func LocalMachineCPUs(scenario models.Scenario) int {
	return max((localMachineSize(scenario).CPU+catalog.MilliCPUPerVCPU-1)/catalog.MilliCPUPerVCPU, 1)
}
