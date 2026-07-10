package trainerclient

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrValidation = errors.New("validation failed")

const (
	defaultPage      = 1
	defaultLimit     = 20
	maxLimit         = 100
	defaultSortBy    = "linked_at"
	sortOrderAsc     = "asc"
	sortOrderDesc    = "desc"
	defaultSortOrder = sortOrderDesc
)

var allowedSortColumns = map[string]string{
	"name":      "display_name",
	"linked_at": "linked_at",
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ListParams struct {
	Page      int
	Limit     int
	SortBy    string
	SortOrder string
	Query     string
}

func ParseListParams(pageStr, limitStr, sortBy, sortOrder, q string) (ListParams, error) {
	params := ListParams{
		Page:      defaultPage,
		Limit:     defaultLimit,
		SortBy:    defaultSortBy,
		SortOrder: defaultSortOrder,
		Query:     strings.TrimSpace(q),
	}

	if pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			return ListParams{}, fmt.Errorf("%w: invalid page", ErrValidation)
		}
		params.Page = page
	}

	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return ListParams{}, fmt.Errorf("%w: invalid limit", ErrValidation)
		}
		if limit > maxLimit {
			return ListParams{}, fmt.Errorf("%w: limit exceeds maximum of %d", ErrValidation, maxLimit)
		}
		params.Limit = limit
	}

	if sortBy != "" {
		col, ok := allowedSortColumns[sortBy]
		if !ok {
			return ListParams{}, fmt.Errorf("%w: invalid sort_by", ErrValidation)
		}
		params.SortBy = col
	}

	if sortOrder != "" {
		order := strings.ToLower(sortOrder)
		if order != sortOrderAsc && order != sortOrderDesc {
			return ListParams{}, fmt.Errorf("%w: invalid sort_order", ErrValidation)
		}
		params.SortOrder = order
	}

	return params, nil
}

func DefaultListParams() ListParams {
	p, _ := ParseListParams("", "", "", "", "")
	return p
}

func (p ListParams) Offset() int {
	return (p.Page - 1) * p.Limit
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

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func qPattern(q string) *string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	pattern := "%" + escapeLike(q) + "%"
	return &pattern
}
