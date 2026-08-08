package terminalController

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	services "soli/formations/src/terminalTrainer/services"
)

// The console proxy used to drop tt-backend's close frame on the floor: the
// relay loop broke on the read error and gorilla closed the browser socket
// without a close frame, so the browser only ever saw 1006 (abnormal closure)
// and offered a "your environment is still running — Reconnect" overlay. These
// tests pin that tt-backend's structured close code now reaches the browser
// unchanged, and that a SIGKILLed shell is reported for permadeath handling.

// newFakeTerminalTrainer serves one websocket connection that immediately
// closes with the given code and reason, mimicking tt-backend's exec-exit
// teardown (execCloseCode in backend/api_session_console.go).
func newFakeTerminalTrainer(t *testing.T, closeCode int, closeReason string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(closeCode, closeReason),
			time.Now().Add(time.Second))
	}))
}

// runConsoleProxy stands up an ocf-core-side proxy that relays a fake
// tt-backend console to the browser through relayTerminalToClient, then
// returns the close error the browser observed.
func runConsoleProxy(t *testing.T, ttServer *httptest.Server, terminalSessionID string) error {
	t.Helper()
	upgrader := websocket.Upgrader{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer clientConn.Close()

		terminalConn, _, dialErr := websocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(ttServer.URL, "http"), nil)
		if dialErr != nil {
			return
		}
		defer terminalConn.Close()

		relayTerminalToClient(terminalConn, clientConn, terminalSessionID)
	}))
	defer proxy.Close()

	browserConn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(proxy.URL, "http"), nil)
	require.NoError(t, err)
	defer browserConn.Close()

	_ = browserConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, readErr := browserConn.ReadMessage(); readErr != nil {
			return readErr
		}
	}
}

func TestConsoleRelay_ForwardsShellKilledCloseCodeToBrowser(t *testing.T) {
	ttServer := newFakeTerminalTrainer(t, services.ConsoleShellKilledCloseCode, "exec_failed")
	defer ttServer.Close()

	err := runConsoleProxy(t, ttServer, "terminal-relay-kill")

	closeErr, ok := err.(*websocket.CloseError)
	require.True(t, ok,
		"the browser must receive a close frame, not an abnormal closure. Got %v", err)
	assert.Equal(t, services.ConsoleShellKilledCloseCode, closeErr.Code,
		"tt-backend's close code must reach the browser unchanged, otherwise the "+
			"frontend cannot tell permadeath from a dropped connection")
	assert.Equal(t, "exec_failed", closeErr.Text,
		"the close reason must be relayed alongside the code")
}

func TestConsoleRelay_ForwardsLearnerExitCloseCodeToBrowser(t *testing.T) {
	// A learner typing `exit 1` produces exit code 1 → close code 4001. It is
	// still relayed (the frontend explains the shell ended) but must not be
	// mistaken for a kill.
	ttServer := newFakeTerminalTrainer(t, 4001, "exec_failed")
	defer ttServer.Close()

	err := runConsoleProxy(t, ttServer, "terminal-relay-exit1")

	closeErr, ok := err.(*websocket.CloseError)
	require.True(t, ok, "expected a close frame, got %v", err)
	assert.Equal(t, 4001, closeErr.Code)
}

func TestConsoleRelay_ReportsShellKillForPermadeath(t *testing.T) {
	reported := make(chan string, 1)
	services.SetConsoleShellKilledObserver(func(terminalSessionID string) {
		reported <- terminalSessionID
	})
	defer services.SetConsoleShellKilledObserver(nil)

	ttServer := newFakeTerminalTrainer(t, services.ConsoleShellKilledCloseCode, "exec_failed")
	defer ttServer.Close()

	_ = runConsoleProxy(t, ttServer, "terminal-permadeath-1")

	select {
	case terminalSessionID := <-reported:
		assert.Equal(t, "terminal-permadeath-1", terminalSessionID,
			"the observer must receive the terminal session whose shell was killed")
	case <-time.After(2 * time.Second):
		t.Fatal("a SIGKILLed shell must be reported so crash-trap runs can end")
	}
}

func TestConsoleRelay_DoesNotReportLearnerInitiatedExit(t *testing.T) {
	// The guard that keeps `exit 1` from ending a learner's run.
	reported := make(chan string, 1)
	services.SetConsoleShellKilledObserver(func(terminalSessionID string) {
		reported <- terminalSessionID
	})
	defer services.SetConsoleShellKilledObserver(nil)

	ttServer := newFakeTerminalTrainer(t, 4001, "exec_failed")
	defer ttServer.Close()

	_ = runConsoleProxy(t, ttServer, "terminal-exit1")

	select {
	case terminalSessionID := <-reported:
		t.Fatalf("close code 4001 (learner ran `exit 1`) must never be reported as "+
			"a kill — it would end the run of anyone who exits their own shell. "+
			"Got a report for %q", terminalSessionID)
	case <-time.After(300 * time.Millisecond):
	}
}
