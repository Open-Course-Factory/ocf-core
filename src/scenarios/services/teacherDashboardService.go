package services

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/repositories"
	"soli/formations/src/scenarios/models"
	ttDto "soli/formations/src/terminalTrainer/dto"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"
)

// Sentinel errors for teacher dashboard operations. Controllers translate these
// into HTTP status codes (404 for the first three to avoid leaking existence).
var (
	ErrSessionNotFound              = errors.New("session not found")
	ErrSessionNotInGroup            = errors.New("session does not belong to this group")
	ErrSessionHasNoTerminal         = errors.New("session has no terminal yet")
	ErrScenarioNotAssignedToGroup   = errors.New("scenario is not assigned to this group")
)

// GroupActivityItem represents an active session for a group member
type GroupActivityItem struct {
	SessionID         uuid.UUID `json:"session_id"`
	UserID            string    `json:"user_id"`
	UserName          string    `json:"user_name,omitempty"`
	UserEmail         string    `json:"user_email,omitempty"`
	ScenarioID        uuid.UUID `json:"scenario_id"`
	ScenarioTitle     string    `json:"scenario_title"`
	CurrentStep       int       `json:"current_step"`
	TotalSteps        int64     `json:"total_steps"`
	Status            string    `json:"status"`
	StartedAt         time.Time `json:"started_at"`
	TerminalSessionID *string   `json:"terminal_session_id,omitempty"`
}

// AssignmentProgressItem re-exports the DTO so callers reference it as
// services.AssignmentProgressItem, consistent with the other item types here.
type AssignmentProgressItem = dto.AssignmentProgressItem

// ScenarioResultItem represents a single session result for a scenario
type ScenarioResultItem struct {
	SessionID      uuid.UUID  `json:"session_id"`
	UserID         string     `json:"user_id"`
	UserName       string     `json:"user_name,omitempty"`
	UserEmail      string     `json:"user_email,omitempty"`
	Status         string     `json:"status"`
	Grade          *float64   `json:"grade,omitempty"`
	CurrentStep    int        `json:"current_step"`
	TotalSteps     int64      `json:"total_steps"`
	CompletedSteps int64      `json:"completed_steps"`
	TotalHintsUsed int64      `json:"total_hints_used"`
	// CorrectCount is the absolute number of correct answers in the session
	// (correct quiz answers + correct flag submissions). Computed in Go from
	// step progress and ScenarioFlag rows; not stored in DB.
	CorrectCount int64 `json:"correct_count"`
	// TotalCorrectPossible is the static maximum for this scenario: total quiz
	// questions across all (non-deleted) quiz steps + count of flag-bearing
	// steps. Does not change as the session progresses.
	TotalCorrectPossible int64      `json:"total_correct_possible"`
	StartedAt            time.Time  `json:"started_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

// PaginatedScenarioResults represents paginated scenario results with total count
type PaginatedScenarioResults struct {
	Items  []ScenarioResultItem `json:"items"`
	Total  int64                `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// ScenarioAnalytics represents aggregated analytics for a scenario within a group
type ScenarioAnalytics struct {
	TotalSessions  int64 `json:"total_sessions"`
	CompletedCount int64 `json:"completed_count"`
	// CompletionRate is a PERCENTAGE (0..100) of completed SESSIONS over TOTAL
	// SESSIONS: a learner who retries a scenario counts once per attempt.
	//
	// Not to be confused with TeacherGroupAssignment.ClassCompletionRate
	// (teacherGroupsService.go), which is the share of the CLASS that finished —
	// distinct members over class size. Same 0..100 scale, different denominators;
	// the two are not interchangeable and must not be unified under one name.
	CompletionRate        float64  `json:"completion_rate"`
	AvgGrade              *float64 `json:"avg_grade,omitempty"`
	AvgCompletionTimeSecs *float64 `json:"avg_completion_time_seconds,omitempty"`
}

// fetchUserMap loads display name and email for a set of user IDs from Casdoor.
// Returns a map[userID] → {name, email}. Errors are logged, not propagated.
type userInfo struct {
	Name  string
	Email string
}

// userCache caches Casdoor user info to avoid N+1 HTTP calls.
// Each entry expires after 5 minutes.
var userCache sync.Map

type cachedUser struct {
	info      userInfo
	expiresAt time.Time
}

func fetchUserMap(userIDs []string) map[string]userInfo {
	m := make(map[string]userInfo, len(userIDs))
	var misses []string
	now := time.Now()

	// Check cache first
	for _, id := range userIDs {
		if cached, ok := userCache.Load(id); ok {
			if cu, ok := cached.(cachedUser); ok && now.Before(cu.expiresAt) {
				m[id] = cu.info
				continue
			}
		}
		misses = append(misses, id)
	}

	// Fetch cache misses individually
	for _, id := range misses {
		user, err := casdoorsdk.GetUserByUserId(id)
		if err != nil || user == nil {
			utils.Debug("Failed to fetch user %s from Casdoor: %v", id, err)
			continue
		}
		name := user.DisplayName
		if name == "" {
			name = user.Name
		}
		info := userInfo{Name: name, Email: user.Email}
		m[id] = info
		userCache.Store(id, cachedUser{info: info, expiresAt: now.Add(5 * time.Minute)})
	}
	return m
}

// resolveUserIdentities attaches display name and email to a slice of
// teacher-facing rows: it collects the distinct user ids `userIDOf` yields,
// resolves them in one cached batch, then writes each result back through
// `apply`. Single home for a pass every teacher surface needs — activity,
// scenario results, and the live class view all differ only in which field holds
// the user id and which fields receive the identity.
//
// An empty user id is never looked up: an unowned row is a data fault, not a
// reason to hit Casdoor.
func resolveUserIdentities[T any](items []T, userIDOf func(T) string, apply func(*T, userInfo)) {
	ids := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		id := userIDOf(item)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	userMap := fetchUserMap(ids)
	for i := range items {
		if info, ok := userMap[userIDOf(items[i])]; ok {
			apply(&items[i], info)
		}
	}
}

func enrichActivityUsers(items []GroupActivityItem) {
	resolveUserIdentities(items,
		func(item GroupActivityItem) string { return item.UserID },
		func(item *GroupActivityItem, info userInfo) {
			item.UserName, item.UserEmail = info.Name, info.Email
		})
}

func enrichResultUsers(items []ScenarioResultItem) {
	resolveUserIdentities(items,
		func(item ScenarioResultItem) string { return item.UserID },
		func(item *ScenarioResultItem, info userInfo) {
			item.UserName, item.UserEmail = info.Name, info.Email
		})
}

// TeacherDashboardService provides teacher-facing queries for group activity and scenario results
type TeacherDashboardService struct {
	db              *gorm.DB
	sessionRepo     repositories.ScenarioSessionRepository
	terminalService ttServices.TerminalTrainerService
	sessionService  *ScenarioSessionService
}

// NewTeacherDashboardService creates a new teacher dashboard service
func NewTeacherDashboardService(db *gorm.DB, terminalService ttServices.TerminalTrainerService, sessionService *ScenarioSessionService) *TeacherDashboardService {
	// Bulk-start provisions containers through this session service, so it needs
	// the terminal callbacks — most of all the one that closes the provisioning
	// window. Wiring them here, where both halves are already in hand, is what
	// stops a caller forgetting them: see WireTerminalCallbacks.
	WireTerminalCallbacks(sessionService, terminalService)
	return &TeacherDashboardService{
		db:              db,
		sessionRepo:     repositories.NewScenarioSessionRepository(db),
		terminalService: terminalService,
		sessionService:  sessionService,
	}
}

// GetGroupActivity returns active sessions for all members of a group (single JOIN query, no N+1)
func (s *TeacherDashboardService) GetGroupActivity(groupID uuid.UUID) ([]GroupActivityItem, error) {
	var results []GroupActivityItem
	err := s.db.Raw(`
		SELECT ss.id as session_id, ss.user_id, ss.current_step, ss.status, ss.started_at, ss.terminal_session_id,
		       sc.title as scenario_title, sc.id as scenario_id,
		       (SELECT COUNT(*) FROM scenario_steps WHERE scenario_id = sc.id AND deleted_at IS NULL) as total_steps
		FROM scenario_sessions ss
		JOIN scenarios sc ON sc.id = ss.scenario_id
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id = ? AND gm.is_active = true
		WHERE ss.status IN ('active', 'provisioning') AND ss.is_preview = false
		ORDER BY ss.started_at DESC
	`, groupID).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []GroupActivityItem{}
	}
	enrichActivityUsers(results)
	return results, nil
}

