package trainerclient

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/workoutcompletion"
)

var (
	// ErrProgramChanged means the assignment or version the app sent is no longer
	// the client's current one, or the client has none.
	ErrProgramChanged = errors.New("program changed")
	// ErrDayNotFound means the day key is not a workout day of the program the
	// client currently sees.
	ErrDayNotFound = errors.New("day not found")
)

// ClientProgramAssignment is the client's current assignment as the app sees it. The
// app sends the three ids back when it marks a day: after a reassignment, even one that
// brings the same assignment and version back, the cycle differs.
type ClientProgramAssignment struct {
	AssignmentID      uuid.UUID `json:"assignment_id"`
	ProgramID         uuid.UUID `json:"program_id"`
	ProgramVersionID  uuid.UUID `json:"program_version_id"`
	CompletionCycleID uuid.UUID `json:"completion_cycle_id"`
	AssignedAt        time.Time `json:"assigned_at"`
}

// ClientProgram is the program as the app shows it: the version's content with only the
// blocks the client may see, and none of the fields the trainer's editor uses.
type ClientProgram struct {
	ProgramID       uuid.UUID           `json:"program_id"`
	Name            string              `json:"name"`
	NameRu          string              `json:"name_ru"`
	Description     string              `json:"description"`
	DescriptionRu   string              `json:"description_ru"`
	Category        *program.Category   `json:"category"`
	Difficulty      *program.Difficulty `json:"difficulty"`
	PreviewImageURL string              `json:"preview_image_url"`
	Weeks           []program.Week      `json:"weeks"`
}

// ClientCompletedDay is a day marked done in the current cycle. It is an object so
// per-day fields can be added later.
type ClientCompletedDay struct {
	DayKey uuid.UUID `json:"day_key"`
}

// ClientProgramResponse is the body of GET /client/trainers/{trainer_id}/program.
// Assignment and Program are null while the trainer has not assigned a program.
type ClientProgramResponse struct {
	TrainerID          uuid.UUID                `json:"trainer_id"`
	TrainerDisplayName string                   `json:"trainer_display_name"`
	Assignment         *ClientProgramAssignment `json:"assignment"`
	Program            *ClientProgram           `json:"program"`
	CompletedDays      []ClientCompletedDay     `json:"completed_days"`
}

// ClientCompletionRequest is the body of POST /client/trainers/{trainer_id}/completions.
// The three ids say which assignment cycle the app was looking at.
type ClientCompletionRequest struct {
	ProgramAssignmentID uuid.UUID `json:"program_assignment_id"`
	ProgramVersionID    uuid.UUID `json:"program_version_id"`
	CompletionCycleID   uuid.UUID `json:"completion_cycle_id"`
	DayKey              uuid.UUID `json:"day_key"`
	ResultText          string    `json:"result_text"`
}

// ClientCompletionResponse is the body of a successful mark.
type ClientCompletionResponse struct {
	ID          uuid.UUID `json:"id"`
	DayKey      uuid.UUID `json:"day_key"`
	WeekNumber  int       `json:"week_number"`
	DayNumber   int       `json:"day_number"`
	CompletedAt time.Time `json:"completed_at"`
	ResultText  string    `json:"result_text"`
}

// completionLog is the part of workoutcompletion.Service the app endpoints use.
type completionLog interface {
	CompletedDayKeys(ctx context.Context, completionCycleID uuid.UUID) (map[uuid.UUID]struct{}, error)
	CompleteForAssignment(ctx context.Context, in workoutcompletion.CompleteInput) (workoutcompletion.Completion, error)
}

// GetClientProgram is the program view of the app: one trainer named by the caller,
// no active-trainer state. The link check is the bot's. A caller who is not linked
// gets program.ErrClientNotLinked, a blocked one program.ErrClientBlocked.
func (s *Service) GetClientProgram(ctx context.Context, clientUserID, trainerID uuid.UUID) (ClientProgramResponse, error) {
	name, assignment, detail, err := s.appProgramView(ctx, clientUserID, trainerID)
	if err != nil {
		return ClientProgramResponse{}, err
	}
	resp := ClientProgramResponse{
		TrainerID:          trainerID,
		TrainerDisplayName: name,
		CompletedDays:      []ClientCompletedDay{},
	}
	if assignment == nil {
		return resp, nil
	}

	done, err := s.completions.CompletedDayKeys(ctx, assignment.CompletionCycleID)
	if err != nil {
		return ClientProgramResponse{}, err
	}
	for key := range done {
		resp.CompletedDays = append(resp.CompletedDays, ClientCompletedDay{DayKey: key})
	}
	sort.Slice(resp.CompletedDays, func(i, j int) bool {
		return resp.CompletedDays[i].DayKey.String() < resp.CompletedDays[j].DayKey.String()
	})

	resp.Assignment = &ClientProgramAssignment{
		AssignmentID:      assignment.ID,
		ProgramID:         assignment.ProgramID,
		ProgramVersionID:  assignment.ProgramVersionID,
		CompletionCycleID: assignment.CompletionCycleID,
		AssignedAt:        assignment.AssignedAt,
	}
	clientProgram := newClientProgram(detail, clientUserID)
	resp.Program = &clientProgram
	return resp, nil
}

