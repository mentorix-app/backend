package trainerclient

import "testing"

func TestDefaultListParams(t *testing.T) {
	p := DefaultListParams()
	if p.Page != 1 || p.Limit != 20 {
		t.Fatalf("page/limit = %d/%d", p.Page, p.Limit)
	}
	if p.SortBy != "linked_at" || p.SortOrder != "desc" {
		t.Fatalf("sort = %s %s", p.SortBy, p.SortOrder)
	}
}

func TestPaginationMeta(t *testing.T) {
	if meta := paginationMeta(1, 20, 0); meta.TotalPages != 0 {
		t.Fatalf("totalPages = %d, want 0", meta.TotalPages)
	}
	meta := paginationMeta(2, 10, 25)
	if meta.Total != 25 || meta.Page != 2 || meta.Limit != 10 || meta.TotalPages != 3 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestQPattern(t *testing.T) {
	if qPattern("  ") != nil {
		t.Fatal("expected nil for blank query")
	}
	p := qPattern("vik")
	if p == nil || *p != "%vik%" {
		t.Fatalf("pattern = %v", p)
	}
	wild := qPattern(`100%_done`)
	if wild == nil || *wild != `%100\%\_done%` {
		t.Fatalf("pattern = %v", wild)
	}
}
