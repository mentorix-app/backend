package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/subscription"
)

type fakePlanService struct {
	grants map[uuid.UUID]subscription.Plan
	err    error
}

func (f *fakePlanService) GrantAdminPlan(_ context.Context, trainerUserID uuid.UUID, plan subscription.Plan) (*subscription.Subscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	if plan == subscription.PlanFree {
		delete(f.grants, trainerUserID)
	} else {
		f.grants[trainerUserID] = plan
	}
	effective := f.grants[trainerUserID]
	if effective == "" {
		effective = subscription.PlanFree
	}
	src := subscription.SourceAdmin
	sub := subscription.Subscription{Plan: effective, Source: &src}
	return &sub, nil
}

func (f *fakePlanService) RevokeAdminPlan(_ context.Context, trainerUserID uuid.UUID) (*subscription.Subscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	delete(f.grants, trainerUserID)
	sub := subscription.Subscription{Plan: subscription.PlanFree, Source: nil}
	return &sub, nil
}

func TestNewService_constructs(t *testing.T) {
	svc := NewService(nil)
	if svc == nil || svc.plans == nil {
		t.Fatal("expected service with plan service")
	}
}

func TestService_GrantAndRevokePlan(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	plans := &fakePlanService{grants: map[uuid.UUID]subscription.Plan{}}
	svc := &Service{plans: plans}

	sub, err := svc.GrantPlan(context.Background(), userID, subscription.PlanElite)
	if err != nil {
		t.Fatalf("GrantPlan: %v", err)
	}
	if sub.Plan != subscription.PlanElite {
		t.Fatalf("plan = %s, want elite", sub.Plan)
	}

	sub, err = svc.GrantPlan(context.Background(), userID, subscription.PlanFree)
	if err != nil {
		t.Fatalf("GrantPlan free: %v", err)
	}
	if sub.Plan != subscription.PlanFree {
		t.Fatalf("plan = %s, want free after free grant", sub.Plan)
	}

	_, _ = svc.GrantPlan(context.Background(), userID, subscription.PlanAdvance)
	sub, err = svc.RevokePlan(context.Background(), userID)
	if err != nil {
		t.Fatalf("RevokePlan: %v", err)
	}
	if sub.Plan != subscription.PlanFree {
		t.Fatalf("plan = %s, want free after revoke", sub.Plan)
	}
}
