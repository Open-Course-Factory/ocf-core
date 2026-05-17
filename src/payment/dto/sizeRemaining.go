// Package dto — canonical leaf types shared across packages that cannot
// import each other without inducing cycles.
//
// SizeRemaining lives here (instead of in payment/services or
// terminalTrainer/dto) so the QuotaService can compute it, the
// terminalTrainer can embed it in its response DTOs, and neither side
// has to re-declare the shape. Keeping the type alone in this file
// keeps payment/dto a true leaf — adding behaviour here would
// reintroduce the cycle this file exists to avoid.
package dto

// SizeRemaining describes how many additional instances of one machine
// size the user could still afford under the current budget.
//
// The struct intentionally has no methods: pure data so both
// payment/services (producer) and terminalTrainer/dto (consumer) can
// reuse the same shape without an import cycle.
type SizeRemaining struct {
	Key            string `json:"key"`
	CPU            int    `json:"cpu"`
	MemoryMB       int    `json:"memory_mb"`
	RemainingCount int    `json:"remaining_count"`
}
