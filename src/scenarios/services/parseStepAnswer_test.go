package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseStepAnswer_ReadsTheDeclaredAnswer(t *testing.T) {
	assert.Equal(t, "Tuesday", parseStepAnswer("OCF_ANSWER: Tuesday\n"))
	assert.Equal(t, "Tuesday", parseStepAnswer("building the world\nOCF_ANSWER: Tuesday\ndone\n"))
	assert.Equal(t, "Tuesday", parseStepAnswer("   OCF_ANSWER:   Tuesday   \n"))
}

func TestParseStepAnswer_LastOneWins(t *testing.T) {
	// A script that recomputes must not be betrayed by an earlier draft.
	assert.Equal(t, "Friday", parseStepAnswer("OCF_ANSWER: Tuesday\nOCF_ANSWER: Friday\n"))
}

func TestParseStepAnswer_IgnoresAMentionMidLine(t *testing.T) {
	// Talking about the marker is not declaring an answer.
	assert.Equal(t, "", parseStepAnswer("write OCF_ANSWER: Tuesday to declare it\n"))
}

func TestParseStepAnswer_EmptyWhenNothingIsDeclared(t *testing.T) {
	assert.Equal(t, "", parseStepAnswer(""))
	assert.Equal(t, "", parseStepAnswer("built the observatory\n"))
	assert.Equal(t, "", parseStepAnswer("OCF_ANSWER:\n"),
		"a marker with nothing after it declares nothing, and must not blank the flag")
}
