package models

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// slide is one Slidev slide: its front-matter layout (empty when none) and its body.
type slide struct {
	layout string
	body   string
}

// frontMatterBlock matches a Slidev front-matter block: "---", key/value lines (possibly indented), "---".
var frontMatterBlock = regexp.MustCompile(`(?m)^---\n((?:[ \w]+:.*\n)+)---\n`)

// parseSlides splits generated Slidev markdown into slides, one per front-matter block.
func parseSlides(markdown string) []slide {
	matches := frontMatterBlock.FindAllStringSubmatchIndex(markdown, -1)
	slides := make([]slide, 0, len(matches))
	for i, m := range matches {
		end := len(markdown)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		layout := ""
		for _, line := range strings.Split(markdown[m[2]:m[3]], "\n") {
			if v, ok := strings.CutPrefix(line, "layout:"); ok {
				layout = strings.TrimSpace(v)
			}
		}
		slides = append(slides, slide{layout: layout, body: markdown[m[1]:end]})
	}
	return slides
}

func countSlides(slides []slide, keep func(slide) bool) int {
	n := 0
	for _, s := range slides {
		if keep(s) {
			n++
		}
	}
	return n
}

func isCourseToc(s slide) bool {
	return s.layout == "maintoc" && strings.Contains(s.body, "Thèmes abordés dans le cours")
}

func isChapterCover(number int) func(slide) bool {
	return func(s slide) bool {
		return s.layout == "cover" && strings.Contains(s.body, "**CHAPITRE "+strconv.Itoa(number)+"**")
	}
}

// isChapterToc matches the chapter table of contents: a maintoc slide listing the chapter sections
// ("- **<section title>** <intro>"). The chapter conclusion (also a maintoc slide) is not matched.
func isChapterToc(sectionTitle string) func(slide) bool {
	return func(s slide) bool {
		return s.layout == "maintoc" && strings.Contains(s.body, "- **"+sectionTitle+"** ")
	}
}

func isSectionCover(chapterTitle string) func(slide) bool {
	return func(s slide) bool {
		return s.layout == "cover" && !strings.Contains(s.body, "**CHAPITRE ") &&
			strings.Contains(s.body, "# "+strings.ToUpper(chapterTitle)+"\n")
	}
}

func hasPageContent(content string) func(slide) bool {
	return func(s slide) bool { return strings.Contains(s.body, content) }
}

func framingTestChapter(number int) *Chapter {
	n := strconv.Itoa(number)
	section := &Section{
		Number:     1,
		Title:      "Section" + n,
		Intro:      "intro de la section " + n,
		Conclusion: "conclusion de la section " + n,
		Pages: []*Page{{
			Order:   1,
			Toc:     []string{"Point " + n},
			Content: []string{"Contenu de la page du chapitre " + n},
		}},
	}
	return &Chapter{
		Number:       number,
		Title:        "Chapitre" + n,
		Introduction: "introduction du chapitre " + n,
		Sections:     []*Section{section},
	}
}

func framingTestCourse(chapterCount int) Course {
	course := Course{Title: "Cours de test", Subtitle: "Sous-titre"}
	for i := 1; i <= chapterCount; i++ {
		course.Chapters = append(course.Chapters, framingTestChapter(i))
	}
	return course
}

func TestSlidevCourse_SingleChapter_OmitsCourseAndChapterFraming(t *testing.T) {
	slides := parseSlides((&SlidevCourseWriter{Course: framingTestCourse(1)}).GetCourse())

	if n := countSlides(slides, isCourseToc); n != 0 {
		t.Errorf("single-chapter course: expected no course TOC slide, got %d", n)
	}
	if n := countSlides(slides, isChapterCover(1)); n != 0 {
		t.Errorf("single-chapter course: expected no chapter cover slide, got %d", n)
	}
	if n := countSlides(slides, isChapterToc("Section1")); n != 0 {
		t.Errorf("single-chapter course: expected no chapter TOC slide, got %d", n)
	}
}

func TestSlidevCourse_SingleChapter_KeepsSectionSlides(t *testing.T) {
	slides := parseSlides((&SlidevCourseWriter{Course: framingTestCourse(1)}).GetCourse())

	if n := countSlides(slides, isSectionCover("Chapitre1")); n != 1 {
		t.Errorf("single-chapter course: expected 1 section cover slide, got %d", n)
	}
	if n := countSlides(slides, hasPageContent("Contenu de la page du chapitre 1")); n != 1 {
		t.Errorf("single-chapter course: expected the section page to be present once, got %d", n)
	}
}

func TestSlidevCourse_MultipleChapters_KeepsCourseAndChapterFraming(t *testing.T) {
	slides := parseSlides((&SlidevCourseWriter{Course: framingTestCourse(2)}).GetCourse())

	if n := countSlides(slides, isCourseToc); n != 1 {
		t.Errorf("multi-chapter course: expected 1 course TOC slide, got %d", n)
	}
	for i := 1; i <= 2; i++ {
		if n := countSlides(slides, isChapterCover(i)); n != 1 {
			t.Errorf("multi-chapter course: expected 1 cover slide for chapter %d, got %d", i, n)
		}
		if n := countSlides(slides, isChapterToc("Section"+strconv.Itoa(i))); n != 1 {
			t.Errorf("multi-chapter course: expected 1 TOC slide for chapter %d, got %d", i, n)
		}
		if n := countSlides(slides, isSectionCover("Chapitre"+strconv.Itoa(i))); n != 1 {
			t.Errorf("multi-chapter course: expected 1 section cover slide for chapter %d, got %d", i, n)
		}
		if n := countSlides(slides, hasPageContent("Contenu de la page du chapitre "+strconv.Itoa(i))); n != 1 {
			t.Errorf("multi-chapter course: expected the page of chapter %d once, got %d", i, n)
		}
	}
}
