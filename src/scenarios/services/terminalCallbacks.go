package services

import (
	ttServices "soli/formations/src/terminalTrainer/services"
)

// WireTerminalCallbacks gives a session service the two terminal callbacks it
// cannot make itself.
//
// ScenarioSessionService reaches the terminal layer through function fields so
// the type does not depend on terminalTrainer, and both fields default to nil.
// finishBuild and tryStopTerminal then return silently when they are unset, so
// a caller that forgets the wiring gets no error, no warning, and a container
// that keeps its build-time features for the whole session — which is how
// bulk-start shipped: two controllers wired the pair by hand and the third
// never did.
//
// One function, called by everyone who builds a session service that will
// provision containers, so the wiring cannot drift again.
func WireTerminalCallbacks(sessionService *ScenarioSessionService, terminalService ttServices.TerminalTrainerService) {
	if sessionService == nil || terminalService == nil {
		return
	}
	sessionService.SetTerminalStopFunc(terminalService.StopSession)
	sessionService.SetTerminalBuildCompleteFunc(terminalService.BuildComplete)
}