// GetGroupAssignmentsProgress returns one progress summary per scenario that has
// non-preview sessions from active group members. It is the single-group view of
// assignmentsProgressByGroup (teacherGroupsService.go), which owns the join and
// filters — the same ones getScenarioResults uses (SSOT): active membership on
// user_id, scoped to the group, excluding preview sessions. avg_grade averages
// only completed sessions — an in-progress session has no meaningful grade yet —
// so it stays NULL (→ nil *float64) until at least one member completes.
func (s *TeacherDashboardService) GetGroupAssignmentsProgress(groupID uuid.UUID) ([]AssignmentProgressItem, error) {
	byGroup, err := s.assignmentsProgressByGroup([]uuid.UUID{groupID})
	if err != nil {
		return nil, err
	}
	rows := byGroup[groupID]
	items := make([]AssignmentProgressItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, AssignmentProgressItem{
			ScenarioID:     row.ScenarioID,
			TotalCount:     row.StartedCount,
			CompletedCount: row.CompletedCount,
			AvgGrade:       row.AvgGrade,
		})
	}
	return items, nil
}

// GetScenarioResults returns sessions for a specific scenario within a group with optional pagination.
// When limit is nil or zero, all results are returned (backward compatible).
// Always returns total count for frontend pagination controls.
func (s *TeacherDashboardService) GetScenarioResults(groupID, scenarioID uuid.UUID, limit, offset *int) (*PaginatedScenarioResults, error) {
	return s.getScenarioResults(groupID, scenarioID, limit, offset, true)
}

// getScenarioResults is the internal implementation backing GetScenarioResults.
// The enrichCorrectCounts flag lets analytics callers skip the per-session
// correct-count enrichment (which adds DB queries per row) when those fields
// are not needed.
func (s *TeacherDashboardService) getScenarioResults(groupID, scenarioID uuid.UUID, limit, offset *int, enrichCorrectCounts bool) (*PaginatedScenarioResults, error) {
	// Count total matching rows first
	var total int64
	countErr := s.db.Raw(`
		SELECT COUNT(*)
		FROM scenario_sessions ss
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id = ? AND gm.is_active = true
		WHERE ss.scenario_id = ? AND ss.is_preview = false
	`, groupID, scenarioID).Scan(&total).Error
	if countErr != nil {
		return nil, countErr
	}

	// Build paginated query
	query := `
		SELECT ss.id as session_id, ss.user_id, ss.status, ss.grade, ss.started_at, ss.completed_at, ss.current_step,
		       (SELECT COUNT(*) FROM scenario_steps WHERE scenario_id = ss.scenario_id AND deleted_at IS NULL) as total_steps,
		       (SELECT COUNT(*) FROM scenario_step_progress WHERE session_id = ss.id AND status = 'completed') as completed_steps,
		       ` + sessionHintsUsedExpr + ` as total_hints_used
		FROM scenario_sessions ss
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id = ? AND gm.is_active = true
		WHERE ss.scenario_id = ? AND ss.is_preview = false
		ORDER BY ss.started_at DESC
	`
	args := []any{groupID, scenarioID}

	effectiveLimit := 0
	effectiveOffset := 0

	if limit != nil && *limit > 0 {
		effectiveLimit = *limit
		query += " LIMIT ?"
		args = append(args, *limit)
		if offset != nil && *offset > 0 {
			effectiveOffset = *offset
			query += " OFFSET ?"
			args = append(args, *offset)
		}
	}

	var results []ScenarioResultItem
	err := s.db.Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []ScenarioResultItem{}
	}
	enrichResultUsers(results)

	// Calculate partial grade for active sessions using the weighted formula
	// (terminal/flag/info → 1.0 if completed else 0; quiz → QuizScore).
	// NOTE: this is N+1 over the page (one extra query per row to load steps
	// and progress). Pages are bounded (limit ≤ 50 in practice), so this is
	// acceptable. If profiling ever shows it as a hotspot, batch-load steps
	// and progress for the whole page and inline ComputeWeightedGradeFromLoaded.
	for i := range results {
		if results[i].Grade == nil && results[i].TotalSteps > 0 {
			partialGrade, gerr := ComputeWeightedGrade(s.db, results[i].SessionID)
			if gerr == nil {
				results[i].Grade = &partialGrade
			}
			// Grade-calc failure must not prevent count enrichment below.
		}

		if enrichCorrectCounts {
			// Populate absolute correct counts (numerator/denominator) so the
			// teacher view can render "X/Y correct" alongside the percentage.
			// Same N+1 shape as the grade enrichment above; same justification.
			// Analytics callers pass false to skip these extra queries since
			// the aggregate computation does not read these fields.
			correctCount, totalCorrectPossible := computeSessionCorrectCounts(s.db, scenarioID, results[i].SessionID)
			results[i].CorrectCount = correctCount
			results[i].TotalCorrectPossible = totalCorrectPossible
		}
	}

	return &PaginatedScenarioResults{
		Items:  results,
		Total:  total,
		Limit:  effectiveLimit,
		Offset: effectiveOffset,
	}, nil
}

// GetScenarioAnalytics computes aggregate statistics for a scenario within a group.
// Calculations are done in Go to avoid SQLite vs PostgreSQL syntax differences.
//
// It deliberately does NOT reuse getScenarioResults: that loader serves the
// paginated results TABLE (per-row step/hint subqueries, Casdoor identity
// enrichment, partial-grade computation for active sessions) and the
// aggregates below use none of it. Reusing it made this endpoint load every
// session unbounded, run one grade query per active session and one Casdoor
// HTTP call per cold user — seconds per scenario on a real class, times one
// call per scenario on the analytics page.
func (s *TeacherDashboardService) GetScenarioAnalytics(groupID, scenarioID uuid.UUID) (*ScenarioAnalytics, error) {
	rows, err := s.sessionRepo.GetSessionAggregatesForGroupScenario(groupID, scenarioID)
	if err != nil {
		return nil, err
	}

	analytics := &ScenarioAnalytics{}
	analytics.TotalSessions = int64(len(rows))

	var gradeSum float64
	var gradeCount int64
	var timeSum float64
	var timeCount int64

	for _, r := range rows {
		if r.Status == "completed" {
			analytics.CompletedCount++
			if r.Grade != nil {
				gradeSum += *r.Grade
				gradeCount++
			}
			if r.CompletedAt != nil {
				duration := r.CompletedAt.Sub(r.StartedAt).Seconds()
				timeSum += duration
				timeCount++
			}
		}
	}

	if analytics.TotalSessions > 0 {
		analytics.CompletionRate = float64(analytics.CompletedCount) / float64(analytics.TotalSessions) * 100.0
	}

	if gradeCount > 0 {
		avgGrade := gradeSum / float64(gradeCount)
		analytics.AvgGrade = &avgGrade
	}

	if timeCount > 0 {
		avgTime := timeSum / float64(timeCount)
		analytics.AvgCompletionTimeSecs = &avgTime
	}

	return analytics, nil
}

