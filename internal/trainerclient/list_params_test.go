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

func TestParseListParams_pageBounds(t *testing.T) {
	tests := []struct {
		name    string
		page    string
		limit   string
		wantErr bool
	}{
		{"page at max", "100000", "", false},
		{"page over max", "100001", "", true},
		{"page overflowing int32 offset", "21474838", "100", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := trainerclient.ParseListParams(tt.page, tt.limit, "", "", "")
			if tt.wantErr != errors.Is(err, trainerclient.ErrValidation) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseListParams_invalidSortBy(t *testing.T) {
	_, err := trainerclient.ParseListParams("", "", "invalid", "", "")
	if err == nil || !errors.Is(err, trainerclient.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}
