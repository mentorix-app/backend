package exercise

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultPage      = 1
	DefaultLimit     = 20
	MaxLimit         = 100
	defaultSortBy    = "name"
	SortOrderAsc     = "asc"
	SortOrderDesc    = "desc"
	defaultSortOrder = SortOrderAsc
	equipmentNone    = "none"
)

var allowedSortColumns = map[string]string{
	"name":         "name",
	"name_ru":      "name_ru",
	"created_at":   "created_at",
	"modified_at":  "modified_at",
	"difficulty":   "difficulty",
	"type":         "type",
	"muscle_group": "muscle_group",
	"equipment":    "equipment",
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ListResult struct {
	Items      []Exercise `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type ListParams struct {
	Page            int
	Limit           int
	SortBy          string
	SortOrder       string
	Query           string
	Type            *ExerciseType
	MuscleGroup     *MuscleGroup
	Difficulty      *Difficulty
	Equipment       *Equipment
	EquipmentIsNull bool
}

func ParseListParams(
	pageStr, limitStr, sortBy, sortOrder, q,
	typeStr, muscleGroupStr, difficultyStr, equipmentStr string,
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

	if typeStr != "" {
		t := ExerciseType(typeStr)
		if !t.valid() {
			return ListParams{}, fmt.Errorf("%w: invalid type", ErrValidation)
		}
		params.Type = &t
	}

	if muscleGroupStr != "" {
		g := MuscleGroup(muscleGroupStr)
		if !g.valid() {
			return ListParams{}, fmt.Errorf("%w: invalid muscle_group", ErrValidation)
		}
		params.MuscleGroup = &g
	}

	if difficultyStr != "" {
		d := Difficulty(difficultyStr)
		if !d.valid() {
			return ListParams{}, fmt.Errorf("%w: invalid difficulty", ErrValidation)
		}
		params.Difficulty = &d
	}

	if equipmentStr != "" {
		if equipmentStr == equipmentNone {
			params.EquipmentIsNull = true
		} else {
			e := Equipment(equipmentStr)
			if !e.valid() {
				return ListParams{}, fmt.Errorf("%w: invalid equipment", ErrValidation)
			}
			params.Equipment = &e
		}
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
