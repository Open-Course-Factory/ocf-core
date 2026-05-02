package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/models"
	ttDto "soli/formations/src/terminalTrainer/dto"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"
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
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
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
	TotalSessions         int64    `json:"total_sessions"`
	CompletedCount        int64    `json:"completed_count"`
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

func enrichActivityUsers(items []GroupActivityItem) {
	ids := make([]string, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if !seen[item.UserID] {
			ids = append(ids, item.UserID)
			seen[item.UserID] = true
		}
	}
	userMap := fetchUserMap(ids)
	for i := range items {
		if info, ok := userMap[items[i].UserID]; ok {
			items[i].UserName = info.Name
			items[i].UserEmail = info.Email
		}
	}
}

func enrichResultUsers(items []ScenarioResultItem) {
	ids := make([]string, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if !seen[item.UserID] {
			ids = append(ids, item.UserID)
			seen[item.UserID] = true
		}
	}
	userMap := fetchUserMap(ids)
	for i := range items {
		if info, ok := userMap[items[i].UserID]; ok {
			items[i].UserName = info.Name
			items[i].UserEmail = info.Email
		}
	}
}

// TeacherDashboardService provides teacher-facing queries for group activity and scenario results
type TeacherDashboardService struct {
	db              *gorm.DB
	terminalService ttServices.TerminalTrainerService
	sessionService  *ScenarioSessionService
}

