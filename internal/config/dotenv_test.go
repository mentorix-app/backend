package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// unsetForTest guarantees the variable is absent during the test and restored afterwards.
func unsetForTest(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func TestLoadDotenvFiles_localOverridesBase(t *testing.T) {
	dir := t.TempDir()
	local := writeEnvFile(t, dir, ".env.local", "DOTENV_TEST_A=local\nDOTENV_TEST_ONLY_LOCAL=yes\n")
	base := writeEnvFile(t, dir, ".env", "DOTENV_TEST_A=base\nDOTENV_TEST_ONLY_BASE=yes\n")
	for _, k := range []string{"DOTENV_TEST_A", "DOTENV_TEST_ONLY_LOCAL", "DOTENV_TEST_ONLY_BASE"} {
		unsetForTest(t, k)
	}

	LoadDotenvFiles(local, base)

	if got := os.Getenv("DOTENV_TEST_A"); got != "local" {
		t.Errorf("DOTENV_TEST_A = %q, want local (the .env.local value wins)", got)
	}
	if got := os.Getenv("DOTENV_TEST_ONLY_LOCAL"); got != "yes" {
		t.Errorf("DOTENV_TEST_ONLY_LOCAL = %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_ONLY_BASE"); got != "yes" {
		t.Errorf("DOTENV_TEST_ONLY_BASE = %q (keys missing from .env.local fall through to .env)", got)
	}
}

func TestLoadDotenvFiles_processEnvWins(t *testing.T) {
	dir := t.TempDir()
	local := writeEnvFile(t, dir, ".env.local", "DOTENV_TEST_B=local\n")
	base := writeEnvFile(t, dir, ".env", "DOTENV_TEST_B=base\n")
	t.Setenv("DOTENV_TEST_B", "shell")

	LoadDotenvFiles(local, base)

	if got := os.Getenv("DOTENV_TEST_B"); got != "shell" {
		t.Errorf("DOTENV_TEST_B = %q, want shell (already-set variables are never overridden)", got)
	}
}

func TestLoadDotenvFiles_missingFilesAreFine(t *testing.T) {
	dir := t.TempDir()
	base := writeEnvFile(t, dir, ".env", "DOTENV_TEST_C=base\n")
	unsetForTest(t, "DOTENV_TEST_C")

	LoadDotenvFiles(filepath.Join(dir, ".env.local"), base)

	if got := os.Getenv("DOTENV_TEST_C"); got != "base" {
		t.Errorf("DOTENV_TEST_C = %q, want base (a missing .env.local is skipped)", got)
	}

	// No files at all must not panic or fail.
	LoadDotenvFiles(filepath.Join(dir, "nope.local"), filepath.Join(dir, "nope"))
}
