package services

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"soli/formations/src/scenarios/models"
)

// ScenarioContentError lists everything wrong with an imported scenario, each
// problem naming the step and the field. It is the author's mistake, not the
// server's, and every one is reported at once: an author fixing a file one
// refusal at a time — often by pasting it back into an assistant — gives up
// long before the last one.
type ScenarioContentError struct {
	Problems []string
}

func (e *ScenarioContentError) Error() string {
	return "the scenario is not valid: " + strings.Join(e.Problems, "; ")
}

// contentErrorOrNil keeps a nil error nil: a typed nil pointer in an error
// interface is not equal to nil, and every caller would then fail.
func contentErrorOrNil(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return &ScenarioContentError{Problems: problems}
}

var (
	knownStepTypes     = []string{models.StepTypeTerminal, models.StepTypeFlag, "quiz", "info"}
	knownQuestionTypes = []string{"multiple_choice", "multi_answer", "free_text", "true_false"}
)

// ScenarioContentProblems checks a scenario built from imported content,
// before anything is written. The seed, import-json and archive paths all
// build a models.Scenario first and all call this, so a file means the same
// thing however it arrives.
//
// ocf-front's src/utils/scenarioAiPrompt.ts documents the rules below for
// teachers' AI assistants; update it with any change here.
//
// Steps must already be through models.NormalizeFlagStep, which every builder
// does: the type checked here is the type that would be stored.
func ScenarioContentProblems(scenario *models.Scenario) []string {
	var problems []string
	if strings.TrimSpace(scenario.Title) == "" {
		problems = append(problems, "title is empty")
	}
	if scenario.Hostname != "" {
		if err := ValidateScenarioHostname(scenario.Hostname); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(scenario.Steps) == 0 {
		problems = append(problems, "the scenario has no steps")
	}
	for i, step := range scenario.Steps {
		problems = append(problems, stepContentProblems(i+1, step)...)
	}
	return problems
}

func stepContentProblems(number int, step models.ScenarioStep) []string {
	label := fmt.Sprintf("step %d (%s)", number, step.Title)
	var problems []string
	if strings.TrimSpace(step.Title) == "" {
		problems = append(problems, fmt.Sprintf("step %d: title is empty", number))
	}
	if !slices.Contains(knownStepTypes, step.StepType) {
		// Nothing else about the step can be judged without knowing its kind.
		return append(problems, fmt.Sprintf("%s: step_type %q is not one of %s",
			label, step.StepType, strings.Join(knownStepTypes, ", ")))
	}
	if !stepHasAWayThrough(step) {
		problems = append(problems, label+": a quiz step needs at least one question")
	}
	for i, q := range step.Questions {
		if problem := questionProblem(q); problem != "" {
			problems = append(problems, fmt.Sprintf("%s, question %d: %s", label, i+1, problem))
		}
	}
	return problems
}

// questionProblem says what is wrong with one quiz question, or "".
func questionProblem(q models.ScenarioStepQuestion) string {
	if strings.TrimSpace(q.QuestionText) == "" {
		return "question_text is empty"
	}
	switch q.QuestionType {
	case "multiple_choice":
		return optionAnswerProblem(q.Options, q.CorrectAnswer, singleOptionIndex)
	case "multi_answer":
		return optionAnswerProblem(q.Options, q.CorrectAnswer, sortedOptionIndexes)
	case "true_false":
		if q.CorrectAnswer != "true" && q.CorrectAnswer != "false" {
			return fmt.Sprintf("correct_answer %q must be \"true\" or \"false\"", q.CorrectAnswer)
		}
	case "free_text":
		if strings.TrimSpace(q.CorrectAnswer) == "" {
			return "correct_answer is empty"
		}
	default:
		return fmt.Sprintf("question_type %q is not one of %s", q.QuestionType, strings.Join(knownQuestionTypes, ", "))
	}
	return ""
}

// optionAnswerProblem checks a choice question's options, then its answer
// with answerProblem.
func optionAnswerProblem(rawOptions, correctAnswer string, answerProblem func(answer string, options int) string) string {
	var options []string
	if err := json.Unmarshal([]byte(rawOptions), &options); err != nil {
		return fmt.Sprintf("options must be a JSON array of strings encoded as a string, like \"[\\\"yes\\\", \\\"no\\\"]\" (got %q)", rawOptions)
	}
	if len(options) < 2 {
		return fmt.Sprintf("options needs at least 2 choices (got %d)", len(options))
	}
	return answerProblem(correctAnswer, len(options))
}

// The grader compares the learner's submission to correct_answer as a string,
// so the answer must be spelled exactly as the quiz panel submits it: an
// option's index for multiple_choice, the sorted index array for multi_answer.

func singleOptionIndex(answer string, options int) string {
	index, err := strconv.Atoi(answer)
	if err != nil || index < 0 || index >= options || strconv.Itoa(index) != answer {
		return fmt.Sprintf("correct_answer %q is not an option index (0 to %d for %d options)", answer, options-1, options)
	}
	return ""
}

func sortedOptionIndexes(answer string, options int) string {
	var indexes []int
	canonical := false
	if json.Unmarshal([]byte(answer), &indexes) == nil && len(indexes) > 0 {
		encoded, _ := json.Marshal(indexes)
		canonical = string(encoded) == answer && slices.IsSorted(indexes) &&
			len(slices.Compact(slices.Clone(indexes))) == len(indexes) &&
			indexes[0] >= 0 && indexes[len(indexes)-1] < options
	}
	if !canonical {
		return fmt.Sprintf("correct_answer %q must list the right option indexes (0 to %d), sorted, without spaces, like \"[0,2]\"", answer, options-1)
	}
	return ""
}