// BulkStartResult represents the result of a bulk start operation
type BulkStartResult struct {
	Created    int              `json:"created"`
	Replaced   int              `json:"replaced"`
	Skipped    int              `json:"skipped"`
	NoKey      int              `json:"no_key"`
	NoKeyUsers []UserKeyMissing `json:"no_key_users,omitempty"`
	Errors     []BulkStartError `json:"errors,omitempty"`
}

// UserKeyMissing identifies a user who doesn't have a terminal key
type UserKeyMissing struct {
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

// BulkStartError represents an error for a specific user during bulk start
type BulkStartError struct {
	UserID string `json:"user_id"`
	Error  string `json:"error"`
}

// CalculateGrade computes the weighted grade for a session as a percentage in
// [0, 100]. terminal/flag/info steps contribute 1.0 if completed; quiz steps
// contribute their persisted QuizScore (0 if nil). Returns 0 on any DB error
// (matches the previous missing-session behaviour).
func (s *TeacherDashboardService) CalculateGrade(sessionID uuid.UUID) float64 {
	grade, err := ComputeWeightedGrade(s.db, sessionID)
	if err != nil {
		return 0
	}
	return grade
}

// BulkStartScenario creates terminal sessions and scenario sessions for the
// LEARNERS of a group. Auto-provisions terminal keys for those who lack one.
//
// provisioning says how each container must be built; it is resolved once by the
// caller from the scenario's own declaration, so a class launch and a single
// learner's launch build the same machine. A provisioning that creates no
// terminal records scenario sessions only.
//
// sessionDurationMinutes controls terminal session lifetime (default: 240 = 4 hours).
// trainerID identifies the trainer who initiated the bulk start (for Qualiopi traceability).
func (s *TeacherDashboardService) BulkStartScenario(groupID uuid.UUID, scenarioID uuid.UUID, provisioning ScenarioProvisioning, sessionDurationMinutes int, trainerID string) (*BulkStartResult, error) {
	// An archived class has no future to start: the same rule the hooks apply
	// to new members and assignments (groupHooks.GroupArchivedHook).
	var group groupModels.ClassGroup
	if err := s.db.Select("archived_at").First(&group, "id = ?", groupID).Error; err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}
	if group.IsArchived() {
		return nil, groupModels.ErrClassArchived
	}

	// Learners only. Starting a scenario for a class means starting it for the
	// people being taught, not for the colleague who co-manages the class and
	// not for its owner — who is typically the trainer clicking the button, and
	// who would get a container they never asked for. groupModels.LearnerRoles
	// is the single definition of who counts as an apprenant; the class card's
	// own learner_count is computed from it, which is why a class of three
	// showed three learners and produced five sessions.
	var members []groupModels.GroupMember
	if err := s.db.
		Where("group_id = ? AND is_active = ?", groupID, true).
		Scopes(groupModels.LearnerRoleScope("group_members")).
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to load group members: %w", err)
	}

	// Verify scenario exists and has steps
	var scenario models.Scenario
	if err := s.db.Preload("Steps").First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	// A retired scenario cannot be started for a whole class either. Sessions
	// already running are untouched — see models.NotArchived.
	if scenario.IsArchived() {
		return nil, models.ErrScenarioArchived
	}

	result := &BulkStartResult{
		NoKeyUsers: make([]UserKeyMissing, 0),
		Errors:     make([]BulkStartError, 0),
	}

	// Default session duration: 4 hours
	if sessionDurationMinutes <= 0 {
		sessionDurationMinutes = 240
	}
	sessionExpirySecs := sessionDurationMinutes * 60

	// Fetch terms from tt-backend for session creation
	var terms string
	if provisioning.CreatesTerminal() {
		var termsErr error
		terms, termsErr = s.terminalService.GetTerms()
		if termsErr != nil {
			return nil, fmt.Errorf("failed to fetch terminal terms: %w", termsErr)
		}
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	run := bulkStartRun{
		scenarioID:        scenarioID,
		scenario:          scenario,
		provisioning:      provisioning,
		terms:             terms,
		sessionExpirySecs: sessionExpirySecs,
		trainerID:         trainerID,
	}

	var mu sync.Mutex
	for _, member := range members {
		member := member // capture range variable
		g.Go(func() error {
			outcome := s.startScenarioForMember(run, member.UserID)
			mu.Lock()
			result.record(outcome)
			mu.Unlock()
			return nil
		})
	}
	g.Wait()

	utils.Debug("BulkStartScenario - Complete: created=%d skipped=%d noKey=%d errors=%d",
		result.Created, result.Skipped, result.NoKey, len(result.Errors))

	return result, nil
}

// bulkStartRun is what every learner in one bulk start shares: the scenario,
// how its container must be built, and the terms and lifetime that go with it.
// Passed whole so starting one member reads as one argument plus who it is for.
type bulkStartRun struct {
	scenarioID        uuid.UUID
	scenario          models.Scenario
	provisioning      ScenarioProvisioning
	terms             string
	sessionExpirySecs int
	trainerID         string
}

// memberOutcome is what starting one learner produced. Exactly one of its
// fields is meaningful per member, which is what lets the caller hold the lock
// once and in one place, instead of around six scattered appends.
type memberOutcome struct {
	replaced bool
	created  bool
	noKey    *UserKeyMissing
	failure  string // non-empty: the error to report for this learner
	userID   string
}

// record folds one learner's outcome into the tally. Caller holds the lock.
func (r *BulkStartResult) record(o memberOutcome) {
	if o.replaced {
		r.Replaced++
	}
	switch {
	case o.noKey != nil:
		r.NoKey++
		r.NoKeyUsers = append(r.NoKeyUsers, *o.noKey)
	case o.failure != "":
		r.Errors = append(r.Errors, BulkStartError{UserID: o.userID, Error: o.failure})
	case o.created:
		r.Created++
	}
}

// startScenarioForMember gives one learner their run: it clears any session
// they still hold on this scenario, makes sure they have a terminal key, builds
// the container, and starts the scenario on it.
//
// Returns what happened rather than recording it, so the concurrency lives at
// the one call site and this reads top to bottom.
func (s *TeacherDashboardService) startScenarioForMember(run bulkStartRun, userID string) memberOutcome {
	out := memberOutcome{userID: userID}

	replaced, err := s.abandonExistingRun(run.scenarioID, userID)
	if err != nil {
		out.failure = "failed to abandon existing session"
		return out
	}
	out.replaced = replaced

	if missing := s.ensureTerminalKey(userID); missing != nil {
		out.noKey = missing
		return out
	}

	terminalSessionID, failure := s.createRunContainer(run, userID)
	if failure != "" {
		out.failure = failure
		return out
	}

	// A bulk start has no per-learner language yet; each session gets the
	// scenario's own content until the launcher can offer a choice.
	session, startErr := s.sessionService.StartScenario(userID, run.scenarioID, terminalSessionID, "")
	if startErr != nil {
		utils.Warn("BulkStartScenario - Failed to start scenario for user %s: %v", userID, startErr)
		out.failure = "failed to start scenario"
		return out
	}

	// Trainer id for Qualiopi traceability. A failure here loses an audit
	// attribution, not the learner's run, so it is logged and not reported.
	if run.trainerID != "" {
		if updateErr := s.db.Model(session).Update("trainer_id", run.trainerID).Error; updateErr != nil {
			utils.Warn("BulkStartScenario - Failed to set trainer_id for session %s: %v", session.ID, updateErr)
		}
	}

	out.created = true
	return out
}

