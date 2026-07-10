package program

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) BulkSetClientProgramAssignment(ctx context.Context, trainerUserID uuid.UUID, req BulkSetClientProgramAssignmentRequest) (BulkAssignmentResult, error) {
	if err := req.Validate(); err != nil {
		return BulkAssignmentResult{}, err
	}

	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BulkAssignmentResult{}, ErrForbidden
		}
		return BulkAssignmentResult{}, err
	}

	clientUserIDs := uniqueUUIDs(req.ClientUserIDs)
	if req.ProgramID != nil {
		if err := s.store.validateProgramForAssignment(ctx, trainerUserID, *req.ProgramID); err != nil {
			return BulkAssignmentResult{}, err
		}
	}

	result := BulkAssignmentResult{
		Assigned: make([]Assignment, 0, len(clientUserIDs)),
		Cleared:  make([]uuid.UUID, 0),
		Skipped:  make([]BulkAssignmentSkipped, 0),
	}

	for _, clientUserID := range clientUserIDs {
		assignment, err := s.store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, clientUserID, req.ProgramID)
		if err != nil {
			if reason := bulkAssignmentSkipReason(err); reason != "" {
				result.Skipped = append(result.Skipped, BulkAssignmentSkipped{
					ClientUserID: clientUserID,
					Reason:       reason,
				})
				continue
			}
			return BulkAssignmentResult{}, err
		}
		if assignment == nil {
			result.Cleared = append(result.Cleared, clientUserID)
			continue
		}
		result.Assigned = append(result.Assigned, *assignment)
	}

	return result, nil
}

func bulkAssignmentSkipReason(err error) string {
	switch {
	case errors.Is(err, ErrClientNotLinked):
		return "not_linked"
	case errors.Is(err, ErrClientBlocked):
		return "blocked"
	case errors.Is(err, ErrClientNotFound):
		return "not_found"
	default:
		return ""
	}
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
