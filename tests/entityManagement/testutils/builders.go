package testutils

import (
	"fmt"

	"github.com/google/uuid"
)

// TestEntityBuilder provides a fluent interface for building test entities.
//
// This is a generic pattern that can be adapted for specific entities.
// Example implementation for a Course entity:
//
//	type CourseBuilder struct {
//	    course *Course
//	}
//
//	func NewCourseBuilder() *CourseBuilder {
//	    return &CourseBuilder{
//	        course: &Course{
//	            ID:    uuid.New(),
//	            Title: "Default Course",
//	            Code:  "DEFAULT",
//	        },
//	    }
//	}
//
//	func (b *CourseBuilder) WithTitle(title string) *CourseBuilder {
//	    b.course.Title = title
//	    return b
//	}
//
//	func (b *CourseBuilder) Build() *Course {
//	    return b.course
//	}
//
// Usage:
//
//	course := NewCourseBuilder().
//	    WithTitle("Advanced Go").
//	    WithCode("GO-ADV").
//	    Build()

// NewTestID generates a new UUID for testing.
func NewTestID() uuid.UUID {
	return uuid.New()
}

// NewTestIDString generates a new UUID string for testing.
func NewTestIDString() string {
	return uuid.New().String()
}

// NewTestSlice creates a slice of n items using the builder function.
//
// Usage:
//
//	courses := NewTestSlice(5, func(i int) *Course {
//	    return &Course{
//	        ID:    uuid.New(),
//	        Title: fmt.Sprintf("Course %d", i),
//	    }
//	})
func NewTestSlice[T any](n int, builder func(int) T) []T {
	slice := make([]T, n)
	for i := 0; i < n; i++ {
		slice[i] = builder(i)
	}
	return slice
}

// NewTestMap creates a map with n entries using the builder function.
//
// Usage:
//
//	courseMap := NewTestMap(3, func(i int) (string, *Course) {
//	    return fmt.Sprintf("course-%d", i), &Course{Title: fmt.Sprintf("Course %d", i)}
//	})
func NewTestMap[K comparable, V any](n int, builder func(int) (K, V)) map[K]V {
	m := make(map[K]V, n)
	for i := 0; i < n; i++ {
		k, v := builder(i)
		m[k] = v
	}
	return m
}

// TestSequence generates sequential values for testing.
//
// Usage:
//
//	seq := NewTestSequence("user")
//	id1 := seq.Next() // "user-1"
//	id2 := seq.Next() // "user-2"
type TestSequence struct {
	prefix  string
	counter int
}

// NewTestSequence creates a new test sequence generator.
func NewTestSequence(prefix string) *TestSequence {
	return &TestSequence{prefix: prefix, counter: 0}
}

// Next returns the next value in the sequence.
func (s *TestSequence) Next() string {
	s.counter++
	return fmt.Sprintf("%s-%d", s.prefix, s.counter)
}

// NextInt returns the next integer in the sequence.
func (s *TestSequence) NextInt() int {
	s.counter++
	return s.counter
}

// Reset resets the sequence counter to 0.
func (s *TestSequence) Reset() {
	s.counter = 0
}

// WithPrefix creates a new sequence with a different prefix but same counter.
func (s *TestSequence) WithPrefix(prefix string) *TestSequence {
	return &TestSequence{prefix: prefix, counter: s.counter}
}
