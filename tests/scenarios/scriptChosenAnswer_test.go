// tests/scenarios/scriptChosenAnswer_test.go
//
// Some questions only have an answer once the session picks one. "What day of
// the week was this date?" is a fine exercise and a poor flag: with a fixed
// date the answer is the same for every student and travels between them in a
// single message. The scenario draws the date per session, so only the scenario
// knows the answer — and it needs a way to tell OCF what it chose.
//
// These pin the two halves: a background script can replace its step's
// generated token with a word, and that word is then graded the way a human
// types it.
package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/scenarios/services"
)

func TestValidateFlag_GeneratedTokenIsComparedExactly(t *testing.T) {
	svc := services.NewFlagService()
	const token = "FLAG{0123456789abcdef}"

	assert.True(t, svc.ValidateFlag(token, token))
	assert.False(t, svc.ValidateFlag(token, "flag{0123456789abcdef}"),
		"a token is copied from the screen, so case is part of it")
	assert.False(t, svc.ValidateFlag(token, "FLAG{0123456789abcdee}"))
}

func TestValidateFlag_ScenarioAnswerIgnoresCaseAndSpace(t *testing.T) {
	svc := services.NewFlagService()

	for _, submitted := range []string{"Tuesday", "tuesday", "TUESDAY", "  Tuesday  ", "\tTuesday\n"} {
		assert.True(t, svc.ValidateFlag("Tuesday", submitted),
			"a word a human types is the same answer however they capitalise it: %q", submitted)
	}
}

func TestValidateFlag_ScenarioAnswerStillRejectsTheWrongWord(t *testing.T) {
	svc := services.NewFlagService()

	assert.False(t, svc.ValidateFlag("Tuesday", "Wednesday"))
	assert.False(t, svc.ValidateFlag("Tuesday", ""),
		"an empty submission must never pass — it is what an unanswered box sends")
	assert.False(t, svc.ValidateFlag("Tuesday", "Tues"))
}

// The marker travels on the background script's stdout, which OCF already
// reads. These pin the parsing rules, because a script that fails to declare
// its answer leaves the step holding a token nobody can guess.
