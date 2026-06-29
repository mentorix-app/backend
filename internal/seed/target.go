package seed

import (
	"fmt"
	"net/url"
	"strings"
)

func IsLocalDatabaseURL(databaseURL string) bool {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "host.docker.internal", "postgres":
		return true
	default:
		return false
	}
}

func DatabaseTargetLabel(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "unknown"
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" {
		db = "(default)"
	}
	return fmt.Sprintf("%s / %s", u.Hostname(), db)
}
