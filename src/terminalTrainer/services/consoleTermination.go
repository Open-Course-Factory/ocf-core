package services

// ConsoleShellKilledCloseCode is the WebSocket close code tt-backend sends when
// the learner's shell was terminated by SIGKILL.
//
// The chain, verified end to end: Incus' forkexec returns 128+WTERMSIG for a
// signalled process (main_forkexec.go) and linux.ExitStatus mirrors it, so a
// SIGKILLed shell surfaces as exit code 137 in the exec operation's "return"
// metadata; tt-backend's execCloseCode maps a non-zero exit code N to close
// code 4000+N (backend/api_session_console.go), giving 4137.
const ConsoleShellKilledCloseCode = 4137

// ConsoleSessionStoppedCloseCode is the close code tt-backend sends when the
// platform itself stops or deletes a session (pause, expiry, idle stop,
// teardown — tt#145). It is outside the 4000+exit-code band on purpose, so a
// platform stop is never mistaken for a killed shell: tt-backend sends it
// instead of whatever the shell's exit code would have been, and 4137 is left
// meaning a real kill.
const ConsoleSessionStoppedCloseCode = 4300

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

var consoleShellKilled consoleObserver

// SetConsoleShellKilledObserver registers the handler for SIGKILLed shells,
// replacing any previous one. Passing nil unregisters.
//
// This inversion exists because terminalTrainer is the lower layer: scenarios
// imports it, so it cannot import scenarios back to end a crash-trap run
// itself. It publishes the event instead, and the scenarios module subscribes
// at route-registration time — the same shape as ScenarioSessionService's
// TerminalStopFunc callback, in the opposite direction.
func SetConsoleShellKilledObserver(observer ConsoleShellKilledObserver) {
	consoleShellKilled.set(observer)
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
	consoleShellKilled.notify(terminalSessionID)
}
