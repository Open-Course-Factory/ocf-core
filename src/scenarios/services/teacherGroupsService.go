package services

// teacherGroupsService.go — the cross-group half of the teacher dashboard
// (issue #465). Every other teacher query answers "what is happening in THIS
// class"; GetManagedGroupsOverview answers "what is happening across ALL my
// classes" in a single call, so the "Mes classes" console does not have to fan
// out one request per group.

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// TeacherGroupSummary is one dashboard ROW: everything the "Mes classes" list
// shows for a class the caller owns or manages, without the member roster (the
// per-group endpoints serve that once a class is opened).
//
// Archived (IsActive=false) and expired (IsExpired=true) classes are listed and
// flagged rather than hidden: a class a teacher closed must stay visible so it
// can be reviewed or reopened. Layer 2 still refuses to act on them.
type TeacherGroupSummary struct {
	GroupID        uuid.UUID  `json:"group_id"`
	Name           string     `json:"name"`
	DisplayName    string     `json:"display_name"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	// CallerRole is how the caller holds this class: "owner" when they created
	// it (ClassGroup.OwnerUserID), "manager" when they reach it through a
	// membership role. It mirrors what the caller may DO — deleting a class is
	// owner-only — not which group_members row they happen to hold.
	CallerRole string     `json:"caller_role"`
	IsActive   bool       `json:"is_active"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	IsExpired  bool       `json:"is_expired"`
	// MemberCount counts the WHOLE active roster, teaching staff included. It is
	// the capacity figure — what fills against ClassGroup.MaxMembers and what an
	// invitation consumes — and deliberately NOT the number of apprenants; see
	// LearnerCount below, which is the same population minus the staff.
	MemberCount int `json:"member_count"`
	// LearnerCount counts the active memberships in groupModels.LearnerRoles —
	// the apprenants (issue #480). Every learner-facing figure on this row is
	// computed over that population: LiveSessionCount, IdleMemberCount, and the
	// denominator of TeacherGroupAssignment.ClassCompletionRate.
	//
	// It is the pair of MemberCount above, and the two are kept as one pair on
	// purpose: a class of 3 students whose teacher and assistant are enrolled
	// reports member_count 5 and learner_count 3, and reading either as the other
	// is the bug this field exists to end.
	LearnerCount int `json:"learner_count"`
	// LiveSessionCount is how many terminal sessions of those LEARNERS are
	// running right now and visible to this teacher (see the org-context rule
	// in terminalTrainer/models.SupervisableByJoinedGroupOrgScope). It renders as
	// the numerator of "X/N connectés" over LearnerCount, so a teacher opening
	// their own terminal must not push it past N.
	LiveSessionCount int `json:"live_session_count"`
	// IdleMemberCount is how many present LEARNERS have gone quiet — isLearnerIdle
	// (teacherLiveProgressService.go), the same predicate the per-learner class
	// view marks a row Idle with, so the console badge and the class view can
	// never disagree about who is stuck.
	//
	// It counts distinct MEMBERS, one per learner however many terminals they
	// hold — deliberately not "sessions" like LiveSessionCount above. "3 inactifs"
	// names three people who may need help, and naming this one for sessions next
	// to a neighbour that genuinely counts sessions would invite rendering the two
	// as a numerator and denominator they cannot form.
	//
	// Always sent, 0 included: 0 means "nobody is idle", never "not computed".
	IdleMemberCount int `json:"idle_member_count"`
	// IdleThresholdMinutes is the window IdleMemberCount was computed with, sent
	// so the UI can label the badge ("N inactifs > 10 min") from the value that
	// actually produced it instead of hard-coding a copy that drifts.
	IdleThresholdMinutes int                      `json:"idle_threshold_minutes"`
	Assignments          []TeacherGroupAssignment `json:"assignments"`
}

