package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseShebang(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "bash shebang",
			script:   "#!/bin/bash\necho hi",
			expected: "/bin/bash",
		},
		{
			name:     "sh shebang",
			script:   "#!/bin/sh\necho hi",
			expected: "/bin/sh",
		},
		{
			name:     "env bash shebang",
			script:   "#!/usr/bin/env bash\necho hi",
			expected: "/bin/bash",
		},
		{
			name:     "no shebang defaults to sh",
			script:   "echo hi",
			expected: "/bin/sh",
		},
		{
			name:     "usr bin bash shebang",
			script:   "#!/usr/bin/bash\necho hi",
			expected: "/usr/bin/bash",
		},
		{
			name:     "empty script defaults to sh",
			script:   "",
			expected: "/bin/sh",
		},
		{
			name:     "shebang with flags strips flags",
			script:   "#!/bin/bash -e\nset -o pipefail",
			expected: "/bin/bash",
		},
		{
			name:     "env python resolves to bin python",
			script:   "#!/usr/bin/env python3\nimport sys",
			expected: "/bin/python3",
		},
		{
			name:     "shebang only no newline",
			script:   "#!/bin/bash",
			expected: "/bin/bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseShebang(tt.script)
			assert.Equal(t, tt.expected, result)
		})
	}
}
