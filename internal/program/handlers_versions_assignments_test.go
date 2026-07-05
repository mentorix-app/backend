package program

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type assignmentVersionStore struct {
	fakeProgramStore
	assignments      AssignmentListResult
	assignmentsErr   error
	syncResult       AssignmentSyncResult
	syncErr          error
	versions         VersionListResult
	versionsErr      error
	cleanupResult    VersionCleanupResult
	cleanupErr       error
	deleteVersionErr error
	freezeResult     Detail
	freezeErr        error
}

func (s *assignmentVersionStore) PublishFromDraft(_ context.Context, _, _ uuid.UUID, d Detail) (Detail, error) {
	if s.err != nil {
		return Detail{}, s.err
	}
	d.Status = StatusPublished
	return d, nil
}

func (s *assignmentVersionStore) FreezePublishedVersion(_ context.Context, _, _ uuid.UUID, d Detail) (Detail, error) {
	if s.freezeErr != nil {
		return Detail{}, s.freezeErr
	}
	if s.freezeResult.ID != uuid.Nil {
		return s.freezeResult, nil
	}
	d.HasUnpublishedChanges = false
	return d, nil
}

func (s *assignmentVersionStore) ListProgramAssignments(context.Context, uuid.UUID) (AssignmentListResult, error) {
	return s.assignments, s.assignmentsErr
}

func (s *assignmentVersionStore) SyncProgramAssignments(context.Context, uuid.UUID, uuid.UUID, AssignmentSyncRequest) (AssignmentSyncResult, error) {
	return s.syncResult, s.syncErr
}

func (s *assignmentVersionStore) ListProgramVersions(context.Context, uuid.UUID) (VersionListResult, error) {
	return s.versions, s.versionsErr
}

func (s *assignmentVersionStore) CleanupProgramVersions(context.Context, uuid.UUID) (VersionCleanupResult, error) {
	return s.cleanupResult, s.cleanupErr
}

func (s *assignmentVersionStore) DeleteProgramVersion(context.Context, uuid.UUID, uuid.UUID) error {
	return s.deleteVersionErr
}

func TestHandlers_PublishUpdate_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := publishableDetail(userID, programID)
	detail.Status = StatusPublished
	detail.HasUnpublishedChanges = true
	store := &assignmentVersionStore{fakeProgramStore: fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish-update", "", userID, map[string]string{"id": programID.String()})

	if err := h.PublishUpdate(c); err != nil {
		t.Fatalf("PublishUpdate: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_PublishUpdate_noChanges(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := publishableDetail(userID, programID)
	detail.Status = StatusPublished
	store := &assignmentVersionStore{fakeProgramStore: fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish-update", "", userID, map[string]string{"id": programID.String()})

	err := h.PublishUpdate(c)
	assertHTTPError(t, err, http.StatusUnprocessableEntity)
}

func TestHandlers_ListAssignments_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	latestID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		assignments: AssignmentListResult{
			Items:                  []Assignment{{ID: uuid.New(), ProgramID: programID}},
			LatestProgramVersionID: &latestID,
		},
	}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodGet, "/programs/"+programID.String()+"/assignments", "", userID, map[string]string{"id": programID.String()})

	if err := h.ListAssignments(c); err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_SyncAssignments_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		syncResult: AssignmentSyncResult{
			Synced: []Assignment{{ID: uuid.New(), ProgramID: programID}},
		},
	}
	h := programHandler(store)
	e := echo.New()
	body := `{"all_active":true}`
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/assignments/sync", body, userID, map[string]string{"id": programID.String()})

	if err := h.SyncAssignments(c); err != nil {
		t.Fatalf("SyncAssignments: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_SyncAssignments_invalidRequest(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/assignments/sync", `{}`, userID, map[string]string{"id": programID.String()})

	err := h.SyncAssignments(c)
	assertHTTPError(t, err, http.StatusUnprocessableEntity)
}

func TestHandlers_ListVersions_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	versionID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		versions: VersionListResult{Items: []VersionSummary{{
			ID: versionID, VersionNumber: 1, PublishedAt: time.Now().UTC(), CanDelete: false,
		}}},
	}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodGet, "/programs/"+programID.String()+"/versions", "", userID, map[string]string{"id": programID.String()})

	if err := h.ListVersions(c); err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_CleanupVersions_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	deletedID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		cleanupResult: VersionCleanupResult{DeletedVersionIDs: []uuid.UUID{deletedID}},
	}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/versions/cleanup", "", userID, map[string]string{"id": programID.String()})

	if err := h.CleanupVersions(c); err != nil {
		t.Fatalf("CleanupVersions: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteVersion_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	versionID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
	}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/versions/"+versionID.String(), "", userID, map[string]string{
		"id":         programID.String(),
		"version_id": versionID.String(),
	})

	if err := h.DeleteVersion(c); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}
	assertStatus(t, rec, http.StatusNoContent)
}

func TestHandlers_DeleteVersion_conflict(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	versionID := uuid.New()
	store := &assignmentVersionStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		deleteVersionErr: ErrVersionHasAssignments,
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/versions/"+versionID.String(), "", userID, map[string]string{
		"id":         programID.String(),
		"version_id": versionID.String(),
	})

	err := h.DeleteVersion(c)
	assertHTTPError(t, err, http.StatusConflict)
}

func TestMapProgramError_newCases(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{ErrReadOnly, http.StatusForbidden},
		{ErrClientNotLinked, http.StatusForbidden},
		{ErrClientBlocked, http.StatusUnprocessableEntity},
		{ErrProgramNotPublished, http.StatusUnprocessableEntity},
		{ErrClientNotFound, http.StatusNotFound},
		{ErrInvalidSyncRequest, http.StatusUnprocessableEntity},
		{ErrVersionHasAssignments, http.StatusConflict},
		{ErrSoleProgramVersion, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			he := HTTPErrorFrom(tt.err)
			if he.Code != tt.code {
				t.Fatalf("HTTPErrorFrom(%v).Code = %d, want %d", tt.err, he.Code, tt.code)
			}
		})
	}
}

func TestHandlers_ListVersions_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/programs/"+uuid.New().String()+"/versions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.ListVersions(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_CleanupVersions_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/versions/cleanup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.CleanupVersions(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_DeleteVersion_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/programs/"+uuid.New().String()+"/versions/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "version_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String())
	err := h.DeleteVersion(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_ListAssignments_invalidProgramID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs/not-a-uuid/assignments", "", uuid.New(), map[string]string{"id": "not-a-uuid"})
	err := h.ListAssignments(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_SyncAssignments_invalidProgramID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/not-a-uuid/assignments/sync", `{"all_active":true}`, uuid.New(), map[string]string{"id": "not-a-uuid"})
	err := h.SyncAssignments(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_ListVersions_invalidProgramID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs/not-a-uuid/versions", "", uuid.New(), map[string]string{"id": "not-a-uuid"})
	err := h.ListVersions(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_DeleteVersion_invalidVersionID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+uuid.New().String()+"/versions/bad", "", uuid.New(), map[string]string{
		"id":         uuid.New().String(),
		"version_id": "bad",
	})
	err := h.DeleteVersion(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_PublishUpdate_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/publish-update", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.PublishUpdate(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_ListAssignments_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/programs/"+uuid.New().String()+"/assignments", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.ListAssignments(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_SyncAssignments_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/assignments/sync", strings.NewReader(`{"all_active":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.SyncAssignments(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestMapProgramError_internal(t *testing.T) {
	he := mapProgramError(errors.New("boom"))
	if he.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", he.Code)
	}
}