// TeacherGroupAssignment is one active scenario assignment on a dashboard row,
// with the class's progress on it. An assignment nobody has started yet is
// reported all-zero rather than omitted — "assigned, untouched" is the state a
// teacher most needs to see.
type TeacherGroupAssignment struct {
	AssignmentID  uuid.UUID  `json:"assignment_id"`
	ScenarioID    uuid.UUID  `json:"scenario_id"`
	ScenarioTitle string     `json:"scenario_title"`
	StartDate     *time.Time `json:"start_date,omitempty"`
	Deadline      *time.Time `json:"deadline,omitempty"`
	// StartedCount / CompletedCount count DISTINCT active APPRENANTS, not
	// sessions and not staff: a teacher walking through their own scenario is
	// preparation, not class progress (#480).
	StartedCount   int `json:"started_count"`
	CompletedCount int `json:"completed_count"`
	// ClassCompletionRate is a PERCENTAGE (0..100) of CompletedCount over
	// TeacherGroupSummary.LearnerCount — "how much of my class is done" — and is
	// 0 for a class with no apprenant.
	//
	// The denominator is the learner count, not the roster (#480): this feeds
	// "3/12 terminé" on a learner-facing row, so enrolled teaching staff must
	// neither dilute the rate nor, being excluded from the numerator too, ever
	// push it past 100.
	//
	// It is deliberately NOT named completion_rate: ScenarioAnalytics.CompletionRate
	// (teacherDashboardService.go) is a different metric that happens to answer a
	// similar-sounding question. That one divides completed SESSIONS by TOTAL
	// SESSIONS, so a learner who retries counts twice; this one divides distinct
	// members who finished by the size of the class. Both are percentages, but they
	// can never be compared or swapped, and sharing a field name invited exactly
	// that. Keep the two definitions in sync only in SCALE — never merge them.
	ClassCompletionRate float64  `json:"class_completion_rate"`
	AvgGrade            *float64 `json:"avg_grade"`
}

// GetManagedGroupsOverview returns one summary row per class-group callerUserID
// owns or manages, carrying everything the list renders: member count, live
// terminal sessions, and each active assignment with its progress. Active
// classes sort first, then by display name.
//
// Scope is derived from callerUserID alone through groupModels.ManagedByScope —
// there is no admin bypass. This route answers "MY classes", so a platform
// administrator sees the classes they personally manage, exactly like anyone
// else; the platform-wide view belongs to the admin surfaces.
//
// Cost does not grow with the number of classes the caller has: the groups are
// loaded once, then every aggregate is batched over their ids. An empty caller
// id or a caller with no classes short-circuits to an empty (non-nil) slice.
//
// Nothing is cached, so a role change (a student promoted to assistant, a
// manager demoted) moves every figure on the next request — the counts derive
// from group_members.role at read time.
func (s *TeacherDashboardService) GetManagedGroupsOverview(callerUserID string) ([]TeacherGroupSummary, error) {
	// An absent caller id must never fall through to `owner_user_id = ''`.
	if callerUserID == "" {
		return []TeacherGroupSummary{}, nil
	}

	var groups []groupModels.ClassGroup
	if err := s.db.Model(&groupModels.ClassGroup{}).
		Scopes(groupModels.ManagedByScope(callerUserID)).
		Order("is_active DESC, display_name ASC").
		Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []TeacherGroupSummary{}, nil
	}

	groupIDs := make([]uuid.UUID, len(groups))
	for i, group := range groups {
		groupIDs[i] = group.ID
	}

	headcounts, err := s.headcountsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	assignments, err := s.activeAssignmentsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	progress, err := s.assignmentsProgressByGroup(groupIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]TeacherGroupSummary, 0, len(groups))
	for _, group := range groups {
		counts := headcounts[group.ID]
		summaries = append(summaries, buildTeacherGroupSummary(
			group, callerUserID, counts,
			buildAssignmentItems(assignments[group.ID], progress[group.ID], counts.learners),
		))
	}
	return summaries, nil
}

