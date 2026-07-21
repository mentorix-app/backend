package apicheck

import (
	"fmt"
	"reflect"
	"strings"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcomment"
)

type schemaBinding struct {
	name string
	typ  reflect.Type
}

func schemaBindings() []schemaBinding {
	return []schemaBinding{
		{name: "HealthStatus", typ: reflect.TypeOf(struct {
			Status string `json:"status"`
		}{})},
		{name: "ReadyResponse", typ: reflect.TypeOf(health.ReadyResponse{})},
		{name: "AuthCredentials", typ: reflect.TypeOf(auth.AuthCredentials{})},
		{name: "RegisterRequest", typ: reflect.TypeOf(auth.RegisterRequest{})},
		{name: "MePatchRequest", typ: reflect.TypeOf(auth.MePatchRequest{})},
		{name: "TokenResponse", typ: reflect.TypeOf(auth.TokenResponse{})},
		{name: "MeResponse", typ: reflect.TypeOf(auth.MeResponse{})},
		{name: "ExerciseListResponse", typ: reflect.TypeOf(exercise.ListResult{})},
		{name: "ExerciseDeleteRequest", typ: reflect.TypeOf(struct {
			IDs []string `json:"ids"`
		}{})},
		{name: "ExerciseDeleteResponse", typ: reflect.TypeOf(struct {
			DeletedCount int64 `json:"deleted_count"`
		}{})},
		{name: "Pagination", typ: reflect.TypeOf(exercise.Pagination{})},
		{name: "Exercise", typ: reflect.TypeOf(exercise.Exercise{})},
		{name: "ExerciseUpsert", typ: reflect.TypeOf(struct {
			Name            string                `json:"name"`
			NameRu          string                `json:"name_ru"`
			Equipment       *exercise.Equipment   `json:"equipment"`
			Type            exercise.ExerciseType `json:"type"`
			MuscleGroup     exercise.MuscleGroup  `json:"muscle_group"`
			Description     string                `json:"description"`
			DescriptionRu   string                `json:"description_ru"`
			Difficulty      exercise.Difficulty   `json:"difficulty"`
			VideoURL        string                `json:"video_url"`
			PreviewImageURL string                `json:"preview_image_url"`
		}{})},
		{name: "Program", typ: reflect.TypeOf(program.Program{})},
		{name: "ProgramWeek", typ: reflect.TypeOf(program.Week{})},
		{name: "ProgramDayExercise", typ: reflect.TypeOf(program.DayExercise{})},
		{name: "ProgramDayBlock", typ: reflect.TypeOf(program.DayBlock{})},
		{name: "ProgramDay", typ: reflect.TypeOf(program.Day{})},
		{name: "ProgramDetail", typ: reflect.TypeOf(program.Detail{})},
		{name: "ProgramListResponse", typ: reflect.TypeOf(program.ListResult{})},
		{name: "ProgramPatch", typ: reflect.TypeOf(struct {
			Name            *string              `json:"name"`
			NameRu          *string              `json:"name_ru"`
			Description     *string              `json:"description"`
			DescriptionRu   *string              `json:"description_ru"`
			Category        *program.Category    `json:"category"`
			Difficulty      *exercise.Difficulty `json:"difficulty"`
			PreviewImageURL *string              `json:"preview_image_url"`
		}{})},
		{name: "ProgramWeeksReorder", typ: reflect.TypeOf(struct {
			WeekIDs []string `json:"week_ids"`
		}{})},
		{name: "ProgramDaysReorder", typ: reflect.TypeOf(struct {
			DayIDs []string `json:"day_ids"`
		}{})},
		{name: "ProgramDayBlockCreate", typ: reflect.TypeOf(struct {
			BlockType program.BlockType `json:"block_type"`
			SortOrder int               `json:"sort_order"`
			Exercise  *struct {
				ExerciseID  string  `json:"exercise_id"`
				Sets        *string `json:"sets"`
				Reps        *string `json:"reps"`
				Instruction *string `json:"instruction"`
			} `json:"exercise"`
		}{})},
		{name: "ProgramDayBlocksReorder", typ: reflect.TypeOf(struct {
			BlockIDs []string `json:"block_ids"`
		}{})},
		{name: "ProgramBlockExercisesReorder", typ: reflect.TypeOf(struct {
			ExerciseItemIDs []string `json:"exercise_item_ids"`
		}{})},
		{name: "ProgramDayBlocksMerge", typ: reflect.TypeOf(struct {
			BlockIDs []string `json:"block_ids"`
		}{})},
		{name: "ProgramDayBlockPatch", typ: reflect.TypeOf(struct {
			BlockType   *program.BlockType `json:"block_type"`
			Instruction *string            `json:"instruction"`
		}{})},
		{name: "ProgramDayBlockMove", typ: reflect.TypeOf(struct {
			TargetDayID string `json:"target_day_id"`
			SortOrder   *int   `json:"sort_order"`
		}{})},
		{name: "ProgramBlockExerciseExtract", typ: reflect.TypeOf(struct {
			SortOrder *int `json:"sort_order"`
		}{})},
		{name: "ProgramExerciseMove", typ: reflect.TypeOf(struct {
			TargetBlockID string `json:"target_block_id"`
		}{})},
		{name: "ProgramDayExerciseUpsert", typ: reflect.TypeOf(struct {
			ExerciseID  string  `json:"exercise_id"`
			Sets        *string `json:"sets"`
			Reps        *string `json:"reps"`
			Instruction *string `json:"instruction"`
		}{})},
		{name: "ProgramAssignment", typ: reflect.TypeOf(program.Assignment{})},
		{name: "ProgramAssignmentListResponse", typ: reflect.TypeOf(program.AssignmentListResult{})},
		{name: "BulkSetClientProgramAssignmentRequest", typ: reflect.TypeOf(program.BulkSetClientProgramAssignmentRequest{})},
		{name: "BulkClientProgramAssignmentSkipped", typ: reflect.TypeOf(program.BulkAssignmentSkipped{})},
		{name: "BulkClientProgramAssignmentResponse", typ: reflect.TypeOf(program.BulkAssignmentResult{})},
		{name: "ProgramAssignmentSyncRequest", typ: reflect.TypeOf(program.AssignmentSyncRequest{})},
		{name: "ProgramAssignmentSyncSkipped", typ: reflect.TypeOf(program.AssignmentSyncSkipped{})},
		{name: "ProgramAssignmentSyncResponse", typ: reflect.TypeOf(program.AssignmentSyncResult{})},
		{name: "ProgramVersionSummary", typ: reflect.TypeOf(program.VersionSummary{})},
		{name: "ProgramVersionListResponse", typ: reflect.TypeOf(program.VersionListResult{})},
		{name: "ProgramVersionCleanupSkipped", typ: reflect.TypeOf(program.VersionCleanupSkipped{})},
		{name: "ProgramVersionCleanupResponse", typ: reflect.TypeOf(program.VersionCleanupResult{})},
		{name: "TrainerInvite", typ: reflect.TypeOf(trainerclient.Invite{})},
		{name: "TrainerClientProgramSummary", typ: reflect.TypeOf(trainerclient.ClientProgramSummary{})},
		{name: "TrainerClient", typ: reflect.TypeOf(trainerclient.Client{})},
		{name: "TrainerClientListResponse", typ: reflect.TypeOf(trainerclient.ClientListResult{})},
		{name: "ClientAnalytics", typ: reflect.TypeOf(analytics.ClientAnalytics{})},
		{name: "AnalyticsClientInfo", typ: reflect.TypeOf(analytics.ClientInfo{})},
		{name: "AnalyticsAssignment", typ: reflect.TypeOf(analytics.AssignmentAnalytics{})},
		{name: "AnalyticsProgress", typ: reflect.TypeOf(analytics.Progress{})},
		{name: "AnalyticsWeekProgress", typ: reflect.TypeOf(analytics.WeekProgress{})},
		{name: "AnalyticsActivity", typ: reflect.TypeOf(analytics.ActivityStats{})},
		{name: "AnalyticsProgramActivity", typ: reflect.TypeOf(analytics.ProgramActivity{})},
		{name: "ClientCompletionsResponse", typ: reflect.TypeOf(analytics.CompletionsResult{})},
		{name: "ClientCompletionItem", typ: reflect.TypeOf(analytics.CompletionItem{})},
		{name: "CompletionComment", typ: reflect.TypeOf(workoutcomment.Comment{})},
		{name: "CreateCompletionCommentRequest", typ: reflect.TypeOf(struct {
			Text string `json:"text"`
		}{})},
		{name: "ProgramsAnalyticsResponse", typ: reflect.TypeOf(analytics.ProgramsAnalyticsResult{})},
		{name: "ProgramAnalyticsItem", typ: reflect.TypeOf(analytics.ProgramAnalyticsItem{})},
		{name: "ProgramAnalytics", typ: reflect.TypeOf(analytics.ProgramAnalytics{})},
		{name: "ProgramAnalyticsHeader", typ: reflect.TypeOf(analytics.ProgramHeader{})},
		{name: "ProgramAnalyticsSummary", typ: reflect.TypeOf(analytics.ProgramSummary{})},
		{name: "ProgramAnalyticsClient", typ: reflect.TypeOf(analytics.ProgramClient{})},
		{name: "ProgramAnalyticsWeek", typ: reflect.TypeOf(analytics.ProgramWeekStats{})},
	}
}

