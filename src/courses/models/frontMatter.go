package models

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// parseFrontMatter unmarshals the leading `---` YAML block of content into v
// and returns the remaining body. It keeps the rules of the adrg/frontmatter
// parser it replaced: leading blank lines are skipped, delimiter lines are
// compared after trimming (so CRLF works), and content without a block, or
// with an unterminated one, is returned whole with v left untouched.
func parseFrontMatter(content []byte, v any) ([]byte, error) {
	meta, body := splitFrontMatter(content)
	return body, yaml.Unmarshal(meta, v)
}

func splitFrontMatter(content []byte) (meta, body []byte) {
	pos := 0
	nextLine := func() (start int, line []byte, ok bool) {
		if pos >= len(content) {
			return pos, nil, false
		}
		start = pos
		if end := bytes.IndexByte(content[pos:], '\n'); end < 0 {
			pos = len(content)
		} else {
			pos += end + 1
		}
		return start, content[start:pos], true
	}
	isDelimiter := func(line []byte) bool { return string(bytes.TrimSpace(line)) == "---" }

	for {
		_, line, ok := nextLine()
		if !ok || (len(bytes.TrimSpace(line)) > 0 && !isDelimiter(line)) {
			return nil, content
		}
		if isDelimiter(line) {
			break
		}
	}
	metaStart := pos
	for {
		start, line, ok := nextLine()
		if !ok {
			return nil, content
		}
		if isDelimiter(line) {
			return content[metaStart:start], content[pos:]
		}
	}
}