// CompleteClientDay marks one day of the client's current program done, as the bot
// does after a text result. The request must still name the current assignment, version
// and cycle: after a reassignment the app gets ErrProgramChanged instead of a mark in
// the wrong cycle. A day that is not a workout day of that version is ErrDayNotFound and
// a day already marked is workoutcompletion.ErrAlreadyCompleted.
//
// The checks here give the clear answers. The write itself is conditional on the same
// three ids (workoutcompletion.Service.CompleteForAssignment), so a reassignment that
// lands between the checks and the insert is refused too.
func (s *Service) CompleteClientDay(ctx context.Context, clientUserID, trainerID uuid.UUID, req ClientCompletionRequest) (ClientCompletionResponse, error) {
	_, assignment, detail, err := s.appProgramView(ctx, clientUserID, trainerID)
	if err != nil {
		return ClientCompletionResponse{}, err
	}
	return s.markDay(ctx, clientUserID, trainerID, assignment, detail, req)
}

// markDay is CompleteClientDay after the program view was read.
func (s *Service) markDay(ctx context.Context, clientUserID, trainerID uuid.UUID, assignment *program.Assignment, detail program.Detail, req ClientCompletionRequest) (ClientCompletionResponse, error) {
	if !matchesCurrentAssignment(assignment, req) {
		return ClientCompletionResponse{}, ErrProgramChanged
	}
	weekNumber, dayNumber, day, ok := findWorkoutDay(detail, req.DayKey)
	if !ok {
		return ClientCompletionResponse{}, ErrDayNotFound
	}

	c, err := s.completions.CompleteForAssignment(ctx, workoutcompletion.CompleteInput{
		ClientUserID:        clientUserID,
		TrainerID:           trainerID,
		ProgramID:           assignment.ProgramID,
		ProgramVersionID:    assignment.ProgramVersionID,
		ProgramAssignmentID: assignment.ID,
		CompletionCycleID:   assignment.CompletionCycleID,
		DayKey:              req.DayKey,
		WeekNumber:          weekNumber,
		DayNumber:           dayNumber,
		ProgramName:         detail.Name,
		ProgramNameRu:       detail.NameRu,
		Day:                 day,
		ResultText:          req.ResultText,
		Source:              workoutcompletion.SourceApp,
	})
	if err != nil {
		if errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
			return ClientCompletionResponse{}, ErrProgramChanged
		}
		return ClientCompletionResponse{}, err
	}
	return ClientCompletionResponse{
		ID:          c.ID,
		DayKey:      c.DayKey,
		WeekNumber:  c.WeekNumber,
		DayNumber:   c.DayNumber,
		CompletedAt: c.CompletedAt,
		ResultText:  c.ResultText,
	}, nil
}

// appProgramView is programViewFor for a caller who is named by the access token. The
// bot's "trainer is not linked" error becomes the one the API already answers with
// client_not_linked, so one text has one code.
func (s *Service) appProgramView(ctx context.Context, clientUserID, trainerID uuid.UUID) (string, *program.Assignment, program.Detail, error) {
	name, assignment, detail, err := s.programViewFor(ctx, clientUserID, trainerID)
	if err != nil {
		if errors.Is(err, ErrTrainerNotLinked) {
			return "", nil, program.Detail{}, program.ErrClientNotLinked
		}
		return "", nil, program.Detail{}, err
	}
	return name, assignment, detail, nil
}

func matchesCurrentAssignment(current *program.Assignment, req ClientCompletionRequest) bool {
	return current != nil &&
		current.ID == req.ProgramAssignmentID &&
		current.ProgramVersionID == req.ProgramVersionID &&
		current.CompletionCycleID == req.CompletionCycleID
}

// newClientProgram builds the app's view of the version the client is on.
func newClientProgram(d program.Detail, clientUserID uuid.UUID) ClientProgram {
	return ClientProgram{
		ProgramID:       d.ID,
		Name:            d.Name,
		NameRu:          d.NameRu,
		Description:     d.Description,
		DescriptionRu:   d.DescriptionRu,
		Category:        d.Category,
		Difficulty:      d.Difficulty,
		PreviewImageURL: d.PreviewImageURL,
		Weeks:           hideOtherClients(d, clientUserID).Weeks,
	}
}

// findWorkoutDay finds the day with the key among the days the client sees. A day
// that holds no exercise is a rest day and cannot be marked, as in the bot.
func findWorkoutDay(d program.Detail, dayKey uuid.UUID) (weekNumber, dayNumber int, day program.Day, ok bool) {
	for _, w := range d.Weeks {
		for _, candidate := range w.Days {
			if candidate.DayKey != dayKey {
				continue
			}
			for _, b := range candidate.Blocks {
				if len(b.Exercises) > 0 {
					return w.WeekNumber, candidate.DayNumber, candidate, true
				}
			}
			return 0, 0, program.Day{}, false
		}
	}
	return 0, 0, program.Day{}, false
}

// hideOtherClients rewrites the visibility rule of every block so that the answer
// never names another client: a restricted block lists only the caller, a shared
// block an empty list. It returns a copy and leaves d alone.
func hideOtherClients(d program.Detail, clientUserID uuid.UUID) program.Detail {
	out := d
	out.Weeks = make([]program.Week, 0, len(d.Weeks))
	for _, week := range d.Weeks {
		w := week
		w.Days = make([]program.Day, 0, len(week.Days))
		for _, day := range week.Days {
			dd := day
			dd.Blocks = make([]program.DayBlock, 0, len(day.Blocks))
			for _, block := range day.Blocks {
				b := block
				b.ClientUserIDs = []uuid.UUID{}
				if len(block.ClientUserIDs) > 0 {
					b.ClientUserIDs = []uuid.UUID{clientUserID}
				}
				dd.Blocks = append(dd.Blocks, b)
			}
			w.Days = append(w.Days, dd)
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out
}
