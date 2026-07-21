package workoutcomment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/telegramnotify"
)

type fakeStore struct {
	trainerID     uuid.UUID
	trainerErr    error
	completion    CompletionInfo
	completionErr error
	comment       Comment
	commentErr    error

	gotCompletionID uuid.UUID
	gotTrainerID    uuid.UUID
	gotText         string
}

func (f *fakeStore) TrainerIDForUser(context.Context, uuid.UUID) (uuid.UUID, error) {
	return f.trainerID, f.trainerErr
}

func (f *fakeStore) CompletionForComment(_ context.Context, completionID, trainerID, _ uuid.UUID) (CompletionInfo, error) {
	f.gotCompletionID = completionID
	f.gotTrainerID = trainerID
	return f.completion, f.completionErr
}

func (f *fakeStore) CreateComment(_ context.Context, _, _ uuid.UUID, text string) (Comment, error) {
	f.gotText = text
	return f.comment, f.commentErr
}

type recordingNotifier struct {
	calls        int
	clientUserID uuid.UUID
	trainerID    uuid.UUID
	comment      telegramnotify.WorkoutComment
	err          error
}

func (r *recordingNotifier) NotifyWorkoutCommented(_ context.Context, clientUserID, trainerID uuid.UUID, comment telegramnotify.WorkoutComment) error {
	r.calls++
	r.clientUserID = clientUserID
	r.trainerID = trainerID
	r.comment = comment
	return r.err
}

func TestService_CreateComment_validation(t *testing.T) {
	svc := NewServiceWithStore(&fakeStore{})
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t "},
		{"too long", strings.Repeat("ы", MaxCommentTextLen+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateComment(context.Background(), uuid.New(), uuid.New(), uuid.New(), tt.text)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}

func TestService_CreateComment_storeErrors(t *testing.T) {
	tests := []struct {
		name  string
		store *fakeStore
		want  error
	}{
		{"not a trainer", &fakeStore{trainerErr: ErrForbidden}, ErrForbidden},
		{"completion not found", &fakeStore{completionErr: ErrCompletionNotFound}, ErrCompletionNotFound},
		{"comment exists", &fakeStore{commentErr: ErrCommentExists}, ErrCommentExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &recordingNotifier{}
			svc := NewServiceWithStore(tt.store, WithCommentNotifier(notifier))
			_, err := svc.CreateComment(context.Background(), uuid.New(), uuid.New(), uuid.New(), "text")
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if notifier.calls != 0 {
				t.Fatalf("notifier calls = %d, want 0", notifier.calls)
			}
		})
	}
}

func TestService_CreateComment_successNotifies(t *testing.T) {
	clientUserID := uuid.New()
	trainerID := uuid.New()
	completionID := uuid.New()
	store := &fakeStore{
		trainerID: trainerID,
		completion: CompletionInfo{
			ID:            completionID,
			ClientUserID:  clientUserID,
			WeekNumber:    2,
			DayNumber:     3,
			ProgramName:   "Strength",
			ProgramNameRu: "Сила",
			ResultText:    "присед 5х5 90 кг",
		},
		comment: Comment{ID: uuid.New(), Text: "Отличная работа!", CreatedAt: time.Now().UTC()},
	}
	notifier := &recordingNotifier{}
	svc := NewServiceWithStore(store, WithCommentNotifier(notifier))

	got, err := svc.CreateComment(context.Background(), uuid.New(), clientUserID, completionID, "  Отличная работа!  ")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if got.Text != "Отличная работа!" {
		t.Fatalf("text = %q", got.Text)
	}
	if store.gotText != "Отличная работа!" {
		t.Fatalf("stored text = %q, want trimmed", store.gotText)
	}
	if store.gotCompletionID != completionID || store.gotTrainerID != trainerID {
		t.Fatalf("store args = %v %v", store.gotCompletionID, store.gotTrainerID)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
	if notifier.clientUserID != clientUserID || notifier.trainerID != trainerID {
		t.Fatalf("notifier ids = %v %v", notifier.clientUserID, notifier.trainerID)
	}
	want := telegramnotify.WorkoutComment{
		ProgramName:   "Strength",
		ProgramNameRu: "Сила",
		WeekNumber:    2,
		DayNumber:     3,
		ResultText:    "присед 5х5 90 кг",
		CommentText:   "Отличная работа!",
	}
	if notifier.comment != want {
		t.Fatalf("notifier comment = %+v, want %+v", notifier.comment, want)
	}
}

func TestService_CreateComment_notifierErrorIgnored(t *testing.T) {
	store := &fakeStore{comment: Comment{ID: uuid.New(), Text: "ok"}}
	notifier := &recordingNotifier{err: errors.New("telegram down")}
	svc := NewServiceWithStore(store, WithCommentNotifier(notifier))

	if _, err := svc.CreateComment(context.Background(), uuid.New(), uuid.New(), uuid.New(), "ok"); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
}

func TestService_CreateComment_nilNotifier(t *testing.T) {
	svc := NewServiceWithStore(&fakeStore{comment: Comment{ID: uuid.New(), Text: "ok"}})
	if _, err := svc.CreateComment(context.Background(), uuid.New(), uuid.New(), uuid.New(), "ok"); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
}
