package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/services"
)

type historyCommand struct {
	SessionUUID string `json:"session_uuid"`
	ExecutedAt  int64  `json:"executed_at"`
}

type historyRequest struct {
	SessionUUIDs []string `json:"session_uuids"`
	Since        int64    `json:"since"`
	Offset       int      `json:"offset"`
}

// newFakeHistoryAPI answers like POST /admin/history/bulk, at most pageSize commands
// per request, and records what it was asked.
func newFakeHistoryAPI(t *testing.T, commands []historyCommand, pageSize int) *[]historyRequest {
	t.Helper()
	var requests []historyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/1.0/admin/history/bulk", r.URL.Path)
		var request historyRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requests = append(requests, request)

		var matching []historyCommand
		for _, command := range commands {
			if slices.Contains(request.SessionUUIDs, command.SessionUUID) {
				matching = append(matching, command)
			}
		}
		end := min(request.Offset+pageSize, len(matching))
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"commands": matching[request.Offset:end], "total": len(matching)}))
	}))
	t.Cleanup(server.Close)
	t.Setenv("TERMINAL_TRAINER_URL", server.URL)
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	return &requests
}

func TestCommandTimes_ReadsEveryPageAndKeepsOnlyThePeriod(t *testing.T) {
	from := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	requests := newFakeHistoryAPI(t, []historyCommand{
		{"s-1", from.Unix()},
		{"s-1", from.Add(time.Hour).Unix()},
		{"s-1", to.Unix()}, // the end of the period is excluded
		{"s-2", from.Add(2 * time.Hour).Unix()},
	}, 2)

	times, err := services.NewCommandActivityReader().CommandTimes([]string{"s-1", "s-2"}, from, to)

	require.NoError(t, err)
	assert.Len(t, times["s-1"], 2)
	assert.Len(t, times["s-2"], 1)
	assert.Len(t, *requests, 2, "two pages")
	assert.Equal(t, from.Unix()-1, (*requests)[0].Since, "tt-backend excludes this bound, and a command at exactly from counts")
}

func TestCommandTimes_SplitsSessionsToRespectTheLimitOfTheAPI(t *testing.T) {
	sessions := make([]string, 501)
	for i := range sessions {
		sessions[i] = "s-" + strconv.Itoa(i)
	}
	requests := newFakeHistoryAPI(t, nil, 10)

	_, err := services.NewCommandActivityReader().CommandTimes(sessions, time.Unix(0, 0), time.Now())

	require.NoError(t, err)
	require.Len(t, *requests, 2)
	assert.Len(t, (*requests)[0].SessionUUIDs, 500)
}

func TestCommandTimes_NoSessions_AsksNothing(t *testing.T) {
	requests := newFakeHistoryAPI(t, nil, 10)

	times, err := services.NewCommandActivityReader().CommandTimes(nil, time.Unix(0, 0), time.Now())

	require.NoError(t, err)
	assert.Empty(t, times)
	assert.Empty(t, *requests)
}
