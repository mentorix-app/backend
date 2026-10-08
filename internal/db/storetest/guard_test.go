package storetest

import (
	"strings"
	"testing"
)

func TestSelectTestDatabaseURL(t *testing.T) {
	const ci = "postgres://app:app@localhost:5432/mentorix_test?sslmode=disable"

	tests := []struct {
		name     string
		testURL  string
		fallback string
		want     string
		wantErr  string
	}{
		{name: "both empty", want: ""},
		{name: "test url accepted", testURL: ci, want: ci},
		{name: "fallback accepted", fallback: ci, want: ci},
		{name: "test url wins over fallback", testURL: ci, fallback: "postgres://u:p@h/prod", want: ci},
		{name: "whitespace trimmed", testURL: "  " + ci + "\n", want: ci},
		{name: "test url refused", testURL: "postgres://u:secretpw@h:5432/mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "fallback refused", fallback: "postgres://u:secretpw@h:5432/mentorix", wantErr: "DATABASE_URL"},
		{name: "database name only in query", testURL: "postgres://u:secretpw@h/prod?application_name=mentorix_test", wantErr: "TEST_DATABASE_URL"},
		{name: "database name is a prefix", testURL: "postgres://u:secretpw@h/mentorix_test_old", wantErr: "TEST_DATABASE_URL"},
		{name: "database name only in host", testURL: "postgres://u:secretpw@mentorix_test/prod", wantErr: "TEST_DATABASE_URL"},
		{name: "no database name", testURL: "postgres://u:secretpw@h:5432", wantErr: "TEST_DATABASE_URL"},
		{name: "query dbname overrides path", testURL: "postgres://u:secretpw@h/mentorix_test?dbname=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "query database overrides path", testURL: "postgres://u:secretpw@h/mentorix_test?database=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "keyword value dsn refused", testURL: "host=h user=u password=secretpw dbname=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "keyword value dsn refused even with test name", testURL: "host=h user=u password=secretpw dbname=mentorix_test", wantErr: "TEST_DATABASE_URL"},
		{name: "query dbname disagrees with path for other parsers", testURL: "postgres://u:secretpw@h/prod?dbname=mentorix_test", wantErr: "TEST_DATABASE_URL"},
		{name: "conflicting query keys", testURL: "postgres://u:secretpw@h/mentorix_test?dbname=mentorix_test&database=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "query service", testURL: "postgres://u:secretpw@h/mentorix_test?service=x", wantErr: "TEST_DATABASE_URL"},
		{name: "scheme not postgres", testURL: "mysql://u:secretpw@h/mentorix_test", wantErr: "TEST_DATABASE_URL"},
		{name: "postgresql scheme accepted", testURL: "postgresql://app:app@localhost:5432/mentorix_test?sslmode=disable", want: "postgresql://app:app@localhost:5432/mentorix_test?sslmode=disable"},
		{name: "query key injection split by lib/pq", testURL: "postgres://u:p@h/mentorix_test?zz%3D%27x%27%20dbname=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "query key with non-breaking space", testURL: "postgres://u:secretpw@h/mentorix_test?%C2%A0dbname=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "query key upper case", testURL: "postgres://u:secretpw@h/mentorix_test?DBNAME=mentorix", wantErr: "TEST_DATABASE_URL"},
		{name: "query key not allow-listed", testURL: "postgres://u:secretpw@h/mentorix_test?application_name=x", wantErr: "TEST_DATABASE_URL"},
		{name: "sslmode key upper case", testURL: "postgres://u:secretpw@h/mentorix_test?SSLMODE=disable", wantErr: "TEST_DATABASE_URL"},
		{name: "trailing slash", testURL: "postgres://u:secretpw@h/mentorix_test/", wantErr: "TEST_DATABASE_URL"},
		{name: "opaque url", testURL: "postgres:mentorix_test", wantErr: "TEST_DATABASE_URL"},
		{name: "no query accepted", testURL: "postgres://u:secretpw@h/mentorix_test", want: "postgres://u:secretpw@h/mentorix_test"},
		{name: "unparsable", testURL: "://secretpw mentorix_test", wantErr: "TEST_DATABASE_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectTestDatabaseURL(tt.testURL, tt.fallback)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got url %q, want error", got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not name %s", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "secretpw") || strings.Contains(err.Error(), "postgres://") {
					t.Fatalf("error leaks the URL: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