// groupHeadcounts bundles the batched per-class counts a dashboard row is built
// from. They are named rather than passed as a row of interchangeable ints
// because three of the four are about APPRENANTS and one deliberately is not —
// see TeacherGroupSummary.MemberCount and .LearnerCount.
type groupHeadcounts struct {
	members      int
	learners     int
	liveSessions int
	idleLearners int
}

// headcountsByGroup runs the count queries behind the dashboard rows and pivots
// them per class. A group absent from a query keeps that count at zero, which is
// the honest answer for a class with nothing going on.
func (s *TeacherDashboardService) headcountsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]groupHeadcounts, error) {
	memberCounts, err := s.activeMemberCountsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	learnerCounts, err := s.activeLearnerCountsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	liveCounts, err := s.liveSessionCountsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}
	idleCounts, err := s.idleMemberCountsByGroup(groupIDs)
	if err != nil {
		return nil, err
	}

	headcounts := make(map[uuid.UUID]groupHeadcounts, len(groupIDs))
	for _, groupID := range groupIDs {
		headcounts[groupID] = groupHeadcounts{
			members:      memberCounts[groupID],
			learners:     learnerCounts[groupID],
			liveSessions: liveCounts[groupID],
			idleLearners: idleCounts[groupID],
		}
	}
	return headcounts, nil
}

// buildAssignmentItems pairs each of a class's active assignments with the
// progress its apprenants made on that scenario, rating each against
// learnerCount. An assignment with no matching progress row keeps the zero value
// — assigned but untouched.
func buildAssignmentItems(assignments []groupAssignmentRow, progress []scenarioProgressRow, learnerCount int) []TeacherGroupAssignment {
	progressByScenario := make(map[uuid.UUID]scenarioProgressRow, len(progress))
	for _, row := range progress {
		progressByScenario[row.ScenarioID] = row
	}

	items := make([]TeacherGroupAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		done := progressByScenario[assignment.ScenarioID]
		items = append(items, TeacherGroupAssignment{
			AssignmentID:   assignment.AssignmentID,
			ScenarioID:     assignment.ScenarioID,
			ScenarioTitle:  assignment.ScenarioTitle,
			StartDate:      assignment.StartDate,
			Deadline:       assignment.Deadline,
			StartedCount:        done.StartedCount,
			CompletedCount:      done.CompletedCount,
			ClassCompletionRate: classCompletionPercent(done.CompletedCount, learnerCount),
			AvgGrade:            done.AvgGrade,
		})
	}
	return items
}

// buildTeacherGroupSummary assembles one dashboard row from the group and its
// already batched aggregates. Pure: no DB access, so the query plan above stays
// readable.
func buildTeacherGroupSummary(
	group groupModels.ClassGroup,
	callerUserID string,
	counts groupHeadcounts,
	assignments []TeacherGroupAssignment,
) TeacherGroupSummary {
	callerRole := string(groupModels.GroupMemberRoleManager)
	if group.IsOwner(callerUserID) {
		callerRole = string(groupModels.GroupMemberRoleOwner)
	}

	return TeacherGroupSummary{
		GroupID:          group.ID,
		Name:             group.Name,
		DisplayName:      group.DisplayName,
		OrganizationID:   group.OrganizationID,
		CallerRole:       callerRole,
		IsActive:         group.IsActive,
		ExpiresAt:        group.ExpiresAt,
		IsExpired:        group.IsExpired(),
		MemberCount:          counts.members,
		LearnerCount:         counts.learners,
		LiveSessionCount:     counts.liveSessions,
		IdleMemberCount:      counts.idleLearners,
		IdleThresholdMinutes: int(LearnerIdleThreshold.Minutes()),
		Assignments:          assignments,
	}
}

// classCompletionPercent returns the share of the class's APPRENANTS that
// completed, on the 0..100 scale the rest of the teacher API already uses for
// rates. Both sides count learners only (#480), so the result cannot exceed 100.
//
// It guards the learner-less class: a group carrying only its teaching staff has
// no meaningful rate, and 0 is the honest answer rather than a division by zero.
func classCompletionPercent(completed, learnerCount int) float64 {
	if learnerCount <= 0 {
		return 0
	}
	return float64(completed) / float64(learnerCount) * 100.0
}

