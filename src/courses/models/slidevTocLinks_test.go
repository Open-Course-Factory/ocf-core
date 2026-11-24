package models

import (
	"regexp"
	"testing"
)

var (
	routeAliasLine = regexp.MustCompile(`(?m)^routeAlias: (\S+)$`)
	tocLinkTarget  = regexp.MustCompile(`<Link to="([^"]+)" class="ocf-toc-link"`)
)

func TestSlidevCourse_TocLinks_AllPointToACover(t *testing.T) {
	markdown := (&SlidevCourseWriter{Course: framingTestCourse(2)}).GetCourse()

	aliases := map[string]int{}
	for _, m := range routeAliasLine.FindAllStringSubmatch(markdown, -1) {
		aliases[m[1]]++
	}
	for _, want := range []string{"c1", "c1-s1", "c2", "c2-s1"} {
		if aliases[want] != 1 {
			t.Errorf("expected exactly one cover with routeAlias %q, got %d", want, aliases[want])
		}
	}

	targets := tocLinkTarget.FindAllStringSubmatch(markdown, -1)
	// Per chapter: 2 course TOC links, 1 chapter TOC link, 1 section cover link, 1 page TOC link.
	if len(targets) != 10 {
		t.Errorf("expected 10 TOC links, got %d", len(targets))
	}
	for _, m := range targets {
		if aliases[m[1]] != 1 {
			t.Errorf("TOC link to %q has no matching cover", m[1])
		}
	}
}

func TestSectionTocLine_WithoutMatchingSection_StaysPlainText(t *testing.T) {
	if got := sectionTocLine(Chapter{Number: 1}, 0, "Orpheline"); got != "Orpheline" {
		t.Errorf("expected plain text, got %q", got)
	}
}
