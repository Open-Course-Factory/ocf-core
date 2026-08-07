package services

import "sync"

// ConsoleShellKilledCloseCode is the WebSocket close code tt-backend sends when
// the learner's shell was terminated by SIGKILL.
//
// The chain, verified end to end: Incus' forkexec returns 128+WTERMSIG for a
// signalled process (main_forkexec.go) and linux.ExitStatus mirrors it, so a
// SIGKILLed shell surfaces as exit code 137 in the exec operation's "return"
// metadata; tt-backend's execCloseCode maps a non-zero exit code N to close
// code 4000+N (backend/api_session_console.go), giving 4137.
const ConsoleShellKilledCloseCode = 4137

// IsShellKilledCloseCode reports whether a console close code means the
// learner's shell was SIGKILLed — the signal a crash-trap payload sends.
//
// The gate is deliberately this narrow, and must stay so. Every non-zero shell
// exit lands in the same 4000-4999 band: a learner typing `exit 1` produces
// 4001, a missing command 4127, a graceful SIGTERM teardown (session expiry,
// idle timeout, container stop) 4143. Widening this to `code >= 4000` — or
// adding SIGTERM because it looks like a sibling — would end a learner's run
// every time they exit their own shell or their session simply times out.
func IsShellKilledCloseCode(closeCode int) bool {
	return closeCode == ConsoleShellKilledCloseCode
}

// ConsoleShellKilledObserver is notified with the terminal session id whose
// shell was SIGKILLed.
type ConsoleShellKilledObserver func(terminalSessionID string)

var (
	consoleShellKilledMu       sync.RWMutex
	consoleShellKilledObserver ConsoleShellKilledObserver
)

// SetConsoleShellKilledObserver registers the handler for SIGKILLed shells,
// replacing any previous one. Passing nil unregisters.
//
// This inversion exists because terminalTrainer is the lower layer: scenarios
// imports it, so it cannot import scenarios back to end a crash-trap run
// itself. It publishes the event instead, and the scenarios module subscribes
// at route-registration time — the same shape as ScenarioSessionService's
// TerminalStopFunc callback, in the opposite direction.
func SetConsoleShellKilledObserver(observer ConsoleShellKilledObserver) {
	consoleShellKilledMu.Lock()
	defer consoleShellKilledMu.Unlock()
	consoleShellKilledObserver = observer
}

// ReportConsoleClose hands a closed console connection to the registered
// observer when — and only when — the close code says the shell was SIGKILLed.
// It is the single owner of that decision: callers relay close frames without
// having to know which codes end a run.
//
// A no-op when no observer is registered, which is the normal state for a
// deployment that never registers the scenarios module.
func ReportConsoleClose(terminalSessionID string, closeCode int) {
	if terminalSessionID == "" || !IsShellKilledCloseCode(closeCode) {
		return
	}

	consoleShellKilledMu.RLock()
	observer := consoleShellKilledObserver
	consoleShellKilledMu.RUnlock()

	if observer != nil {
		observer(terminalSessionID)
	}
}