func jsonFieldNames(t reflect.Type) map[string]struct{} {
	out := make(map[string]struct{})
	collectJSONFields(t, out)
	return out
}

func collectJSONFields(t reflect.Type, out map[string]struct{}) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			collectJSONFields(f.Type, out)
			continue
		}
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = struct{}{}
	}
}

func comparePropertySets(schemaName string, openapi, goFields map[string]struct{}) error {
	var missingInGo, missingInOpenAPI []string
	for k := range openapi {
		if _, ok := goFields[k]; !ok {
			missingInGo = append(missingInGo, k)
		}
	}
	for k := range goFields {
		if _, ok := openapi[k]; !ok {
			missingInOpenAPI = append(missingInOpenAPI, k)
		}
	}
	if len(missingInGo) == 0 && len(missingInOpenAPI) == 0 {
		return nil
	}
	return fmt.Errorf("schema %s: openapi-only %v; go-only %v", schemaName, missingInGo, missingInOpenAPI)
}

type enumBinding struct {
	name   string
	values []string
}

func enumBindings() []enumBinding {
	return []enumBinding{
		{name: "MuscleGroup", values: []string{
			string(exercise.MuscleGroupCompound), string(exercise.MuscleGroupChest), string(exercise.MuscleGroupBack),
			string(exercise.MuscleGroupLegs), string(exercise.MuscleGroupShoulders), string(exercise.MuscleGroupArms),
			string(exercise.MuscleGroupCore), string(exercise.MuscleGroupFullBody),
		}},
		{name: "ExerciseType", values: []string{
			string(exercise.ExerciseTypeStrength), string(exercise.ExerciseTypeCardio), string(exercise.ExerciseTypeMixedModal),
			string(exercise.ExerciseTypeIntervals), string(exercise.ExerciseTypeStretching), string(exercise.ExerciseTypeMetcon),
			string(exercise.ExerciseTypeSkillWork), string(exercise.ExerciseTypeAccessory),
		}},
		{name: "Equipment", values: []string{
			string(exercise.EquipmentBarbell), string(exercise.EquipmentDumbbells), string(exercise.EquipmentKettlebell),
			string(exercise.EquipmentPullUpBar), string(exercise.EquipmentSquatRack), string(exercise.EquipmentRowingMachine),
			string(exercise.EquipmentAssaultBike), string(exercise.EquipmentBikeErg), string(exercise.EquipmentSkiErg),
			string(exercise.EquipmentJumpRope), string(exercise.EquipmentPlyoBox),
			string(exercise.EquipmentMedicineBall), string(exercise.EquipmentWallBall), string(exercise.EquipmentResistanceBands),
			string(exercise.EquipmentBattleRopes), string(exercise.EquipmentGymnasticRings), string(exercise.EquipmentSandbag),
			string(exercise.EquipmentSled), string(exercise.EquipmentWeightPlates),
		}},
		{name: "Difficulty", values: []string{
			string(exercise.DifficultyBeginner), string(exercise.DifficultyIntermediate),
			string(exercise.DifficultyAdvanced), string(exercise.DifficultyExpert),
		}},
		{name: "ProgramStatus", values: []string{
			string(program.StatusDraft), string(program.StatusPublished), string(program.StatusArchived),
		}},
		{name: "ProgramCategory", values: []string{
			string(program.CategoryWeightLoss), string(program.CategoryMuscleGain), string(program.CategoryRehabilitation),
			string(program.CategoryEndurance), string(program.CategoryFunctional),
		}},
		{name: "ProgramAssignmentStatus", values: []string{
			string(program.AssignmentStatusActive), string(program.AssignmentStatusCompleted), string(program.AssignmentStatusCancelled),
		}},
		{name: "ProgramBlockType", values: []string{
			string(program.BlockTypeSingle), string(program.BlockTypeEMOM), string(program.BlockTypeAMRAP),
			string(program.BlockTypeForTime), string(program.BlockTypeIntervals), string(program.BlockTypeChipper),
			string(program.BlockTypeLadder), string(program.BlockTypeDeathBy), string(program.BlockTypeSuperset),
			string(program.BlockTypeComplex), string(program.BlockTypeSkillWork), string(program.BlockTypeStrength),
			string(program.BlockTypeConditioning), string(program.BlockTypeGymnastics), string(program.BlockTypeWeightlifting),
		}},
	}
}

func compareEnumSets(name string, openapi, goVals []string) error {
	openSet := make(map[string]struct{}, len(openapi))
	for _, v := range openapi {
		openSet[v] = struct{}{}
	}
	goSet := make(map[string]struct{}, len(goVals))
	for _, v := range goVals {
		goSet[v] = struct{}{}
	}
	var missingInGo, missingInOpenAPI []string
	for v := range openSet {
		if _, ok := goSet[v]; !ok {
			missingInGo = append(missingInGo, v)
		}
	}
	for v := range goSet {
		if _, ok := openSet[v]; !ok {
			missingInOpenAPI = append(missingInOpenAPI, v)
		}
	}
	if len(missingInGo) == 0 && len(missingInOpenAPI) == 0 {
		return nil
	}
	return fmt.Errorf("enum %s: openapi-only %v; go-only %v", name, missingInGo, missingInOpenAPI)
}