// abandonExistingRun clears whatever run the learner still holds on this
// scenario, so the class can be restarted without each learner cleaning up
// first. Reports whether one was actually replaced.
//
// The lookup and the abandon are both the session service's: the same
// definition of "open" the launch path resumes against, and the same abandon
// the learner's own button performs.
func (s *TeacherDashboardService) abandonExistingRun(scenarioID uuid.UUID, userID string) (bool, error) {
	var existing models.ScenarioSession
	err := s.db.Where("user_id = ? AND scenario_id = ? AND status IN ?",
		userID, scenarioID, models.OpenSessionStatuses).First(&existing).Error
	if err != nil {
		return false, nil // nothing to replace
	}
	if abandonErr := s.sessionService.AbandonSession(existing.ID); abandonErr != nil {
		utils.Warn("BulkStartScenario - Failed to abandon existing session %s for user %s: %v", existing.ID, userID, abandonErr)
		return false, abandonErr
	}
	return true, nil
}

// ensureTerminalKey gives the learner a terminal key if they have none.
// Returns who to report when one cannot be made, nil when they are ready.
func (s *TeacherDashboardService) ensureTerminalKey(userID string) *UserKeyMissing {
	if _, err := s.terminalService.GetUserKey(userID); err == nil {
		return nil
	}
	user, userErr := casdoorsdk.GetUserByUserId(userID)
	keyName := "auto-" + userID
	if userErr == nil && user != nil && user.Email != "" {
		keyName = "auto-" + user.Email
	}
	if createErr := s.terminalService.CreateUserKey(userID, keyName); createErr == nil {
		return nil
	}
	missing := &UserKeyMissing{UserID: userID}
	if userErr == nil && user != nil {
		missing.UserName = user.DisplayName
		if missing.UserName == "" {
			missing.UserName = user.Name
		}
		missing.UserEmail = user.Email
	}
	return missing
}

// createRunContainer builds the learner's machine, or returns "" when the
// scenario needs none. The second value is non-empty when it could not be
// built, and is the error to report for this learner.
func (s *TeacherDashboardService) createRunContainer(run bulkStartRun, userID string) (string, string) {
	if !run.provisioning.CreatesTerminal() {
		utils.Debug("BulkStartScenario - No distribution resolved, skipping terminal creation for member %s", userID)
		return "", ""
	}

	utils.Debug("BulkStartScenario - Creating terminal for member %s (distribution=%s size=%s)",
		userID, run.provisioning.Distribution, run.provisioning.Size)

	// Resolve the member's effective subscription plan, scoping it to the
	// scenario's organization when present so the plan matches the org context
	// the bulk-start is running in (#334 — gate/display SSOT).
	// scenario.OrganizationID is nil for platform-wide scenarios, which
	// correctly falls back to the member's globally highest-priority plan.
	//
	// Resolved before the composition because the persistence mode depends on
	// it: an empty mode resolves to ephemeral, and this path used to leave it
	// empty unless crash traps forced it — so a class started from the button
	// got containers it could not pause while the same learner, launching
	// alone, got persistent ones from the same plan.
	planResult, planErr := paymentServices.NewEffectivePlanService(s.db).
		GetUserEffectivePlan(userID, run.scenario.OrganizationID)
	if planErr != nil {
		utils.Warn("BulkStartScenario - Failed to resolve plan for user %s: %v", userID, planErr)
		return "", "failed to resolve subscription plan"
	}

	orgID := ""
	if run.scenario.OrganizationID != nil {
		orgID = run.scenario.OrganizationID.String()
	}
	composedInput := ttDto.CreateComposedSessionInput{
		Distribution:   run.provisioning.Distribution,
		Size:           run.provisioning.Size,
		Features:       run.provisioning.Features,
		BuildFeatures:  run.provisioning.BuildFeatures,
		Terms:          run.terms,
		Backend:        run.provisioning.Backend,
		Name:           fmt.Sprintf("scenario-%s", run.scenario.Title),
		Expiry:         run.sessionExpirySecs,
		Hostname:       run.scenario.Hostname,
		OrganizationID: orgID,
		// RecordingEnabled: recording is always on (RGPD Art. 6.1.f — legitimate interest)
		RecordingEnabled: 1,
		// Persistence: SSOT lives in ResolveScenarioPersistenceMode, shared
		// with the single-learner launch (crash_traps → ephemeral, otherwise
		// whatever the plan allows).
		PersistenceMode: ttServices.ResolveScenarioPersistenceMode(run.scenario.CrashTraps, planResult.Plan),
	}

	terminalResp, termErr := s.terminalService.StartComposedSession(userID, composedInput, planResult.Plan)
	if termErr != nil {
		utils.Warn("BulkStartScenario - Failed to create terminal for user %s: %v", userID, termErr)
		return "", "failed to create terminal session"
	}
	utils.Debug("BulkStartScenario - Terminal created for member %s: sessionID=%s", userID, terminalResp.SessionID)
	return terminalResp.SessionID, ""
}

