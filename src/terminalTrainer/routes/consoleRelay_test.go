package terminalController

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	services "soli/formations/src/terminalTrainer/services"
)

// The console proxy used to drop tt-backend's close frame on the floor: the
// relay loop broke on the read error and gorilla closed the browser socket
// without a close frame, so the browser only ever saw 1006 (abnormal closure)
// and offered a "your environment is still running — Reconnect" overlay. These
// tests pin that tt-backend's structured close code now reaches the browser
// unchanged, and that a SIGKILLed shell is reported for permadeath handling.

// newFakeTerminalTrainer serves one websocket connection that sends greeting as
// console output (none when empty) and then closes with the given code and
// reason, mimicking tt-backend's exec-exit teardown (execCloseCode in
// backend/api_session_console.go).
func newFakeTerminalTrainer(t *testing.T, greeting string, closeCode int, closeReason string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if greeting != "" {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(greeting))
		}
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
	ttServer := newFakeTerminalTrainer(t, "", services.ConsoleShellKilledCloseCode, "exec_failed")
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
	ttServer := newFakeTerminalTrainer(t, "", 4001, "exec_failed")
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

	ttServer := newFakeTerminalTrainer(t, "", services.ConsoleShellKilledCloseCode, "exec_failed")
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
	assertCloseNotReported(t, 4001, "exec_failed", "terminal-exit1",
		"(learner ran `exit 1`) must never be reported as a kill — it would end "+
			"the run of anyone who exits their own shell")
}

func TestConsoleRelay_RelaysSessionStoppedWithoutReportingIt(t *testing.T) {
	// tt-backend closes the console with 4300 "session_stopped" whenever the
	// platform stops or deletes the session (tt#145). That is a pause or a
	// teardown, never a crash: it must reach the browser unchanged so the
	// front can tell them apart, and it must never end a crash-trap run.
	err := assertCloseNotReported(t, services.ConsoleSessionStoppedCloseCode,
		"session_stopped", "terminal-session-stopped",
		"(the platform stopped the session) must never be reported as a kill — "+
			"pausing a crash-trap run would delete it")

	closeErr, ok := err.(*websocket.CloseError)
	require.True(t, ok, "expected a close frame, got %v", err)
	assert.Equal(t, services.ConsoleSessionStoppedCloseCode, closeErr.Code)
	assert.Equal(t, "session_stopped", closeErr.Text)
}

// assertCloseNotReported relays a console that closes with closeCode and
// closeReason, fails if the shell-killed observer hears about it, and returns
// the close error the browser observed.
func assertCloseNotReported(t *testing.T, closeCode int, closeReason, terminalSessionID, why string) error {
	t.Helper()
	reported := make(chan string, 1)
	services.SetConsoleShellKilledObserver(func(terminalSessionID string) {
		reported <- terminalSessionID
	})
	defer services.SetConsoleShellKilledObserver(nil)

	ttServer := newFakeTerminalTrainer(t, "", closeCode, closeReason)
	defer ttServer.Close()

	err := runConsoleProxy(t, ttServer, terminalSessionID)

	select {
	case got := <-reported:
		t.Fatalf("close code %d %s. Got a report for %q", closeCode, why, got)
	case <-time.After(300 * time.Millisecond):
	}
	return err
}

// TestIsShellKilledCloseCode pins that only a SIGKILLed shell (4137) arms
// permadeath. Every other non-zero exit shares the 4000-4999 band and must
// leave the run alone.
func TestIsShellKilledCloseCode(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"SIGKILL (crash trap, or an Incus-side force-kill)", services.ConsoleShellKilledCloseCode, true},
		{"learner ran `exit 1`", 4001, false},
		{"SIGHUP from a dropped console", 4129, false},
		// Defensive: a learner or a script SIGTERMing the shell. A pause does
		// not produce it — Incus's graceful stop signals init (SIGPWR/halt),
		// not the shell.
		{"SIGTERM of the shell", 4143, false},
		{"platform stopped the session (tt#145)", services.ConsoleSessionStoppedCloseCode, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, services.IsShellKilledCloseCode(tc.code))
		})
	}
}

// A step's foreground script is typed into the learner's live shell, so it can
// only be delivered once that shell has a console attached — which never
// happens while a run is provisioning. The learner's console attach is
// therefore published, the same inversion as the shell-killed report above,
// and the scenarios module types the pending script on it.
//
// Only the learner's own attach counts. A supervisor watching the terminal, or a
// teacher or admin opening it through the console route, is not the learner
// sitting down at their shell: typing the demonstration then would play it to
// the wrong audience and use it up before the learner ever saw it.