// groupCountRow carries one grouped COUNT keyed by class-group. The column is
// aliased `total` rather than `count` so the SELECT never collides with the
// aggregate function's name on either dialect.
type groupCountRow struct {
	GroupID uuid.UUID
	Total   int
}

func countsByGroupID(rows []groupCountRow) map[uuid.UUID]int {
	counts := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		counts[row.GroupID] = row.Total
	}
	return counts
}

// activeMemberCountsByGroup counts the WHOLE active roster of each group,
// teaching staff included — the capacity population behind
// TeacherGroupSummary.MemberCount.
func (s *TeacherDashboardService) activeMemberCountsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	return s.countMembershipsByGroup(groupIDs, nil)
}

// activeLearnerCountsByGroup counts the APPRENANTS of each group — the same
// active roster restricted to groupModels.LearnerRoles, which owns the
// definition of who that is.
func (s *TeacherDashboardService) activeLearnerCountsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	return s.countMembershipsByGroup(groupIDs, groupModels.LearnerRoleScope("group_members"))
}

// countMembershipsByGroup counts memberships per group in one grouped query,
// under an optional extra restriction. The active-membership half is written
// once here so the roster count and the learner count can never disagree about
// who is on the roster at all — they differ by the role filter and nothing else.
func (s *TeacherDashboardService) countMembershipsByGroup(groupIDs []uuid.UUID, roleScope func(*gorm.DB) *gorm.DB) (map[uuid.UUID]int, error) {
	if len(groupIDs) == 0 {
		return map[uuid.UUID]int{}, nil
	}
	query := s.db.Model(&groupModels.GroupMember{}).
		Select("group_id, COUNT(*) as total").
		Where("group_id IN ? AND is_active = ?", groupIDs, true)
	if roleScope != nil {
		query = query.Scopes(roleScope)
	}

	var rows []groupCountRow
	if err := query.Group("group_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return countsByGroupID(rows), nil
}

// liveSessionCountsByGroup counts, per group, the terminal sessions of its
// active APPRENANTS that are alive right now AND visible to a teacher of that
// group. Every predicate comes from its single home — RunningDisplayScope and
// SupervisableByJoinedGroupOrgScope in terminalTrainer/models, LearnerRoleScope
// in groups/models — rather than being re-spelled here, so the dashboard can
// never disagree with the supervision wall about liveness, nor with
// TeacherGroupSummary.LearnerCount about who is an apprenant.
func (s *TeacherDashboardService) liveSessionCountsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	if len(groupIDs) == 0 {
		return map[uuid.UUID]int{}, nil
	}
	var rows []groupCountRow
	err := s.db.Table("terminals").
		Select("gm.group_id as group_id, COUNT(DISTINCT terminals.id) as total").
		Joins("JOIN group_members gm ON gm.user_id = terminals.user_id AND gm.is_active = ? AND gm.deleted_at IS NULL", true).
		Joins("JOIN class_groups cg ON cg.id = gm.group_id AND cg.deleted_at IS NULL").
		Where("gm.group_id IN ?", groupIDs).
		Scopes(
			terminalModels.RunningDisplayScope,
			terminalModels.SupervisableByJoinedGroupOrgScope("cg"),
			groupModels.LearnerRoleScope("gm"),
		).
		Group("gm.group_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return countsByGroupID(rows), nil
}

// groupAssignmentRow is one active scenario assignment of a class, joined to its
// scenario for the display title.
type groupAssignmentRow struct {
	GroupID       uuid.UUID
	AssignmentID  uuid.UUID
	ScenarioID    uuid.UUID
	ScenarioTitle string
	StartDate     *time.Time
	Deadline      *time.Time
}

// activeAssignmentsByGroup loads the active group-scoped assignments of every
// group in one query, nearest deadline first (undated ones last).
//
// The scenario join is a LEFT JOIN so an assignment whose scenario was deleted
// still surfaces — with an empty title — instead of silently disappearing from
// the teacher's list. Org-scoped assignments (group_id NULL) are out of scope
// here, matching every other group-assignment predicate in this service.
func (s *TeacherDashboardService) activeAssignmentsByGroup(groupIDs []uuid.UUID) (map[uuid.UUID][]groupAssignmentRow, error) {
	if len(groupIDs) == 0 {
		return map[uuid.UUID][]groupAssignmentRow{}, nil
	}
	var rows []groupAssignmentRow
	err := s.db.Table("scenario_assignments sa").
		Select("sa.group_id as group_id, sa.id as assignment_id, sa.scenario_id as scenario_id, sc.title as scenario_title, sa.start_date as start_date, sa.deadline as deadline").
		Joins("LEFT JOIN scenarios sc ON sc.id = sa.scenario_id AND sc.deleted_at IS NULL").
		Where("sa.group_id IN ? AND sa.is_active = ? AND sa.deleted_at IS NULL", groupIDs, true).
		Order("(sa.deadline IS NULL), sa.deadline ASC, sc.title ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byGroup := make(map[uuid.UUID][]groupAssignmentRow, len(groupIDs))
	for _, row := range rows {
		byGroup[row.GroupID] = append(byGroup[row.GroupID], row)
	}
	return byGroup, nil
}

// scenarioProgressRow is the per-(group, scenario) progress aggregate backing
// both GetGroupAssignmentsProgress and the cross-group overview.
type scenarioProgressRow struct {
	GroupID        uuid.UUID
	ScenarioID     uuid.UUID
	StartedCount   int
	CompletedCount int
	AvgGrade       *float64
}

// assignmentsProgressByGroup aggregates, for every group at once, one row per
// scenario its active APPRENANTS have a non-preview session on: how many
// distinct learners started it, how many completed it, and the average grade
// over COMPLETED sessions only (NULL — hence nil — until someone finishes, since
// an in-progress session has no meaningful grade yet).
//
// Teaching staff are excluded on the same grounds preview sessions are (#480):
// a teacher running the scenario is preparing it, not making class progress, and
// counting them would let CompletedCount exceed the learner count these figures
// are rendered against.
//
// This is the shared internal behind the single-group GetGroupAssignmentsProgress:
// the join and filters (learner-role active membership on user_id,
// is_preview = false) are written once so the per-class tab and the cross-class
// dashboard cannot drift.
func (s *TeacherDashboardService) assignmentsProgressByGroup(groupIDs []uuid.UUID) (map[uuid.UUID][]scenarioProgressRow, error) {
	if len(groupIDs) == 0 {
		return map[uuid.UUID][]scenarioProgressRow{}, nil
	}
	var rows []scenarioProgressRow
	// Raw SQL cannot take groupModels.LearnerRoleScope, so the role list itself
	// is bound from groupModels.LearnerRoles — still one definition of who counts
	// as an apprenant, only the `IN` spelled locally.
	err := s.db.Raw(`
		SELECT gm.group_id as group_id,
		       ss.scenario_id as scenario_id,
		       COUNT(DISTINCT ss.user_id) as started_count,
		       COUNT(DISTINCT CASE WHEN ss.status = 'completed' THEN ss.user_id END) as completed_count,
		       AVG(CASE WHEN ss.status = 'completed' THEN ss.grade END) as avg_grade
		FROM scenario_sessions ss
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id IN ? AND gm.is_active = true
		     AND gm.role IN ? AND gm.deleted_at IS NULL
		WHERE ss.is_preview = false
		  AND ss.deleted_at IS NULL
		GROUP BY gm.group_id, ss.scenario_id
		ORDER BY gm.group_id, ss.scenario_id
	`, groupIDs, groupModels.LearnerRoles).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byGroup := make(map[uuid.UUID][]scenarioProgressRow, len(groupIDs))
	for _, row := range rows {
		byGroup[row.GroupID] = append(byGroup[row.GroupID], row)
	}
	return byGroup, nil
}
