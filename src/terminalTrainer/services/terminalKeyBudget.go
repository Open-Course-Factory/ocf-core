package services

import (
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

// mCPUPerVCPU is the ocf-core CPU unit scale: SubscriptionPlan.MaxCPU and
// the size catalog are both expressed in millicores, where 1000 = 1 vCPU.
const mCPUPerVCPU = 1000

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
// nil is returned (meaning "no cap", NULL in tt-backend) in two cases:
//   - the axis is unlimited (0 on the plan), and
//   - the user holds no entitlement at all — tt-backend has no way to express
//     a zero budget (it rejects 0 as invalid), so nothing is sent and
//     ocf-core's gate remains what refuses them.
func BudgetForTerminalKey(ceiling paymentServices.UserBudgetCeiling) (maxCPUTotal, maxMemoryMBTotal *int64) {
	if !ceiling.HasEntitlement {
		return nil, nil
	}

	if !paymentModels.IsUnlimitedBudget(ceiling.MaxCPU) {
		// Ceiling division: any fraction of a vCPU claims a whole one.
		vcpu := int64((ceiling.MaxCPU + mCPUPerVCPU - 1) / mCPUPerVCPU)
		maxCPUTotal = &vcpu
	}

	if !paymentModels.IsUnlimitedBudget(ceiling.MaxMemoryMB) {
		mem := int64(ceiling.MaxMemoryMB)
		maxMemoryMBTotal = &mem
	}

	return maxCPUTotal, maxMemoryMBTotal
}