// observeConsoleAttach registers an attach observer for the test and returns
// the channel it reports on.
func observeConsoleAttach(t *testing.T) chan string {
	t.Helper()
	attached := make(chan string, 4)
	services.SetConsoleAttachedObserver(func(terminalSessionID string) {
		attached <- terminalSessionID
	})
	t.Cleanup(func() { services.SetConsoleAttachedObserver(nil) })
	return attached
}

// consoleRequest is the gin context of a console request as the auth
// middlewares leave it: userId is the effective user, and impersonatorId is set
// only when an admin is acting as that user.
func consoleRequest(userID, impersonatorID string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("userId", userID)
	if impersonatorID != "" {
		ctx.Set("impersonatorId", impersonatorID)
	}
	return ctx
}

func assertNoAttachReported(t *testing.T, attached chan string, why string) {
	t.Helper()
	select {
	case got := <-attached:
		t.Fatalf("%s. Got an attach report for %q", why, got)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestConsoleRelay_ReportsLearnerAttach(t *testing.T) {
	attached := observeConsoleAttach(t)
	terminal := &models.Terminal{SessionID: "terminal-learner-attach", UserID: "learner-1"}

	reportLearnerAttach(consoleRequest("learner-1", ""), terminal)

	select {
	case terminalSessionID := <-attached:
		// The tt-backend session id, like ReportConsoleClose: it is what a
		// scenario run records and what the console input is addressed to.
		assert.Equal(t, "terminal-learner-attach", terminalSessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("the learner opening their own console must be reported, " +
			"otherwise a step's foreground script is never typed")
	}
	assertNoAttachReported(t, attached, "one attach must be reported once")
}

// A teacher or an admin may open a learner's terminal through the console route
// itself (hasTerminalAccess lets group owners and admins in). That is still not
// the learner's attach.
func TestConsoleRelay_TeacherConsoleAttachIsNotReported(t *testing.T) {
	attached := observeConsoleAttach(t)
	terminal := &models.Terminal{SessionID: "terminal-teacher-attach", UserID: "learner-1"}

	reportLearnerAttach(consoleRequest("teacher-1", ""), terminal)

	assertNoAttachReported(t, attached,
		"a console opened by someone other than the terminal's owner must not "+
			"be reported as the learner's attach")
}

// An admin impersonating the learner opens the console as the learner — userId
// is the owner — but it is still the admin looking, not the learner. Typing the
// pending demonstration then would spend it on the admin.
func TestConsoleRelay_ImpersonatedAttachIsNotReported(t *testing.T) {
	attached := observeConsoleAttach(t)
	terminal := &models.Terminal{SessionID: "terminal-impersonated-attach", UserID: "learner-1"}

	reportLearnerAttach(consoleRequest("learner-1", "admin-1"), terminal)

	assertNoAttachReported(t, attached,
		"a console opened under impersonation must not be reported as the "+
			"learner's attach, even though the effective user owns the terminal")
}

func TestConsoleRelay_SupervisionAttachIsNotReported(t *testing.T) {
	attached := observeConsoleAttach(t)

	ttServer := newFakeTerminalTrainer(t, "learner@lab:~$ ", websocket.CloseNormalClosure, "")
	defer ttServer.Close()

	upgrader := websocket.Upgrader{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		upstream, dialErr := dialSupervisionUpstream(
			"ws"+strings.TrimPrefix(ttServer.URL, "http"), "owner-key")
		if dialErr != nil {
			clientConn.Close()
			return
		}
		broker := &superviseBroker{clientConn: clientConn, upstream: upstream, ttSessionID: "terminal-supervised"}
		broker.teardown = func() { clientConn.Close(); upstream.Close() }
		broker.pumpUpstream()
	}))
	defer proxy.Close()

	browserConn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(proxy.URL, "http"), nil)
	require.NoError(t, err)
	defer browserConn.Close()
	_ = browserConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := browserConn.ReadMessage()
	require.NoError(t, err, "the supervisor must still see the learner's output")
	assert.Equal(t, "learner@lab:~$ ", string(data))

	assertNoAttachReported(t, attached,
		"a supervisor observing the terminal must never be reported as the "+
			"learner's attach")
}
