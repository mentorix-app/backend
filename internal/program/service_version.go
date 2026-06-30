package program

import (
	"context"

	"github.com/google/uuid"
)

func (s *Service) ListAssignments(ctx context.Context, userID, programID uuid.UUID) (AssignmentListResult, error) {
	if err := s.ensureAccess(ctx, userID, programID); err != nil {
		return AssignmentListResult{}, err
	}
	return s.store.ListProgramAssignments(ctx, programID)
}

func (s *Service) SyncAssignments(ctx context.Context, userID, programID uuid.UUID, req AssignmentSyncRequest) (AssignmentSyncResult, error) {
	if err := s.ensureAccess(ctx, userID, programID); err != nil {
		return AssignmentSyncResult{}, err
	}
	if err := validateAssignmentSyncRequest(req); err != nil {
		return AssignmentSyncResult{}, err
	}
	return s.store.SyncProgramAssignments(ctx, programID, req)
}

func validateAssignmentSyncRequest(req AssignmentSyncRequest) error {
	allActive := req.AllActive != nil && *req.AllActive
	if allActive || len(req.AssignmentIDs) > 0 {
		return nil
	}
	return ErrInvalidSyncRequest
}

func (s *Service) ListVersions(ctx context.Context, userID, programID uuid.UUID) (VersionListResult, error) {
	if err := s.ensureAccess(ctx, userID, programID); err != nil {
		return VersionListResult{}, err
	}
	return s.store.ListProgramVersions(ctx, programID)
}

func (s *Service) DeleteVersion(ctx context.Context, userID, programID, versionID uuid.UUID) error {
	if err := s.ensureAccess(ctx, userID, programID); err != nil {
		return err
	}
	return s.store.DeleteProgramVersion(ctx, programID, versionID)
}

func (s *Service) CleanupVersions(ctx context.Context, userID, programID uuid.UUID) (VersionCleanupResult, error) {
	if err := s.ensureAccess(ctx, userID, programID); err != nil {
		return VersionCleanupResult{}, err
	}
	return s.store.CleanupProgramVersions(ctx, programID)
}
