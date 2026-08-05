package services

// teacherLiveProgressService.go — the merged per-learner class view (ocf-front
// #310). "Sessions en direct" (the supervision wall) and "Activité" (scenario
// progress) used to answer the same teacher question from two backends joined on
// nothing. GetGroupLiveProgress answers it once, keyed per learner: presence,
// scenario position, and assignment results on a single row.
//
// The invariant the whole file exists to protect: EVERY apprenant of the class
// gets a row. The surface invigilates exams, so a learner who has done nothing
// must read as "not started" — never be silently absent from the list.
//
// Its counterpart since #480: the invigilator is not invigilated. Teaching staff
// hold class memberships too (the creator is auto-enrolled as owner), and
// groupModels.LearnerRoles decides which of those memberships are apprenants.
//
// Nothing here re-derives a predicate that already has a home. Presence uses the
// terminal scopes shared with the supervision wall, the assignment list comes
// from activeAssignmentsByGroup (teacherGroupsService.go), and hint counts use
// the same SQL expression as the per-scenario results table.

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// Learner standing on an assignment, as rendered by the class view.
const (
	LearnerStatusNotStarted = "not_started"
	LearnerStatusInProgress = "in_progress"
	LearnerStatusCompleted  = "completed"
)

// sessionStatusCompleted is the scenario_sessions.status value that ends a run.
const sessionStatusCompleted = "completed"

// LearnerIdleThreshold is how long a PRESENT learner may go without scenario
// activity before the teacher surfaces flag them as idle — the "stuck but not
// asking" detector. Exported (and echoed in the API as idle_threshold_minutes)
// so the frontend labels the badge from the value that actually produced the
// count, instead of hard-coding a second copy that can drift.
const LearnerIdleThreshold = 10 * time.Minute

// sessionHintsUsedExpr is the canonical SQL for "how many hints has this session
// revealed": the sum over its step progress rows. Shared by the per-scenario
// results table (ScenarioResultItem.TotalHintsUsed) and the live class view so
// the two can never report different hint counts for the same learner. Assumes
// the scenario_sessions row is aliased `ss`.
const sessionHintsUsedExpr = `(SELECT COALESCE(SUM(hints_revealed), 0) FROM scenario_step_progress WHERE session_id = ss.id)`

// LearnerLiveProgress is one row of the class view: who the learner is, whether
// they are present, and where they stand on each of the class's assignments.
type LearnerLiveProgress struct {
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	// Connected is presence in THIS class: a terminal session that is alive right
	// now (models.RunningDisplayScope) AND supervisable in the group's
	// organization (models.SupervisableByGroupOrgScope). A personal session, one
	// launched in another org, and a past-expiry zombie row all read false — the
	// same rule the supervision wall lists tiles by.
	Connected bool `json:"connected"`
	// TerminalSessionID is the live session the "watch / take hand" actions
	// target, most recent first when a learner has several. Nil when absent.
	TerminalSessionID *string `json:"terminal_session_id,omitempty"`
	// LastActivityAt is the learner's most recent scenario interaction across the
	// class's assignments — the step-progress row's update stamp, which every
	// verify attempt, hint reveal, quiz submission and step completion bumps.
	//
	// It is scenario activity, NOT keystrokes: ocf-core stores no per-command
	// timestamp (tt-backend owns command history), so a learner typing in the
	// terminal without touching a step does not refresh it. Nil when the learner
	// has not started any assignment.
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	// Idle is isLearnerIdle applied to this row — present, but no scenario
	// activity for LearnerIdleThreshold. Sent computed so the row and the "N
	// inactifs" badge on the classes console cannot disagree.
	Idle        bool                        `json:"idle"`
	Assignments []LearnerAssignmentProgress `json:"assignments"`
}

