// tests/terminalTrainer/hostCapacity_test.go
//
// tt-backend reports the host's total RAM (ram_total_gb) and CPU count
// (cpu_total) on GET /metrics. ocf-core used to recover the total from the
// available/percent pair in two places, with two copies of the formula. The
// total now has one owner: the field, read through
// ServerMetricsResponse.TotalRAMGB, which derives it only when an older
// tt-backend did not send it.
package terminalTrainer_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

func TestServerMetrics_TotalRAMGB_ReadsTheReportedTotal(t *testing.T) {
	// Deliberately inconsistent with the available/percent pair (which would
	// derive 4 GB): the reported total wins when it is present.
	m := dto.ServerMetricsResponse{RAMTotalGB: 64, RAMAvailableGB: 2, RAMPercent: 50}

	assert.Equal(t, 64.0, m.TotalRAMGB())
}

func TestServerMetrics_TotalRAMGB_DerivesWhenAbsent(t *testing.T) {
	// An older tt-backend sends no ram_total_gb; the field decodes as 0.
	m := dto.ServerMetricsResponse{RAMAvailableGB: 10, RAMPercent: 50}

	assert.InDelta(t, 20.0, m.TotalRAMGB(), 1e-9)
}

func TestServerMetrics_TotalRAMGB_ZeroWhenUnderivable(t *testing.T) {
	m := dto.ServerMetricsResponse{RAMAvailableGB: 0, RAMPercent: 100}

	assert.Equal(t, 0.0, m.TotalRAMGB(), "a fully used host with no reported total has no derivable total")
}

// The launch reserve is a fraction of the total, so the reported total must
// be what the reserve is taken from. Derived from the pair these metrics
// would give a 4 GB host and a 0.2 GB reserve; the host reports 100 GB.
func TestEvaluateLaunchCapacity_ReservesFromTheReportedTotal(t *testing.T) {
	metrics := &dto.ServerMetricsResponse{RAMTotalGB: 100, RAMAvailableGB: 2, RAMPercent: 50}

	result := services.EvaluateLaunchCapacity(&paymentModels.SubscriptionPlan{}, "XS", metrics)

	assert.Equal(t, services.CapacityStatusCritical, result.Status)
	assert.Equal(t, "insufficient_ram_for_size", result.Reason)
}

// The bulk pre-flight is the other reader of the total. Same shape: the pair
// would derive a 6 GB host (0.3 GB reserve, one S terminal fits), the host
// reports 100 GB (5 GB reserve, it does not).
func TestBulkCreateTerminals_ReservesFromTheReportedTotal(t *testing.T) {
	ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dto.ServerMetricsResponse{
			RAMTotalGB:     100,
			RAMAvailableGB: 3,
			RAMPercent:     50,
			Timestamp:      time.Now().Unix(),
		})
	}))
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")

	db := freshTestDB(t)
	ownerID := "bulk-total-owner-" + uuid.New().String()
	group := createTestGroup(t, db, ownerID)
	addActiveGroupMember(t, db, group.ID, "bulk-total-member-"+uuid.New().String(), groupModels.GroupMemberRoleMember)
	plan := &paymentModels.SubscriptionPlan{Name: "BulkTotal", IsActive: true, MaxCPU: 100000, MaxMemoryMB: 51200, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)

	_, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"},
		dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04"}, plan)

	require.True(t, errors.Is(err, services.ErrBulkInsufficientRAM),
		"the reserve must come from the reported total; got %v", err)
}
