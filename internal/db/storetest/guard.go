package storetest

import (
	"fmt"
	"net/url"
	"strings"
)

const testDatabaseName = "mentorix_test"

// selectTestDatabaseURL picks TEST_DATABASE_URL, falling back to DATABASE_URL.
// It returns "" with no error when both are empty. The tests drop the mentorix
// schema, so the URL must match one unambiguous form: scheme postgres or
// postgresql, path exactly /mentorix_test, and no query key except sslmode.
// pgx and lib/pq (golang-migrate) read query keys differently and either can
// take the database name from one, so every other key is refused.
// Errors name the variable only, never the URL or the parse error (they carry
// the password).
func selectTestDatabaseURL(testURL, fallbackURL string) (string, error) {
	name, raw := "TEST_DATABASE_URL", strings.TrimSpace(testURL)
	if raw == "" {
		name, raw = "DATABASE_URL", strings.TrimSpace(fallbackURL)
	}
	if raw == "" {
		return "", nil
	}
	if !isTestDatabaseURL(raw) {
		return "", fmt.Errorf("refusing %s: the database name must be %s", name, testDatabaseName)
	}
	return raw, nil
}

func isTestDatabaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return false
	}
	if u.Path != "/"+testDatabaseName {
		return false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	for key := range query {
		if key != "sslmode" {
			return false
		}
	}
	return true
}
