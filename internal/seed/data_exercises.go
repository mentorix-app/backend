package seed

import "mentorix-backend/internal/exercise"

func exerciseCatalog() []exercise.UpsertInput {
	barbell := exercise.EquipmentBarbell
	dumbbells := exercise.EquipmentDumbbells
	kettlebell := exercise.EquipmentKettlebell
	pullUpBar := exercise.EquipmentPullUpBar
	squatRack := exercise.EquipmentSquatRack
	rowing := exercise.EquipmentRowingMachine
	assaultBike := exercise.EquipmentAssaultBike
	jumpRope := exercise.EquipmentJumpRope
	plyoBox := exercise.EquipmentPlyoBox
	medicineBall := exercise.EquipmentMedicineBall
	resistanceBands := exercise.EquipmentResistanceBands
	rings := exercise.EquipmentGymnasticRings

	return []exercise.UpsertInput{
		{Name: "Back Squat", NameRu: "Присед со штангой", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyIntermediate, Equipment: &barbell},
		{Name: "Bench Press", NameRu: "Жим лёжа", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupChest, Difficulty: exercise.DifficultyIntermediate, Equipment: &barbell},
		{Name: "Deadlift", NameRu: "Становая тяга", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupBack, Difficulty: exercise.DifficultyAdvanced, Equipment: &barbell},
		{Name: "Overhead Press", NameRu: "Жим стоя", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupShoulders, Difficulty: exercise.DifficultyIntermediate, Equipment: &barbell},
		{Name: "Pull-up", NameRu: "Подтягивание", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupBack, Difficulty: exercise.DifficultyAdvanced, Equipment: &pullUpBar},
		{Name: "Front Squat", NameRu: "Фронтальный присед", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyAdvanced, Equipment: &squatRack},
		{Name: "Dumbbell Row", NameRu: "Тяга гантели", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupBack, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Kettlebell Swing", NameRu: "Мах гири", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyIntermediate, Equipment: &kettlebell},
		{Name: "Rowing Machine", NameRu: "Гребля", Type: exercise.ExerciseTypeCardio, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyBeginner, Equipment: &rowing},
		{Name: "Assault Bike Sprint", NameRu: "Спринт на байке", Type: exercise.ExerciseTypeIntervals, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyIntermediate, Equipment: &assaultBike},
		{Name: "Jump Rope", NameRu: "Скакалка", Type: exercise.ExerciseTypeCardio, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyBeginner, Equipment: &jumpRope},
		{Name: "Burpee", NameRu: "Берпи", Type: exercise.ExerciseTypeMetcon, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyIntermediate},
		{Name: "Box Jump", NameRu: "Прыжок на тумбу", Type: exercise.ExerciseTypeMixedModal, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyIntermediate, Equipment: &plyoBox},
		{Name: "Plank", NameRu: "Планка", Type: exercise.ExerciseTypeStretching, MuscleGroup: exercise.MuscleGroupCore, Difficulty: exercise.DifficultyBeginner},
		{Name: "Ring Dip", NameRu: "Отжимание на кольцах", Type: exercise.ExerciseTypeSkillWork, MuscleGroup: exercise.MuscleGroupArms, Difficulty: exercise.DifficultyExpert, Equipment: &rings},
		{Name: "Band Pull Apart", NameRu: "Разведение резинки", Type: exercise.ExerciseTypeAccessory, MuscleGroup: exercise.MuscleGroupShoulders, Difficulty: exercise.DifficultyBeginner, Equipment: &resistanceBands},
		{Name: "Wall Ball Shot", NameRu: "Бросок мяча", Type: exercise.ExerciseTypeMetcon, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyIntermediate, Equipment: &medicineBall},
		{Name: "Farmer Carry", NameRu: "Прогулка фермера", Type: exercise.ExerciseTypeMixedModal, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Push-up", NameRu: "Отжимание", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupChest, Difficulty: exercise.DifficultyBeginner},
		{Name: "Hip Thrust", NameRu: "Ягодичный мост", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyIntermediate, Equipment: &barbell},
		{Name: "Romanian Deadlift", NameRu: "Румынская тяга", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyIntermediate, Equipment: &barbell},
		{Name: "Lat Pulldown", NameRu: "Тяга верхнего блока", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupBack, Difficulty: exercise.DifficultyBeginner, Equipment: &resistanceBands},
		{Name: "Bicep Curl", NameRu: "Сгибание на бицепс", Type: exercise.ExerciseTypeAccessory, MuscleGroup: exercise.MuscleGroupArms, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Tricep Extension", NameRu: "Разгибание на трицепс", Type: exercise.ExerciseTypeAccessory, MuscleGroup: exercise.MuscleGroupArms, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Lunge", NameRu: "Выпады", Type: exercise.ExerciseTypeStrength, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Mountain Climber", NameRu: "Альпинист", Type: exercise.ExerciseTypeCardio, MuscleGroup: exercise.MuscleGroupCore, Difficulty: exercise.DifficultyIntermediate},
		{Name: "Turkish Get-up", NameRu: "Турецкий подъём", Type: exercise.ExerciseTypeSkillWork, MuscleGroup: exercise.MuscleGroupFullBody, Difficulty: exercise.DifficultyExpert, Equipment: &kettlebell},
		{Name: "Thruster Complex", NameRu: "Трастер комплекс", Type: exercise.ExerciseTypeMetcon, MuscleGroup: exercise.MuscleGroupCompound, Difficulty: exercise.DifficultyAdvanced, Equipment: &barbell},
		{Name: "Calf Raise", NameRu: "Подъём на носки", Type: exercise.ExerciseTypeAccessory, MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyBeginner, Equipment: &dumbbells},
		{Name: "Russian Twist", NameRu: "Русский твист", Type: exercise.ExerciseTypeStretching, MuscleGroup: exercise.MuscleGroupCore, Difficulty: exercise.DifficultyBeginner, Equipment: &medicineBall},
	}
}
