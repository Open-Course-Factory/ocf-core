package services

import (
	"encoding/base64"
	"fmt"
	"time"

	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

const (
	// localBootWait is how long the engine waits for the agent of a machine
	// that never connected: the learner's tab is still booting it.
	localBootWait = time.Minute
	// localReplyGrace is what an answer may take beyond the command's own
	// timeout, for the round trip to the tab.
	localReplyGrace = 10 * time.Second
)

// LocalMachineService runs the engine's scripts in a machine in the learner's
// browser, through its agent: what VerificationService does through
// tt-backend for a container.
type LocalMachineService struct {
	agents     *LocalAgents
	bootWait   time.Duration
	replyGrace time.Duration
}

func NewLocalMachineService(agents *LocalAgents, bootWait, replyGrace time.Duration) *LocalMachineService {
	return &LocalMachineService{agents: agents, bootWait: bootWait, replyGrace: replyGrace}
}

func (s *LocalMachineService) ExecInContainer(sessionID string, command []string, env map[string]string, timeout int) (int, string, string, error) {
	agent, err := s.agents.await(sessionID, s.bootWait)
	if err != nil {
		return -1, "", "", err
	}
	reply, err := agent.call(agentRequest{Op: "exec", Command: command, Env: env, Timeout: timeout},
		time.Duration(timeout)*time.Second+s.replyGrace)
	if err != nil {
		return -1, "", "", err
	}
	if reply.Error != "" {
		return -1, "", "", fmt.Errorf("the machine in the learner's browser could not run the command: %s", capped(reply.Error, 200))
	}
	return reply.ExitCode, capped(reply.Stdout, MaxAgentOutput), capped(reply.Stderr, MaxAgentOutput), nil
}

// PushFile writes the file with a command, as tt-backend's file push would:
// the content travels base64-encoded as an argument, never as shell text.
//
// ponytail: one argument holds at most 128 KiB, so a file above ~96 KiB fails;
// add a chunked push if a scenario ever ships one.
func (s *LocalMachineService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	exitCode, _, stderr, err := s.ExecInContainer(sessionID, []string{
		"sh", "-c", `printf %s "$1" | base64 -d > "$2" && chmod "$3" "$2"`, "sh",
		base64.StdEncoding.EncodeToString([]byte(content)), targetPath, mode,
	}, nil, 10)
	if err != nil {
		return fmt.Errorf("push file to the machine failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("push file to the machine failed: exit %d: %s", exitCode, capped(stderr, 200))
	}
	return nil
}

// WriteToConsole types text into the learner's shell, as one line.
func (s *LocalMachineService) WriteToConsole(sessionID string, text string) error {
	agent, err := s.agents.await(sessionID, s.bootWait)
	if err != nil {
		return err
	}
	reply, err := agent.call(agentRequest{Op: "console", Data: text + "\n"}, s.replyGrace)
	if err != nil {
		return err
	}
	switch reply.Error {
	case "":
		return nil
	case "no_console":
		return ErrNoLiveConsole
	}
	return fmt.Errorf("write to console failed: %s", capped(reply.Error, 200))
}

func (s *LocalMachineService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return verifyStep(s, terminalSessionID, step)
}

func capped(text string, limit int) string {
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

// RunMachines sends each of the engine's calls to the machine of its
// terminal: the learner's browser for a local one, tt-backend for the others.
type RunMachines struct {
	incus VerificationServiceInterface
	local VerificationServiceInterface
}

func NewRunMachines(incus, local VerificationServiceInterface) *RunMachines {
	return &RunMachines{incus: incus, local: local}
}

// PlatformRunMachines is what the engine runs scripts with: tt-backend's
// containers, and the agents the platform serves.
func PlatformRunMachines() *RunMachines {
	return NewRunMachines(NewVerificationService(), NewLocalMachineService(Agents(), localBootWait, localReplyGrace))
}

func (m *RunMachines) of(sessionID string) VerificationServiceInterface {
	if terminalModels.IsLocalSessionID(sessionID) {
		return m.local
	}
	return m.incus
}

func (m *RunMachines) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return m.of(terminalSessionID).VerifyStep(terminalSessionID, step)
}

func (m *RunMachines) PushFile(sessionID string, targetPath string, content string, mode string) error {
	return m.of(sessionID).PushFile(sessionID, targetPath, content, mode)
}

func (m *RunMachines) WriteToConsole(sessionID string, text string) error {
	return m.of(sessionID).WriteToConsole(sessionID, text)
}

func (m *RunMachines) ExecInContainer(sessionID string, command []string, env map[string]string, timeout int) (int, string, string, error) {
	return m.of(sessionID).ExecInContainer(sessionID, command, env, timeout)
}

var (
	_ VerificationServiceInterface = (*VerificationService)(nil)
	_ VerificationServiceInterface = (*LocalMachineService)(nil)
	_ VerificationServiceInterface = (*RunMachines)(nil)
)
