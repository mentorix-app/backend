package apicheck

import (
	"fmt"
	"reflect"
	"strings"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	"mentorix-backend/internal/program"
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
		{name: "ProgramDayExercise", typ: reflect.TypeOf(program.DayExercise{})},
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
		{name: "ProgramDayExerciseUpsert", typ: reflect.TypeOf(struct {
			ExerciseID  string   `json:"exercise_id"`
			Sets        *int     `json:"sets"`
			Reps        *int     `json:"reps"`
			WeightKg    *float64 `json:"weight_kg"`
			Instruction *string  `json:"instruction"`
		}{})},
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
			string(exercise.EquipmentAssaultBike), string(exercise.EquipmentJumpRope), string(exercise.EquipmentPlyoBox),
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
