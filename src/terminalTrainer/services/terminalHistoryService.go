package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strconv"
	"time"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// terminalHistoryService owns the command-history concern: reading per-session
// and per-group command history from tt-backend, computing group-level history
// statistics, and the RGPD erasure paths (single-session and bulk-per-key).
//
// It was carved out of terminalTrainerService to shrink that god object;
// terminalTrainerService holds a *terminalHistoryService and delegates the
// relevant interface methods to it.
//
// Per-session reads/deletes resolve the owning Terminal row (and its API key)
// through the repository, then call tt-backend through the shared
// terminalProxyClient's buildAPIPath helper. Group history fans out over the
// group's members via the db, using the OCF admin key for the bulk endpoints.
// The connection settings (baseURL, apiVersion, adminKey) are passed in at
// construction — mirroring terminalCatalogService — so the history service owns
// the full read path without re-reading the environment.
type terminalHistoryService struct {
	proxy      *terminalProxyClient
	repository repositories.TerminalRepository
	db         *gorm.DB
	baseURL    string
	apiVersion string
	adminKey   string
}

// newTerminalHistoryService returns a history service reading from tt-backend
// through the supplied proxy (buildAPIPath) and connection settings. The proxy,
// repository and db are shared with the facade so all reuse the same HTTP
// configuration and DB.
func newTerminalHistoryService(proxy *terminalProxyClient, repository repositories.TerminalRepository, db *gorm.DB, baseURL, apiVersion, adminKey string) *terminalHistoryService {
	return &terminalHistoryService{
		proxy:      proxy,
		repository: repository,
		db:         db,
		baseURL:    baseURL,
		apiVersion: apiVersion,
		adminKey:   adminKey,
	}
}

// GetSessionCommandHistory retrieves command history from tt-backend
func (h *terminalHistoryService) GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error) {
	// Validate format against whitelist to prevent URL parameter injection
	if format != "" && format != "json" && format != "csv" {
		format = "json" // default to json for unknown formats
	}

	terminal, err := h.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("session not found: %w", err)
	}

	path := h.proxy.buildAPIPath("/history", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", h.baseURL, path, neturl.QueryEscape(sessionID))
	if since != nil {
		url += fmt.Sprintf("&since=%d", *since)
	}
	if format != "" {
		url += fmt.Sprintf("&format=%s", format)
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	if offset > 0 {
		url += fmt.Sprintf("&offset=%d", offset)
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "GET", url, nil, opts)
	if err != nil {
		return nil, "", err
	}

	// Enforce a 10MB cap on response body to prevent OOM from oversized payloads
	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	if len(resp.Body) > maxResponseSize {
		return nil, "", fmt.Errorf("response body exceeds maximum allowed size (%d bytes > %d bytes)", len(resp.Body), maxResponseSize)
	}

	// Read content-type from tt-backend response when available; fall back to
	// format-based heuristic when the upstream does not provide a header.
	contentType := resp.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
		if format == "csv" {
			contentType = "text/csv"
		}
	}

	return resp.Body, contentType, nil
}

