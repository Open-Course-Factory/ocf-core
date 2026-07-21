package terminalController

// RED tests for the supervise-proxy self-attachment binding (live incident).
//
// SuperviseSession's upstream pump (supervisionController.go) must learn its OWN
// tt-backend attachment id so its take-hand / release-hand PATCHes address the
// supervisor's attachment and never the learner's console. The old heuristic —
// "the FIRST binary control frame carrying an attachment_id is ours" — bound the
// NEXT attachment's id whenever the supervisor's attachment was the hub's first
// (tt-backend then sent no self-snapshot). A supervisor's release-hand
// consequently demoted the LEARNER's own console. Incident of today.
//
// tt-backend is being fixed in parallel to send, as the FIRST event frame to
// every control attachment, an explicit self-snapshot:
//     {"type":"attachment","event":"self","attachment_id":"<own id>"}
// distinct from the "joined" broadcast for other attachments (closes the
// tt-backend #126 heuristic referenced in the pump comment). ocf-core must now
// bind ONLY on that self frame.
//
// SEAM CONTRACT pinned for backend-dev (referenced below; not yet implemented):
//
//   // bindSelfAttachmentID returns the attachment id the supervise proxy should
//   // be bound to after observing frame `data`, given it is currently bound to
//   // `current` ("" = not yet bound). It binds ONLY on the tt-backend
//   // self-snapshot control frame (type=="attachment" && event=="self") carrying
//   // a non-empty attachment_id, and only while not already bound (first self
//   // frame wins). Any other frame — a "joined" broadcast for another
//   // attachment, a non-attachment event, or malformed/empty bytes — returns
//   // `current` unchanged.
//   func bindSelfAttachmentID(current string, data []byte) string
//
// The pump's upstream goroutine replaces its inline unmarshal+guard with:
//   stMu.Lock(); attachmentID = bindSelfAttachmentID(attachmentID, data); stMu.Unlock()
//
// This is an internal white-box test (package terminalController) so the seam
// stays unexported, mirroring src/scenarios/routes/instance_selection_test.go.
// Referencing the not-yet-existing symbol is unavoidable — the binding decision
// is currently inline in a WebSocket goroutine with no testable surface. The
// compile dependency is isolated to THIS package (go build ./... is unaffected;
// _test.go files are excluded from builds), so the committed tests/terminalTrainer
// suite is untouched.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBindSelfAttachmentID_SelfFrameBinds pins that the explicit self-snapshot
// frame binds its own attachment id when the proxy is not yet bound.
func TestBindSelfAttachmentID_SelfFrameBinds(t *testing.T) {
	self := []byte(`{"type":"attachment","event":"self","attachment_id":"obs-1"}`)

	got := bindSelfAttachmentID("", self)

	assert.Equal(t, "obs-1", got, "the self-snapshot frame must bind its own attachment id")
}

// TestBindSelfAttachmentID_JoinedFrameDoesNotBind is the incident guard: a
// "joined" broadcast carrying ANOTHER attachment's id (e.g. the learner's own
// console joining) must NOT bind, even though it carries a non-empty
// attachment_id. Binding it is exactly what let a release-hand demote the
// learner's console.
func TestBindSelfAttachmentID_JoinedFrameDoesNotBind(t *testing.T) {
	joined := []byte(`{"type":"attachment","event":"joined","attachment_id":"learner-console-9"}`)

	got := bindSelfAttachmentID("", joined)

	assert.Equal(t, "", got, "a joined broadcast for another attachment must not bind the proxy")
}

// TestBindSelfAttachmentID_MalformedOrIncompleteDoesNotBind pins that anything
// that is not a well-formed self frame with a non-empty attachment_id leaves the
// binding untouched: empty bytes, non-JSON, a JSON object lacking the type, an
// attachment event that is neither self nor id-bearing, a self frame with an
// empty id, and a terminal-output binary frame that happens to be non-JSON.
func TestBindSelfAttachmentID_MalformedOrIncompleteDoesNotBind(t *testing.T) {
	cases := map[string][]byte{
		"empty bytes":               []byte(``),
		"non-JSON garbage":          []byte(`\x00\x01not json`),
		"empty JSON object":         []byte(`{}`),
		"attachment event missing":  []byte(`{"type":"attachment","attachment_id":"x-1"}`),
		"self event but empty id":   []byte(`{"type":"attachment","event":"self","attachment_id":""}`),
		"self event but wrong type": []byte(`{"type":"session","event":"self","attachment_id":"x-1"}`),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			got := bindSelfAttachmentID("", data)
			assert.Equal(t, "", got, "a non-self / incomplete frame must not bind")
		})
	}
}

// TestBindSelfAttachmentID_FirstSelfWins pins the defensive first-arrival
// precedence: once bound, a SECOND self frame (were tt-backend to send one)
// must NOT rebind the proxy to a different attachment id.
func TestBindSelfAttachmentID_FirstSelfWins(t *testing.T) {
	secondSelf := []byte(`{"type":"attachment","event":"self","attachment_id":"obs-2"}`)

	got := bindSelfAttachmentID("obs-1", secondSelf)

	assert.Equal(t, "obs-1", got, "once bound, a later self frame must not rebind (first self wins)")
}
