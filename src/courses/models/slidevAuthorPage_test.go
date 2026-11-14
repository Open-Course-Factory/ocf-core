package models

import (
	"strings"
	"testing"
)

// SubstituteAuthorPage replaces the @@author_page_content@@ placeholder emitted by
// SlidevCourseWriter.SetAuthorPage. The CLI (src/cli/course_generator.go) calls it instead of a raw
// strings.ReplaceAll, so an empty author page does not leave an empty slide behind.

func authorSlides(markdown string) []slide {
	var found []slide
	for _, s := range parseSlides(markdown) {
		if s.layout == "default" {
			found = append(found, s)
		}
	}
	return found
}

func TestSlidevAuthorPage_EmptyContent_NoAuthorSlide(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "whitespace only": " \n\n"} {
		t.Run(name, func(t *testing.T) {
			course := (&SlidevCourseWriter{Course: framingTestCourse(2)}).GetCourse()
			out := SubstituteAuthorPage(course, content)

			if strings.Contains(out, "@@author_page_content@@") {
				t.Fatalf("placeholder left in the output")
			}
			if n := len(authorSlides(out)); n != 0 {
				t.Errorf("expected no author slide for an empty author page, got %d", n)
			}
		})
	}
}

func TestSlidevAuthorPage_WithContent_KeepsAuthorSlide(t *testing.T) {
	content := "**Jeanne Formatrice**\n\nFormatrice Linux depuis 2010"
	course := (&SlidevCourseWriter{Course: framingTestCourse(2)}).GetCourse()
	out := SubstituteAuthorPage(course, content)

	if strings.Contains(out, "@@author_page_content@@") {
		t.Fatalf("placeholder left in the output")
	}
	slides := authorSlides(out)
	if len(slides) != 1 {
		t.Fatalf("expected exactly 1 author slide, got %d", len(slides))
	}
	if !strings.Contains(slides[0].body, content) {
		t.Errorf("author slide does not contain the author page content, got body %q", slides[0].body)
	}
}

func TestSlidevAuthorPage_EmptyContent_KeepsOtherSlides(t *testing.T) {
	course := (&SlidevCourseWriter{Course: framingTestCourse(2)}).GetCourse()
	before := len(parseSlides(course))
	after := len(parseSlides(SubstituteAuthorPage(course, "")))

	if after != before-1 {
		t.Errorf("expected only the author slide to be removed (%d -> %d slides), got %d", before, before-1, after)
	}
}
