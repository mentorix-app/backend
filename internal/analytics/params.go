package analytics

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrValidation = errors.New("validation failed")

const (
	defaultPage      = 1
	defaultLimit     = 20
	maxLimit         = 100
	sortOrderAsc     = "asc"
	sortOrderDesc    = "desc"
	defaultSortOrder = sortOrderDesc

	sortByName         = "name"
	sortByLastActivity = "last_activity"
)

type CompletionsParams struct {
	Page  int
	Limit int
	From  *time.Time
	To    *time.Time
}

func ParseCompletionsParams(pageStr, limitStr, fromStr, toStr string) (CompletionsParams, error) {
	page, limit, err := parsePageLimit(pageStr, limitStr)
	if err != nil {
		return CompletionsParams{}, err
	}
	params := CompletionsParams{Page: page, Limit: limit}

	if fromStr != "" {
		from, err := parseTimeParam(fromStr)
		if err != nil {
			return CompletionsParams{}, fmt.Errorf("%w: invalid from", ErrValidation)
		}
		params.From = &from
	}
	if toStr != "" {
		to, err := parseTimeParam(toStr)
		if err != nil {
			return CompletionsParams{}, fmt.Errorf("%w: invalid to", ErrValidation)
		}
		params.To = &to
	}
	if params.From != nil && params.To != nil && !params.From.Before(*params.To) {
		return CompletionsParams{}, fmt.Errorf("%w: from must be before to", ErrValidation)
	}
	return params, nil
}

func (p CompletionsParams) Offset() int {
	return (p.Page - 1) * p.Limit
}

type ProgramsParams struct {
	Page      int
	Limit     int
	SortBy    string
	SortOrder string
}

func ParseProgramsParams(pageStr, limitStr, sortBy, sortOrder string) (ProgramsParams, error) {
	page, limit, err := parsePageLimit(pageStr, limitStr)
	if err != nil {
		return ProgramsParams{}, err
	}
	params := ProgramsParams{
		Page:      page,
		Limit:     limit,
		SortBy:    sortByLastActivity,
		SortOrder: defaultSortOrder,
	}

	if sortBy != "" {
		if sortBy != sortByName && sortBy != sortByLastActivity {
			return ProgramsParams{}, fmt.Errorf("%w: invalid sort_by", ErrValidation)
		}
		params.SortBy = sortBy
	}
	if sortOrder != "" {
		order := strings.ToLower(sortOrder)
		if order != sortOrderAsc && order != sortOrderDesc {
			return ProgramsParams{}, fmt.Errorf("%w: invalid sort_order", ErrValidation)
		}
		params.SortOrder = order
	}
	return params, nil
}

func (p ProgramsParams) Offset() int {
	return (p.Page - 1) * p.Limit
}

func parsePageLimit(pageStr, limitStr string) (int, int, error) {
	page, limit := defaultPage, defaultLimit
	if pageStr != "" {
		v, err := strconv.Atoi(pageStr)
		if err != nil || v < 1 {
			return 0, 0, fmt.Errorf("%w: invalid page", ErrValidation)
		}
		page = v
	}
	if limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 1 {
			return 0, 0, fmt.Errorf("%w: invalid limit", ErrValidation)
		}
		if v > maxLimit {
			return 0, 0, fmt.Errorf("%w: limit exceeds maximum of %d", ErrValidation, maxLimit)
		}
		limit = v
	}
	return page, limit, nil
}

// parseTimeParam accepts RFC3339 timestamps and plain dates (2026-07-20, UTC).
func parseTimeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func paginationMeta(page, limit, total int) Pagination {
	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}
	return Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
