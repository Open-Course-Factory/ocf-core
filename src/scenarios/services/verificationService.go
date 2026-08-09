package services

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"
)

// VerificationService calls tt-backend's exec and file-push APIs
// to run verification scripts inside containers
type VerificationService struct {
	ttBackendURL string
	ttAPIKey     string
}

// NewVerificationService creates a new VerificationService reading config from env vars
func NewVerificationService() *VerificationService {
	return &VerificationService{
		ttBackendURL: os.Getenv("TERMINAL_TRAINER_URL"),
		ttAPIKey:     os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
	}
}

// NewVerificationServiceWithConfig creates a VerificationService with explicit config (for testing)
func NewVerificationServiceWithConfig(ttBackendURL, ttAPIKey string) *VerificationService {
	return &VerificationService{
		ttBackendURL: ttBackendURL,
		ttAPIKey:     ttAPIKey,
	}
}

// execResponse represents the JSON response from tt-backend /1.0/exec endpoint
type execResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ExecInContainer runs a command inside a container and returns the result.
func (s *VerificationService) ExecInContainer(sessionID string, command []string, timeout int) (exitCode int, stdout string, stderr string, err error) {
	url := fmt.Sprintf("%s/1.0/exec", s.ttBackendURL)

	payload := map[string]any{
		"session_id": sessionID,
		"command":    command,
		"timeout":    timeout,
	}

	var result execResponse
	// HTTP timeout must exceed the exec timeout to avoid killing the connection early
	httpTimeout := time.Duration(timeout+30) * time.Second
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(s.ttAPIKey), utils.WithTimeout(httpTimeout))

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, payload, &result, opts)
	if err != nil {
		return -1, "", "", fmt.Errorf("exec in container failed: %w", err)
	}

	return result.ExitCode, result.Stdout, result.Stderr, nil
}

// PushFile pushes a file into a container.
func (s *VerificationService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	url := fmt.Sprintf("%s/1.0/file-push", s.ttBackendURL)

	payload := map[string]any{
		"session_id":  sessionID,
		"target_path": targetPath,
		"content":     content,
		"mode":        mode,
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(s.ttAPIKey))

	_, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, payload, opts)
	if err != nil {
		return fmt.Errorf("push file to container failed: %w", err)
	}

	return nil
}

// ErrNoLiveConsole means the learner has no console attached, so there is no
// shell to type into. It is a normal condition — they may not have opened their
// terminal yet — and callers skip rather than fail on it.
var ErrNoLiveConsole = errors.New("session has no live console")

// WriteToConsole types text into the learner's live shell, as if they had
// entered it themselves.
//
// This is how a step's foreground script runs. It is NOT ExecInContainer with
// the output redirected: that would run in a separate process with its own
// environment, so `cd`, `export` and shell functions would have no effect on the
// shell the learner is actually using — which is the whole point of a foreground
// script.
//
// tt-backend answers 409 when nothing is attached; that becomes ErrNoLiveConsole
// so the caller can tell "nobody was watching" from "the write failed".
func (s *VerificationService) WriteToConsole(sessionID string, text string) error {
	url := fmt.Sprintf("%s/1.0/sessions/%s/console/input", s.ttBackendURL, sessionID)

	payload := map[string]any{
		"data":           text,
		"append_newline": true,
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(s.ttAPIKey))

	_, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, payload, opts)
	if err != nil {
		if strings.Contains(err.Error(), "409") {
			return ErrNoLiveConsole
		}
		return fmt.Errorf("write to console failed: %w", err)
	}

	return nil
}

// VerifyStep executes the verify script inside the container via sh -c.
// The script is passed as a command argument — never written to the filesystem,
// so the learner cannot inspect it.
// Exit code 0 = passed, non-zero = failed. Returns stdout as output.
func (s *VerificationService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (passed bool, output string, err error) {
	if step.VerifyScript == "" {
		return false, "", fmt.Errorf("step %d has no verify script", step.Order)
	}

	// Execute the verify script inline with a 10s timeout.
	// Parse the shebang to use the correct interpreter (e.g., bash vs sh).
	exitCode, stdout, stderr, err := s.ExecInContainer(
		terminalSessionID,
		[]string{parseShebang(step.VerifyScript), "-c", step.VerifyScript},
		10,
	)
	if err != nil {
		return false, "", fmt.Errorf("failed to execute verify script: %w", err)
	}

	// Surface stdout AND stderr so the learner sees what actually failed when
	// the script exits non-zero. Without this, a script like `[ -f /foo ] ||
	// echo "missing" >&2` produces an empty output panel — Karim's UX feedback.
	output = stdout
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += stderr
	}

	return exitCode == 0, output, nil
}