// GetSessionCommandHistoryAdmin retrieves command history for a single tt-backend
// session UUID using the OCF admin API key. Used by trainer endpoints where the
// requesting user is a group manager and does not own the student's session key.
//
// It proxies to tt-backend's bulk history endpoint with a single UUID, returning
// the response body verbatim ({commands, total, limit, offset}). Returns the
// raw JSON bytes plus the content type. limit defaults to 50 and is capped at
// 1000 by tt-backend; offset defaults to 0.
func (h *terminalHistoryService) GetSessionCommandHistoryAdmin(sessionUUID string, limit, offset int) ([]byte, string, error) {
	if h.baseURL == "" || h.adminKey == "" {
		return nil, "", fmt.Errorf("terminal trainer not configured")
	}

	url := fmt.Sprintf("%s/%s/admin/history/bulk", h.baseURL, h.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": []string{sessionUUID},
		"limit":         limit,
		"offset":        offset,
		"format":        "json",
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(h.adminKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, reqBody, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch session history: %w", err)
	}

	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	if len(resp.Body) > maxResponseSize {
		return nil, "", fmt.Errorf("response body exceeds maximum allowed size (%d bytes > %d bytes)", len(resp.Body), maxResponseSize)
	}

	contentType := resp.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	return resp.Body, contentType, nil
}

// DeleteSessionCommandHistory deletes command history (RGPD right to erasure)
func (h *terminalHistoryService) DeleteSessionCommandHistory(sessionID string) error {
	terminal, err := h.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	path := h.proxy.buildAPIPath("/history", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", h.baseURL, path, neturl.QueryEscape(sessionID))

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	return err
}

// DeleteAllUserCommandHistory deletes all command history across all sessions for an API key (RGPD bulk erasure)
func (h *terminalHistoryService) DeleteAllUserCommandHistory(apiKey string) (int64, error) {
	path := fmt.Sprintf("/%s/history/all", h.apiVersion)
	url := fmt.Sprintf("%s%s", h.baseURL, path)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(apiKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	if err != nil {
		return 0, err
	}

	var result struct {
		SessionsCleared int64 `json:"sessions_cleared"`
	}
	if err := resp.DecodeJSON(&result); err != nil {
		return 0, fmt.Errorf("failed to decode bulk delete response: %w", err)
	}

	return result.SessionsCleared, nil
}

// History row-count caps.
//
// JSON pagination is 1k — loading more rows into the DOM is a UX/perf issue.
// CSV export is 100k — sized for exam-scale exports (~5k observed in the field)
// with two orders of magnitude of headroom while still bounded for memory safety.
const (
	defaultHistoryLimit = 50
	maxJSONHistoryLimit = 1000
	maxCSVHistoryLimit  = 100000
)

// ClampHistoryLimit returns the effective row limit for a command-history
// request, applying defaults (when limit <= 0) and a format-aware ceiling
// (1000 for JSON/default, 100000 for CSV).
func ClampHistoryLimit(limit int, format string) int {
	if limit <= 0 {
		return defaultHistoryLimit
	}
	cap := maxJSONHistoryLimit
	if format == "csv" {
		cap = maxCSVHistoryLimit
	}
	if limit > cap {
		return cap
	}
	return limit
}

// GetGroupCommandHistory aggregates command history for all active members of a group.
// Only group owner, admin, or assistant can access this endpoint.
func (h *terminalHistoryService) GetGroupCommandHistory(groupID string, userID string, since *int64, format string, limit, offset int, includeStopped bool, search string) ([]byte, string, error) {
	// Validate and default format
	if format != "" && format != "json" && format != "csv" {
		format = "json"
	}
	if format == "" {
		format = "json"
	}

	limit = ClampHistoryLimit(limit, format)

	// Parse groupID to UUID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid group ID: %w", err)
	}

	// Fetch group with members from DB
	var group groupModels.ClassGroup
	if err := h.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, "", fmt.Errorf("group not found")
	}

	// Check authorization - user must be owner, admin, or assistant
	var userMember *groupModels.GroupMember
	for i := range group.Members {
		if group.Members[i].UserID == userID && group.Members[i].IsActive {
			userMember = &group.Members[i]
			break
		}
	}
	if userMember == nil || !(userMember.Role == groupModels.GroupMemberRoleOwner || userMember.Role == groupModels.GroupMemberRoleManager) {
		return nil, "", fmt.Errorf("unauthorized: only group owner or manager can view group command history")
	}

	// Collect active member user IDs
	var memberUserIDs []string
	for _, m := range group.Members {
		if m.IsActive {
			memberUserIDs = append(memberUserIDs, m.UserID)
		}
	}

	// Fetch terminals for all members, gated by the org-context visibility rule
	// (single home: models.SupervisableByGroupOrgScope) — a NULL-org group sees
	// NOTHING; otherwise only same-org sessions. Same rule the supervision wall uses.
	var terminals []models.Terminal
	query := h.db.Where("user_id IN ?", memberUserIDs).
		Scopes(models.SupervisableByGroupOrgScope(group.OrganizationID))
	if !includeStopped {
		// SSOT: "running display" lives in models.RunningDisplayScope.
		// Inline `status = "active"` predicates drift away from the UI's
		// per-second-aware view (past-expiry zombies remain in results
		// even though their proxy session is gone).
		query = query.Scopes(models.RunningDisplayScope)
	}
	if err := query.Find(&terminals).Error; err != nil {
		return nil, "", fmt.Errorf("failed to query terminals: %w", err)
	}

	// Collect session UUIDs and track unique user IDs
	sessionUUIDs := make([]string, 0, len(terminals))
	userIDSet := make(map[string]bool)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUUIDs = append(sessionUUIDs, t.SessionID)
			userIDSet[t.UserID] = true
		}
	}

	// If no sessions found, return empty result
	if len(sessionUUIDs) == 0 {
		if format == "csv" {
			var buf bytes.Buffer
			writer := csv.NewWriter(&buf)
			_ = writer.Write([]string{"student_name", "student_email", "session_uuid", "sequence_num", "command", "executed_at"})
			writer.Flush()
			return buf.Bytes(), "text/csv", nil
		}
		result := map[string]interface{}{
			"commands": []interface{}{},
			"total":    0,
			"limit":    limit,
			"offset":   offset,
		}
		body, _ := json.Marshal(result)
		return body, "application/json", nil
	}

	// Fetch user info for enrichment using Casdoor SDK
	type userInfo struct {
		DisplayName string
		Email       string
	}
	userInfoMap := make(map[string]userInfo)
	for uid := range userIDSet {
		user, err := casdoorsdk.GetUserByUserId(uid)
		if err == nil && user != nil {
			userInfoMap[uid] = userInfo{
				DisplayName: user.DisplayName,
				Email:       user.Email,
			}
		}
	}

	// Build sessionUUID -> userInfo map
	sessionUserMap := make(map[string]userInfo)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUserMap[t.SessionID] = userInfoMap[t.UserID]
		}
	}

	// Call tt-backend bulk endpoint
	url := fmt.Sprintf("%s/%s/admin/history/bulk", h.baseURL, h.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": sessionUUIDs,
		"limit":         limit,
		"offset":        offset,
		"format":        "json", // Always get JSON from tt-backend, we transform to CSV ourselves if needed
	}
	if since != nil {
		reqBody["since"] = *since
	}
	if search != "" {
		reqBody["search"] = search
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(h.adminKey))

	var bulkResponse struct {
		Commands []struct {
			SessionUUID string `json:"session_uuid"`
			SequenceNum int    `json:"sequence_num"`
			CommandText string `json:"command_text"`
			ExecutedAt  int64  `json:"executed_at"`
		} `json:"commands"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, reqBody, &bulkResponse, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch bulk history: %w", err)
	}

	// Enrich commands with student info
	type enrichedCommand struct {
		StudentName  string `json:"student_name"`
		StudentEmail string `json:"student_email"`
		SessionUUID  string `json:"session_uuid"`
		SequenceNum  int    `json:"sequence_num"`
		CommandText  string `json:"command_text"`
		ExecutedAt   int64  `json:"executed_at"`
	}

	enriched := make([]enrichedCommand, 0, len(bulkResponse.Commands))
	for _, cmd := range bulkResponse.Commands {
		info := sessionUserMap[cmd.SessionUUID]
		enriched = append(enriched, enrichedCommand{
			StudentName:  info.DisplayName,
			StudentEmail: info.Email,
			SessionUUID:  cmd.SessionUUID,
			SequenceNum:  cmd.SequenceNum,
			CommandText:  cmd.CommandText,
			ExecutedAt:   cmd.ExecutedAt,
		})
	}

	// Return in requested format
	if format == "csv" {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"student_name", "student_email", "session_uuid", "sequence_num", "command", "executed_at"})
		for _, cmd := range enriched {
			_ = writer.Write([]string{
				cmd.StudentName,
				cmd.StudentEmail,
				cmd.SessionUUID,
				strconv.Itoa(cmd.SequenceNum),
				cmd.CommandText,
				time.Unix(cmd.ExecutedAt, 0).UTC().Format(time.RFC3339),
			})
		}
		writer.Flush()
		return buf.Bytes(), "text/csv", nil
	}

	// JSON format (default)
	result := map[string]interface{}{
		"commands": enriched,
		"total":    bulkResponse.Total,
		"limit":    bulkResponse.Limit,
		"offset":   bulkResponse.Offset,
	}
	body, _ := json.Marshal(result)
	return body, "application/json", nil
}

// GetGroupCommandHistoryStats returns aggregate command history statistics for all members of a group.
// Only group owner, admin, or assistant can access this endpoint.
func (h *terminalHistoryService) GetGroupCommandHistoryStats(groupID string, userID string, includeStopped bool) ([]byte, string, error) {
	// Parse groupID to UUID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid group ID: %w", err)
	}

	// Fetch group with members from DB
	var group groupModels.ClassGroup
	if err := h.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, "", fmt.Errorf("group not found")
	}

	// Check authorization - user must be owner, admin, or assistant
	var userMember *groupModels.GroupMember
	for i := range group.Members {
		if group.Members[i].UserID == userID && group.Members[i].IsActive {
			userMember = &group.Members[i]
			break
		}
	}
	if userMember == nil || !(userMember.Role == groupModels.GroupMemberRoleOwner || userMember.Role == groupModels.GroupMemberRoleManager) {
		return nil, "", fmt.Errorf("unauthorized: only group owner or manager can view group command history stats")
	}

	// Collect active member user IDs
	var memberUserIDs []string
	for _, m := range group.Members {
		if m.IsActive {
			memberUserIDs = append(memberUserIDs, m.UserID)
		}
	}

	// Fetch terminals for all members, gated by the org-context visibility rule
	// (single home: models.SupervisableByGroupOrgScope) — see the matching block in
	// GetGroupCommandHistory above. A NULL-org group sees NOTHING.
	var terminals []models.Terminal
	query := h.db.Where("user_id IN ?", memberUserIDs).
		Scopes(models.SupervisableByGroupOrgScope(group.OrganizationID))
	if !includeStopped {
		// SSOT alignment — see RunningDisplayScope rationale on the
		// matching block in GetGroupCommandHistory above.
		query = query.Scopes(models.RunningDisplayScope)
	}
	if err := query.Find(&terminals).Error; err != nil {
		return nil, "", fmt.Errorf("failed to query terminals: %w", err)
	}

	// Collect session UUIDs and build terminal -> user mapping
	sessionUUIDs := make([]string, 0, len(terminals))
	userIDSet := make(map[string]bool)
	sessionToUserID := make(map[string]string)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUUIDs = append(sessionUUIDs, t.SessionID)
			userIDSet[t.UserID] = true
			sessionToUserID[t.SessionID] = t.UserID
		}
	}

	// If no sessions found, return empty stats
	if len(sessionUUIDs) == 0 {
		result := map[string]interface{}{
			"summary": map[string]interface{}{
				"total_commands":               0,
				"total_sessions":               0,
				"active_students":              0,
				"avg_commands_per_student":     0.0,
				"avg_time_per_student_seconds": 0,
			},
			"students": []interface{}{},
		}
		body, _ := json.Marshal(result)
		return body, "application/json", nil
	}

	// Fetch user info for enrichment using Casdoor SDK
	type userInfo struct {
		DisplayName string
		Email       string
	}
	userInfoMap := make(map[string]userInfo)
	for uid := range userIDSet {
		user, err := casdoorsdk.GetUserByUserId(uid)
		if err == nil && user != nil {
			userInfoMap[uid] = userInfo{
				DisplayName: user.DisplayName,
				Email:       user.Email,
			}
		}
	}

	// Call tt-backend bulk-stats endpoint
	url := fmt.Sprintf("%s/%s/admin/history/bulk-stats", h.baseURL, h.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": sessionUUIDs,
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(h.adminKey))

	var bulkStatsResponse struct {
		Sessions []struct {
			SessionUUID    string `json:"session_uuid"`
			CommandCount   int    `json:"command_count"`
			FirstCommandAt int64  `json:"first_command_at"`
			LastCommandAt  int64  `json:"last_command_at"`
		} `json:"sessions"`
		TotalCommands int `json:"total_commands"`
		TotalSessions int `json:"total_sessions"`
	}

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, reqBody, &bulkStatsResponse, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch bulk stats: %w", err)
	}

	// Build per-student stats
	type studentStats struct {
		StudentName      string `json:"student_name"`
		StudentEmail     string `json:"student_email"`
		TotalCommands    int    `json:"total_commands"`
		SessionCount     int    `json:"session_count"`
		TotalTimeSeconds int64  `json:"total_time_seconds"`
		LastActiveAt     int64  `json:"last_active_at"`
	}

	studentMap := make(map[string]*studentStats)
	for _, sess := range bulkStatsResponse.Sessions {
		uid, ok := sessionToUserID[sess.SessionUUID]
		if !ok {
			continue
		}
		st, exists := studentMap[uid]
		if !exists {
			info := userInfoMap[uid]
			st = &studentStats{
				StudentName:  info.DisplayName,
				StudentEmail: info.Email,
			}
			studentMap[uid] = st
		}
		st.TotalCommands += sess.CommandCount
		st.SessionCount++
		if sess.LastCommandAt > sess.FirstCommandAt {
			st.TotalTimeSeconds += sess.LastCommandAt - sess.FirstCommandAt
		}
		if sess.LastCommandAt > st.LastActiveAt {
			st.LastActiveAt = sess.LastCommandAt
		}
	}

	// Convert to slice
	students := make([]studentStats, 0, len(studentMap))
	for _, st := range studentMap {
		students = append(students, *st)
	}

	// Build summary
	activeStudents := len(studentMap)
	var avgCommandsPerStudent float64
	var avgTimePerStudentSecs int64
	if activeStudents > 0 {
		avgCommandsPerStudent = float64(bulkStatsResponse.TotalCommands) / float64(activeStudents)
		var totalTime int64
		for _, st := range students {
			totalTime += st.TotalTimeSeconds
		}
		avgTimePerStudentSecs = totalTime / int64(activeStudents)
	}

	type statsSummary struct {
		TotalCommands         int     `json:"total_commands"`
		TotalSessions         int     `json:"total_sessions"`
		ActiveStudents        int     `json:"active_students"`
		AvgCommandsPerStudent float64 `json:"avg_commands_per_student"`
		AvgTimePerStudentSecs int64   `json:"avg_time_per_student_seconds"`
	}

	result := map[string]interface{}{
		"summary": statsSummary{
			TotalCommands:         bulkStatsResponse.TotalCommands,
			TotalSessions:         bulkStatsResponse.TotalSessions,
			ActiveStudents:        activeStudents,
			AvgCommandsPerStudent: avgCommandsPerStudent,
			AvgTimePerStudentSecs: avgTimePerStudentSecs,
		},
		"students": students,
	}

	body, _ := json.Marshal(result)
	return body, "application/json", nil
}
