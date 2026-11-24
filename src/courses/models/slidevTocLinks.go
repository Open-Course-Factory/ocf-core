package models

import "strconv"

// Each chapter and section cover gets a Slidev routeAlias, so a TOC line links to it by name and the
// link survives slides being added or removed.
func chapterRouteAlias(chapterNumber int) string {
	return "c" + strconv.Itoa(chapterNumber)
}

func sectionRouteAlias(chapterNumber, sectionNumber int) string {
	return chapterRouteAlias(chapterNumber) + "-s" + strconv.Itoa(sectionNumber)
}

// tocLinkStyle keeps a TOC line looking like plain text whatever the theme's link style: the
// pointer cursor is the only hint, and a theme can add a hover cue on the ocf-toc-link class.
const tocLinkStyle = "color:inherit;background:none;border:none;text-decoration:none"

// tocLink wraps a TOC line in a Slidev <Link>, which moves the presenter and the audience window together.
// The <span> matters: Slidev reads a line that starts with a component tag as a raw HTML block, which
// stops Markdown inside it and swallows the next line.
func tocLink(routeAlias, text string) string {
	return `<span><Link to="` + routeAlias + `" class="ocf-toc-link" style="` + tocLinkStyle + `">` + text + "</Link></span>"
}

// sectionTocLine links line index of a chapter's section list (built in section order by InitTocs)
// to that section's cover; a line with no matching section stays plain text.
func sectionTocLine(chapter Chapter, index int, text string) string {
	if index >= len(chapter.Sections) {
		return text
	}
	return tocLink(sectionRouteAlias(chapter.Number, chapter.Sections[index].Number), text)
}
