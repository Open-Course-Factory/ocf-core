package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"soli/formations/src/scenarios/models"
)

// QuizAnswersContainerPath is where a session's quiz and survey answers are
// written in the container, so a later step's scripts can act on the choices
// the learner already made (a simulation replaying their decisions).
const QuizAnswersContainerPath = "/etc/ocf-quiz-answers.json"

// QuizAnswersDocument renders the answers already submitted in a session as
// {"<step order>": {"<question order>": "<raw answer>"}}, the raw answer being
// exactly what the quiz panel submitted. Only answers: no correct_answer, no
// explanation — the learner can read the file, and it holds nothing they did
// not type themselves.
//
// Returns "" when nothing has been answered, so the caller writes no file.
// Steps need their Questions loaded.
func QuizAnswersDocument(steps []models.ScenarioStep, progress []models.ScenarioStepProgress) string {
	stepByOrder := make(map[int]models.ScenarioStep, len(steps))
	for _, step := range steps {
		stepByOrder[step.Order] = step
	}

	document := map[string]map[string]string{}
	for _, p := range progress {
		step, ok := stepByOrder[p.StepOrder]
		if !ok || p.QuizAnswers == "" {
			continue
		}
		var byQuestionID map[string]string
		if json.Unmarshal([]byte(p.QuizAnswers), &byQuestionID) != nil {
			continue
		}
		byQuestionOrder := map[string]string{}
		for _, q := range step.Questions {
			if answer, ok := byQuestionID[q.ID.String()]; ok {
				byQuestionOrder[strconv.Itoa(q.Order)] = answer
			}
		}
		if len(byQuestionOrder) > 0 {
			document[strconv.Itoa(p.StepOrder)] = byQuestionOrder
		}
	}
	if len(document) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(document)
	return string(encoded)
}

// quizAnswersInstall is the shell that writes a session's answers file, or ""
// when there is nothing to write. Base64 keeps free-text answers — quotes,
// newlines, a line that happens to match a heredoc delimiter — from ever being
// read as shell.
func quizAnswersInstall(document string) string {
	if document == "" {
		return ""
	}
	return fmt.Sprintf("echo %s | base64 -d > %s\n",
		base64.StdEncoding.EncodeToString([]byte(document)), QuizAnswersContainerPath)
}

// quizAnswersTimeoutSeconds bounds the write: one base64 decode into a file.
const quizAnswersTimeoutSeconds = 10

// writeQuizAnswers refreshes the answers file before a step is provisioned, so
// its background script, its verify script and the learner all read the
// choices made so far. Best effort, like a banner: a step whose scripts do not
// read the file must not fail because it could not be written.
func (s *ScenarioSessionService) writeQuizAnswers(sessionID uuid.UUID, terminalID string) {
	if s.verificationService == nil {
		return
	}
	var progress []models.ScenarioStepProgress
	if err := s.db.Where("session_id = ? AND quiz_answers <> ''", sessionID).Find(&progress).Error; err != nil {
		slog.Warn("could not load quiz answers", "session_id", sessionID, "err", err)
		return
	}
	if len(progress) == 0 {
		return
	}
	var session models.ScenarioSession
	if err := s.db.Select("scenario_id").First(&session, "id = ?", sessionID).Error; err != nil {
		slog.Warn("could not load session for quiz answers", "session_id", sessionID, "err", err)
		return
	}
	var steps []models.ScenarioStep
	if err := s.db.Preload("Questions").Where("scenario_id = ?", session.ScenarioID).Find(&steps).Error; err != nil {
		slog.Warn("could not load questions for quiz answers", "session_id", sessionID, "err", err)
		return
	}

	install := quizAnswersInstall(QuizAnswersDocument(steps, progress))
	if install == "" {
		return
	}
	exitCode, _, stderr, err := s.verificationService.ExecInContainer(terminalID, []string{"/bin/sh", "-c", install}, nil, quizAnswersTimeoutSeconds)
	if err != nil || exitCode != 0 {
		slog.Warn("could not write quiz answers", "session_id", sessionID, "exit_code", exitCode, "stderr", truncateLog(stderr), "err", err)
	}
}
