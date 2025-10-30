package testutils

import (
	"fmt"
	"reflect"
	"testing"
)

// AssertNoError fails the test if err is not nil.
//
// Usage:
//
//	AssertNoError(t, err)
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// AssertError fails the test if err is nil.
//
// Usage:
//
//	AssertError(t, err)
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
}

// AssertEqual fails the test if expected != actual.
//
// Usage:
//
//	AssertEqual(t, "expected", actual)
func AssertEqual(t *testing.T, expected, actual any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %v (type %T), got %v (type %T)", expected, expected, actual, actual)
	}
}

// AssertNotEqual fails the test if expected == actual.
//
// Usage:
//
//	AssertNotEqual(t, "old", new)
func AssertNotEqual(t *testing.T, expected, actual any) {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected values to be different, but both are %v", expected)
	}
}

// AssertNil fails the test if value is not nil.
//
// Usage:
//
//	AssertNil(t, value)
func AssertNil(t *testing.T, value any) {
	t.Helper()
	if value != nil && !reflect.ValueOf(value).IsNil() {
		t.Errorf("Expected nil, got %v", value)
	}
}

// AssertNotNil fails the test if value is nil.
//
// Usage:
//
//	AssertNotNil(t, value)
func AssertNotNil(t *testing.T, value any) {
	t.Helper()
	if value == nil || reflect.ValueOf(value).IsNil() {
		t.Error("Expected non-nil value, got nil")
	}
}

// AssertTrue fails the test if condition is false.
//
// Usage:
//
//	AssertTrue(t, len(items) > 0)
func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Errorf("Expected true: %s", message)
	}
}

// AssertFalse fails the test if condition is true.
//
// Usage:
//
//	AssertFalse(t, items == nil, "items should not be nil")
func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()
	if condition {
		t.Errorf("Expected false: %s", message)
	}
}

// AssertContains fails the test if slice doesn't contain item.
//
// Usage:
//
//	AssertContains(t, []string{"a", "b", "c"}, "b")
func AssertContains[T comparable](t *testing.T, slice []T, item T) {
	t.Helper()
	for _, v := range slice {
		if v == item {
			return
		}
	}
	t.Errorf("Expected slice to contain %v, but it doesn't. Slice: %v", item, slice)
}

// AssertLen fails the test if the length of slice != expected.
//
// Usage:
//
//	AssertLen(t, items, 5)
func AssertLen(t *testing.T, slice any, expected int) {
	t.Helper()
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		t.Fatalf("AssertLen called on non-slice/array: %T", slice)
	}
	actual := v.Len()
	if actual != expected {
		t.Errorf("Expected length %d, got %d", expected, actual)
	}
}

// AssertPanic fails the test if f doesn't panic.
//
// Usage:
//
//	AssertPanic(t, func() { someFunction() })
func AssertPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected function to panic, but it didn't")
		}
	}()
	f()
}

// AssertErrorContains fails if err is nil or doesn't contain substring.
//
// Usage:
//
//	AssertErrorContains(t, err, "not found")
func AssertErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error containing %q, got nil", substring)
	}
	if !contains(err.Error(), substring) {
		t.Errorf("Expected error to contain %q, got: %v", substring, err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// AssertFieldEqual checks if a specific field of a struct equals expected value.
//
// Usage:
//
//	AssertFieldEqual(t, entity, "Title", "Expected Title")
func AssertFieldEqual(t *testing.T, obj any, fieldName string, expected any) {
	t.Helper()

	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		t.Fatalf("AssertFieldEqual: expected struct, got %T", obj)
	}

	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("AssertFieldEqual: field %q not found in %T", fieldName, obj)
	}

	actual := field.Interface()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Field %q: expected %v, got %v", fieldName, expected, actual)
	}
}

// AssertWithinDelta fails if the difference between expected and actual is greater than delta.
//
// Usage:
//
//	AssertWithinDelta(t, 3.14, actualPi, 0.01)
func AssertWithinDelta(t *testing.T, expected, actual, delta float64) {
	t.Helper()
	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}
	if diff > delta {
		t.Errorf("Expected %v ± %v, got %v (diff: %v)", expected, delta, actual, diff)
	}
}

// AssertMapContains fails if map doesn't contain the key.
//
// Usage:
//
//	AssertMapContains(t, myMap, "key")
func AssertMapContains[K comparable, V any](t *testing.T, m map[K]V, key K) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("Expected map to contain key %v, but it doesn't. Keys: %v", key, mapKeys(m))
	}
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// AssertJSONEqual compares two objects by marshaling them to JSON.
// Useful for comparing complex structs ignoring unexported fields.
//
// Usage:
//
//	AssertJSONEqual(t, expected, actual)
func AssertJSONEqual(t *testing.T, expected, actual any) {
	t.Helper()

	// For simplicity, use reflect.DeepEqual
	// A full implementation would use json.Marshal
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("JSON objects not equal:\nExpected: %+v\nActual:   %+v", expected, actual)
	}
}

// Failf formats a failure message and fails the test.
//
// Usage:
//
//	Failf(t, "Expected %d items, got %d", 5, len(items))
func Failf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Errorf(format, args...)
}

// FatalIf fails the test immediately if condition is true.
//
// Usage:
//
//	FatalIf(t, err != nil, "Setup failed: %v", err)
func FatalIf(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if condition {
		t.Fatalf(format, args...)
	}
}

// LogIf logs a message only if condition is true.
//
// Usage:
//
//	LogIf(t, verbose, "Processing item %d", i)
func LogIf(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if condition {
		t.Logf(format, args...)
	}
}

// Must fails the test if err is not nil, otherwise returns value.
// Useful for inline error checking in table-driven tests.
//
// Usage:
//
//	entity := Must(t, repo.GetEntity(id))
func Must[T any](t *testing.T, value T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatalf("Must: unexpected error: %v", err)
	}
	return value
}

// MustNot fails the test if value is nil.
//
// Usage:
//
//	entity := MustNot(t, findEntity())
func MustNot[T any](t *testing.T, value *T) *T {
	t.Helper()
	if value == nil {
		t.Fatal("MustNot: unexpected nil value")
	}
	return value
}

// AssertEventually retries condition function until it returns true or timeout.
// Not implemented in this basic version - would require time-based testing.
//
// Placeholder for future enhancement.
func AssertEventually(t *testing.T, condition func() bool, message string) {
	t.Helper()
	// Simple implementation - just check once
	if !condition() {
		t.Errorf("Condition not met: %s", message)
	}
}

// TableTest is a helper for running table-driven tests.
//
// Usage:
//
//	tests := []struct{
//	    name string
//	    input int
//	    want int
//	}{
//	    {"positive", 5, 10},
//	    {"negative", -5, -10},
//	}
//
//	TableTest(t, tests, func(t *testing.T, tc testCase) {
//	    got := double(tc.input)
//	    AssertEqual(t, tc.want, got)
//	})
func TableTest[T any](t *testing.T, tests []T, testFunc func(*testing.T, T)) {
	t.Helper()
	for i, tc := range tests {
		// Try to get name from struct field
		v := reflect.ValueOf(tc)
		var name string
		if v.Kind() == reflect.Struct {
			nameField := v.FieldByName("name")
			if nameField.IsValid() && nameField.Kind() == reflect.String {
				name = nameField.String()
			}
		}
		if name == "" {
			name = fmt.Sprintf("test_%d", i)
		}

		t.Run(name, func(t *testing.T) {
			testFunc(t, tc)
		})
	}
}