// NewTeacherDashboardService creates a new teacher dashboard service
func NewTeacherDashboardService(db *gorm.DB, terminalService ttServices.TerminalTrainerService, sessionService *ScenarioSessionService) *TeacherDashboardService {
	return &TeacherDashboardService{db: db, terminalService: terminalService, sessionService: sessionService}
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
		WHERE ss.status = 'active' AND ss.is_preview = false
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

// GetScenarioResults returns sessions for a specific scenario within a group with optional pagination.
// When limit is nil or zero, all results are returned (backward compatible).
// Always returns total count for frontend pagination controls.
func (s *TeacherDashboardService) GetScenarioResults(groupID, scenarioID uuid.UUID, limit, offset *int) (*PaginatedScenarioResults, error) {
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
		       (SELECT COALESCE(SUM(hints_revealed), 0) FROM scenario_step_progress WHERE session_id = ss.id) as total_hints_used
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

	// Calculate partial grade for active sessions (completed_steps / total_steps * 100)
	for i := range results {
		if results[i].Grade == nil && results[i].TotalSteps > 0 {
			partialGrade := float64(results[i].CompletedSteps) / float64(results[i].TotalSteps) * 100.0
			results[i].Grade = &partialGrade
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
func (s *TeacherDashboardService) GetScenarioAnalytics(groupID, scenarioID uuid.UUID) (*ScenarioAnalytics, error) {
	paginated, err := s.GetScenarioResults(groupID, scenarioID, nil, nil)
	if err != nil {
		return nil, err
	}

	analytics := &ScenarioAnalytics{}
	analytics.TotalSessions = int64(len(paginated.Items))

	var gradeSum float64
	var gradeCount int64
	var timeSum float64
	var timeCount int64

	for _, r := range paginated.Items {
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

// CalculateGrade computes the grade for a session as percentage of completed steps
func (s *TeacherDashboardService) CalculateGrade(sessionID uuid.UUID) float64 {
	var session models.ScenarioSession
	if err := s.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return 0
	}

	var totalSteps int64
	s.db.Model(&models.ScenarioStep{}).Where("scenario_id = ?", session.ScenarioID).Count(&totalSteps)
	if totalSteps == 0 {
		return 0
	}

	var completedSteps int64
	s.db.Model(&models.ScenarioStepProgress{}).Where("session_id = ? AND status = ?", sessionID, "completed").Count(&completedSteps)

	return (float64(completedSteps) / float64(totalSteps)) * 100
}

// BulkStartScenario creates terminal sessions and scenario sessions for group members.
// Auto-provisions terminal keys for members who don't have one.
// If instanceType is empty, only creates scenario sessions (no terminals).
// sessionDurationMinutes controls terminal session lifetime (default: 240 = 4 hours).
// trainerID identifies the trainer who initiated the bulk start (for Qualiopi traceability).
func (s *TeacherDashboardService) BulkStartScenario(groupID uuid.UUID, scenarioID uuid.UUID, instanceType, backend string, sessionDurationMinutes int, trainerID string) (*BulkStartResult, error) {
	var members []groupModels.GroupMember
	if err := s.db.Where("group_id = ? AND is_active = ?", groupID, true).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to load group members: %w", err)
	}

	// Verify scenario exists and has steps
	var scenario models.Scenario
	if err := s.db.Preload("Steps").First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
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
	if instanceType != "" {
		var termsErr error
		terms, termsErr = s.terminalService.GetTerms()
		if termsErr != nil {
			return nil, fmt.Errorf("failed to fetch terminal terms: %w", termsErr)
		}
	}

	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	for _, member := range members {
		member := member // capture range variable
		g.Go(func() error {
			utils.Debug("BulkStartScenario - Processing member %s (role=%s)", member.UserID, member.Role)

			// Check for existing active session — abandon it and create a new one
			var existing models.ScenarioSession
			err := s.db.Where("user_id = ? AND scenario_id = ? AND status IN ?",
				member.UserID, scenarioID, []string{"active", "in_progress", "provisioning", "setup_failed"}).First(&existing).Error
			if err == nil {
				utils.Debug("BulkStartScenario - Member %s has existing session %s (status=%s), abandoning it", member.UserID, existing.ID, existing.Status)
				if abandonErr := s.db.Model(&existing).Update("status", "abandoned").Error; abandonErr != nil {
					utils.Warn("BulkStartScenario - Failed to abandon existing session %s for user %s: %v", existing.ID, member.UserID, abandonErr)
					mu.Lock()
					result.Errors = append(result.Errors, BulkStartError{
						UserID: member.UserID,
						Error:  "failed to abandon existing session",
					})
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.Replaced++
				mu.Unlock()
				// Fall through to create a new session
			}

			// Auto-provision terminal key if missing
			_, keyErr := s.terminalService.GetUserKey(member.UserID)
			if keyErr != nil {
				user, userErr := casdoorsdk.GetUserByUserId(member.UserID)
				keyName := "auto-" + member.UserID
				if userErr == nil && user != nil && user.Email != "" {
					keyName = "auto-" + user.Email
				}
				if createErr := s.terminalService.CreateUserKey(member.UserID, keyName); createErr != nil {
					// Cannot create key — skip and report
					missing := UserKeyMissing{UserID: member.UserID}
					if userErr == nil && user != nil {
						missing.UserName = user.DisplayName
						if missing.UserName == "" {
							missing.UserName = user.Name
						}
						missing.UserEmail = user.Email
					}
					mu.Lock()
					result.NoKey++
					result.NoKeyUsers = append(result.NoKeyUsers, missing)
					mu.Unlock()
					return nil
				}
			}

			// Create terminal session if instance type (distribution) is provided
			var terminalSessionID *string
			if instanceType != "" {
				utils.Debug("BulkStartScenario - Creating terminal for member %s (distribution=%s)", member.UserID, instanceType)
				composedInput := ttDto.CreateComposedSessionInput{
					Distribution: instanceType, // instanceType now maps to distribution name
					Size:         "S",          // Default size for bulk scenario creation
					Terms:        terms,
					Backend:      backend,
					Name:         fmt.Sprintf("scenario-%s", scenario.Title),
					Expiry:       sessionExpirySecs,
					Hostname:     scenario.Hostname,
					OrganizationID: func() string {
						if scenario.OrganizationID != nil {
							return scenario.OrganizationID.String()
						}
						return ""
					}(),
					// RecordingEnabled: recording is always on (RGPD Art. 6.1.f — legitimate interest)
					RecordingEnabled: 1,
				}

				// Resolve the member's effective subscription plan
				planResult, planErr := paymentServices.NewEffectivePlanService(s.db).GetUserEffectivePlan(member.UserID)
				if planErr != nil {
					utils.Warn("BulkStartScenario - Failed to resolve plan for user %s: %v", member.UserID, planErr)
					mu.Lock()
					result.Errors = append(result.Errors, BulkStartError{
						UserID: member.UserID,
						Error:  "failed to resolve subscription plan",
					})
					mu.Unlock()
					return nil
				}

				terminalResp, termErr := s.terminalService.StartComposedSession(member.UserID, composedInput, planResult.Plan)
				if termErr != nil {
					utils.Warn("BulkStartScenario - Failed to create terminal for user %s: %v", member.UserID, termErr)
					mu.Lock()
					result.Errors = append(result.Errors, BulkStartError{
						UserID: member.UserID,
						Error:  "failed to create terminal session",
					})
					mu.Unlock()
					return nil
				}
				utils.Debug("BulkStartScenario - Terminal created for member %s: sessionID=%s", member.UserID, terminalResp.SessionID)
				terminalSessionID = &terminalResp.SessionID
			} else {
				utils.Debug("BulkStartScenario - No instanceType provided, skipping terminal creation for member %s", member.UserID)
			}

			// Use ScenarioSessionService.StartScenario to create session, step progress,
			// generate flags, and deploy them to the container
			termSessionID := ""
			if terminalSessionID != nil {
				termSessionID = *terminalSessionID
			}

			session, startErr := s.sessionService.StartScenario(member.UserID, scenarioID, termSessionID)
			if startErr != nil {
				utils.Warn("BulkStartScenario - Failed to start scenario for user %s: %v", member.UserID, startErr)
				mu.Lock()
				result.Errors = append(result.Errors, BulkStartError{
					UserID: member.UserID,
					Error:  "failed to start scenario",
				})
				mu.Unlock()
				return nil
			}
			// Set trainer ID for Qualiopi traceability
			if trainerID != "" {
				if updateErr := s.db.Model(session).Update("trainer_id", trainerID).Error; updateErr != nil {
					utils.Warn("BulkStartScenario - Failed to set trainer_id for session %s: %v", session.ID, updateErr)
				}
			}

			utils.Debug("BulkStartScenario - ScenarioSession created for member %s: sessionID=%s terminalSessionID=%v", member.UserID, session.ID, terminalSessionID)

			mu.Lock()
			result.Created++
			mu.Unlock()
			return nil
		})
	}
	g.Wait()

	utils.Debug("BulkStartScenario - Complete: created=%d skipped=%d noKey=%d errors=%d",
		result.Created, result.Skipped, result.NoKey, len(result.Errors))

	return result, nil
}

// ResetGroupScenarioSessions abandons all active sessions for a group+scenario combination.
// Used to clean up orphaned sessions (e.g., created without terminal keys).
func (s *TeacherDashboardService) ResetGroupScenarioSessions(groupID uuid.UUID, scenarioID uuid.UUID) (int64, error) {
	// Get active group member user IDs
	var memberUserIDs []string
	if err := s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", groupID, true).
		Pluck("user_id", &memberUserIDs).Error; err != nil {
		return 0, fmt.Errorf("failed to load group members: %w", err)
	}

	if len(memberUserIDs) == 0 {
		return 0, nil
	}

	// Abandon all active/in_progress sessions for these users on this scenario
	result := s.db.Model(&models.ScenarioSession{}).
		Where("user_id IN ? AND scenario_id = ? AND status IN ?",
			memberUserIDs, scenarioID, []string{"active", "in_progress"}).
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
	// QuizScore is the aggregate score in [0, 1] for quiz steps after submission.
	// Nil for non-quiz steps or quizzes that have not been submitted.
	// Per-question answers are intentionally not exposed (privacy invariant).
	QuizScore *float64 `json:"quiz_score,omitempty"`
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
	StartedAt         time.Time          `json:"started_at"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
	TerminalSessionID *string            `json:"terminal_session_id,omitempty"`
	Steps             []SessionStepDetail `json:"steps"`
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

	// Get step-level progress joined with step metadata.
	// COALESCE on step_type defaults legacy rows (empty string) to "terminal" to match
	// the player's normalization. quiz_score is included; quiz_answers is intentionally
	// excluded — trainers see the aggregate score, never per-question answers.
	var steps []SessionStepDetail
	err := s.db.Raw(`
		SELECT sp.step_order, ss.title as step_title,
		       COALESCE(NULLIF(ss.step_type, ''), 'terminal') as step_type,
		       sp.status, sp.verify_attempts, sp.hints_revealed, sp.completed_at,
		       sp.time_spent_seconds, sp.quiz_score
		FROM scenario_step_progress sp
		JOIN scenario_steps ss ON ss.scenario_id = ? AND ss."order" = sp.step_order AND ss.deleted_at IS NULL
		WHERE sp.session_id = ?
		ORDER BY sp.step_order ASC
	`, session.ScenarioID, sessionID).Scan(&steps).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load step progress: %w", err)
	}
	if steps == nil {
		steps = []SessionStepDetail{}
	}

	resp := &SessionDetailResponse{
		SessionID:         session.ID,
		UserID:            session.UserID,
		TrainerID:         session.TrainerID,
		ScenarioID:        session.ScenarioID,
		ScenarioTitle:     scenario.Title,
		Status:            session.Status,
		Grade:             session.Grade,
		StartedAt:         session.StartedAt,
		CompletedAt:       session.CompletedAt,
		TerminalSessionID: session.TerminalSessionID,
		Steps:             steps,
	}

	// Enrich with user info
	userMap := fetchUserMap([]string{session.UserID})
	if info, ok := userMap[session.UserID]; ok {
		resp.UserName = info.Name
		resp.UserEmail = info.Email
	}

	return resp, nil
}