// ResetGroupScenarioSessions abandons every open run of this scenario held by
// the class's learners, so the class can be started again cleanly. Used to
// clear orphaned runs (e.g. created without terminal keys) and failed launches.
//
// Learners only, the mirror of BulkStartScenario: the runs a reset clears are
// the ones a start creates, and groupModels.LearnerRoles is the one definition
// of who that is.
func (s *TeacherDashboardService) ResetGroupScenarioSessions(groupID uuid.UUID, scenarioID uuid.UUID) (int64, error) {
	var memberUserIDs []string
	if err := s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", groupID, true).
		Scopes(groupModels.LearnerRoleScope("group_members")).
		Pluck("user_id", &memberUserIDs).Error; err != nil {
		return 0, fmt.Errorf("failed to load group members: %w", err)
	}

	if len(memberUserIDs) == 0 {
		return 0, nil
	}

	// Abandon every session these users could still resume on this scenario.
	// The status list is models.OpenSessionStatuses: a reset that
	// skipped one would leave the learner unable to restart — 'provisioning'
	// because the unique partial index covers it, 'setup_failed' because a
	// class whose launch died is exactly the one a trainer resets.
	result := s.db.Model(&models.ScenarioSession{}).
		Where("user_id IN ? AND scenario_id = ? AND status IN ?",
			memberUserIDs, scenarioID, models.OpenSessionStatuses).
		Updates(map[string]any{"status": "abandoned"})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to reset sessions: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// SessionStepDetail represents a single step's progress with its metadata
type SessionStepDetail struct {
	StepOrder        int        `json:"step_order"`
	StepTitle        string     `json:"step_title"`
	StepType         string     `json:"step_type"`
	Status           string     `json:"status"`
	VerifyAttempts   int        `json:"verify_attempts"`
	HintsRevealed    int        `json:"hints_revealed"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	TimeSpentSeconds int        `json:"time_spent_seconds"`
	// StartedAt is derived (no DB column): for step 0 it is session.StartedAt;
	// for subsequent steps it is the previous step's CompletedAt (nil if the
	// previous step is not yet completed). Lets the trainer see when a student
	// actually began each step.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// QuizScore is the aggregate score in [0, 1] for quiz steps after submission.
	// Nil for non-quiz steps or quizzes that have not been submitted.
	// Per-question answers are intentionally not exposed in this aggregate
	// field (privacy invariant). Per-question detail for quiz steps is
	// surfaced via the Questions field below.
	QuizScore *float64 `json:"quiz_score,omitempty"`
	// Questions is populated for quiz steps only — exposes per-question detail
	// (student's answer + correct answer + correctness flag) so trainers can
	// see HOW the student answered. This route is gated by Layer 2
	// GroupRole(manager), so only group managers / platform admins ever reach
	// this code path. Learners never see this field.
	// gorm:"-" — never scanned from a SQL row; populated post-query by
	// populateQuizQuestions().
	Questions []dto.SessionStepQuestionDetail `gorm:"-" json:"questions,omitempty"`
}

// SessionDetailResponse contains full session info with step-by-step progress
type SessionDetailResponse struct {
	SessionID         uuid.UUID          `json:"session_id"`
	UserID            string             `json:"user_id"`
	UserName          string             `json:"user_name,omitempty"`
	UserEmail         string             `json:"user_email,omitempty"`
	TrainerID         *string            `json:"trainer_id,omitempty"`
	ScenarioID        uuid.UUID          `json:"scenario_id"`
	ScenarioTitle     string             `json:"scenario_title"`
	Status            string             `json:"status"`
	Grade             *float64           `json:"grade,omitempty"`
	// CorrectCount mirrors ScenarioResultItem.CorrectCount at the session level
	// so the detail modal header can render absolute progress without a second
	// trip through the list endpoint.
	CorrectCount int64 `json:"correct_count"`
	// TotalCorrectPossible mirrors ScenarioResultItem.TotalCorrectPossible.
	TotalCorrectPossible int64               `json:"total_correct_possible"`
	StartedAt            time.Time           `json:"started_at"`
	CompletedAt          *time.Time          `json:"completed_at,omitempty"`
	TerminalSessionID    *string             `json:"terminal_session_id,omitempty"`
	Steps                []SessionStepDetail `json:"steps"`
}

// GetSessionDetail returns full session details with step-by-step progress for a specific session.
// It verifies the session's user belongs to the specified group to prevent IDOR.
func (s *TeacherDashboardService) GetSessionDetail(groupID, sessionID uuid.UUID) (*SessionDetailResponse, error) {
	var session models.ScenarioSession
	if err := s.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Verify the session's user is a member of this group
	var memberCount int64
	s.db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ? AND is_active = true", groupID, session.UserID).Count(&memberCount)
	if memberCount == 0 {
		return nil, fmt.Errorf("session does not belong to this group")
	}

	// Verify the session's scenario is assigned to this group
	var assignmentCount int64
	s.db.Table("scenario_assignments").
		Where("group_id = ? AND scenario_id = ? AND deleted_at IS NULL", groupID, session.ScenarioID).
		Count(&assignmentCount)
	if assignmentCount == 0 {
		return nil, fmt.Errorf("scenario is not assigned to this group")
	}

	var scenario models.Scenario
	if err := s.db.First(&scenario, "id = ?", session.ScenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	// Load progress and step metadata separately, then merge in Go. We deliberately
	// avoid `JOIN scenario_steps ON (scenario_id, order)` because nothing in the
	// schema enforces (scenario_id, order) as unique on scenario_steps — an editor
	// bug can leave two steps with the same order, which under a SQL JOIN turns
	// into a Cartesian explosion (N progress × M duplicate steps = N*M rows).
	// Driving from progress and picking the first matching step per order keeps
	// the result row count == progress row count regardless of editor data.
	var progress []models.ScenarioStepProgress
	if err := s.db.Where("session_id = ?", sessionID).Order("step_order ASC, created_at ASC").Find(&progress).Error; err != nil {
		return nil, fmt.Errorf("failed to load step progress: %w", err)
	}

	var stepRows []models.ScenarioStep
	if err := s.db.Where("scenario_id = ?", session.ScenarioID).Order("\"order\" ASC, id ASC").Find(&stepRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load steps: %w", err)
	}
	stepByOrder := make(map[int]models.ScenarioStep, len(stepRows))
	for _, st := range stepRows {
		// Keep the first occurrence per order (driven by ORDER BY id ASC above).
		if _, ok := stepByOrder[st.Order]; !ok {
			stepByOrder[st.Order] = st
		}
	}

	steps := buildSessionStepDetails(session, progress, stepByOrder)

	// Populate Questions for quiz steps. Single batch query for all questions
	// across all quiz steps in the session, plus a lookup of QuizAnswers JSON.
	if err := populateQuizQuestions(s.db, session.ScenarioID, sessionID, steps); err != nil {
		return nil, fmt.Errorf("failed to populate quiz questions: %w", err)
	}

	// Compute absolute correct counts so the modal header can render
	// "Correct answers: X/Y" next to the percentage.
	correctCount, totalCorrectPossible := computeSessionCorrectCounts(s.db, session.ScenarioID, session.ID)

	// Enrich with user info
	userMap := fetchUserMap([]string{session.UserID})
	info := userMap[session.UserID]

	return buildSessionDetailResponse(session, scenario, steps, correctCount, totalCorrectPossible, info), nil
}

// buildSessionStepDetails merges progress rows with their scenario_step metadata
// and derives per-step StartedAt. Extracted so both the single-session
// (GetSessionDetail) and batched (GetSessionDetails) paths produce byte-identical
// step slices.
func buildSessionStepDetails(session models.ScenarioSession, progress []models.ScenarioStepProgress, stepByOrder map[int]models.ScenarioStep) []SessionStepDetail {
	steps := make([]SessionStepDetail, 0, len(progress))
	for _, p := range progress {
		// Skip progress rows whose scenario_step has been soft-deleted —
		// matches the previous inner-JOIN semantics.
		st, ok := stepByOrder[p.StepOrder]
		if !ok {
			continue
		}
		stepType := p.StepType
		if stepType == "" {
			stepType = st.StepType
		}
		if stepType == "" {
			stepType = "terminal"
		}
		steps = append(steps, SessionStepDetail{
			StepOrder:        p.StepOrder,
			StepTitle:        st.Title,
			StepType:         stepType,
			Status:           p.Status,
			VerifyAttempts:   p.VerifyAttempts,
			HintsRevealed:    p.HintsRevealed,
			CompletedAt:      p.CompletedAt,
			TimeSpentSeconds: p.TimeSpentSeconds,
			QuizScore:        p.QuizScore,
		})
	}

	// Derive per-step StartedAt:
	//   step 0 → session.StartedAt
	//   step N>0 → previous step's CompletedAt (nil if previous incomplete)
	for i := range steps {
		if i == 0 {
			started := session.StartedAt
			steps[i].StartedAt = &started
		} else {
			steps[i].StartedAt = steps[i-1].CompletedAt
		}
	}
	return steps
}

// buildSessionDetailResponse assembles the final response object from
// already-populated parts. Used by both GetSessionDetail and GetSessionDetails
// so byte-equivalence is guaranteed by construction.
func buildSessionDetailResponse(
	session models.ScenarioSession,
	scenario models.Scenario,
	steps []SessionStepDetail,
	correctCount, totalCorrectPossible int64,
	info userInfo,
) *SessionDetailResponse {
	resp := &SessionDetailResponse{
		SessionID:            session.ID,
		UserID:               session.UserID,
		TrainerID:            session.TrainerID,
		ScenarioID:           session.ScenarioID,
		ScenarioTitle:        scenario.Title,
		Status:               session.Status,
		Grade:                session.Grade,
		CorrectCount:         correctCount,
		TotalCorrectPossible: totalCorrectPossible,
		StartedAt:            session.StartedAt,
		CompletedAt:          session.CompletedAt,
		TerminalSessionID:    session.TerminalSessionID,
		Steps:                steps,
	}
	resp.UserName = info.Name
	resp.UserEmail = info.Email
	return resp
}

// maxSessionDetailsBulkSize caps how many session details can be requested in
// one bulk call. Typical class size is 30-50; 200 covers larger amphis.
const maxSessionDetailsBulkSize = 200

// GetSessionDetails returns session details for a batch of session IDs, in the
// same order as the input. Mirrors GetSessionDetail's authorization semantics
// per session (verifies the session's user is a group member AND the scenario
// is assigned to the group). Returns an error if any single lookup fails — for
// CSV export, a partial result is worse than no result.
//
// Performance: this batched implementation issues a constant number of queries
// (5-7) regardless of the input size, versus ~6N queries for the previous
// per-session loop. At class scale (N=50) this is the difference between ~300
// round-trips and 7. Output is guaranteed byte-equivalent to a loop of
// GetSessionDetail calls (see TestGetSessionDetails_Batch_MatchesIndividualCalls).
func (s *TeacherDashboardService) GetSessionDetails(groupID uuid.UUID, sessionIDs []uuid.UUID) ([]*SessionDetailResponse, error) {
	if len(sessionIDs) > maxSessionDetailsBulkSize {
		return nil, fmt.Errorf("too many session IDs: %d (max %d)", len(sessionIDs), maxSessionDetailsBulkSize)
	}
	if len(sessionIDs) == 0 {
		return []*SessionDetailResponse{}, nil
	}

	sessions, err := s.loadSessionsForBulkDetails(sessionIDs)
	if err != nil {
		return nil, err
	}

	userIDs, scenarioIDs := distinctIDsFromSessions(sessions)

	if err := s.verifyGroupAccessForBulkDetails(groupID, userIDs, scenarioIDs); err != nil {
		return nil, err
	}

	graph, err := s.loadScenarioGraph(scenarioIDs)
	if err != nil {
		return nil, err
	}
	tracking, err := s.loadSessionTracking(sessionIDs)
	if err != nil {
		return nil, err
	}
	userMap := fetchUserMap(userIDs)

	return assembleBulkSessionDetails(sessionIDs, sessions, graph, tracking, userMap), nil
}

// distinctIDsFromSessions extracts the unique user_ids and scenario_ids from
// a batch of sessions. Output order is undefined (set semantics).
func distinctIDsFromSessions(sessions []models.ScenarioSession) (userIDs []string, scenarioIDs []uuid.UUID) {
	userSet := make(map[string]struct{}, len(sessions))
	scenarioSet := make(map[uuid.UUID]struct{}, len(sessions))
	for _, ss := range sessions {
		userSet[ss.UserID] = struct{}{}
		scenarioSet[ss.ScenarioID] = struct{}{}
	}
	userIDs = make([]string, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}
	scenarioIDs = make([]uuid.UUID, 0, len(scenarioSet))
	for id := range scenarioSet {
		scenarioIDs = append(scenarioIDs, id)
	}
	return
}

// loadSessionsForBulkDetails loads every session referenced by sessionIDs in a
// single query. If any ID is missing, it returns the same "session not found"
// error a looped GetSessionDetail would have hit at the first miss (walks the
// input order so the error is deterministic).
func (s *TeacherDashboardService) loadSessionsForBulkDetails(sessionIDs []uuid.UUID) ([]models.ScenarioSession, error) {
	var sessions []models.ScenarioSession
	if err := s.db.Where("id IN ?", sessionIDs).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	if len(sessions) != len(sessionIDs) {
		sessionByID := make(map[uuid.UUID]struct{}, len(sessions))
		for _, ss := range sessions {
			sessionByID[ss.ID] = struct{}{}
		}
		for _, id := range sessionIDs {
			if _, ok := sessionByID[id]; !ok {
				return nil, fmt.Errorf("session not found: %s", id)
			}
		}
	}
	return sessions, nil
}

// verifyGroupAccessForBulkDetails is the IDOR gate for the bulk path: every
// session's user must be an active member of groupID, AND every referenced
// scenario must be assigned to groupID. Two COUNT(DISTINCT) queries cover the
// whole batch regardless of size.
func (s *TeacherDashboardService) verifyGroupAccessForBulkDetails(groupID uuid.UUID, userIDs []string, scenarioIDs []uuid.UUID) error {
	// Every session user must be a group member.
	// COUNT(DISTINCT user_id) so duplicate active rows don't inflate the count.
	var memberCount int64
	if err := s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id IN ? AND is_active = true", groupID, userIDs).
		Distinct("user_id").Count(&memberCount).Error; err != nil {
		return fmt.Errorf("failed to verify group membership: %w", err)
	}
	if memberCount != int64(len(userIDs)) {
		return fmt.Errorf("session does not belong to this group")
	}

	// Every session scenario must be assigned to the group.
	var assignmentCount int64
	if err := s.db.Table("scenario_assignments").
		Where("group_id = ? AND scenario_id IN ? AND deleted_at IS NULL", groupID, scenarioIDs).
		Distinct("scenario_id").Count(&assignmentCount).Error; err != nil {
		return fmt.Errorf("failed to verify scenario assignments: %w", err)
	}
	if assignmentCount != int64(len(scenarioIDs)) {
		return fmt.Errorf("scenario is not assigned to this group")
	}
	return nil
}

// scenarioGraph holds all scenario-keyed data needed to render session details:
// the scenarios themselves, their steps (in both per-order map and full-list
// views), and quiz questions for any quiz steps.
type scenarioGraph struct {
	byID                  map[uuid.UUID]models.Scenario
	stepByOrder           map[uuid.UUID]map[int]models.ScenarioStep // [scenario_id][order]
	stepsByScenario       map[uuid.UUID][]models.ScenarioStep
	questionsByStepID     map[uuid.UUID][]models.ScenarioStepQuestion
	questionCountByStepID map[uuid.UUID]int
}

// sessionTracking holds all per-session-keyed data used by the assembler:
// step progress and flag submissions.
type sessionTracking struct {
	progressBySession map[uuid.UUID][]models.ScenarioStepProgress
	flagsBySession    map[uuid.UUID][]models.ScenarioFlag
}

// loadScenarioGraph runs the scenario-keyed query plan that feeds the assembler:
// scenarios, scenario steps (two views: per-order map and per-scenario list),
// and quiz questions (with per-step counts). Independent from per-session data.
func (s *TeacherDashboardService) loadScenarioGraph(scenarioIDs []uuid.UUID) (*scenarioGraph, error) {
	// Load all referenced scenarios in one query.
	var scenarios []models.Scenario
	if err := s.db.Where("id IN ?", scenarioIDs).Find(&scenarios).Error; err != nil {
		return nil, fmt.Errorf("failed to load scenarios: %w", err)
	}
	byID := make(map[uuid.UUID]models.Scenario, len(scenarios))
	for _, sc := range scenarios {
		byID[sc.ID] = sc
	}

	// Load all steps for all referenced scenarios in one query.
	// Two views of this data are needed downstream:
	//   - stepByOrder[scenario_id][order] for the step-merge pass
	//     (first-win on duplicate (scenario, order) to match GetSessionDetail)
	//   - stepsByScenario[scenario_id] = []ScenarioStep for
	//     ComputeCorrectCountsFromLoaded, which expects the raw list.
	var allSteps []models.ScenarioStep
	if err := s.db.Where("scenario_id IN ?", scenarioIDs).
		Order("\"order\" ASC, id ASC").
		Find(&allSteps).Error; err != nil {
		return nil, fmt.Errorf("failed to load steps: %w", err)
	}
	stepByOrder := make(map[uuid.UUID]map[int]models.ScenarioStep, len(scenarioIDs))
	stepsByScenario := make(map[uuid.UUID][]models.ScenarioStep, len(scenarioIDs))
	allQuizStepIDs := make([]uuid.UUID, 0)
	for _, st := range allSteps {
		stepsByScenario[st.ScenarioID] = append(stepsByScenario[st.ScenarioID], st)
		if stepByOrder[st.ScenarioID] == nil {
			stepByOrder[st.ScenarioID] = make(map[int]models.ScenarioStep)
		}
		if _, exists := stepByOrder[st.ScenarioID][st.Order]; !exists {
			stepByOrder[st.ScenarioID][st.Order] = st
		}
		if normalizeStepType(st.StepType) == "quiz" {
			allQuizStepIDs = append(allQuizStepIDs, st.ID)
		}
	}

	// Load all quiz questions for every quiz step across all scenarios in
	// one query — and a single COUNT(*) GROUP BY for the per-step question
	// counts ComputeCorrectCountsFromLoaded requires.
	questionsByStepID := make(map[uuid.UUID][]models.ScenarioStepQuestion, len(allQuizStepIDs))
	questionCountByStepID := make(map[uuid.UUID]int, len(allQuizStepIDs))
	if len(allQuizStepIDs) > 0 {
		var allQuestions []models.ScenarioStepQuestion
		if err := s.db.Where("step_id IN ?", allQuizStepIDs).
			Order("\"order\" ASC").
			Find(&allQuestions).Error; err != nil {
			return nil, fmt.Errorf("failed to load quiz questions: %w", err)
		}
		for _, q := range allQuestions {
			questionsByStepID[q.StepID] = append(questionsByStepID[q.StepID], q)
			questionCountByStepID[q.StepID]++
		}
	}

	return &scenarioGraph{
		byID:                  byID,
		stepByOrder:           stepByOrder,
		stepsByScenario:       stepsByScenario,
		questionsByStepID:     questionsByStepID,
		questionCountByStepID: questionCountByStepID,
	}, nil
}

// stepProgressBySession loads the step progress of every given session in ONE
// query, grouped by session and ordered so each session's steps read in order.
// Single home for the batched progress fetch behind the bulk session details and
// the live class view. An empty input yields an empty map without touching the DB.
func (s *TeacherDashboardService) stepProgressBySession(sessionIDs []uuid.UUID) (map[uuid.UUID][]models.ScenarioStepProgress, error) {
	progressBySession := make(map[uuid.UUID][]models.ScenarioStepProgress, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return progressBySession, nil
	}

	var allProgress []models.ScenarioStepProgress
	if err := s.db.Where("session_id IN ?", sessionIDs).
		Order("step_order ASC, created_at ASC").
		Find(&allProgress).Error; err != nil {
		return nil, fmt.Errorf("failed to load step progress: %w", err)
	}
	for _, p := range allProgress {
		progressBySession[p.SessionID] = append(progressBySession[p.SessionID], p)
	}
	return progressBySession, nil
}

// loadSessionTracking runs the per-session-keyed query plan: step progress and
// flag submissions, both grouped by session ID. Independent from scenario data.
func (s *TeacherDashboardService) loadSessionTracking(sessionIDs []uuid.UUID) (*sessionTracking, error) {
	progressBySession, err := s.stepProgressBySession(sessionIDs)
	if err != nil {
		return nil, err
	}

	// Load all flag submissions in one query, grouped by session.
	var allFlags []models.ScenarioFlag
	if err := s.db.Where("session_id IN ?", sessionIDs).Find(&allFlags).Error; err != nil {
		return nil, fmt.Errorf("failed to load flags: %w", err)
	}
	flagsBySession := make(map[uuid.UUID][]models.ScenarioFlag, len(sessionIDs))
	for _, f := range allFlags {
		flagsBySession[f.SessionID] = append(flagsBySession[f.SessionID], f)
	}

	return &sessionTracking{
		progressBySession: progressBySession,
		flagsBySession:    flagsBySession,
	}, nil
}

// assembleBulkSessionDetails walks sessionIDs in input order and stitches each
// response from the pre-loaded data. Pure Go, no DB calls — every map lookup
// hits data already loaded by the scenarioGraph / sessionTracking loaders.
func assembleBulkSessionDetails(
	sessionIDs []uuid.UUID,
	sessions []models.ScenarioSession,
	graph *scenarioGraph,
	tracking *sessionTracking,
	userMap map[string]userInfo,
) []*SessionDetailResponse {
	sessionByID := make(map[uuid.UUID]models.ScenarioSession, len(sessions))
	for _, ss := range sessions {
		sessionByID[ss.ID] = ss
	}
	details := make([]*SessionDetailResponse, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		session := sessionByID[id]
		scenario := graph.byID[session.ScenarioID]
		stepByOrder := graph.stepByOrder[session.ScenarioID]
		if stepByOrder == nil {
			stepByOrder = map[int]models.ScenarioStep{}
		}
		sessionProgress := tracking.progressBySession[session.ID]

		steps := buildSessionStepDetails(session, sessionProgress, stepByOrder)
		populateQuizQuestionsFromLoaded(session.ID, steps, stepByOrder, graph.questionsByStepID, sessionProgress)

		correctCount, totalCorrectPossible := ComputeCorrectCountsFromLoaded(
			graph.stepsByScenario[session.ScenarioID],
			sessionProgress,
			tracking.flagsBySession[session.ID],
			graph.questionCountByStepID,
		)

		details = append(details, buildSessionDetailResponse(session, scenario, steps, correctCount, totalCorrectPossible, userMap[session.UserID]))
	}
	return details
}

// populateQuizQuestionsFromLoaded is the pre-loaded-data variant of
// populateQuizQuestions. It fills the Questions slice on each quiz step using
// already-loaded scenario steps, questions, and progress rows — no DB calls.
// Behavior must match populateQuizQuestions exactly so the batched path stays
// byte-equivalent to the single-session path.
func populateQuizQuestionsFromLoaded(
	sessionID uuid.UUID,
	steps []SessionStepDetail,
	stepByOrder map[int]models.ScenarioStep,
	questionsByStepID map[uuid.UUID][]models.ScenarioStepQuestion,
	progress []models.ScenarioStepProgress,
) {
	// Index quiz answers by step_order for O(1) lookup. We mirror
	// populateQuizQuestions's "step_order IN quizStepOrders" filter implicitly
	// since we only consult this map for quiz steps below.
	answersByStepOrder := make(map[int]string, len(progress))
	for _, p := range progress {
		answersByStepOrder[p.StepOrder] = p.QuizAnswers
	}

	for i := range steps {
		if steps[i].StepType != "quiz" {
			continue
		}
		st, ok := stepByOrder[steps[i].StepOrder]
		if !ok {
			continue
		}
		questions := questionsByStepID[st.ID]
		if len(questions) == 0 {
			continue
		}

		studentAnswers := map[string]string{}
		if raw, ok := answersByStepOrder[steps[i].StepOrder]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &studentAnswers); err != nil {
				slog.Warn("malformed quiz answers JSON, degrading to empty map",
					"session_id", sessionID, "step_order", steps[i].StepOrder, "err", err)
				studentAnswers = map[string]string{}
			}
		}

		details := make([]dto.SessionStepQuestionDetail, 0, len(questions))
		for _, q := range questions {
			submitted := studentAnswers[q.ID.String()]
			isCorrect := false
			if submitted != "" {
				isCorrect = subtle.ConstantTimeCompare([]byte(submitted), []byte(q.CorrectAnswer)) == 1
			}
			details = append(details, dto.SessionStepQuestionDetail{
				ID:            q.ID,
				Order:         q.Order,
				QuestionText:  q.QuestionText,
				QuestionType:  q.QuestionType,
				Options:       q.Options,
				CorrectAnswer: q.CorrectAnswer,
				StudentAnswer: submitted,
				IsCorrect:     isCorrect,
				Points:        q.Points,
				Explanation:   q.Explanation,
			})
		}
		steps[i].Questions = details
	}
}

// GetSessionCommands proxies the terminal command history for a scenario session
// to tt-backend, using the OCF admin API key. It enforces the same group-membership
// invariant as GetSessionDetail (the session's user must belong to the group) so
// trainers cannot read commands from sessions outside their group.
//
// Returns sentinel errors (ErrSessionNotFound, ErrSessionNotInGroup,
// ErrSessionHasNoTerminal) so the controller can map them to 404 responses
// without leaking session existence.
func (s *TeacherDashboardService) GetSessionCommands(groupID, sessionID uuid.UUID, limit, offset int) ([]byte, string, error) {
	var session models.ScenarioSession
	if err := s.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, "", ErrSessionNotFound
	}

	// Verify the session's user is a member of this group (mirror GetSessionDetail).
	var memberCount int64
	s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ? AND is_active = true", groupID, session.UserID).
		Count(&memberCount)
	if memberCount == 0 {
		return nil, "", ErrSessionNotInGroup
	}

	if session.TerminalSessionID == nil || *session.TerminalSessionID == "" {
		return nil, "", ErrSessionHasNoTerminal
	}

	// Verify the session's scenario is assigned to this group. Without this
	// check, a manager of group A could read commands from a session whose
	// student happens to also be in group A but is running a scenario only
	// assigned to group B (IDOR).
	var assignmentCount int64
	s.db.Table("scenario_assignments").
		Where("group_id = ? AND scenario_id = ? AND deleted_at IS NULL", groupID, session.ScenarioID).
		Count(&assignmentCount)
	if assignmentCount == 0 {
		return nil, "", ErrScenarioNotAssignedToGroup
	}

	body, contentType, err := s.terminalService.GetSessionCommandHistoryAdmin(*session.TerminalSessionID, limit, offset)
	if err != nil {
		return nil, "", err
	}
	return body, contentType, nil
}

// computeSessionCorrectCounts loads the data required by
// ComputeCorrectCountsFromLoaded (steps, progress, flags, per-step question
// counts) for a single session and returns the absolute correct count
// (numerator) and the static total possible (denominator).
//
// Soft-deleted scenario steps and questions are excluded — the standard GORM
// soft-delete predicate (deleted_at IS NULL) is applied via the model query
// for steps and explicitly via the Where clause for questions (which are
// scanned into a small struct rather than the full model).
//
// On any DB error this returns (0, 0); callers continue rendering the row
// without the absolute count rather than failing the whole list.
func computeSessionCorrectCounts(db *gorm.DB, scenarioID, sessionID uuid.UUID) (int64, int64) {
	var steps []models.ScenarioStep
	if err := db.Where("scenario_id = ?", scenarioID).Find(&steps).Error; err != nil {
		return 0, 0
	}
	if len(steps) == 0 {
		return 0, 0
	}

	// Collect quiz step IDs, then count questions per step (excluding
	// soft-deleted questions).
	quizStepIDs := make([]uuid.UUID, 0, len(steps))
	for _, st := range steps {
		if normalizeStepType(st.StepType) == "quiz" {
			quizStepIDs = append(quizStepIDs, st.ID)
		}
	}
	questionCountByStepID := make(map[uuid.UUID]int, len(quizStepIDs))
	if len(quizStepIDs) > 0 {
		type countRow struct {
			StepID uuid.UUID
			Count  int
		}
		var rows []countRow
		if err := db.Table("scenario_step_questions").
			Select("step_id, COUNT(*) as count").
			Where("step_id IN ? AND deleted_at IS NULL", quizStepIDs).
			Group("step_id").
			Scan(&rows).Error; err == nil {
			for _, r := range rows {
				questionCountByStepID[r.StepID] = r.Count
			}
		}
	}

	var progress []models.ScenarioStepProgress
	if err := db.Where("session_id = ?", sessionID).Find(&progress).Error; err != nil {
		return 0, 0
	}

	var flags []models.ScenarioFlag
	if err := db.Where("session_id = ?", sessionID).Find(&flags).Error; err != nil {
		return 0, 0
	}

	return ComputeCorrectCountsFromLoaded(steps, progress, flags, questionCountByStepID)
}

// populateQuizQuestions fills the Questions slice on every quiz step in `steps`.
// It batches the question lookup (one query for all quiz steps in the session)
// and the QuizAnswers lookup (one query for the relevant progress rows).
// Malformed QuizAnswers JSON is logged at warn level but never propagated —
// the questions metadata is still surfaced so the trainer view doesn't break.
func populateQuizQuestions(db *gorm.DB, scenarioID, sessionID uuid.UUID, steps []SessionStepDetail) error {
	// Collect step orders for quiz steps.
	quizStepOrders := make([]int, 0)
	for i := range steps {
		if steps[i].StepType == "quiz" {
			quizStepOrders = append(quizStepOrders, steps[i].StepOrder)
		}
	}
	if len(quizStepOrders) == 0 {
		return nil
	}

	// Load the matching ScenarioStep rows so we can resolve their IDs to
	// fetch questions. Order is required to map back to step.Order.
	type stepRow struct {
		ID    uuid.UUID
		Order int
	}
	var stepRows []stepRow
	if err := db.Table("scenario_steps").
		Select("id, \"order\"").
		Where("scenario_id = ? AND \"order\" IN ? AND deleted_at IS NULL", scenarioID, quizStepOrders).
		Scan(&stepRows).Error; err != nil {
		return fmt.Errorf("failed to load quiz step IDs: %w", err)
	}
	if len(stepRows) == 0 {
		return nil
	}

	stepIDByOrder := make(map[int]uuid.UUID, len(stepRows))
	stepIDs := make([]uuid.UUID, 0, len(stepRows))
	for _, sr := range stepRows {
		stepIDByOrder[sr.Order] = sr.ID
		stepIDs = append(stepIDs, sr.ID)
	}

	// Load all questions for the involved quiz steps in one query.
	var allQuestions []models.ScenarioStepQuestion
	if err := db.Where("step_id IN ?", stepIDs).
		Order("\"order\" ASC").
		Find(&allQuestions).Error; err != nil {
		return fmt.Errorf("failed to load quiz questions: %w", err)
	}
	questionsByStepID := make(map[uuid.UUID][]models.ScenarioStepQuestion, len(stepIDs))
	for _, q := range allQuestions {
		questionsByStepID[q.StepID] = append(questionsByStepID[q.StepID], q)
	}

	// Load QuizAnswers JSON per quiz step in one query.
	type answersRow struct {
		StepOrder   int
		QuizAnswers string
	}
	var answersRows []answersRow
	if err := db.Table("scenario_step_progress").
		Select("step_order, quiz_answers").
		Where("session_id = ? AND step_order IN ?", sessionID, quizStepOrders).
		Scan(&answersRows).Error; err != nil {
		return fmt.Errorf("failed to load quiz answers: %w", err)
	}
	answersByStepOrder := make(map[int]string, len(answersRows))
	for _, ar := range answersRows {
		answersByStepOrder[ar.StepOrder] = ar.QuizAnswers
	}

	// Build per-step Questions slice.
	for i := range steps {
		if steps[i].StepType != "quiz" {
			continue
		}
		stepID, ok := stepIDByOrder[steps[i].StepOrder]
		if !ok {
			continue
		}
		questions := questionsByStepID[stepID]
		if len(questions) == 0 {
			continue
		}

		// Parse the student's submitted answers. Malformed JSON degrades to
		// an empty map (every student_answer ends up empty).
		studentAnswers := map[string]string{}
		if rawAnswers, ok := answersByStepOrder[steps[i].StepOrder]; ok && rawAnswers != "" {
			if err := json.Unmarshal([]byte(rawAnswers), &studentAnswers); err != nil {
				slog.Warn("malformed quiz answers JSON, degrading to empty map",
					"session_id", sessionID, "step_order", steps[i].StepOrder, "err", err)
				studentAnswers = map[string]string{}
			}
		}

		details := make([]dto.SessionStepQuestionDetail, 0, len(questions))
		for _, q := range questions {
			submitted := studentAnswers[q.ID.String()]
			isCorrect := false
			if submitted != "" {
				isCorrect = subtle.ConstantTimeCompare([]byte(submitted), []byte(q.CorrectAnswer)) == 1
			}
			details = append(details, dto.SessionStepQuestionDetail{
				ID:            q.ID,
				Order:         q.Order,
				QuestionText:  q.QuestionText,
				QuestionType:  q.QuestionType,
				Options:       q.Options,
				CorrectAnswer: q.CorrectAnswer,
				StudentAnswer: submitted,
				IsCorrect:     isCorrect,
				Points:        q.Points,
				Explanation:   q.Explanation,
			})
		}
		steps[i].Questions = details
	}
	return nil
}
