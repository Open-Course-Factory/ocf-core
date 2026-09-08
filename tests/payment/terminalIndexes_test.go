package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relaxTerminalColumnsForRawInserts rebuilds the terminals table, which in
// SQLite drops every index the model declares. This pins them back.
func TestTerminalsTable_KeepsModelIndexesAfterRelaxingColumns(t *testing.T) {
	db := freshTestDB(t)

	insert := `INSERT INTO terminals (id, session_id, user_id, name, state) VALUES (?, ?, ?, ?, 'running')`
	require.NoError(t, db.Exec(insert, uuid.New().String(), "same-session", "u1", "first").Error)
	assert.Error(t, db.Exec(insert, uuid.New().String(), "same-session", "u2", "second").Error,
		"session_id must stay unique")

	var indexes []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'terminals'`).Scan(&indexes).Error)
	for _, want := range []string{
		"idx_terminals_session_id",
		"idx_terminals_user_id",
		"idx_terminals_organization_id",
		"idx_terminals_subscription_plan_id",
		"idx_terminals_user_terminal_key_id",
		"idx_terminals_deleted_at",
	} {
		assert.Contains(t, indexes, want)
	}
}
