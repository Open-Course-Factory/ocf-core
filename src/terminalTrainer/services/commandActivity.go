package services

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"time"

	"soli/formations/src/utils"
)

// tt-backend's limits on POST /admin/history/bulk.
const (
	maxSessionsPerHistoryRequest = 500
	maxCommandsPerHistoryRequest = 100000
)

type bulkCommandHistory struct {
	Commands []struct {
		SessionUUID string `json:"session_uuid"`
		ExecutedAt  int64  `json:"executed_at"`
	} `json:"commands"`
	Total int `json:"total"`
}

// CommandActivityReader reads from tt-backend when commands were typed, never what
// they were. A session spans several days, so per-session totals could not say which
// days it was used on.
type CommandActivityReader struct {
	baseURL, apiVersion, adminKey string
}

func NewCommandActivityReader() *CommandActivityReader {
	return &CommandActivityReader{
		baseURL:    os.Getenv("TERMINAL_TRAINER_URL"),
		apiVersion: cmp.Or(os.Getenv("TERMINAL_TRAINER_API_VERSION"), "1.0"),
		adminKey:   os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
	}
}

// CommandTimes returns when each session's commands ran within [from, to).
func (r *CommandActivityReader) CommandTimes(sessionUUIDs []string, from, to time.Time) (map[string][]time.Time, error) {
	times := make(map[string][]time.Time, len(sessionUUIDs))
	for sessions := range slices.Chunk(sessionUUIDs, maxSessionsPerHistoryRequest) {
		if err := r.collectCommandTimes(sessions, from, to, times); err != nil {
			return nil, err
		}
	}
	return times, nil
}

func (r *CommandActivityReader) collectCommandTimes(sessionUUIDs []string, from, to time.Time, times map[string][]time.Time) error {
	for offset := 0; ; {
		page, err := r.fetchHistoryPage(sessionUUIDs, from, offset)
		if err != nil {
			return err
		}
		for _, command := range page.Commands {
			executedAt := time.Unix(command.ExecutedAt, 0)
			if !executedAt.Before(to) {
				return nil // tt-backend sorts by time, so the rest is later still
			}
			if !executedAt.Before(from) {
				times[command.SessionUUID] = append(times[command.SessionUUID], executedAt)
			}
		}
		offset += len(page.Commands)
		if len(page.Commands) == 0 || offset >= page.Total {
			return nil
		}
	}
}

func (r *CommandActivityReader) fetchHistoryPage(sessionUUIDs []string, since time.Time, offset int) (*bulkCommandHistory, error) {
	url := fmt.Sprintf("%s/%s/admin/history/bulk", r.baseURL, r.apiVersion)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(r.adminKey))

	body := map[string]any{
		"session_uuids": sessionUUIDs,
		"since":         since.Unix() - 1, // tt-backend's bound is exclusive
		"limit":         maxCommandsPerHistoryRequest,
		"offset":        offset,
	}
	var page bulkCommandHistory
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, body, &page, opts); err != nil {
		return nil, fmt.Errorf("failed to fetch command history: %w", err)
	}
	return &page, nil
}
