package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n\nPLAIN=value\nQUOTED=\"http://host:8000\"\nSINGLE='x=y'\nPRESET=from_file\nNOEQUALS\n  SPACED = padded \n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRESET", "from_env")
	for _, k := range []string{"PLAIN", "QUOTED", "SINGLE", "SPACED", "NOEQUALS"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PLAIN":  "value",
		"QUOTED": "http://host:8000",
		"SINGLE": "x=y",
		"SPACED": "padded",
		"PRESET": "from_env",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, set := os.LookupEnv("NOEQUALS"); set {
		t.Error("line without = must be ignored")
	}
}

func TestLoadDotEnv_MissingFileReturnsError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
