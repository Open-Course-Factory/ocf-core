package testutils_test

import (
	"testing"

	"soli/formations/tests/entityManagement/testutils"
)

// Example test entity
type ExampleEntity struct {
	ID    string
	Title string
	Count int
}

// Example showing how to use testutils

func TestExampleAssertions(t *testing.T) {
	// Test assertions
	testutils.AssertEqual(t, 5, 5)
	testutils.AssertNotEqual(t, 5, 10)
	testutils.AssertTrue(t, true, "should be true")
	testutils.AssertFalse(t, false, "should be false")

	slice := []string{"a", "b", "c"}
	testutils.AssertLen(t, slice, 3)
	testutils.AssertContains(t, slice, "b")

	entity := &ExampleEntity{Title: "Test"}
	testutils.AssertNotNil(t, entity)
	testutils.AssertFieldEqual(t, entity, "Title", "Test")
}

func TestExampleBuilders(t *testing.T) {
	// Test ID generation
	id := testutils.NewTestID()
	testutils.AssertNotEqual(t, "", id.String())

	// Test sequence
	seq := testutils.NewTestSequence("test")
	testutils.AssertEqual(t, "test-1", seq.Next())
	testutils.AssertEqual(t, "test-2", seq.Next())
	testutils.AssertEqual(t, 3, seq.NextInt())

	// Test slice builder
	entities := testutils.NewTestSlice(3, func(i int) *ExampleEntity {
		return &ExampleEntity{
			ID:    seq.Next(),
			Title: seq.WithPrefix("title").Next(),
			Count: i,
		}
	})
	testutils.AssertLen(t, entities, 3)
	testutils.AssertFieldEqual(t, entities[0], "Count", 0)
	testutils.AssertFieldEqual(t, entities[2], "Count", 2)
}

func TestExampleErrorHandling(t *testing.T) {
	// Example of error assertions
	var err error
	testutils.AssertNoError(t, err)

	// Simulate an error
	// err = errors.New("test error")
	// testutils.AssertError(t, err)
	// testutils.AssertErrorContains(t, err, "test")
}

func TestExampleTableDriven(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"double positive", 5, 10},
		{"double negative", -3, -6},
		{"double zero", 0, 0},
	}

	testutils.TableTest(t, tests, func(t *testing.T, tc struct {
		name  string
		input int
		want  int
	}) {
		got := tc.input * 2
		testutils.AssertEqual(t, tc.want, got)
	})
}
