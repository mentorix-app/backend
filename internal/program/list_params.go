package program

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	DefaultPage      = 1
	DefaultLimit     = 20
	MaxLimit         = 100
	defaultSortBy    = "created_at"
	SortOrderAsc     = "asc"
	SortOrderDesc    = "desc"
	defaultSortOrder = SortOrderDesc
)

var allowedSortColumns = map[string]string{
	"name":        "name",
	"name_ru":     "name_ru",
	"created_at":  "created_at",
	"modified_at": "modified_at",
	"status":      "status",
	"category":    "category",
	"difficulty":  "difficulty",
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ListResult struct {
	Items      []Program  `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type ListParams struct {
	Page       int
	Limit      int
	SortBy     string
	SortOrder  string
	Query      string
	Statuses   []Status
	Category   *Category
	Difficulty *Difficulty
	CreatedBy  *uuid.UUID
}

func ParseListParams(
	pageStr, limitStr, sortBy, sortOrder, q, statusStr, categoryStr, difficultyStr string,
) (ListParams, error) {
	params := ListParams{
		Page:      DefaultPage,
		Limit:     DefaultLimit,
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
		if limit > MaxLimit {
			return ListParams{}, fmt.Errorf("%w: limit exceeds maximum of %d", ErrValidation, MaxLimit)
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
		if order != SortOrderAsc && order != SortOrderDesc {
			return ListParams{}, fmt.Errorf("%w: invalid sort_order", ErrValidation)
		}
		params.SortOrder = order
	}

	if statusStr != "" {
		for _, part := range strings.Split(statusStr, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			st := Status(part)
			if !st.valid() {
				return ListParams{}, fmt.Errorf("%w: invalid status", ErrValidation)
			}
			params.Statuses = append(params.Statuses, st)
		}
	}

	if categoryStr != "" {
		cat := Category(categoryStr)
		if !cat.valid() {
			return ListParams{}, fmt.Errorf("%w: invalid category", ErrValidation)
		}
		params.Category = &cat
	}

	if difficultyStr != "" {
		diff := Difficulty(difficultyStr)
		if !difficultyValid(diff) {
			return ListParams{}, fmt.Errorf("%w: invalid difficulty", ErrValidation)
		}
		params.Difficulty = &diff
	}

	return params, nil
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
