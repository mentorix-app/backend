package seed

import (
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

type programTargetStatus string

const (
	programStatusDraft     programTargetStatus = "draft"
	programStatusPublished programTargetStatus = "published"
	programStatusArchived  programTargetStatus = "archived"
)

type programSeed struct {
	Name          string
	NameRu        string
	Description   string
	DescriptionRu string
	Category      *program.Category
	Difficulty    *exercise.Difficulty
	TargetStatus  programTargetStatus
	ExerciseNames []string
	SkipMetadata  bool
}

func programCatalog() []programSeed {
	catWeightLoss := program.CategoryWeightLoss
	catMuscleGain := program.CategoryMuscleGain
	catRehab := program.CategoryRehabilitation
	catEndurance := program.CategoryEndurance
	catFunctional := program.CategoryFunctional

	diffBeginner := exercise.DifficultyBeginner
	diffIntermediate := exercise.DifficultyIntermediate
	diffAdvanced := exercise.DifficultyAdvanced
	diffExpert := exercise.DifficultyExpert

	return []programSeed{
		// published (8)
		{Name: "Mass gain basics", NameRu: "Набор массы — база", Description: "8-week hypertrophy block", DescriptionRu: "8-недельный блок гипертрофии", Category: &catMuscleGain, Difficulty: &diffBeginner, TargetStatus: programStatusPublished, ExerciseNames: []string{"Back Squat", "Bench Press"}},
		{Name: "Weight loss starter", NameRu: "Похудение — старт", Description: "Fat loss fundamentals", DescriptionRu: "Основы снижения веса", Category: &catWeightLoss, Difficulty: &diffBeginner, TargetStatus: programStatusPublished, ExerciseNames: []string{"Burpee", "Rowing Machine"}},
		{Name: "Endurance block", NameRu: "Выносливость — блок", Description: "Cardio capacity", DescriptionRu: "Развитие выносливости", Category: &catEndurance, Difficulty: &diffIntermediate, TargetStatus: programStatusPublished, ExerciseNames: []string{"Assault Bike Sprint", "Jump Rope"}},
		{Name: "Functional strength", NameRu: "Функциональная сила", Description: "Full-body functional work", DescriptionRu: "Функциональная работа", Category: &catFunctional, Difficulty: &diffIntermediate, TargetStatus: programStatusPublished, ExerciseNames: []string{"Kettlebell Swing", "Farmer Carry"}},
		{Name: "Rehab mobility", NameRu: "Реабилитация — мобильность", Description: "Gentle return to training", DescriptionRu: "Мягкий возврат к тренировкам", Category: &catRehab, Difficulty: &diffBeginner, TargetStatus: programStatusPublished, ExerciseNames: []string{"Plank", "Band Pull Apart"}},
		{Name: "Advanced hypertrophy", NameRu: "Гипертрофия — продвинутый", Description: "High volume strength", DescriptionRu: "Силовой объём", Category: &catMuscleGain, Difficulty: &diffAdvanced, TargetStatus: programStatusPublished, ExerciseNames: []string{"Deadlift", "Front Squat"}},
		{Name: "Expert power cycle", NameRu: "Сила — эксперт", Description: "Peak strength phase", DescriptionRu: "Пиковая силовая фаза", Category: &catMuscleGain, Difficulty: &diffExpert, TargetStatus: programStatusPublished, ExerciseNames: []string{"Back Squat", "Overhead Press"}},
		{Name: "Cardio burn", NameRu: "Кардио — сжигание", Description: "Metcon focus", DescriptionRu: "Меткон фокус", Category: &catWeightLoss, Difficulty: &diffIntermediate, TargetStatus: programStatusPublished, ExerciseNames: []string{"Wall Ball Shot", "Mountain Climber"}},

		// draft (7)
		{Name: "Strength block WIP", NameRu: "Силовой блок — черновик", Description: "Work in progress", DescriptionRu: "В работе", Category: &catMuscleGain, Difficulty: &diffIntermediate, TargetStatus: programStatusDraft, ExerciseNames: []string{"Bench Press"}},
		{Name: "Draft endurance plan", NameRu: "Черновик выносливости", Description: "", DescriptionRu: "", Category: &catEndurance, Difficulty: &diffBeginner, TargetStatus: programStatusDraft},
		{Name: "Draft functional WIP", NameRu: "Черновик функционал", Description: "Unfinished", DescriptionRu: "Не завершено", Category: &catFunctional, Difficulty: &diffBeginner, TargetStatus: programStatusDraft},
		{Name: "Draft rehab outline", NameRu: "Черновик реабилитации", Category: &catRehab, Difficulty: &diffBeginner, TargetStatus: programStatusDraft},
		{Name: "Draft weight loss", NameRu: "Черновик похудения", Category: &catWeightLoss, Difficulty: &diffIntermediate, TargetStatus: programStatusDraft, ExerciseNames: []string{"Burpee"}},
		{Name: "Wizard empty draft", NameRu: "", TargetStatus: programStatusDraft, SkipMetadata: true},
		{Name: "Draft expert prep", NameRu: "Черновик эксперт подготовка", Category: &catMuscleGain, Difficulty: &diffExpert, TargetStatus: programStatusDraft},

		// archived (5)
		{Name: "Archived mass block", NameRu: "Архив — набор массы", Description: "Old hypertrophy template", DescriptionRu: "Старый шаблон", Category: &catMuscleGain, Difficulty: &diffBeginner, TargetStatus: programStatusArchived, ExerciseNames: []string{"Back Squat", "Romanian Deadlift"}},
		{Name: "Archived cut phase", NameRu: "Архив — сушка", Category: &catWeightLoss, Difficulty: &diffIntermediate, TargetStatus: programStatusArchived, ExerciseNames: []string{"Rowing Machine", "Burpee"}},
		{Name: "Archived endurance old", NameRu: "Архив — выносливость", Category: &catEndurance, Difficulty: &diffAdvanced, TargetStatus: programStatusArchived, ExerciseNames: []string{"Assault Bike Sprint"}},
		{Name: "Archived functional old", NameRu: "Архив — функционал", Category: &catFunctional, Difficulty: &diffIntermediate, TargetStatus: programStatusArchived, ExerciseNames: []string{"Box Jump", "Kettlebell Swing"}},
		{Name: "Archived rehab old", NameRu: "Архив — реабилитация", Category: &catRehab, Difficulty: &diffBeginner, TargetStatus: programStatusArchived, ExerciseNames: []string{"Plank", "Russian Twist"}},
	}
}
