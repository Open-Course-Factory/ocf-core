package services

import (
	"soli/formations/src/payment/catalog"
	paymentServices "soli/formations/src/payment/services"
)

// BudgetForTerminalKey converts a user's plan ceiling into the per-key budget
// units Terminal Trainer expects, returning nil on an axis that must carry no
// cap.
//
// The two systems do not count CPU the same way, and this is the seam:
//
//   - ocf-core prices CPU in allowance-aware mCPU — size "xs" runs at a 50%
//     CPU allowance and costs 500 mCPU.
//   - tt-backend's max_cpu_total counts WHOLE CPUs off its own size catalog,
//     where "xs" is cpu: 1.
//
// An mCPU budget therefore cannot map exactly onto tt-backend's units, so the
// conversion rounds UP. That is the fail-safe direction: it can only grant
// more headroom on the tt-backend side, never less, which keeps the per-key
// budget a backstop rather than a second, stricter limit that silently
// contradicts the plan the learner was sold. ocf-core's own budget gate stays
// the authoritative one.
//
// nil is returned (meaning "no cap", NULL in tt-backend) on an axis whose
// ceiling is not positive, which is a user holding no budget on it in any
// context: tt-backend has no way to express a zero budget (it rejects 0 as
// invalid), so nothing is sent and ocf-core's own gate remains what refuses
// them. Every plan carries a positive budget, so an entitled user always gets
// a cap.
func BudgetForTerminalKey(ceiling paymentServices.UserBudgetCeiling) (maxCPUTotal, maxMemoryMBTotal *int64) {
	if ceiling.MaxCPU > 0 {
		// Ceiling division: any fraction of a vCPU claims a whole one.
		vcpu := int64((ceiling.MaxCPU + catalog.MilliCPUPerVCPU - 1) / catalog.MilliCPUPerVCPU)
		maxCPUTotal = &vcpu
	}

	if ceiling.MaxMemoryMB > 0 {
		mem := int64(ceiling.MaxMemoryMB)
		maxMemoryMBTotal = &mem
	}

	return maxCPUTotal, maxMemoryMBTotal
}
