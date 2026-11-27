package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// localMachineBootAppendMaxLength caps the kernel command line an
// administrator adds: it travels as a URL parameter to the learner's page.
const localMachineBootAppendMaxLength = 512

// LocalMachineBoot is how unkernel boots a machine in the learner's browser.
// A nil field is unset: the platform defaults leave it to the built-in value,
// a scenario leaves it to the platform defaults.
type LocalMachineBoot struct {
	JIT     *bool `json:"jit,omitempty"`
	Verbose *bool `json:"verbose,omitempty"`
	Display *bool `json:"display,omitempty"`
	Sound   *bool `json:"sound,omitempty"`
	// Append is added to the machine's kernel command line. Only the
	// platform defaults may set it; see ScenarioMachineBootProblem.
	Append        *string `json:"append,omitempty"`
	ConsoleHidden *bool   `json:"console_hidden,omitempty"`
}

// BuiltInLocalMachineBoot is the boot used when an administrator stored
// nothing: every field set.
func BuiltInLocalMachineBoot() LocalMachineBoot {
	yes, no, none := true, false, ""
	return LocalMachineBoot{JIT: &yes, Verbose: &no, Display: &yes, Sound: &yes, Append: &none, ConsoleHidden: &no}
}

// ParseLocalMachineBoot reads a stored or submitted boot object. "" is an
// empty object; an unknown key, a second value or a bad append is an error.
func ParseLocalMachineBoot(raw string) (LocalMachineBoot, error) {
	var boot LocalMachineBoot
	if strings.TrimSpace(raw) == "" {
		return boot, nil
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&boot); err != nil {
		return LocalMachineBoot{}, fmt.Errorf("invalid machine boot options: %w", err)
	}
	if decoder.More() {
		return LocalMachineBoot{}, fmt.Errorf("invalid machine boot options: more than one JSON value")
	}
	if boot.Append != nil {
		if problem := localMachineBootAppendProblem(*boot.Append); problem != "" {
			return LocalMachineBoot{}, fmt.Errorf("%s", problem)
		}
	}
	return boot, nil
}

func localMachineBootAppendProblem(value string) string {
	if len(value) > localMachineBootAppendMaxLength {
		return fmt.Sprintf("append is longer than %d characters", localMachineBootAppendMaxLength)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "append must not contain control characters or line breaks"
	}
	return ""
}

// ScenarioMachineBootProblem says what is wrong with a scenario's
// machine_boot value, "" when nothing is. A scenario may not set append:
// the kernel command line belongs to the platform's administrators.
func ScenarioMachineBootProblem(raw string) string {
	boot, err := ParseLocalMachineBoot(raw)
	if err != nil {
		return err.Error()
	}
	if boot.Append != nil {
		return "machine_boot: append is set by the platform only"
	}
	return ""
}

// Over returns base with every field this boot sets replacing base's.
func (b LocalMachineBoot) Over(base LocalMachineBoot) LocalMachineBoot {
	return LocalMachineBoot{
		JIT:           setOr(b.JIT, base.JIT),
		Verbose:       setOr(b.Verbose, base.Verbose),
		Display:       setOr(b.Display, base.Display),
		Sound:         setOr(b.Sound, base.Sound),
		Append:        setOr(b.Append, base.Append),
		ConsoleHidden: setOr(b.ConsoleHidden, base.ConsoleHidden),
	}
}

func setOr[T any](value, fallback *T) *T {
	if value != nil {
		return value
	}
	return fallback
}

// EncodeScenarioMachineBoot is the machine_boot column for a scenario's
// overrides; none is "".
func EncodeScenarioMachineBoot(boot *LocalMachineBoot) (string, error) {
	if boot == nil {
		return "", nil
	}
	encoded, err := json.Marshal(boot)
	return string(encoded), err
}

// DecodeScenarioMachineBoot reads a scenario's machine_boot column back; ""
// or an unreadable value is no overrides.
func DecodeScenarioMachineBoot(raw string) *LocalMachineBoot {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	boot, err := ParseLocalMachineBoot(raw)
	if err != nil {
		return nil
	}
	return &boot
}