// LearnerAssignmentProgress is where one learner stands on one of the class's
// active assignments. An assignment the learner never opened is reported
// all-zero rather than omitted — "assigned, untouched" is the state an
// invigilator most needs to see.
type LearnerAssignmentProgress struct {
	AssignmentID  uuid.UUID  `json:"assignment_id"`
	ScenarioID    uuid.UUID  `json:"scenario_id"`
	ScenarioTitle string     `json:"scenario_title"`
	Deadline      *time.Time `json:"deadline,omitempty"`
	// SessionID is the attempt this row describes, nil when not started.
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	// Status is the three-value standing the class view renders:
	// not_started / in_progress / completed.
	Status string `json:"status"`
	// SessionStatus is the RAW scenario_sessions.status behind Status, empty when
	// not started. It exists because Status collapses every non-completed attempt
	// into in_progress: an abandoned or setup_failed run must not be relabelled
	// "never started" in an exam view, so the honest value stays available for the
	// UI to display.
	SessionStatus    string `json:"session_status,omitempty"`
	CurrentStep      int    `json:"current_step"`
	CurrentStepTitle string `json:"current_step_title,omitempty"`
	TotalSteps       int    `json:"total_steps"`
	// CurrentStepElapsedSeconds is how long the learner has been on the step they
	// are on now. Nil unless the attempt is in progress.
	CurrentStepElapsedSeconds *int       `json:"current_step_elapsed_seconds,omitempty"`
	HintsUsed                 int        `json:"hints_used"`
	Grade                     *float64   `json:"grade,omitempty"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	CompletedAt               *time.Time `json:"completed_at,omitempty"`
}

// GetGroupLiveProgress returns one row per ACTIVE APPRENANT of groupID, joining
// supervision presence, scenario position and assignment results. Staff
// memberships get no row — a teacher does not invigilate themselves (#480).
//
// Cost is a constant seven queries regardless of class size: the group, its
// learners, its assignments, live terminals, learner attempts, those attempts'
// step progress, and the assigned scenarios' steps. Identity resolution adds no
// SQL (cached Casdoor lookups).
//
// An unknown group, a group with no apprenant (a staff-only class included), and
// a group with no assignment all yield a valid (non-nil) slice rather than an
// error: the caller has already cleared Layer 2, so "nothing to show" is an
// answer, not a fault.
func (s *TeacherDashboardService) GetGroupLiveProgress(groupID uuid.UUID) ([]LearnerLiveProgress, error) {
	var group groupModels.ClassGroup
	if err := s.db.Where("id = ?", groupID).First(&group).Error; err != nil {
		// A well-formed id for a group that is gone lists nothing.
		return []LearnerLiveProgress{}, nil
	}

	var learnerIDs []string
	if err := s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", groupID, true).
		Scopes(groupModels.LearnerRoleScope("group_members")).
		Order("user_id ASC").
		Pluck("user_id", &learnerIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load class learners: %w", err)
	}
	if len(learnerIDs) == 0 {
		return []LearnerLiveProgress{}, nil
	}

	sources, err := s.loadLiveProgressSources(group, learnerIDs)
	if err != nil {
		return nil, err
	}

	rows := make([]LearnerLiveProgress, 0, len(learnerIDs))
	for _, userID := range learnerIDs {
		rows = append(rows, buildLearnerLiveProgress(userID, sources))
	}
	resolveUserIdentities(rows,
		func(row LearnerLiveProgress) string { return row.UserID },
		func(row *LearnerLiveProgress, info userInfo) {
			row.UserName, row.UserEmail = info.Name, info.Email
		})
	return rows, nil
}

// liveProgressSources bundles the batched lookups the per-learner assembler
// reads, so assembling a row stays a pure function of already-loaded data — the
// same split loadScenarioGraph / sessionTracking use for the bulk detail path.
type liveProgressSources struct {
	assignments []groupAssignmentRow
	// presenceByUser maps a present learner to their live terminal session id.
	// Absence from the map IS "not connected".
	presenceByUser         map[string]string
	sessionsByUserScenario map[learnerScenarioKey]learnerSessionRow
	stepsByScenario        map[uuid.UUID]scenarioStepIndex
	now                    time.Time
}

// learnerScenarioKey identifies one learner's standing on one scenario.
type learnerScenarioKey struct {
	userID     string
	scenarioID uuid.UUID
}

// scenarioStepIndex is a scenario's step list in the two views the class view
// needs: how many steps there are, and the title at each order.
type scenarioStepIndex struct {
	total        int
	titleByOrder map[int]string
}

// learnerSessionRow is one learner attempt at one scenario, with the aggregates
// the class view renders folded in.
type learnerSessionRow struct {
	SessionID   uuid.UUID
	UserID      string
	ScenarioID  uuid.UUID
	Status      string
	Grade       *float64
	CurrentStep int
	StartedAt   time.Time
	CompletedAt *time.Time
	HintsUsed   int
	// LastStepCompletedAt is when the learner finished their most recent step —
	// i.e. when they entered the one they are on now.
	LastStepCompletedAt *time.Time
	LastActivityAt      *time.Time
}

// loadLiveProgressSources runs the whole batched query plan behind one class
// view. Every lookup is keyed by the class's learner list or its assignments, so
// the query count does not grow with class size.
func (s *TeacherDashboardService) loadLiveProgressSources(group groupModels.ClassGroup, learnerIDs []string) (liveProgressSources, error) {
	assignmentsByGroup, err := s.activeAssignmentsByGroup([]uuid.UUID{group.ID})
	if err != nil {
		return liveProgressSources{}, fmt.Errorf("failed to load class assignments: %w", err)
	}
	assignments := assignmentsByGroup[group.ID]
	scenarioIDs := assignedScenarioIDs(assignments)

	presence, err := s.livePresenceByUser(group.OrganizationID, learnerIDs)
	if err != nil {
		return liveProgressSources{}, err
	}
	sessions, err := s.learnerAttemptsByScenario(learnerIDs, scenarioIDs)
	if err != nil {
		return liveProgressSources{}, err
	}
	steps, err := s.stepIndexByScenario(scenarioIDs)
	if err != nil {
		return liveProgressSources{}, err
	}

	return liveProgressSources{
		assignments:            assignments,
		presenceByUser:         presence,
		sessionsByUserScenario: sessions,
		stepsByScenario:        steps,
		now:                    time.Now(),
	}, nil
}

// assignedScenarioIDs lists the distinct scenarios a class's active assignments
// point at. Two assignments of the same scenario must not fan the IN () clause out.
func assignedScenarioIDs(assignments []groupAssignmentRow) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(assignments))
	ids := make([]uuid.UUID, 0, len(assignments))
	for _, assignment := range assignments {
		if !seen[assignment.ScenarioID] {
			seen[assignment.ScenarioID] = true
			ids = append(ids, assignment.ScenarioID)
		}
	}
	return ids
}

// livePresenceByUser maps each present learner to their live terminal session
// id. The caller passes an already learner-filtered id list, so this query needs
// no role predicate of its own.
//
// Both halves of "present" come from their single homes in terminalTrainer/models
// — RunningDisplayScope for "alive right now", SupervisableByGroupOrgScope for
// "visible to this class" — so a row can never disagree with the supervision wall
// about who is connected.
//
// The single-group scope form is used deliberately: the group's org is already in
// hand, so the comparison binds a parameter instead of correlating two columns,
// side-stepping the terminals.organization_id (text) vs class_groups.organization_id
// (uuid) mismatch that the joined form has to CAST around.
//
// Platform administrators get NO org bypass here, unlike the ad-hoc supervision
// listing: this row sits next to TeacherGroupSummary.LiveSessionCount, which is
// computed without a bypass, and a view where one learner shows "connected" while
// the class counter says zero is worse than a strict answer.
func (s *TeacherDashboardService) livePresenceByUser(groupOrgID *uuid.UUID, learnerIDs []string) (map[string]string, error) {
	presence := make(map[string]string, len(learnerIDs))
	if len(learnerIDs) == 0 {
		return presence, nil
	}

	var rows []struct {
		UserID    string
		SessionID string
	}
	err := s.db.Table("terminals").
		Select("terminals.user_id as user_id, terminals.session_id as session_id").
		Where("terminals.user_id IN ?", learnerIDs).
		Scopes(terminalModels.RunningDisplayScope, terminalModels.SupervisableByGroupOrgScope(groupOrgID)).
		Order("terminals.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load live sessions: %w", err)
	}
	for _, row := range rows {
		// Most recent wins: a learner with several live sessions is watched on
		// the one they most likely just opened (ORDER BY created_at DESC above).
		if _, seen := presence[row.UserID]; !seen {
			presence[row.UserID] = row.SessionID
		}
	}
	return presence, nil
}

// learnerAttemptsByScenario returns the attempt that represents each learner's
// standing on each assigned scenario, keyed by (learner, scenario) and carrying
// the timestamps the class view renders.
func (s *TeacherDashboardService) learnerAttemptsByScenario(learnerIDs []string, scenarioIDs []uuid.UUID) (map[learnerScenarioKey]learnerSessionRow, error) {
	attempts, err := s.chooseLearnerAttempts(learnerIDs, scenarioIDs)
	if err != nil {
		return nil, err
	}
	if err := s.attachProgressTimestamps(attempts); err != nil {
		return nil, err
	}
	return attempts, nil
}

// chooseLearnerAttempts loads every non-preview attempt the class's members made
// at the assigned scenarios and keeps one per (learner, scenario) — see
// supersedesAttempt for which one and why.
func (s *TeacherDashboardService) chooseLearnerAttempts(learnerIDs []string, scenarioIDs []uuid.UUID) (map[learnerScenarioKey]learnerSessionRow, error) {
	chosen := make(map[learnerScenarioKey]learnerSessionRow)
	if len(learnerIDs) == 0 || len(scenarioIDs) == 0 {
		return chosen, nil
	}

	var rows []learnerSessionRow
	err := s.db.Raw(`
		SELECT ss.id as session_id, ss.user_id, ss.scenario_id, ss.status, ss.grade,
		       ss.current_step, ss.started_at, ss.completed_at,
		       `+sessionHintsUsedExpr+` as hints_used
		FROM scenario_sessions ss
		WHERE ss.user_id IN ? AND ss.scenario_id IN ?
		  AND ss.is_preview = false AND ss.deleted_at IS NULL
		ORDER BY ss.started_at ASC
	`, learnerIDs, scenarioIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load learner sessions: %w", err)
	}

	for _, row := range rows {
		key := learnerScenarioKey{userID: row.UserID, scenarioID: row.ScenarioID}
		if current, seen := chosen[key]; !seen || supersedesAttempt(row, current) {
			chosen[key] = row
		}
	}
	return chosen, nil
}

// attachProgressTimestamps fills in, for each chosen attempt, when the learner
// entered their current step and when they last did anything.
//
// The timestamps are derived in Go rather than as SQL aggregates because SQLite
// (the unit-test dialect) loses a column's datetime affinity through MAX(),
// handing the driver a string that will not scan into *time.Time. Reading them
// off the loaded rows is dialect-portable, and only the attempts that survived
// selection are queried.
func (s *TeacherDashboardService) attachProgressTimestamps(attempts map[learnerScenarioKey]learnerSessionRow) error {
	sessionIDs := make([]uuid.UUID, 0, len(attempts))
	for _, attempt := range attempts {
		sessionIDs = append(sessionIDs, attempt.SessionID)
	}

	progressBySession, err := s.stepProgressBySession(sessionIDs)
	if err != nil {
		return err
	}
	for key, attempt := range attempts {
		attempt.LastStepCompletedAt, attempt.LastActivityAt = summarizeProgressTimestamps(progressBySession[attempt.SessionID])
		attempts[key] = attempt
	}
	return nil
}

// summarizeProgressTimestamps derives, from one session's step progress rows,
// when the learner finished their most recent step (i.e. entered the one they
// are on) and when they last did anything at all.
//
// "Anything at all" is the row's update stamp, which GORM bumps on every verify
// attempt, hint reveal, quiz submission and step transition. It is scenario
// activity, not keystrokes — see LearnerLiveProgress.LastActivityAt. The
// group-wide counterpart of this staleness rule is the HAVING clause in
// staleActivityMembersByGroup; both are driven by LearnerIdleThreshold and must
// change together.
//
// Both results are nil for a session with no progress rows at all.
func summarizeProgressTimestamps(progress []models.ScenarioStepProgress) (lastStepCompletedAt, lastActivityAt *time.Time) {
	for _, step := range progress {
		lastStepCompletedAt = laterTime(lastStepCompletedAt, step.CompletedAt)
		updatedAt := step.UpdatedAt
		lastActivityAt = laterTime(lastActivityAt, &updatedAt)
	}
	return lastStepCompletedAt, lastActivityAt
}

// supersedesAttempt decides which of two attempts at the same scenario better
// represents a learner's standing: a completed run always wins (the class view
// reports achievement, and a retry after passing must not hide the pass), then a
// live one over an abandoned one, and finally the more recent start.
func supersedesAttempt(candidate, current learnerSessionRow) bool {
	if candidateRank, currentRank := attemptRank(candidate.Status), attemptRank(current.Status); candidateRank != currentRank {
		return candidateRank > currentRank
	}
	return candidate.StartedAt.After(current.StartedAt)
}

// attemptRank orders scenario_sessions.status values by how much standing they
// confer. Only the relative order matters, never the numbers themselves.
func attemptRank(status string) int {
	switch status {
	case sessionStatusCompleted:
		return 3
	case "active", "provisioning":
		return 2
	default: // abandoned, setup_failed, anything a later migration adds
		return 1
	}
}

// stepIndexByScenario loads the steps of the assigned scenarios and indexes them
// by order, so a learner's current_step can be named without a per-row query.
//
// It deliberately does NOT reuse loadScenarioGraph, which answers a superset of
// this question: that loader also fetches the scenarios and every quiz question
// of every step, and this endpoint is polled live by an open class view.
//
// The duplicate-order rule is the one loadScenarioGraph and GetSessionDetail
// already follow — nothing enforces (scenario_id, order) as unique, so the first
// row wins rather than a JOIN multiplying the result set.
func (s *TeacherDashboardService) stepIndexByScenario(scenarioIDs []uuid.UUID) (map[uuid.UUID]scenarioStepIndex, error) {
	index := make(map[uuid.UUID]scenarioStepIndex, len(scenarioIDs))
	if len(scenarioIDs) == 0 {
		return index, nil
	}

	var steps []models.ScenarioStep
	if err := s.db.Select("id", "scenario_id", `"order"`, "title").
		Where("scenario_id IN ?", scenarioIDs).
		Order(`"order" ASC, id ASC`).
		Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("failed to load scenario steps: %w", err)
	}

	for _, step := range steps {
		entry, ok := index[step.ScenarioID]
		if !ok {
			entry = scenarioStepIndex{titleByOrder: make(map[int]string)}
		}
		entry.total++
		if _, duplicate := entry.titleByOrder[step.Order]; !duplicate {
			entry.titleByOrder[step.Order] = step.Title
		}
		index[step.ScenarioID] = entry
	}
	return index, nil
}

// buildLearnerLiveProgress assembles one class-view row. Pure: no DB access, so
// the query plan above stays readable in one place.
func buildLearnerLiveProgress(userID string, src liveProgressSources) LearnerLiveProgress {
	terminalSessionID, connected := src.presenceByUser[userID]
	row := LearnerLiveProgress{
		UserID:      userID,
		Connected:   connected,
		Assignments: make([]LearnerAssignmentProgress, 0, len(src.assignments)),
	}
	if connected {
		row.TerminalSessionID = &terminalSessionID
	}

	for _, assignment := range src.assignments {
		var attempt *learnerSessionRow
		if found, started := src.sessionsByUserScenario[learnerScenarioKey{userID: userID, scenarioID: assignment.ScenarioID}]; started {
			attempt = &found
			row.LastActivityAt = laterTime(row.LastActivityAt, found.LastActivityAt)
		}
		row.Assignments = append(row.Assignments,
			buildLearnerAssignmentProgress(assignment, attempt, src.stepsByScenario[assignment.ScenarioID], src.now))
	}

	row.Idle = isLearnerIdle(row.Connected, row.LastActivityAt, src.now)
	return row
}

// buildLearnerAssignmentProgress states where one learner stands on one
// assignment. A nil attempt means they never opened it — a reported state
// (not_started), not a reason to omit the entry.
func buildLearnerAssignmentProgress(assignment groupAssignmentRow, attempt *learnerSessionRow, steps scenarioStepIndex, now time.Time) LearnerAssignmentProgress {
	progress := LearnerAssignmentProgress{
		AssignmentID:  assignment.AssignmentID,
		ScenarioID:    assignment.ScenarioID,
		ScenarioTitle: assignment.ScenarioTitle,
		Deadline:      assignment.Deadline,
		TotalSteps:    steps.total,
		Status:        LearnerStatusNotStarted,
	}
	if attempt == nil {
		return progress
	}

	progress.SessionID = &attempt.SessionID
	progress.SessionStatus = attempt.Status
	progress.Status = learnerStatusFor(attempt.Status)
	progress.CurrentStep = attempt.CurrentStep
	progress.CurrentStepTitle = steps.titleByOrder[attempt.CurrentStep]
	progress.HintsUsed = attempt.HintsUsed
	progress.Grade = attempt.Grade
	progress.StartedAt = &attempt.StartedAt
	progress.CompletedAt = attempt.CompletedAt
	if progress.Status == LearnerStatusInProgress {
		progress.CurrentStepElapsedSeconds = elapsedOnCurrentStep(*attempt, now)
	}
	return progress
}

// learnerStatusFor maps a raw scenario_sessions.status onto the three-value
// standing the class view renders. Everything that exists and is not completed
// counts as engagement — an abandoned run included, since telling an invigilator
// a student "never started" when they started and gave up is the worse error.
// LearnerAssignmentProgress.SessionStatus keeps the raw value available.
func learnerStatusFor(sessionStatus string) string {
	if sessionStatus == sessionStatusCompleted {
		return LearnerStatusCompleted
	}
	return LearnerStatusInProgress
}

// elapsedOnCurrentStep is how long the learner has been sitting on the step they
// are on now: time since they finished the previous step, falling back to the
// session start for the first one. It mirrors how SessionStepDetail.StartedAt
// derives a step's start — no column records it, because a step begins exactly
// when its predecessor ends.
func elapsedOnCurrentStep(session learnerSessionRow, now time.Time) *int {
	enteredAt := session.StartedAt
	if session.LastStepCompletedAt != nil && session.LastStepCompletedAt.After(enteredAt) {
		enteredAt = *session.LastStepCompletedAt
	}
	elapsed := int(now.Sub(enteredAt).Seconds())
	if elapsed < 0 {
		// Rows written under a different clock must not render as negative time.
		elapsed = 0
	}
	return &elapsed
}

// isLearnerIdle is the SINGLE definition of the "stuck but not asking" signal,
// shared by the per-learner class view and the "N inactifs" badge on the classes
// console: PRESENT, but no scenario activity for LearnerIdleThreshold.
//
// Presence is required on purpose. A learner with no live session is ABSENT,
// which the class view already reports as connected=false; counting them idle as
// well would show one student twice under two labels that mean different things.
// A present learner with no activity at all is likewise not idle — "not started"
// is its own reported state.
func isLearnerIdle(connected bool, lastActivityAt *time.Time, now time.Time) bool {
	if !connected || lastActivityAt == nil {
		return false
	}
	return now.Sub(*lastActivityAt) > LearnerIdleThreshold
}

// laterTime returns whichever of two optional instants is more recent.
func laterTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.After(*current) {
		return candidate
	}
	return current
}

// idleMemberCountsByGroup counts, per group, the APPRENANTS isLearnerIdle flags:
// present in the class, but with stale scenario activity. It is the cross-class
// aggregate of the same predicate the per-learner view applies, evaluated in Go
// against two batched lookups rather than re-spelled as SQL — one definition of
// "idle", two surfaces.
//
// It counts LEARNERS, one per learner regardless of how many terminals they
// hold, and feeds TeacherGroupSummary.IdleMemberCount. Both halves restrict to
// groupModels.LearnerRoles, so an enrolled teacher who left a terminal open
// never lands in the "N inactifs" badge.
func (s *TeacherDashboardService) idleMemberCountsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	counts := make(map[uuid.UUID]int, len(groupIDs))
	if len(groupIDs) == 0 {
		return counts, nil
	}

	presentByGroup, err := s.livePresentMembersByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	staleByGroup, err := s.staleActivityMembersByGroup(groupIDs, time.Now().Add(-LearnerIdleThreshold))
	if err != nil {
		return nil, err
	}

	// Present AND stale — the same conjunction isLearnerIdle applies to a single
	// row, with the staleness half answered in SQL because this surface counts
	// across every class the caller teaches.
	for groupID, presentUsers := range presentByGroup {
		stale := staleByGroup[groupID]
		for userID := range presentUsers {
			if stale[userID] {
				counts[groupID]++
			}
		}
	}
	return counts, nil
}

// livePresentMembersByGroup lists, per group, the active APPRENANTS with a live
// supervisable terminal. It is the multi-group form of livePresenceByUser and
// uses the joined org scope, which CASTs both sides because the live schema has
// terminals.organization_id as text and class_groups.organization_id as uuid.
//
// Unlike livePresenceByUser it joins group_members itself rather than receiving
// a filtered id list, so it applies groupModels.LearnerRoleScope directly.
func (s *TeacherDashboardService) livePresentMembersByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]map[string]bool, error) {
	present := make(map[uuid.UUID]map[string]bool, len(groupIDs))
	if len(groupIDs) == 0 {
		return present, nil
	}

	var rows []struct {
		GroupID uuid.UUID
		UserID  string
	}
	err := s.db.Table("terminals").
		Select("gm.group_id as group_id, gm.user_id as user_id").
		Joins("JOIN group_members gm ON gm.user_id = terminals.user_id AND gm.is_active = ? AND gm.deleted_at IS NULL", true).
		Joins("JOIN class_groups cg ON cg.id = gm.group_id AND cg.deleted_at IS NULL").
		Where("gm.group_id IN ?", groupIDs).
		Scopes(
			terminalModels.RunningDisplayScope,
			terminalModels.SupervisableByJoinedGroupOrgScope("cg"),
			groupModels.LearnerRoleScope("gm"),
		).
		Group("gm.group_id, gm.user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load present learners: %w", err)
	}
	for _, row := range rows {
		if present[row.GroupID] == nil {
			present[row.GroupID] = make(map[string]bool)
		}
		present[row.GroupID][row.UserID] = true
	}
	return present, nil
}

// staleActivityMembersByGroup lists, per group, the APPRENANTS whose last
// scenario interaction on that group's ACTIVE assignments predates `before`.
//
// It answers the staleness half of "idle" for a caller's whole class list. The
// per-learner counterpart is summarizeProgressTimestamps feeding isLearnerIdle;
// the comparison is spelled twice — once as this HAVING clause, once in Go —
// because one surface counts across many classes while the other reports a
// timestamp for a single row. Both are driven by LearnerIdleThreshold, and both
// read the same population (non-preview sessions of learner-role members on the
// group's active assignments), so they must change together.
//
// The role list is bound from groupModels.LearnerRoles rather than spelled out,
// since raw SQL cannot take its LearnerRoleScope. Restricting here is belt and
// braces — idleMemberCountsByGroup only ever looks these users up among the
// already learner-filtered present ones — but it keeps the returned set honest
// for anyone who reads it on its own.
//
// Only the group/user pair is selected: SQLite loses a column's datetime
// affinity through MAX(), so the aggregate stays inside HAVING where it is
// compared rather than returned.
func (s *TeacherDashboardService) staleActivityMembersByGroup(groupIDs []uuid.UUID, before time.Time) (map[uuid.UUID]map[string]bool, error) {
	stale := make(map[uuid.UUID]map[string]bool, len(groupIDs))
	if len(groupIDs) == 0 {
		return stale, nil
	}

	var rows []struct {
		GroupID uuid.UUID
		UserID  string
	}
	err := s.db.Raw(`
		SELECT gm.group_id as group_id, ss.user_id as user_id
		FROM scenario_step_progress sp
		JOIN scenario_sessions ss ON ss.id = sp.session_id AND ss.is_preview = false AND ss.deleted_at IS NULL
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id IN ? AND gm.is_active = true AND gm.deleted_at IS NULL
		     AND gm.role IN ?
		JOIN scenario_assignments sa ON sa.scenario_id = ss.scenario_id AND sa.group_id = gm.group_id
		     AND sa.is_active = true AND sa.deleted_at IS NULL
		WHERE sp.deleted_at IS NULL
		GROUP BY gm.group_id, ss.user_id
		HAVING MAX(sp.updated_at) < ?
	`, groupIDs, groupModels.LearnerRoles, before).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load scenario activity: %w", err)
	}
	for _, row := range rows {
		if stale[row.GroupID] == nil {
			stale[row.GroupID] = make(map[string]bool)
		}
		stale[row.GroupID][row.UserID] = true
	}
	return stale, nil
}
