package models

import (
	"strconv"
	"strings"
	"testing"
)

func tocTestCourse(themeName string, n int, title, intro string) Course {
	course := Course{Title: "Cours", Theme: &Theme{Name: themeName}}
	for i := 1; i <= n; i++ {
		course.Chapters = append(course.Chapters, &Chapter{Number: i, Title: title, Introduction: intro})
	}
	return course
}

func tocPages(toc string) int {
	return strings.Count(toc, "layout: maintoc")
}

func TestSetToc_SixOneLineChaptersFitOnOnePage(t *testing.T) {
	writer := SlidevCourseWriter{Course: tocTestCourse("sdv", 6, "Introduction", "Dans lequel nous aborderons les concepts de base")}
	if pages := tocPages(writer.SetToc()); pages != 1 {
		t.Fatalf("expected 1 TOC page for 6 one-line chapters, got %d", pages)
	}
}

func TestSetToc_LongIntroductionsAreSplitOverSeveralPages(t *testing.T) {
	longIntro := strings.Repeat("Comprendre et mettre en oeuvre une notion importante du module, ", 3)
	course := tocTestCourse("mds", 16, "U1 - Un titre de chapitre", longIntro)
	toc := (&SlidevCourseWriter{Course: course}).SetToc()
	layout := loadTocLayout("mds")
	perPage := (layout.LinesPerPage - 1) / layout.chapterLines(course.Chapters[0])
	expected := (16 + perPage - 1) / perPage
	if pages := tocPages(toc); pages != expected {
		t.Fatalf("expected %d TOC pages (%d chapters per page), got %d", expected, perPage, pages)
	}
	for i := 1; i <= 16; i++ {
		if !strings.Contains(toc, "Chapitre **"+strconv.Itoa(i)+"**") {
			t.Fatalf("chapter %d missing from the TOC", i)
		}
	}
}

func TestSetToc_A4ThemesAreNotPaginated(t *testing.T) {
	writer := SlidevCourseWriter{Course: tocTestCourse("sdvA4", 16, "Titre", strings.Repeat("x", 200))}
	if pages := tocPages(writer.SetToc()); pages != 1 {
		t.Fatalf("expected 1 TOC page for an A4 theme, got %d", pages)
	}
}

func TestSetToc_NoEmptyPageWhenAChapterIsTallerThanAPage(t *testing.T) {
	writer := SlidevCourseWriter{Course: tocTestCourse("mds", 2, "Titre", strings.Repeat("mot ", 400))}
	if pages := tocPages(writer.SetToc()); pages != 2 {
		t.Fatalf("expected one page per oversized chapter, got %d", pages)
	}
}
