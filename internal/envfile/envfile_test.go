package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFirst(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.dev")
	body := "# comment\nFOO=bar\nQUOTED=\"hello world\"\nALREADY=overwritten\n"
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY", "kept")
	if err := os.Unsetenv("FOO"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("QUOTED"); err != nil {
		t.Fatal(err)
	}

	if err := LoadFirst(filepath.Join(dir, "nope.env"), envPath); err != nil {
		t.Fatalf("LoadFirst: %v", err)
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Errorf("FOO = %q, want bar", got)
	}
	if got := os.Getenv("QUOTED"); got != "hello world" {
		t.Errorf("QUOTED = %q, want hello world", got)
	}
	if got := os.Getenv("ALREADY"); got != "kept" {
		t.Errorf("ALREADY = %q, want kept (explicit env should win)", got)
	}
}

func TestLoadFirstMissingIsOK(t *testing.T) {
	if err := LoadFirst("/no/such/file.env"); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
}
