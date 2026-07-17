package subscription

import (
	"errors"
	"fmt"
	"time"
)

// Plan is a subscription tier code.
type Plan string

const (
	PlanFree    Plan = "free"
	PlanAdvance Plan = "advance"
	PlanElite   Plan = "elite"
)

func (p Plan) Valid() bool {
	switch p {
	case PlanFree, PlanAdvance, PlanElite:
		return true
	default:
		return false
	}
}

func (p Plan) rank() int {
	switch p {
	case PlanElite:
		return 2
	case PlanAdvance:
		return 1
	default:
		return 0
	}
}

// Source describes who issued an entitlement.
type Source string

const (
	SourceAdmin      Source = "admin"
	SourceAppStore   Source = "app_store"
	SourceGooglePlay Source = "google_play"
)

// Resource is a quota-limited resource category.
type Resource string

const (
	ResourceExercises Resource = "exercises"
	ResourcePrograms  Resource = "programs"
	ResourceClients   Resource = "clients"
)

// Limits holds per-plan caps; nil means unlimited.
type Limits struct {
	Exercises      *int `json:"exercises"`
	ActivePrograms *int `json:"active_programs"`
	ActiveClients  *int `json:"active_clients"`
}

// Usage holds actual counted resources of a trainer.
type Usage struct {
	Exercises      int `json:"exercises"`
	ActivePrograms int `json:"active_programs"`
	ActiveClients  int `json:"active_clients"`
}

// Permissions are derived from limits vs usage for mobile/web clients.
type Permissions struct {
	CanCreateExercise bool `json:"can_create_exercise"`
	CanEditExercises  bool `json:"can_edit_exercises"`
	CanCreateProgram  bool `json:"can_create_program"`
	CanEditPrograms   bool `json:"can_edit_programs"`
	CanCreateInvite   bool `json:"can_create_invite"`
	CanManageClients  bool `json:"can_manage_clients"`
}

// Subscription is the effective plan state returned in /auth/me.
type Subscription struct {
	Plan        Plan        `json:"plan"`
	Source      *Source     `json:"source"`
	ValidUntil  *time.Time  `json:"valid_until"`
	Limits      Limits      `json:"limits"`
	Usage       Usage       `json:"usage"`
	Permissions Permissions `json:"permissions"`
}

// PlanInfo is the effective plan without usage.
type PlanInfo struct {
	Plan       Plan
	Source     *Source
	ValidUntil *time.Time
}

func intPtr(v int) *int { return &v }

func planLimits(p Plan) Limits {
	switch p {
	case PlanElite:
		return Limits{Exercises: nil, ActivePrograms: nil, ActiveClients: intPtr(50)}
	case PlanAdvance:
		return Limits{Exercises: intPtr(50), ActivePrograms: intPtr(15), ActiveClients: intPtr(15)}
	default:
		return Limits{Exercises: intPtr(10), ActivePrograms: intPtr(3), ActiveClients: intPtr(3)}
	}
}

// PlanCatalogItem describes one tier in the public plan catalog.
type PlanCatalogItem struct {
	Code   Plan   `json:"code"`
	Limits Limits `json:"limits"`
}

// Catalog lists all tiers in ascending order.
func Catalog() []PlanCatalogItem {
	return []PlanCatalogItem{
		{Code: PlanFree, Limits: planLimits(PlanFree)},
		{Code: PlanAdvance, Limits: planLimits(PlanAdvance)},
		{Code: PlanElite, Limits: planLimits(PlanElite)},
	}
}

// ErrTrainerNotFound is returned when a plan operation targets a user without a trainer profile.
var ErrTrainerNotFound = errors.New("trainer not found")

// ErrInvalidPlan is returned for unknown plan codes.
var ErrInvalidPlan = errors.New("invalid plan")

// QuotaError reports that an operation is blocked by the current plan quota.
type QuotaError struct {
	Resource Resource `json:"resource"`
	Plan     Plan     `json:"plan"`
	Limit    int      `json:"limit"`
	Usage    int      `json:"usage"`
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded: %s limit %d reached on plan %s (usage %d)", e.Resource, e.Limit, e.Plan, e.Usage)
}
