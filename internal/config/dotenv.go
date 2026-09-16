package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// Local env files, in priority order: personal secrets first, committed defaults second.
const (
	dotenvLocalFile = ".env.local"
	dotenvFile      = ".env"
)

// LoadDotenv loads .env.local and then .env from the working directory.
// Variables already present in the process environment are never overridden,
// so the effective precedence is: shell/Render environment > .env.local > .env.
// Missing files are skipped; on Render neither file exists.
func LoadDotenv() {
	LoadDotenvFiles(dotenvLocalFile, dotenvFile)
}

// LoadDotenvFiles loads the given files in order without overriding variables
// that are already set. Because godotenv keeps the first value it sees for a key,
// earlier files take precedence over later ones. Missing files are ignored.
func LoadDotenvFiles(paths ...string) {
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		// A malformed file is a local setup problem, not a runtime failure:
		// keep the same best-effort behaviour godotenv.Load() had before.
		_ = godotenv.Load(path)
	}
}
