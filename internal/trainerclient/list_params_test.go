package trainerclient_test

import (
	"errors"
	"testing"

	"mentorix-backend/internal/trainerclient"
)

func TestParseListParams_defaults(t *testing.T) {
	params, err := trainerclient.ParseListParams("", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}
	if params.Page != 1 || params.Limit != 20 {
		t.Fatalf("page/limit = %d/%d", params.Page, params.Limit)
	}
	if params.SortBy != "linked_at" || params.SortOrder != "desc" {
		t.Fatalf("sort = %s %s", params.SortBy, params.SortOrder)
	}
}

func TestParseListParams_nameSortAsc(t *testing.T) {
	params, err := trainerclient.ParseListParams("2", "10", "name", "asc", "vik")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}
	if params.Page != 2 || params.Limit != 10 || params.Query != "vik" {
		t.Fatalf("params = %+v", params)
	}
	if params.SortBy != "display_name" || params.SortOrder != "asc" {
		t.Fatalf("sort = %s %s", params.SortBy, params.SortOrder)
	}
	if params.Offset() != 10 {
		t.Fatalf("offset = %d", params.Offset())
	}
}

func TestParseListParams_invalidSortBy(t *testing.T) {
	_, err := trainerclient.ParseListParams("", "", "invalid", "", "")
	if err == nil || !errors.Is(err, trainerclient.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}
