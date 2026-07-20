package analytics

import (
	"errors"
	"testing"
	"time"
)

func TestParseCompletionsParams(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		p, err := ParseCompletionsParams("", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if p.Page != 1 || p.Limit != 20 || p.From != nil || p.To != nil {
			t.Fatalf("params = %+v", p)
		}
		if p.Offset() != 0 {
			t.Fatalf("offset = %d", p.Offset())
		}
	})

	t.Run("date and rfc3339", func(t *testing.T) {
		p, err := ParseCompletionsParams("2", "50", "2026-07-01", "2026-07-20T10:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
		if p.Offset() != 50 {
			t.Fatalf("offset = %d", p.Offset())
		}
		wantFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		if !p.From.Equal(wantFrom) {
			t.Fatalf("from = %s", p.From)
		}
		if p.To.Hour() != 10 {
			t.Fatalf("to = %s", p.To)
		}
	})

	invalid := []struct {
		name                    string
		page, limit, fromS, toS string
	}{
		{"bad page", "0", "", "", ""},
		{"bad limit", "1", "abc", "", ""},
		{"limit over max", "1", "101", "", ""},
		{"bad from", "1", "10", "yesterday", ""},
		{"bad to", "1", "10", "", "tomorrow"},
		{"from after to", "1", "10", "2026-07-20", "2026-07-01"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCompletionsParams(tt.page, tt.limit, tt.fromS, tt.toS)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}

func TestParseProgramsParams(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		p, err := ParseProgramsParams("", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if p.SortBy != "last_activity" || p.SortOrder != "desc" || p.Page != 1 || p.Limit != 20 {
			t.Fatalf("params = %+v", p)
		}
	})

	t.Run("explicit", func(t *testing.T) {
		p, err := ParseProgramsParams("3", "10", "name", "ASC")
		if err != nil {
			t.Fatal(err)
		}
		if p.SortBy != "name" || p.SortOrder != "asc" || p.Offset() != 20 {
			t.Fatalf("params = %+v", p)
		}
	})

	if _, err := ParseProgramsParams("1", "10", "created_at", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("sort_by err = %v", err)
	}
	if _, err := ParseProgramsParams("1", "10", "name", "up"); !errors.Is(err, ErrValidation) {
		t.Fatalf("sort_order err = %v", err)
	}
}

func TestPaginationMeta(t *testing.T) {
	p := paginationMeta(2, 20, 45)
	if p.TotalPages != 3 || p.Total != 45 || p.Page != 2 {
		t.Fatalf("pagination = %+v", p)
	}
	if paginationMeta(1, 20, 0).TotalPages != 0 {
		t.Fatal("empty total should have 0 pages")
	}
}
