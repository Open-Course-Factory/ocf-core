package services

import "sync"

// ConsoleAttachedObserver is notified with the terminal session id whose
// owner has just opened their console.
type ConsoleAttachedObserver func(terminalSessionID string)

var consoleAttached consoleObserver

// SetConsoleAttachedObserver registers the handler for the learner's console
// attach, replacing any previous one. Passing nil unregisters. The same
// inversion as SetConsoleShellKilledObserver: the scenarios module subscribes,
// so terminalTrainer never imports it.
func SetConsoleAttachedObserver(observer ConsoleAttachedObserver) {
	consoleAttached.set(observer)
}

// ReportConsoleAttach hands the learner's console attach to the registered
// observer. Callers report only the terminal owner's own console: a supervisor
// or a teacher looking in is not the learner sitting down at their shell.
//
// A no-op when no observer is registered.
func ReportConsoleAttach(terminalSessionID string) {
	if terminalSessionID == "" {
		return
	}
	consoleAttached.notify(terminalSessionID)
}

// consoleObserver holds the one handler registered for a console event. It is
// guarded because handlers are registered at route setup while consoles are
// already being relayed.
type consoleObserver struct {
	mu       sync.RWMutex
	observer func(terminalSessionID string)
}

func (o *consoleObserver) set(observer func(terminalSessionID string)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observer = observer
}

func (o *consoleObserver) notify(terminalSessionID string) {
	o.mu.RLock()
	observer := o.observer
	o.mu.RUnlock()

	if observer != nil {
		observer(terminalSessionID)
	}
}
