package program

import (
	"testing"
)

func TestListParams_offsetAndPagination(t *testing.T) {
	params, err := ParseListParams("3", "10", "created_at", "desc", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}
	if params.Offset() != 20 {
		t.Errorf("Offset() = %d, want 20", params.Offset())
	}
	meta := paginationMeta(params.Page, params.Limit, 45)
	if meta.Total != 45 || meta.Page != 3 || meta.Limit != 10 || meta.TotalPages != 5 {
		t.Fatalf("pagination = %+v", meta)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`100%_done`); got != `100\%\_done` {
		t.Errorf("escapeLike = %q", got)
	}
}
