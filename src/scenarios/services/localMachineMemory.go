package services

import (
	"soli/formations/src/payment/catalog"
	"soli/formations/src/scenarios/models"
)

// Memory of a machine in the learner's browser, in MiB. Below 1 GiB a Debian
// lab does not run.
const (
	localMachineMinMemoryMiB     = 1024
	localMachineDefaultMemoryMiB = 2048
	localMachineDefaultCPUs      = 2
)

// LocalMachineMemoryMiB is the memory a run of scenario gets on a machine in
// the learner's browser: its size's, from the size catalog, at least 1 GiB;
// 2 GiB when it asks for no size the catalog knows. Above 2 GiB the page warns
// that the learner's computer needs that much (the front's rule).
func LocalMachineMemoryMiB(scenario models.Scenario) int {
	size, ok := catalog.LookupSize(scenario.InstanceType)
	if !ok {
		return localMachineDefaultMemoryMiB
	}
	return max(size.MemoryMB, localMachineMinMemoryMiB)
}

// LocalMachineCPUs is the scenario size's vCPUs, rounded up: the catalog
// counts millicores, and half a vCPU still needs a whole one in the browser.
func LocalMachineCPUs(scenario models.Scenario) int {
	size, ok := catalog.LookupSize(scenario.InstanceType)
	if !ok {
		return localMachineDefaultCPUs
	}
	return max((size.CPU+catalog.MilliCPUPerVCPU-1)/catalog.MilliCPUPerVCPU, 1)
}
