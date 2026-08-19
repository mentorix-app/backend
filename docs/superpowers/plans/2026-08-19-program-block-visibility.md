# Персональная видимость блоков программы — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Тренер отмечает в блоке дня конкретных клиентов, и блок виден только им; блок без отметок виден всем, кому назначена программа.

**Architecture:** Правила видимости лежат вне дерева версий — в таблице `program_block_clients`, ключ `(program_id, block_key)`. `block_key` — новый стабильный uuid блока, переживающий publish и discard, по образцу существующего `day_key`. Клиентское чтение накладывает правила фильтром поверх любой замороженной версии, поэтому изменение списка не требует ни `publish-update`, ни `sync`.

**Tech Stack:** Go 1.x, Echo v4, pgx/v5, sqlc, PostgreSQL (схема `mentorix`), golang-migrate.

**Spec:** [2026-08-19-program-block-visibility-design.md](../specs/2026-08-19-program-block-visibility-design.md)

## Global Constraints

- Миграция называется `000026_program_block_visibility` (`.up.sql` + `.down.sql`).
- Схема БД — `mentorix`. Таблицы во множественном числе, `snake_case`, домен-префикс `program_`.
- Имена ограничений: `{table}_{cols}_uniq`, индексов — `{table}_{col(s)}_idx`. Лимит идентификатора Postgres — 63 символа.
- Связь с клиентом в колонках — `client_user_id`, не `client_id`.
- JSON-ключи API — `snake_case`; в Go всегда явный тег `json:"..."`.
- Пустой набор строк для `(program_id, block_key)` означает «блок виден всем». Отдельного флага «блок персональный» нет.
- Инвариант: в дне, где есть хотя бы один блок, должен остаться хотя бы один блок без правил. Проверяется только на `PUT …/clients` и на publish/publish-update. `DELETE`, `move`, `merge` его не проверяют.
- Проверка на `PUT …/clients` запускается только при переходе блока «общий → персональный».
- Проверка идёт против working copy **и** каждой версии, на которой висит активное назначение этой программы.
- `merge` блоков с разными наборами клиентов запрещён (`400`).
- `block_key` наружу в API не выходит: тег `json:"-"`.
- `client_user_ids` всегда сериализуется массивом — `[]` для общего блока,
  никогда `null`. В OpenAPI поле не `nullable`.
- `//nolint` в репозитории нет ни одного — не вводить. Функция без вызывающего
  покрывается тестом (`run.tests: true` в `.golangci.yml`), а не подавлением.
- После изменений в `db/migrations/` — `make schema-sync`; после изменений в `db/queries/` — `sqlc generate`.
- Каждая задача завершается коммитом; pre-commit прогоняет `make check-ci`.
- Коммиты — Conventional Commits на английском. `feat` только для user-facing поведения.

---

### Task 1: Миграция — `block_key` и таблица правил

**Files:**
- Create: `db/migrations/000026_program_block_visibility.up.sql`
- Create: `db/migrations/000026_program_block_visibility.down.sql`
- Modify: `db/schema.sql` (генерируется `make schema-sync`)
- Test: `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: колонки `program_week_day_blocks.block_key` и `program_version_week_day_blocks.block_key` (обе `NOT NULL DEFAULT gen_random_uuid()`); таблица `mentorix.program_block_clients (id, program_id, block_key, client_user_id, created_at, created_by)`.

Колонки `program_id` на блоке **нет**: её единственным назначением был бы
`UNIQUE (program_id, block_key)`, но ни один запрос фичи её не читает, а
уникальность ключа обеспечивается генерацией uuid. `program_id` для блока
достаётся джойном через `program_week_days`, где он уже есть.

- [ ] **Step 1: Написать падающий интеграционный тест**

Создать `internal/db/storetest/program_block_visibility_integration_test.go`:

```go
//go:build integration

package storetest

import (
	"context"
	"strings"
	"testing"
)

func TestMigration_BlockKeyColumnsExist(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	// Both block_key columns must be NOT NULL *and* carry a generated default:
	// the existing insert queries do not mention block_key until Tasks 2 and 3,
	// so without the default this migration breaks every block insert.
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		var isNullable string
		var columnDefault *string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = 'mentorix' AND table_name = $1 AND column_name = 'block_key'`,
			table).Scan(&isNullable, &columnDefault)
		if err != nil {
			t.Fatalf("query %s.block_key: %v", table, err)
		}
		if isNullable != "NO" {
			t.Fatalf("%s.block_key is_nullable = %q, want NO", table, isNullable)
		}
		if columnDefault == nil || !strings.Contains(*columnDefault, "gen_random_uuid") {
			t.Fatalf("%s.block_key default = %v, want gen_random_uuid()", table, columnDefault)
		}
	}

	var hasTable bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'mentorix' AND table_name = 'program_block_clients')`).Scan(&hasTable)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if !hasTable {
		t.Fatal("table mentorix.program_block_clients does not exist")
	}

	var hasUniq bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint
			WHERE conname = 'program_block_clients_program_block_key_client_uniq')`).Scan(&hasUniq)
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	if !hasUniq {
		t.Fatal("unique constraint on program_block_clients is missing")
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться что падает**

Run: `go test -tags integration ./internal/db/storetest/ -run TestMigration_BlockKeyColumnsExist -count=1`
Expected: FAIL — `column "block_key" does not exist`.

- [ ] **Step 3: Написать up-миграцию**

Создать `db/migrations/000026_program_block_visibility.up.sql`:

Both columns carry `DEFAULT gen_random_uuid()`. That is load-bearing, not
cosmetic: existing `InsertDayBlock` and `InsertProgramVersionDayBlock` do not
mention `block_key` and only start doing so in Tasks 2 and 3. Without the
default, this migration alone would break every block insert in the suite.

```sql
-- Stable block identity across publish/discard + per-client block visibility rules.

ALTER TABLE mentorix.program_week_day_blocks
  ADD COLUMN block_key uuid NOT NULL DEFAULT gen_random_uuid();

CREATE INDEX program_week_day_blocks_block_key_idx
  ON mentorix.program_week_day_blocks (block_key);

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD COLUMN block_key uuid NOT NULL DEFAULT gen_random_uuid();

-- Best-effort backfill: point a frozen block at its template block's key, matched
-- by (week_number, day_number, sort_order). Same approach 000020 used for day_key.
-- Rows with no template match keep the random key the column default gave them.
UPDATE mentorix.program_version_week_day_blocks vb
SET block_key = sub.block_key
FROM (
  SELECT
    vb2.id AS version_block_id,
    tb.block_key AS block_key
  FROM mentorix.program_version_week_day_blocks vb2
  JOIN mentorix.program_version_week_days vd
    ON vd.id = vb2.program_version_week_day_id
  JOIN mentorix.program_version_weeks vw
    ON vw.id = vd.program_version_week_id
  JOIN mentorix.program_versions v
    ON v.id = vd.program_version_id
  JOIN mentorix.program_weeks tw
    ON tw.program_id = v.program_id
   AND tw.week_number = vw.week_number
  JOIN mentorix.program_week_days td
    ON td.week_id = tw.id
   AND td.day_number = vd.day_number
  JOIN mentorix.program_week_day_blocks tb
    ON tb.program_week_day_id = td.id
   AND tb.sort_order = vb2.sort_order
) sub
WHERE vb.id = sub.version_block_id;

CREATE INDEX program_version_week_day_blocks_block_key_idx
  ON mentorix.program_version_week_day_blocks (block_key);

CREATE TABLE mentorix.program_block_clients (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id     uuid NOT NULL REFERENCES mentorix.programs (id) ON DELETE CASCADE,
  block_key      uuid NOT NULL,
  client_user_id uuid NOT NULL REFERENCES mentorix.users (id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  created_by     uuid REFERENCES mentorix.users (id),
  CONSTRAINT program_block_clients_program_block_key_client_uniq
    UNIQUE (program_id, block_key, client_user_id)
);

-- No index on (program_id, block_key): the UNIQUE constraint above already
-- creates a b-tree on (program_id, block_key, client_user_id), whose leftmost
-- prefix serves those lookups. client_user_id is the third column, so it needs
-- its own index for "all rules of this client".
CREATE INDEX program_block_clients_client_user_id_idx
  ON mentorix.program_block_clients (client_user_id);
```

- [ ] **Step 4: Написать down-миграцию**

Создать `db/migrations/000026_program_block_visibility.down.sql`:

```sql
DROP TABLE IF EXISTS mentorix.program_block_clients;

DROP INDEX IF EXISTS mentorix.program_version_week_day_blocks_block_key_idx;
ALTER TABLE mentorix.program_version_week_day_blocks
  DROP COLUMN IF EXISTS block_key;

DROP INDEX IF EXISTS mentorix.program_week_day_blocks_block_key_idx;
ALTER TABLE mentorix.program_week_day_blocks
  DROP COLUMN IF EXISTS block_key;
```

- [ ] **Step 5: Применить миграцию и пересобрать схему**

Run: `make migrate && make schema-sync`
Expected: миграция применяется, `db/schema.sql` содержит `program_block_clients`.

- [ ] **Step 6: Запустить тест, убедиться что проходит**

Run: `go test -tags integration ./internal/db/storetest/ -run TestMigration_BlockKeyColumnsExist -count=1`
Expected: PASS

- [ ] **Step 7: Проверить откат миграции**

Run: `make migrate-down && make migrate`
Expected: обе команды без ошибок.

- [ ] **Step 8: Обновить номер версии в доках**

В `docs/status.md` заменить `_Миграции: версия 25 (\`./scripts/migrate-check.sh\`)._` на `версия 26`. В `docs/architecture.md` найти упоминание версии миграций и заменить `25` на `26`.

Run: `./scripts/docs-check.sh`
Expected: `docs-check passed.`

- [ ] **Step 8a: Дописать колонку в feature-док**

`docs/features/program-blocks.md` перечисляет колонки `program_week_day_blocks`
таблицей. Добавить в неё строку — иначе список молча расходится со схемой, а
`CLAUDE.md` § «Docs sync» требует обновлять таблицы feature-дока при изменении
`db/migrations/`:

```markdown
| `block_key` | uuid NOT NULL DEFAULT `gen_random_uuid()` | стабильная идентичность блока между publish и discard |
```

`docs-check.sh` это не ловит — он проверяет структуру, а не соответствие схеме.

- [ ] **Step 9: Коммит**

```bash
git add db/migrations/000026_program_block_visibility.up.sql \
        db/migrations/000026_program_block_visibility.down.sql \
        db/schema.sql docs/status.md docs/architecture.md \
        docs/features/program-blocks.md \
        internal/db/sqlc/ \
        internal/db/storetest/program_block_visibility_integration_test.go
git commit -m "feat(db): stable block_key and program_block_clients table"
```

---

### Task 2: Домен — `BlockKey` и `ClientUserIDs` в модели, загрузка правил

**Files:**
- Create: `db/queries/program_block_client.sql`
- Modify: `db/queries/program.sql` (запросы `ListDayBlocks`, `GetDayBlockByID`, новый `InsertDayBlockWithKey`)
- Modify: `internal/program/model.go:95-102` (`DayBlock`)
- Modify: `internal/program/store.go:229-254` (`listDayBlocks`), `internal/program/store.go:164-177` (`loadDetail`)
- Test: `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: колонки из Task 1.
- Produces:
  - `program.DayBlock.BlockKey uuid.UUID` (тег `json:"-"`)
  - `program.DayBlock.ClientUserIDs []uuid.UUID` (тег `json:"client_user_ids"`)
  - `func (s *Store) listProgramBlockClients(ctx context.Context, programID uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)` — ключ мапы `block_key`
  - sqlc-запрос `ListProgramBlockClients`

- [ ] **Step 1: Написать падающий тест**

Дописать в `internal/db/storetest/program_block_visibility_integration_test.go`
локальный хелпер и тест. Хелпер повторяет паттерн засева из
`internal/db/storetest/program_store_integration_test.go:20-45` — готового
`createTrainerUser` в пакете нет:

```go
// seedTrainerAndExercise registers a trainer and one catalog exercise.
// Returns the trainer's user id and the exercise id.
func seedTrainerAndExercise(t *testing.T, pool *pgxpool.Pool, emailPrefix string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(
		ctx, emailPrefix+"@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	ex, err := exercise.NewStore(pool).Create(ctx, userID, nil, exercise.UpsertInput{
		Name:        "Bench Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	return userID, ex.ID
}

func TestProgramDetail_BlockCarriesKeyAndEmptyClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "block-key-detail")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}

	block, ok := firstDayBlock(detail.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected one block in day")
	}
	if block.BlockKey == uuid.Nil {
		t.Fatal("block.BlockKey is uuid.Nil, want generated key")
	}
	if len(block.ClientUserIDs) != 0 {
		t.Fatalf("block.ClientUserIDs = %v, want empty (shared block)", block.ClientUserIDs)
	}
}
```

Импорты файла: `context`, `testing`, `github.com/google/uuid`,
`github.com/jackc/pgx/v5/pgxpool`, `mentorix-backend/internal/auth`,
`mentorix-backend/internal/exercise`, `mentorix-backend/internal/program`.

- [ ] **Step 2: Запустить тест, убедиться что падает**

Run: `go test -tags integration ./internal/db/storetest/ -run TestProgramDetail_BlockCarriesKeyAndEmptyClients -count=1`
Expected: FAIL — компиляция падает, `block.BlockKey undefined`.

- [ ] **Step 3: Добавить поля в модель**

В `internal/program/model.go` заменить структуру `DayBlock`:

```go
type DayBlock struct {
	ID            uuid.UUID     `json:"id"`
	BlockKey      uuid.UUID     `json:"-"`
	BlockType     BlockType     `json:"block_type"`
	Instruction   string        `json:"instruction"`
	SortOrder     int           `json:"sort_order"`
	ClientUserIDs []uuid.UUID   `json:"client_user_ids"`
	Exercises     []DayExercise `json:"exercises"`
	CreatedAt     time.Time     `json:"created_at"`
}
```

- [ ] **Step 4: Добавить SQL-запросы**

Создать `db/queries/program_block_client.sql`:

```sql
-- name: ListProgramBlockClients :many
SELECT block_key, client_user_id
FROM mentorix.program_block_clients
WHERE program_id = $1
ORDER BY block_key, client_user_id;
```

В `db/queries/program.sql` заменить `ListDayBlocks` и `GetDayBlockByID`, добавив
`block_key`, и добавить вставку с явным ключом:

```sql
-- name: ListDayBlocks :many
SELECT id, block_key, block_type, instruction, sort_order, created_at
FROM mentorix.program_week_day_blocks
WHERE program_week_day_id = $1
ORDER BY sort_order ASC, created_at ASC;

-- name: GetDayBlockByID :one
SELECT id, block_key, program_week_day_id, block_type, instruction, sort_order, created_at
FROM mentorix.program_week_day_blocks
WHERE id = $1;

-- name: InsertDayBlockWithKey :one
INSERT INTO mentorix.program_week_day_blocks (
  program_week_day_id, block_key, block_type, instruction,
  sort_order, modified_at, modified_by
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, block_key;
```

Существующий `InsertDayBlock` менять только в части `RETURNING` — колонки
вставки те же, `block_key` проставит дефолт:

```sql
-- name: InsertDayBlock :one
INSERT INTO mentorix.program_week_day_blocks (
  program_week_day_id, block_type, instruction, sort_order,
  modified_at, modified_by
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, block_key;
```

- [ ] **Step 5: Перегенерировать sqlc**

Run: `sqlc generate`
Expected: без ошибок; в `internal/db/sqlc/` появились `ListProgramBlockClients` и `InsertDayBlockWithKey`.

- [ ] **Step 6: Починить вызовы `InsertDayBlock` и заполнить новые поля**

Найти все места вызова: `grep -rn 'InsertDayBlock(' internal/`.

`RETURNING id, block_key` меняет тип результата с `pgtype.UUID` на структуру
строки. Параметры вставки те же. Вызовы вида
`blockID, err := qtx.InsertDayBlock(...)` больше не компилируются — заменить на:

```go
blockRow, err := qtx.InsertDayBlock(ctx, sqlc.InsertDayBlockParams{ /* … */ })
if err != nil {
	return Detail{}, fmt.Errorf("insert day block: %w", err)
}
blockID := blockRow.ID
```

Точное имя типа результата взять из сгенерированного `internal/db/sqlc/`.

В `internal/program/store.go` в `listDayBlocks` заполнить `BlockKey` из строки:

```go
blocks = append(blocks, DayBlock{
	ID:          pgconv.FromPGUUID(row.ID),
	BlockKey:    pgconv.FromPGUUID(row.BlockKey),
	BlockType:   BlockType(row.BlockType),
	Instruction: row.Instruction,
	SortOrder:   int(row.SortOrder),
	CreatedAt:   row.CreatedAt.UTC(),
})
```

- [ ] **Step 7: Добавить загрузку правил и применить её в `loadDetail`**

В `internal/program/store.go` добавить метод:

```go
// listProgramBlockClients returns visibility rules of a program keyed by block_key.
// A block_key absent from the map has no rules and is visible to every client.
func (s *Store) listProgramBlockClients(ctx context.Context, programID uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := s.q.ListProgramBlockClients(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return nil, fmt.Errorf("list program block clients: %w", err)
	}
	out := make(map[uuid.UUID][]uuid.UUID, len(rows))
	for _, row := range rows {
		key := pgconv.FromPGUUID(row.BlockKey)
		out[key] = append(out[key], pgconv.FromPGUUID(row.ClientUserID))
	}
	return out, nil
}

// applyBlockClients fills DayBlock.ClientUserIDs from the rules map.
// A block with no rules gets an empty, non-nil slice: the field must serialize
// as [] and never as null, so a consumer has one shape to handle, not two.
func applyBlockClients(d *Detail, rules map[uuid.UUID][]uuid.UUID) {
	for wi := range d.Weeks {
		for di := range d.Weeks[wi].Days {
			for bi := range d.Weeks[wi].Days[di].Blocks {
				block := &d.Weeks[wi].Days[di].Blocks[bi]
				ids := rules[block.BlockKey]
				if ids == nil {
					ids = []uuid.UUID{}
				}
				block.ClientUserIDs = ids
			}
		}
	}
}
```

В `loadDetail` после сборки недель вызвать:

```go
rules, err := s.listProgramBlockClients(ctx, id)
if err != nil {
	return Detail{}, err
}
applyBlockClients(&detail, rules)
```

- [ ] **Step 8: Запустить тест, убедиться что проходит**

Run: `go test -tags integration ./internal/db/storetest/ -run TestProgramDetail_BlockCarriesKeyAndEmptyClients -count=1`
Expected: PASS

- [ ] **Step 9: Прогнать весь пакет**

Run: `go test ./internal/program/... -count=1`
Expected: PASS

- [ ] **Step 10: Коммит**

```bash
git add db/queries/ internal/db/sqlc/ internal/program/ internal/db/storetest/
git commit -m "feat(program): expose block_key and per-block client list in detail"
```

---

### Task 3: `block_key` переживает publish и discard

**Files:**
- Modify: `db/queries/program_version.sql` (`InsertProgramVersionDayBlock`)
- Modify: `internal/program/store_version.go:190-282` (`freezeVersion`), `internal/program/store_version.go:71-189` (`RestoreWorkingTreeFromLatestVersion`)
- Modify: `internal/program/store_client_program.go:54-65` (сборка `DayBlock` из версии)
- Test: `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: `program.DayBlock.BlockKey` из Task 2.
- Produces: гарантия, что `block_key` блока в шаблоне, в замороженной версии и в восстановленной working copy совпадают.

- [ ] **Step 1: Написать падающий тест**

Дописать в `internal/db/storetest/program_block_visibility_integration_test.go`:

```go
func TestBlockKey_SurvivesPublishAndDiscard(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "block-key-publish")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]
	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	block, _ := firstDayBlock(detail.Weeks[0].Days[0])
	wantKey := block.BlockKey

	// Publish freezes the tree; the frozen block must keep the same key.
	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	_ = published

	var frozenKey uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT vb.block_key
		FROM mentorix.program_version_week_day_blocks vb
		JOIN mentorix.program_version_week_days vd ON vd.id = vb.program_version_week_day_id
		JOIN mentorix.program_versions v ON v.id = vd.program_version_id
		WHERE v.program_id = $1`, detail.ID).Scan(&frozenKey)
	if err != nil {
		t.Fatalf("query frozen block_key: %v", err)
	}
	if frozenKey != wantKey {
		t.Fatalf("frozen block_key = %s, want %s", frozenKey, wantKey)
	}

	// Discard rebuilds the working copy from the latest version; the key must survive.
	restored, err := store.RestoreWorkingTreeFromLatestVersion(ctx, detail.ID, userID)
	if err != nil {
		t.Fatalf("RestoreWorkingTreeFromLatestVersion: %v", err)
	}
	restoredBlock, ok := firstDayBlock(restored.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected one block after restore")
	}
	if restoredBlock.BlockKey != wantKey {
		t.Fatalf("restored block_key = %s, want %s", restoredBlock.BlockKey, wantKey)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться что падает**

Run: `go test -tags integration ./internal/db/storetest/ -run TestBlockKey_SurvivesPublishAndDiscard -count=1`
Expected: FAIL — frozen `block_key` не совпадает (в версию пишется свежий uuid).

- [ ] **Step 3: Принять `block_key` во вставке блока версии**

В `db/queries/program_version.sql` заменить:

```sql
-- name: InsertProgramVersionDayBlock :one
INSERT INTO mentorix.program_version_week_day_blocks (
  program_version_week_day_id,
  block_key,
  block_type,
  instruction,
  sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;
```

Run: `sqlc generate`

- [ ] **Step 4: Передавать `block_key` при freeze**

В `internal/program/store_version.go`, в `freezeVersion`, в вызове
`InsertProgramVersionDayBlock` добавить параметр:

```go
BlockKey: pgconv.ToPGUUID(block.BlockKey),
```

- [ ] **Step 5: Передавать `block_key` при restore**

В `internal/program/store_version.go`, в `RestoreWorkingTreeFromLatestVersion`,
заменить вызов `InsertDayBlock` на `InsertDayBlockWithKey`:

```go
blockKey := block.BlockKey
if blockKey == uuid.Nil {
	blockKey = uuid.New()
}
blockRow, err := qtx.InsertDayBlockWithKey(ctx, sqlc.InsertDayBlockWithKeyParams{
	ProgramWeekDayID: dayRow.ID,
	BlockKey:         pgconv.ToPGUUID(blockKey),
	BlockType:        string(block.BlockType),
	Instruction:      block.Instruction,
	SortOrder:        int32(block.SortOrder),
	ModifiedAt:       now,
	ModifiedBy:       userPG,
})
if err != nil {
	return Detail{}, fmt.Errorf("insert day block: %w", err)
}
blockID := blockRow.ID
```

- [ ] **Step 6: Заполнять `BlockKey` и правила при чтении версии**

`GetVersionDetail` — третье место в коде, где собирается `DayBlock` (кроме
`loadDetail` и будущего клиентского пути). Сейчас оно не заполняет ни `BlockKey`,
ни `ClientUserIDs`.

Незаполненный `BlockKey` здесь — не косметика, а порча данных: этот метод читает
`RestoreWorkingTreeFromLatestVersion`, и при `uuid.Nil` его фолбэк выдаст каждому
блоку новый ключ, так что любой `discard-unpublished` молча порвёт привязку всех
правил видимости. Пустой `ClientUserIDs` нарушает гарантию «всегда массив» на
структурах, которые отдаёт бот, и понадобится Task 8 для фильтрации.

Публичного GET на одну версию в API нет (только `DELETE`), так что HTTP-контракт
здесь ни при чём.

В `internal/program/store_client_program.go` в сборке `DayBlock` добавить:

```go
BlockKey: pgconv.FromPGUUID(blockRow.BlockKey),
```

и в конце `GetVersionDetail`, перед возвратом, наложить правила программы —
`program_id` берётся из уже загруженной строки версии:

```go
rules, err := s.listProgramBlockClients(ctx, pgconv.FromPGUUID(version.ProgramID))
if err != nil {
	return Detail{}, err
}
applyBlockClients(&detail, rules)
```

Правила действуют на любую версию, к которой относится их `block_key`, поэтому
показывать их тренеру в просмотре версии — правда, а `[]` на каждом блоке было бы
ложью. Заодно это чинит `null` и упрощает Task 8.

- [ ] **Step 7: Запустить тест, убедиться что проходит**

Run: `go test -tags integration ./internal/db/storetest/ -run TestBlockKey_SurvivesPublishAndDiscard -count=1`
Expected: PASS

- [ ] **Step 8: Прогнать интеграционные тесты программ целиком**

Run: `go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1`
Expected: PASS

- [ ] **Step 9: Коммит**

```bash
git add db/queries/program_version.sql internal/db/sqlc/ internal/program/ internal/db/storetest/
git commit -m "feat(program): carry block_key through publish and discard"
```

---

### Task 4: Чистые функции видимости и инварианта

**Files:**
- Create: `internal/program/block_visibility.go`
- Create: `internal/program/block_visibility_test.go`

**Interfaces:**
- Consumes: `program.Detail`, `program.Day`, `program.DayBlock` из Task 2.
- Produces:
  - `func BlockVisibleToClient(block DayBlock, clientUserID uuid.UUID) bool`
  - `func FilterDetailForClient(d Detail, clientUserID uuid.UUID) Detail`
  - `func dayHasSharedBlock(day Day) bool`
  - `func daySharedAfterRestrict(day Day, blockKey uuid.UUID) bool`

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/program/block_visibility_test.go`:

```go
package program

import (
	"testing"

	"github.com/google/uuid"
)

func TestBlockVisibleToClient(t *testing.T) {
	petya := uuid.New()
	masha := uuid.New()
	vasya := uuid.New()

	tests := []struct {
		name   string
		block  DayBlock
		client uuid.UUID
		want   bool
	}{
		{
			name:   "shared block is visible to anyone",
			block:  DayBlock{ClientUserIDs: nil},
			client: vasya,
			want:   true,
		},
		{
			name:   "restricted block is visible to a listed client",
			block:  DayBlock{ClientUserIDs: []uuid.UUID{petya, masha}},
			client: petya,
			want:   true,
		},
		{
			name:   "restricted block is hidden from an unlisted client",
			block:  DayBlock{ClientUserIDs: []uuid.UUID{petya, masha}},
			client: vasya,
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BlockVisibleToClient(tc.block, tc.client); got != tc.want {
				t.Fatalf("BlockVisibleToClient() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterDetailForClient_dropsHiddenBlocks(t *testing.T) {
	petya := uuid.New()
	vasya := uuid.New()
	shared := DayBlock{ID: uuid.New(), BlockKey: uuid.New()}
	personal := DayBlock{ID: uuid.New(), BlockKey: uuid.New(), ClientUserIDs: []uuid.UUID{petya}}

	d := Detail{Weeks: []Week{{
		WeekNumber: 1,
		Days:       []Day{{DayNumber: 1, Blocks: []DayBlock{shared, personal}}},
	}}}

	forPetya := FilterDetailForClient(d, petya)
	if got := len(forPetya.Weeks[0].Days[0].Blocks); got != 2 {
		t.Fatalf("blocks for listed client = %d, want 2", got)
	}

	forVasya := FilterDetailForClient(d, vasya)
	if got := len(forVasya.Weeks[0].Days[0].Blocks); got != 1 {
		t.Fatalf("blocks for unlisted client = %d, want 1", got)
	}
	if forVasya.Weeks[0].Days[0].Blocks[0].ID != shared.ID {
		t.Fatal("unlisted client kept the wrong block")
	}

	// The source detail must not be mutated.
	if got := len(d.Weeks[0].Days[0].Blocks); got != 2 {
		t.Fatalf("source detail mutated: blocks = %d, want 2", got)
	}
}

func TestDayHasSharedBlock(t *testing.T) {
	petya := uuid.New()

	tests := []struct {
		name string
		day  Day
		want bool
	}{
		{
			name: "day keeps one shared block",
			day:  Day{Blocks: []DayBlock{{ClientUserIDs: []uuid.UUID{petya}}, {}}},
			want: true,
		},
		{
			name: "every block in the day is restricted",
			day:  Day{Blocks: []DayBlock{{ClientUserIDs: []uuid.UUID{petya}}}},
			want: false,
		},
		{
			name: "day with no blocks at all",
			day:  Day{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dayHasSharedBlock(tc.day); got != tc.want {
				t.Fatalf("dayHasSharedBlock() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDaySharedAfterRestrict(t *testing.T) {
	petya := uuid.New()
	keyA := uuid.New()
	keyB := uuid.New()
	keyC := uuid.New()

	tests := []struct {
		name    string
		day     Day
		restrict uuid.UUID
		want    bool
	}{
		{
			name: "another shared block remains",
			day: Day{Blocks: []DayBlock{
				{BlockKey: keyA},
				{BlockKey: keyB, ClientUserIDs: []uuid.UUID{petya}},
				{BlockKey: keyC},
			}},
			restrict: keyA,
			want:     true,
		},
		{
			name: "restricting the last shared block leaves none",
			day: Day{Blocks: []DayBlock{
				{BlockKey: keyA},
				{BlockKey: keyB, ClientUserIDs: []uuid.UUID{petya}},
			}},
			restrict: keyA,
			want:     false,
		},
		{
			name:     "day without the key is unaffected",
			day:      Day{Blocks: []DayBlock{{BlockKey: keyA}}},
			restrict: keyC,
			want:     true,
		},
		{
			name:     "empty day is unaffected",
			day:      Day{},
			restrict: keyA,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := daySharedAfterRestrict(tc.day, tc.restrict); got != tc.want {
				t.Fatalf("daySharedAfterRestrict() = %v, want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Запустить тесты, убедиться что падают**

Run: `go test ./internal/program/ -run 'TestBlockVisibleToClient|TestFilterDetailForClient|TestDaySharedAfterRestrict' -count=1`
Expected: FAIL — `undefined: BlockVisibleToClient`.

- [ ] **Step 3: Реализовать чистые функции**

Создать `internal/program/block_visibility.go`:

```go
package program

import "github.com/google/uuid"

// BlockVisibleToClient reports whether a client sees this block.
// A block without rules is shared and visible to everyone.
func BlockVisibleToClient(block DayBlock, clientUserID uuid.UUID) bool {
	if len(block.ClientUserIDs) == 0 {
		return true
	}
	for _, id := range block.ClientUserIDs {
		if id == clientUserID {
			return true
		}
	}
	return false
}

// FilterDetailForClient returns a copy of d with blocks the client must not see
// removed. The receiver is not mutated.
func FilterDetailForClient(d Detail, clientUserID uuid.UUID) Detail {
	out := d
	out.Weeks = make([]Week, 0, len(d.Weeks))
	for _, week := range d.Weeks {
		w := week
		w.Days = make([]Day, 0, len(week.Days))
		for _, day := range week.Days {
			dd := day
			dd.Blocks = make([]DayBlock, 0, len(day.Blocks))
			for _, block := range day.Blocks {
				if BlockVisibleToClient(block, clientUserID) {
					dd.Blocks = append(dd.Blocks, block)
				}
			}
			w.Days = append(w.Days, dd)
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out
}

// dayHasSharedBlock reports whether the day keeps at least one block without rules.
func dayHasSharedBlock(day Day) bool {
	for _, block := range day.Blocks {
		if len(block.ClientUserIDs) == 0 {
			return true
		}
	}
	return false
}

// daySharedAfterRestrict reports whether the day still holds a shared block once
// the block with blockKey becomes restricted. A day that does not hold the key,
// or holds no blocks at all, is unaffected.
func daySharedAfterRestrict(day Day, blockKey uuid.UUID) bool {
	holdsKey := false
	for _, block := range day.Blocks {
		if block.BlockKey == blockKey {
			holdsKey = true
			break
		}
	}
	if !holdsKey {
		return true
	}
	for _, block := range day.Blocks {
		if block.BlockKey == blockKey {
			continue
		}
		if len(block.ClientUserIDs) == 0 {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Запустить тесты, убедиться что проходят**

Run: `go test ./internal/program/ -run 'TestBlockVisibleToClient|TestFilterDetailForClient|TestDaySharedAfterRestrict' -count=1 -v`
Expected: PASS по всем подтестам.

- [ ] **Step 5: Коммит**

```bash
git add internal/program/block_visibility.go internal/program/block_visibility_test.go
git commit -m "feat(program): block visibility predicates and day invariant helper"
```

---

### Task 5: Эндпоинт `PUT …/blocks/{block_id}/clients`

**Files:**
- Modify: `db/queries/program_block_client.sql`
- Modify: `internal/program/model.go:16-33` (новые ошибки)
- Create: `internal/program/store_block_clients.go`
- Modify: `internal/program/service.go` (метод `SetBlockClients` + интерфейс стора)
- Modify: `internal/program/handlers_blocks.go`, `internal/program/handlers.go:28-67` (маршрут), `internal/program/handlers.go:719-752` (`mapProgramError`)
- Test: `internal/program/handlers_blocks_test.go`, `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: `daySharedAfterRestrict` из Task 4; `GetDayBlockByID` из Task 2.
- Produces:
  - `program.ErrLastSharedBlock`, `program.ErrClientNotAssignedToProgram`
  - `func (s *Service) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error)`
  - `func (s *Store) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error)`
  - маршрут `PUT /programs/:id/weeks/:week_id/blocks/:block_id/clients`

- [ ] **Step 1: Написать падающий handler-тест**

Использовать существующие хелперы пакета: `blockHandlerFixture()`,
`programHandler(store)`, `programContext(e, method, path, body, userID, params)`,
`assertStatus`, `assertHTTPError`, `fakeProgramStore`. Паттерн подмены ошибки —
как в `internal/program/handlers_blocks_test.go:352-370`
(`reorderDayBlocksErrStore`).

Дописать в `internal/program/handlers_blocks_test.go`:

```go
type setBlockClientsErrStore struct {
	fakeProgramStore
	setErr error
}

func (s *setBlockClientsErrStore) SetBlockClients(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []uuid.UUID) (Detail, error) {
	return Detail{}, s.setErr
}

func setBlockClientsContext(t *testing.T, userID, programID, weekID, blockID uuid.UUID, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	return programContext(e, http.MethodPut,
		"/programs/"+programID.String()+"/weeks/"+weekID.String()+
			"/blocks/"+blockID.String()+"/clients",
		body, userID, map[string]string{
			"id":       programID.String(),
			"week_id":  weekID.String(),
			"block_id": blockID.String(),
		})
}

func TestHandlers_SetBlockClients(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, rec := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	if err := h.SetBlockClients(c); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_SetBlockClients_mapsLastSharedBlock(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &setBlockClientsErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		setErr: fmt.Errorf("%w: week 1 day 3 in working copy", ErrLastSharedBlock),
	}
	h := programHandler(store)
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, uuid.New(), uuid.New(), body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_mapsClientNotAssigned(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &setBlockClientsErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		setErr: ErrClientNotAssignedToProgram,
	}
	h := programHandler(store)
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, uuid.New(), uuid.New(), body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_rejectsDuplicateIDs(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	dup := uuid.New().String()
	body := `{"client_user_ids":["` + dup + `","` + dup + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_rejectsNonUUID(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	body := `{"client_user_ids":["not-a-uuid"]}`
	c, _ := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}
```

Добавить в импорты файла `context` и `fmt`, если их там ещё нет.

**Важно:** после добавления `SetBlockClients` в интерфейс стора (Step 7) базовый
`fakeProgramStore` в `internal/program/service_test.go` перестанет его
удовлетворять. Дописать туда метод:

```go
func (f *fakeProgramStore) SetBlockClients(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []uuid.UUID) (Detail, error) {
	return f.detail, f.err
}
```

- [ ] **Step 1a: Укрепить тест `daySharedAfterRestrict`**

`ensureDaysKeepSharedBlock` (Step 6) вызывает `daySharedAfterRestrict` для
**каждого** дня каждого дерева, включая дни, где ограничиваемого блока нет. Для
них корректный ответ — `true` независимо от того, есть ли в дне общий блок; за
это отвечает короткое замыкание по `holdsKey`. Существующий тест это не
различает: в его фикстуре день без ключа состоит из уже общего блока, поэтому
наивная реализация без короткого замыкания прошла бы тоже.

Дописать кейс в `TestDaySharedAfterRestrict` в `internal/program/block_visibility_test.go`:

```go
{
	name: "day of only restricted blocks is unaffected by a key it does not hold",
	day: Day{Blocks: []DayBlock{
		{BlockKey: keyA, ClientUserIDs: []uuid.UUID{petya}},
	}},
	restrict: keyC,
	want:     true,
},
```

Без него рефакторинг, схлопывающий два цикла в один, прошёл бы тесты и сломал
проверку инварианта для всех остальных дней программы.

- [ ] **Step 2: Запустить тесты, убедиться что падают**

Run: `go test ./internal/program/ -run TestSetBlockClients -count=1`
Expected: FAIL — `undefined: ErrLastSharedBlock`, `h.SetBlockClients undefined`.

- [ ] **Step 3: Добавить ошибки домена**

В `internal/program/model.go` в блок `var (...)` дописать:

```go
ErrLastSharedBlock            = errors.New("day would have no shared block")
ErrClientNotAssignedToProgram = errors.New("client is not assigned to this program")
```

- [ ] **Step 4: Замапить ошибки на HTTP**

В `internal/program/handlers.go` в `mapProgramError` добавить перед `default`:

```go
case errors.Is(err, ErrLastSharedBlock), errors.Is(err, ErrClientNotAssignedToProgram):
	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
```

- [ ] **Step 5: Добавить SQL для замены списка и проверки назначений**

В `db/queries/program_block_client.sql` дописать:

```sql
-- name: DeleteProgramBlockClients :exec
DELETE FROM mentorix.program_block_clients
WHERE program_id = $1 AND block_key = $2;

-- name: InsertProgramBlockClient :exec
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (program_id, block_key, client_user_id) DO NOTHING;

-- name: ListAssignedClientUserIDs :many
SELECT DISTINCT client_user_id
FROM mentorix.program_assignments
WHERE program_id = $1;

-- name: ListAssignedProgramVersionIDs :many
SELECT DISTINCT program_version_id
FROM mentorix.program_assignments
WHERE program_id = $1;

-- name: DeleteProgramBlockClientsForClient :exec
DELETE FROM mentorix.program_block_clients
WHERE program_id = $1 AND client_user_id = $2;
```

Run: `sqlc generate`

- [ ] **Step 6: Реализовать стор**

Создать `internal/program/store_block_clients.go`:

```go
package program

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// SetBlockClients replaces the visibility rules of one block. An empty
// clientUserIDs makes the block shared again. Restricting a block is refused
// when it would leave a day without a shared block — checked against the
// working copy and against every version that still has an active assignment.
func (s *Store) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error) {
	ok, err := s.q.WeekBelongsToProgram(ctx, sqlc.WeekBelongsToProgramParams{
		ID:        pgconv.ToPGUUID(weekID),
		ProgramID: pgconv.ToPGUUID(programID),
	})
	if err != nil {
		return Detail{}, fmt.Errorf("week belongs to program: %w", err)
	}
	if !ok {
		return Detail{}, ErrNotFound
	}

	blockRow, err := s.q.GetDayBlockByID(ctx, pgconv.ToPGUUID(blockID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get day block: %w", err)
	}
	blockKey := pgconv.FromPGUUID(blockRow.BlockKey)

	if err := s.ensureClientsAssigned(ctx, programID, clientUserIDs); err != nil {
		return Detail{}, err
	}

	current, err := s.GetDetail(ctx, programID)
	if err != nil {
		return Detail{}, err
	}
	wasShared := blockCurrentlyShared(current, blockKey)
	becomesRestricted := len(clientUserIDs) > 0

	// Only the shared → restricted transition can break the day invariant.
	if wasShared && becomesRestricted {
		if err := s.ensureDaysKeepSharedBlock(ctx, programID, blockKey, current); err != nil {
			return Detail{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := qtx.DeleteProgramBlockClients(ctx, sqlc.DeleteProgramBlockClientsParams{
		ProgramID: pgconv.ToPGUUID(programID),
		BlockKey:  pgconv.ToPGUUID(blockKey),
	}); err != nil {
		return Detail{}, fmt.Errorf("delete block clients: %w", err)
	}
	for _, clientUserID := range clientUserIDs {
		if err := qtx.InsertProgramBlockClient(ctx, sqlc.InsertProgramBlockClientParams{
			ProgramID:    pgconv.ToPGUUID(programID),
			BlockKey:     pgconv.ToPGUUID(blockKey),
			ClientUserID: pgconv.ToPGUUID(clientUserID),
			CreatedBy:    pgconv.ToPGUUID(userID),
		}); err != nil {
			return Detail{}, fmt.Errorf("insert block client: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetDetail(ctx, programID)
}

func blockCurrentlyShared(d Detail, blockKey uuid.UUID) bool {
	for _, week := range d.Weeks {
		for _, day := range week.Days {
			for _, block := range day.Blocks {
				if block.BlockKey == blockKey {
					return len(block.ClientUserIDs) == 0
				}
			}
		}
	}
	return true
}

func (s *Store) ensureClientsAssigned(ctx context.Context, programID uuid.UUID, clientUserIDs []uuid.UUID) error {
	if len(clientUserIDs) == 0 {
		return nil
	}
	rows, err := s.q.ListAssignedClientUserIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list assigned clients: %w", err)
	}
	assigned := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		assigned[pgconv.FromPGUUID(row)] = struct{}{}
	}
	for _, id := range clientUserIDs {
		if _, ok := assigned[id]; !ok {
			return fmt.Errorf("%w: %s", ErrClientNotAssignedToProgram, id)
		}
	}
	return nil
}

// ensureDaysKeepSharedBlock checks the day invariant in the working copy and in
// every version an active assignment still points at.
func (s *Store) ensureDaysKeepSharedBlock(ctx context.Context, programID, blockKey uuid.UUID, working Detail) error {
	trees := []struct {
		label  string
		detail Detail
	}{{label: "working copy", detail: working}}

	versionIDs, err := s.q.ListAssignedProgramVersionIDs(ctx, pgconv.ToPGUUID(programID))
	if err != nil {
		return fmt.Errorf("list assigned versions: %w", err)
	}
	for _, versionPG := range versionIDs {
		versionID := pgconv.FromPGUUID(versionPG)
		detail, err := s.GetVersionDetail(ctx, versionID)
		if err != nil {
			return err
		}
		// GetVersionDetail already applies the program's rules (Task 3).
		trees = append(trees, struct {
			label  string
			detail Detail
		}{label: "version " + versionID.String(), detail: detail})
	}

	for _, tree := range trees {
		for _, week := range tree.detail.Weeks {
			for _, day := range week.Days {
				if !daySharedAfterRestrict(day, blockKey) {
					return fmt.Errorf("%w: week %d day %d in %s",
						ErrLastSharedBlock, week.WeekNumber, day.DayNumber, tree.label)
				}
			}
		}
	}
	return nil
}
```

- [ ] **Step 7: Добавить метод сервиса**

В `internal/program/service.go` в интерфейс стора (там, где перечислены
`MergeDayBlocks`, `UngroupDayBlock` и т.п.) добавить:

```go
SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error)
```

И метод сервиса рядом с `PatchDayBlock`:

```go
func (s *Service) SetBlockClients(ctx context.Context, userID, programID, weekID, blockID uuid.UUID, clientUserIDs []uuid.UUID) (Detail, error) {
	if err := s.ensureMutable(ctx, userID, programID); err != nil {
		return Detail{}, err
	}
	seen := make(map[uuid.UUID]struct{}, len(clientUserIDs))
	for _, id := range clientUserIDs {
		if id == uuid.Nil {
			return Detail{}, fmt.Errorf("%w: client_user_ids must not contain a nil uuid", ErrValidation)
		}
		if _, dup := seen[id]; dup {
			return Detail{}, fmt.Errorf("%w: client_user_ids must not contain duplicates", ErrValidation)
		}
		seen[id] = struct{}{}
	}
	return s.store.SetBlockClients(ctx, userID, programID, weekID, blockID, clientUserIDs)
}
```

- [ ] **Step 8: Добавить handler и маршрут**

В `internal/program/handlers_blocks.go`:

```go
type setBlockClientsBody struct {
	ClientUserIDs []string `json:"client_user_ids"`
}

func (h *Handlers) SetBlockClients(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, weekID, blockID, err := parseWeekBlockIDs(c)
	if err != nil {
		return err
	}
	var body setBlockClientsBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	clientUserIDs := make([]uuid.UUID, 0, len(body.ClientUserIDs))
	for _, raw := range body.ClientUserIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid client_user_ids: not a uuid")
		}
		clientUserIDs = append(clientUserIDs, id)
	}
	d, err := h.svc.SetBlockClients(c.Request().Context(), uid, programID, weekID, blockID, clientUserIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}
```

В `internal/program/handlers.go` в `Mount` дописать рядом с остальными block-маршрутами:

```go
g.PUT("/:id/weeks/:week_id/blocks/:block_id/clients", h.SetBlockClients)
```

- [ ] **Step 9: Запустить handler-тесты, убедиться что проходят**

Run: `go test ./internal/program/ -run TestSetBlockClients -count=1 -v`
Expected: PASS

- [ ] **Step 10: Написать интеграционный тест инварианта**

Дописать в `internal/db/storetest/program_block_visibility_integration_test.go`
второй общий хелпер и тест. Хелпер повторяет засев из
`internal/db/storetest/program_version_assignment_integration_test.go:25-49`:

```go
// seedAssignedClient registers a client user and links them to the trainer.
// Returns the client's user id and the trainer id.
func seedAssignedClient(t *testing.T, pool *pgxpool.Pool, trainerUserID uuid.UUID, emailPrefix string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	clientUserID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(
		ctx, emailPrefix+"@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client user: %v", err)
	}

	var trainerID uuid.UUID
	err = pool.QueryRow(ctx,
		`SELECT id FROM mentorix.trainers WHERE user_id = $1`, trainerUserID).Scan(&trainerID)
	if err != nil {
		t.Fatalf("select trainer id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}
	return clientUserID, trainerID
}

func TestSetBlockClients_refusesLastSharedBlock(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "last-shared")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	for i := 0; i < 2; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	blocks := detail.Weeks[0].Days[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}

	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, "last-shared-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	// Restricting the first block is fine — the second one stays shared.
	if _, err := store.SetBlockClients(ctx, userID, programID, week.ID, blocks[0].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients on first block: %v", err)
	}

	// Restricting the second one would leave the day without a shared block.
	_, err = store.SetBlockClients(ctx, userID, programID, week.ID, blocks[1].ID,
		[]uuid.UUID{clientUserID})
	if !errors.Is(err, program.ErrLastSharedBlock) {
		t.Fatalf("SetBlockClients on last shared block error = %v, want ErrLastSharedBlock", err)
	}
}

func TestSetBlockClients_refusesUnassignedClient(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "unassigned")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]
	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	block, _ := firstDayBlock(detail.Weeks[0].Days[0])

	stranger, _ := seedAssignedClient(t, pool, userID, "unassigned-client")
	_, err = store.SetBlockClients(ctx, userID, detail.ID, week.ID, block.ID,
		[]uuid.UUID{stranger})
	if !errors.Is(err, program.ErrClientNotAssignedToProgram) {
		t.Fatalf("SetBlockClients error = %v, want ErrClientNotAssignedToProgram", err)
	}
}
```

Добавить в импорты файла `errors`.

- [ ] **Step 11: Прогнать интеграционные тесты**

Run: `go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1`
Expected: PASS

- [ ] **Step 12: Коммит**

```bash
git add db/queries/ internal/db/sqlc/ internal/program/ internal/db/storetest/
git commit -m "feat(program): set per-client visibility on a day block"
```

---

### Task 6: Инвариант на publish

**Files:**
- Modify: `internal/program/model.go:263-310` (`validatePublishDetail`)
- Test: `internal/program/model_test.go`

**Interfaces:**
- Consumes: `dayHasSharedBlock` из Task 4.
- Produces: `validatePublishDetail` отклоняет день, где все блоки персональные.

- [ ] **Step 1: Написать падающий тест**

Дописать в `internal/program/model_test.go`:

```go
func TestValidatePublishDetail_dayWithoutSharedBlock(t *testing.T) {
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	client := uuid.New()

	d := Detail{
		Program: Program{
			Name:       "Program",
			Category:   &category,
			Difficulty: (*Difficulty)(&difficulty),
		},
		Weeks: []Week{{
			WeekNumber: 1,
			SortOrder:  1,
			Days: []Day{{
				DayNumber: 1,
				Blocks: []DayBlock{{
					BlockType:     BlockTypeSingle,
					BlockKey:      uuid.New(),
					ClientUserIDs: []uuid.UUID{client},
					Exercises:     []DayExercise{{ExerciseID: uuid.New(), SortOrder: 1}},
				}},
			}},
		}},
	}

	err := validatePublishDetail(d)
	if !errors.Is(err, ErrLastSharedBlock) {
		t.Fatalf("validatePublishDetail() error = %v, want ErrLastSharedBlock", err)
	}
}
```

- [ ] **Step 2: Запустить тест, убедиться что падает**

Run: `go test ./internal/program/ -run TestValidatePublishDetail_dayWithoutSharedBlock -count=1`
Expected: FAIL — `error = <nil>, want ErrLastSharedBlock`.

- [ ] **Step 3: Добавить проверку в валидатор**

В `internal/program/model.go` в `validatePublishDetail`, внутри цикла по дням,
сразу после блока, где вычисляется `dayHasExercises`, добавить:

```go
if dayHasExercises && !dayHasSharedBlock(day) {
	return fmt.Errorf("%w: week %d day %d has no shared block",
		ErrLastSharedBlock, week.WeekNumber, day.DayNumber)
}
```

- [ ] **Step 4: Запустить тест, убедиться что проходит**

Run: `go test ./internal/program/ -run TestValidatePublishDetail -count=1 -v`
Expected: PASS, включая существующие подтесты валидации.

- [ ] **Step 5: Прогнать пакет целиком**

Run: `go test ./internal/program/... -count=1`
Expected: PASS

- [ ] **Step 6: Коммит**

```bash
git add internal/program/model.go internal/program/model_test.go
git commit -m "feat(program): reject publish when a day has no shared block"
```

---

### Task 7: Наследование при ungroup и extract, запрет merge

**Files:**
- Modify: `internal/program/store_blocks.go:109-190` (`MergeDayBlocks`), `:191-279` (`UngroupDayBlock`), `:332-397` (`ExtractBlockExercise`)
- Modify: `db/queries/program_block_client.sql` (копирование правил на новый ключ)
- Test: `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: `ErrValidation`, `listProgramBlockClients` из Task 2.
- Produces: `CopyProgramBlockClients` (sqlc); `merge` отклоняет разные наборы клиентов через `ErrValidation`.

- [ ] **Step 1: Написать падающие интеграционные тесты**

Дописать в `internal/db/storetest/program_block_visibility_integration_test.go`.
Во всех трёх тестах в дне создаётся **четыре** блока: инвариант требует, чтобы
после ограничения в дне оставался общий блок.

```go
// seedDayWithBlocks publishes a program with n single blocks in week 1 day 1 and
// assigns it to one client. Returns program id, week id, the blocks and the client.
func seedDayWithBlocks(t *testing.T, pool *pgxpool.Pool, emailPrefix string, n int) (uuid.UUID, uuid.UUID, []program.DayBlock, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, emailPrefix)
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]
	for i := 0; i < n; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, emailPrefix+"-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}
	return programID, week.ID, published.Weeks[0].Days[0].Blocks, clientUserID
}

func TestMerge_rejectsDifferentClientSets(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "merge-mismatch", 4)
	trainerUserID := ownerUserID(t, pool, programID)
	dayID := dayIDOfBlock(t, pool, blocks[0].ID)

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[0].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	_, err := store.MergeDayBlocks(ctx, trainerUserID, programID, weekID, dayID,
		[]uuid.UUID{blocks[0].ID, blocks[1].ID})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("MergeDayBlocks error = %v, want ErrValidation", err)
	}
}

func TestUngroup_inheritsBlockClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "ungroup-inherit", 4)
	trainerUserID := ownerUserID(t, pool, programID)
	dayID := dayIDOfBlock(t, pool, blocks[0].ID)

	merged, err := store.MergeDayBlocks(ctx, trainerUserID, programID, weekID, dayID,
		[]uuid.UUID{blocks[0].ID, blocks[1].ID})
	if err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	group := groupBlock(t, merged.Weeks[0].Days[0])

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, group.ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	after, err := store.UngroupDayBlock(ctx, trainerUserID, programID, weekID, group.ID)
	if err != nil {
		t.Fatalf("UngroupDayBlock: %v", err)
	}

	restricted := 0
	for _, b := range after.Weeks[0].Days[0].Blocks {
		if len(b.ClientUserIDs) == 1 && b.ClientUserIDs[0] == clientUserID {
			restricted++
		}
	}
	if restricted != 2 {
		t.Fatalf("blocks inheriting the client list = %d, want 2", restricted)
	}
}

func TestExtract_inheritsBlockClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "extract-inherit", 4)
	trainerUserID := ownerUserID(t, pool, programID)
	dayID := dayIDOfBlock(t, pool, blocks[0].ID)

	merged, err := store.MergeDayBlocks(ctx, trainerUserID, programID, weekID, dayID,
		[]uuid.UUID{blocks[0].ID, blocks[1].ID})
	if err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	group := groupBlock(t, merged.Weeks[0].Days[0])
	itemID := group.Exercises[0].ID

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, group.ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	after, err := store.ExtractBlockExercise(ctx, trainerUserID, programID, weekID, group.ID, itemID, 1)
	if err != nil {
		t.Fatalf("ExtractBlockExercise: %v", err)
	}

	extracted := false
	for _, b := range after.Weeks[0].Days[0].Blocks {
		if b.BlockType == program.BlockTypeSingle && len(b.ClientUserIDs) == 1 &&
			b.ClientUserIDs[0] == clientUserID {
			extracted = true
		}
	}
	if !extracted {
		t.Fatal("extracted single block did not inherit the client list")
	}
}

func TestMove_keepsBlockClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "move-keeps", 4)
	trainerUserID := ownerUserID(t, pool, programID)
	targetDayID := secondDayID(t, pool, weekID)

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[0].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	after, err := store.MoveDayBlock(ctx, trainerUserID, programID, weekID, blocks[0].ID, targetDayID, 1)
	if err != nil {
		t.Fatalf("MoveDayBlock: %v", err)
	}

	found := false
	for _, day := range after.Weeks[0].Days {
		for _, b := range day.Blocks {
			if b.ID == blocks[0].ID {
				found = true
				if len(b.ClientUserIDs) != 1 || b.ClientUserIDs[0] != clientUserID {
					t.Fatalf("moved block ClientUserIDs = %v, want [%s]", b.ClientUserIDs, clientUserID)
				}
			}
		}
	}
	if !found {
		t.Fatal("moved block not found after move")
	}
}
```

Понадобятся четыре маленьких хелпера в том же файле — `ownerUserID` (`SELECT
created_by FROM mentorix.programs WHERE id = $1`), `dayIDOfBlock` (`SELECT
program_week_day_id FROM mentorix.program_week_day_blocks WHERE id = $1`),
`groupBlock` (первый блок дня с `BlockType != program.BlockTypeSingle`,
`t.Fatal` если такого нет) и `secondDayID` (`SELECT id FROM
mentorix.program_week_days WHERE week_id = $1 ORDER BY day_number OFFSET 1 LIMIT 1`).

- [ ] **Step 2: Запустить тесты, убедиться что падают**

Run: `go test -tags integration ./internal/db/storetest/ -run 'TestUngroup_inheritsBlockClients|TestExtract_inheritsBlockClients|TestMerge_rejectsDifferentClientSets|TestMove_keepsBlockClients' -count=1`
Expected: FAIL — правила не наследуются, merge не отклоняет.

- [ ] **Step 3: Добавить SQL копирования правил**

В `db/queries/program_block_client.sql` дописать:

```sql
-- name: CopyProgramBlockClients :exec
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by)
SELECT program_id, sqlc.arg('target_block_key'), client_user_id, created_by
FROM mentorix.program_block_clients
WHERE program_id = sqlc.arg('program_id')
  AND block_key = sqlc.arg('source_block_key')
ON CONFLICT (program_id, block_key, client_user_id) DO NOTHING;
```

Run: `sqlc generate`

- [ ] **Step 4: Запретить merge разных наборов**

В `internal/program/store_blocks.go` в `MergeDayBlocks`, после загрузки блоков
(`loadMergeBlocks`) и до создания группы, добавить проверку: собрать
`block_key` каждого участника, взять их правила из `listProgramBlockClients`,
сравнить как множества. При расхождении вернуть:

```go
return Detail{}, fmt.Errorf("%w: blocks to merge must have the same client list", ErrValidation)
```

- [ ] **Step 5: Наследовать правила при ungroup и extract**

В `UngroupDayBlock` после вставки каждого нового `single`-блока вызвать
`qtx.CopyProgramBlockClients` с `source_block_key` исходной группы и
`target_block_key` нового блока. Ключ нового блока вернуть из `InsertDayBlock`
(запрос уже возвращает `block_key`, см. Task 2).

В `ExtractBlockExercise` сделать то же самое для одного нового `single`-блока.

- [ ] **Step 6: Запустить тесты, убедиться что проходят**

Run: `go test -tags integration ./internal/db/storetest/ -run 'TestUngroup_inheritsBlockClients|TestExtract_inheritsBlockClients|TestMerge_rejectsDifferentClientSets|TestMove_keepsBlockClients' -count=1 -v`
Expected: PASS

- [ ] **Step 7: Прогнать интеграционные тесты целиком**

Run: `go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1`
Expected: PASS

- [ ] **Step 8: Коммит**

```bash
git add db/queries/ internal/db/sqlc/ internal/program/ internal/db/storetest/
git commit -m "feat(program): inherit block clients on ungroup and extract, guard merge"
```

---

### Task 8: Клиент видит только свои блоки

**Files:**
- Modify: `internal/program/store_client_program.go` (новый метод)
- Modify: `internal/program/service_version.go:74-76`
- Modify: `internal/trainerclient/bot_service.go:14` (интерфейс `ClientProgramReader`), `:165`
- Test: `internal/trainerclient/bot_service_internal_test.go`, `internal/db/storetest/program_block_visibility_integration_test.go`

**Interfaces:**
- Consumes: `FilterDetailForClient` из Task 4, `listProgramBlockClients` из Task 2.
- Produces:
  - `func (s *Store) GetVersionDetailForClient(ctx context.Context, versionID, clientUserID uuid.UUID) (Detail, error)`
  - тот же метод на `*Service`
  - `ClientProgramReader` требует `GetVersionDetailForClient`

- [ ] **Step 1: Написать падающий интеграционный тест**

Дописать в `internal/db/storetest/program_block_visibility_integration_test.go`:

```go
func TestGetVersionDetailForClient_hidesForeignBlocks(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, petya := seedDayWithBlocks(t, pool, "client-filter", 2)
	trainerUserID := ownerUserID(t, pool, programID)

	vasya, trainerID := seedAssignedClient(t, pool, trainerUserID, "client-filter-vasya")
	pid := programID
	if _, err := store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, vasya, &pid); err != nil {
		t.Fatalf("SetClientProgramAssignment(vasya): %v", err)
	}

	// blocks[1] becomes personal to Petya; blocks[0] stays shared.
	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[1].ID,
		[]uuid.UUID{petya}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	versionID := assignedVersionID(t, pool, programID, petya)

	forPetya, err := store.GetVersionDetailForClient(ctx, versionID, petya)
	if err != nil {
		t.Fatalf("GetVersionDetailForClient(petya): %v", err)
	}
	if got := len(forPetya.Weeks[0].Days[0].Blocks); got != 2 {
		t.Fatalf("blocks for listed client = %d, want 2", got)
	}

	forVasya, err := store.GetVersionDetailForClient(ctx, versionID, vasya)
	if err != nil {
		t.Fatalf("GetVersionDetailForClient(vasya): %v", err)
	}
	if got := len(forVasya.Weeks[0].Days[0].Blocks); got != 1 {
		t.Fatalf("blocks for unlisted client = %d, want 1", got)
	}
	if len(forVasya.Weeks[0].Days[0].Blocks[0].ClientUserIDs) != 0 {
		t.Fatal("unlisted client kept a restricted block")
	}
}
```

Хелпер `assignedVersionID` — `SELECT program_version_id FROM
mentorix.program_assignments WHERE program_id = $1 AND client_user_id = $2`.

- [ ] **Step 2: Запустить тест, убедиться что падает**

Run: `go test -tags integration ./internal/db/storetest/ -run TestGetVersionDetailForClient_hidesForeignBlocks -count=1`
Expected: FAIL — `store.GetVersionDetailForClient undefined`.

- [ ] **Step 3: Реализовать метод стора**

В `internal/program/store_client_program.go` дописать:

```go
// GetVersionDetailForClient loads a frozen version and drops the blocks this
// client must not see. Visibility rules live on the program, not on the
// version, so they apply to whichever version the client is currently on.
// GetVersionDetail already applies them (Task 3), so this only filters.
func (s *Store) GetVersionDetailForClient(ctx context.Context, versionID, clientUserID uuid.UUID) (Detail, error) {
	detail, err := s.GetVersionDetail(ctx, versionID)
	if err != nil {
		return Detail{}, err
	}
	return FilterDetailForClient(detail, clientUserID), nil
}
```

- [ ] **Step 4: Пробросить через сервис**

В `internal/program/service_version.go` рядом с `GetVersionDetail`:

```go
func (s *Service) GetVersionDetailForClient(ctx context.Context, versionID, clientUserID uuid.UUID) (Detail, error) {
	return s.store.GetVersionDetailForClient(ctx, versionID, clientUserID)
}
```

Добавить метод в интерфейс стора в `internal/program/service.go` (там же, где
объявлен `GetVersionDetail`).

- [ ] **Step 5: Переключить клиентский путь бота**

В `internal/trainerclient/bot_service.go` в интерфейс `ClientProgramReader`
добавить:

```go
GetVersionDetailForClient(ctx context.Context, versionID, clientUserID uuid.UUID) (program.Detail, error)
```

и заменить вызов на строке 165:

```go
detail, err := reader.GetVersionDetailForClient(ctx, assignment.ProgramVersionID, clientUserID)
```



- [ ] **Step 6: Починить фейки в тестах бота**

Найти реализации `ClientProgramReader` в тестах:
`grep -rn "GetVersionDetail" internal/trainerclient/ internal/telegrambot/ | grep _test`.
В каждый фейк добавить новый метод, делегирующий в существующий с фильтрацией
через `program.FilterDetailForClient`.

- [ ] **Step 7: Запустить тесты, убедиться что проходят**

Run: `go test ./internal/trainerclient/... ./internal/telegrambot/... -count=1`
Expected: PASS

Run: `go test -tags integration ./internal/db/storetest/ -run TestGetVersionDetailForClient_hidesForeignBlocks -count=1`
Expected: PASS

- [ ] **Step 8: Проверить снапшот выполненного дня**

`workoutcompletion.CompleteInput.Day` — это `program.Day`, из которого
`buildDaySnapshot` собирает `day_snapshot`. Значит достаточно передать туда день
из отфильтрованного `Detail`. Дописать в тот же файл:

```go
func TestDaySnapshot_containsOnlyVisibleBlocks(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, petya := seedDayWithBlocks(t, pool, "snapshot-filter", 2)
	trainerUserID := ownerUserID(t, pool, programID)
	vasya, trainerID := seedAssignedClient(t, pool, trainerUserID, "snapshot-vasya")
	pid := programID
	assignment, err := store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, vasya, &pid)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment(vasya): %v", err)
	}
	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[1].ID,
		[]uuid.UUID{petya}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	detail, err := store.GetVersionDetailForClient(ctx, assignment.ProgramVersionID, vasya)
	if err != nil {
		t.Fatalf("GetVersionDetailForClient: %v", err)
	}
	day := detail.Weeks[0].Days[0]

	completion, err := workoutcompletion.NewService(pool).Complete(ctx, workoutcompletion.CompleteInput{
		ClientUserID:        vasya,
		TrainerID:           trainerID,
		ProgramID:           programID,
		ProgramVersionID:    assignment.ProgramVersionID,
		ProgramAssignmentID: assignment.ID,
		CompletionCycleID:   assignment.CompletionCycleID,
		DayKey:              day.DayKey,
		WeekNumber:          detail.Weeks[0].WeekNumber,
		DayNumber:           day.DayNumber,
		ProgramName:         detail.Name,
		Day:                 day,
		ResultText:          "done",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var snapshot string
	if err := pool.QueryRow(ctx,
		`SELECT day_snapshot::text FROM mentorix.client_workout_completions WHERE id = $1`,
		completion.ID).Scan(&snapshot); err != nil {
		t.Fatalf("select day_snapshot: %v", err)
	}
	var parsed struct {
		Blocks []struct {
			BlockType string `json:"block_type"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(snapshot), &parsed); err != nil {
		t.Fatalf("unmarshal day_snapshot: %v", err)
	}
	if len(parsed.Blocks) != 1 {
		t.Fatalf("blocks in day_snapshot = %d, want 1", len(parsed.Blocks))
	}
}
```

Сверить имена полей `Assignment` (`ProgramVersionID`, `ID`, `CompletionCycleID`)
с `internal/program/assignment_types.go` и поправить, если отличаются.
Добавить в импорты `encoding/json` и `mentorix-backend/internal/workoutcompletion`.

Run: `go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1`
Expected: PASS

- [ ] **Step 9: Коммит**

```bash
git add internal/program/ internal/trainerclient/ internal/telegrambot/ internal/db/storetest/
git commit -m "feat(telegram): show a client only the blocks assigned to them"
```

---

### Task 9: Снятие программы и чистка осиротевших правил

**Files:**
- Modify: `internal/program/store_assignment.go:42-173` (`SetClientProgramAssignment`)
- Modify: `internal/program/store_version.go:389-432` (`CleanupProgramVersions`), `:433+` (`cleanupUnusedVersionsBestEffort`)
- Modify: `db/queries/program_block_client.sql`
- Modify: `internal/cleanup/janitor.go`
- Test: `internal/db/storetest/program_block_visibility_integration_test.go`, `internal/db/storetest/cleanup_integration_test.go`

**Interfaces:**
- Consumes: `DeleteProgramBlockClientsForClient` из Task 5.
- Produces: `PurgeOrphanProgramBlockClients` (sqlc, по программе и глобально); `cleanup.Result.ProgramBlockClients int64`.

- [ ] **Step 1: Написать падающие тесты**

Первый — в `internal/db/storetest/program_block_visibility_integration_test.go`:

```go
func TestClearAssignment_removesClientFromBlocks(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "clear-assign", 2)
	trainerUserID := ownerUserID(t, pool, programID)
	var trainerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM mentorix.trainers WHERE user_id = $1`, trainerUserID).Scan(&trainerID); err != nil {
		t.Fatalf("select trainer id: %v", err)
	}

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[1].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	if _, err := store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, clientUserID, nil); err != nil {
		t.Fatalf("SetClientProgramAssignment(clear): %v", err)
	}

	var left int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM mentorix.program_block_clients
		WHERE program_id = $1 AND client_user_id = $2`, programID, clientUserID).Scan(&left); err != nil {
		t.Fatalf("count block clients: %v", err)
	}
	if left != 0 {
		t.Fatalf("block client rows after clear = %d, want 0", left)
	}
}
```

Второй — в `internal/db/storetest/cleanup_integration_test.go`:

```go
func TestCleanup_purgesOrphanBlockClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	userID, _ := seedTrainerAndExercise(t, pool, "orphan-rules")
	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	// A rule pointing at a block_key that exists in neither the template nor any version.
	orphanKey := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
		VALUES ($1, $2, $3)`, draft.ID, orphanKey, userID); err != nil {
		t.Fatalf("insert orphan rule: %v", err)
	}

	if _, err := cleanup.Run(ctx, pool); err != nil {
		t.Fatalf("cleanup.Run: %v", err)
	}

	var left int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM mentorix.program_block_clients
		WHERE block_key = $1`, orphanKey).Scan(&left); err != nil {
		t.Fatalf("count orphan rules: %v", err)
	}
	if left != 0 {
		t.Fatalf("orphan rules after cleanup = %d, want 0", left)
	}
}
```

`seedTrainerAndExercise` живёт в `program_block_visibility_integration_test.go`,
но пакет тестов один (`storetest`), поэтому вызывается напрямую.

- [ ] **Step 2: Запустить тесты, убедиться что падают**

Run: `go test -tags integration ./internal/db/storetest/ -run 'TestClearAssignment_removesClientFromBlocks|TestCleanup_purgesOrphanBlockClients' -count=1`
Expected: FAIL

- [ ] **Step 3: Добавить SQL чистки**

В `db/queries/program_block_client.sql` дописать:

```sql
-- name: PurgeOrphanProgramBlockClientsForProgram :execrows
DELETE FROM mentorix.program_block_clients pbc
WHERE pbc.program_id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_week_day_blocks b
    JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
    WHERE d.program_id = pbc.program_id AND b.block_key = pbc.block_key
  )
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_version_week_day_blocks vb
    JOIN mentorix.program_version_week_days vd
      ON vd.id = vb.program_version_week_day_id
    JOIN mentorix.program_versions v ON v.id = vd.program_version_id
    WHERE v.program_id = pbc.program_id AND vb.block_key = pbc.block_key
  );

-- name: PurgeOrphanProgramBlockClients :execrows
DELETE FROM mentorix.program_block_clients pbc
WHERE NOT EXISTS (
    SELECT 1
    FROM mentorix.program_week_day_blocks b
    JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
    WHERE d.program_id = pbc.program_id AND b.block_key = pbc.block_key
  )
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_version_week_day_blocks vb
    JOIN mentorix.program_version_week_days vd
      ON vd.id = vb.program_version_week_day_id
    JOIN mentorix.program_versions v ON v.id = vd.program_version_id
    WHERE v.program_id = pbc.program_id AND vb.block_key = pbc.block_key
  );
```

Run: `sqlc generate`

- [ ] **Step 4: Чистить клиента при снятии и смене программы**

В `internal/program/store_assignment.go` в `SetClientProgramAssignment`, в ветках
`clear` (удаление строки) и `reassign` (смена `program_id`), вызвать в той же
транзакции:

```go
if err := qtx.DeleteProgramBlockClientsForClient(ctx, sqlc.DeleteProgramBlockClientsForClientParams{
	ProgramID:    pgconv.ToPGUUID(previousProgramID),
	ClientUserID: pgconv.ToPGUUID(clientUserID),
}); err != nil {
	return nil, fmt.Errorf("delete block clients for client: %w", err)
}
```

`previousProgramID` — `program_id` строки назначения **до** изменения.

- [ ] **Step 5: Чистить осиротевшие правила вместе с версиями**

В `internal/program/store_version.go` в `CleanupProgramVersions` и в
`cleanupUnusedVersionsBestEffort` после удаления версий вызвать
`s.q.PurgeOrphanProgramBlockClientsForProgram(ctx, pgconv.ToPGUUID(programID))`.
В best-effort варианте ошибку логировать так же, как это уже сделано для версий,
и не возвращать наверх.

- [ ] **Step 6: Добавить чистку в janitor**

В `internal/cleanup/janitor.go`:

```go
type Result struct {
	TrainerInvites     int64
	RefreshSessions    int64
	ProgramBlockClients int64
}
```

и в `Run` после существующих purge-вызовов:

```go
blockClients, err := q.PurgeOrphanProgramBlockClients(ctx)
if err != nil {
	return Result{}, fmt.Errorf("purge orphan program block clients: %w", err)
}
```

добавив поле в возвращаемый `Result`. В `cmd/janitor/main.go` дописать в лог:

```go
"program_block_clients_deleted", result.ProgramBlockClients,
```

- [ ] **Step 7: Запустить тесты, убедиться что проходят**

Run: `go test -tags integration ./internal/db/storetest/ -run 'TestClearAssignment_removesClientFromBlocks|TestCleanup_purgesOrphanBlockClients' -count=1 -v`
Expected: PASS

- [ ] **Step 8: Прогнать всё**

Run: `go test ./... -count=1 && go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1`
Expected: PASS

- [ ] **Step 9: Коммит**

```bash
git add db/queries/ internal/db/sqlc/ internal/program/ internal/cleanup/ cmd/janitor/ internal/db/storetest/
git commit -m "feat(program): drop block visibility rules on unassign and cleanup"
```

---

### Task 10: Контракт API и документация

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `postman/mentorix-backend.postman_collection.json`
- Modify: `internal/apicheck/schema.go:76-91`
- Create: `docs/features/program-block-visibility.md`
- Modify: `docs/README.md`, `docs/status.md`
- Modify: `.claude/rules/database-naming.md`, `.claude/rules/api-endpoints.md`

**Interfaces:**
- Consumes: маршрут и DTO из Task 5.
- Produces: согласованный контракт; `docs-check` и `apicheck` зелёные.

- [ ] **Step 1: Добавить схему в apicheck**

В `internal/apicheck/schema.go` в список схем дописать:

```go
{name: "ProgramBlockClientsUpdate", typ: reflect.TypeOf(struct {
	ClientUserIDs []string `json:"client_user_ids"`
}{})},
```

- [ ] **Step 2: Запустить контрактный тест, убедиться что падает**

Run: `go test ./internal/apicheck/... -count=1`
Expected: FAIL — схемы `ProgramBlockClientsUpdate` нет в `api/openapi.yaml`.

- [ ] **Step 3: Описать эндпоинт и поле в OpenAPI**

В `api/openapi.yaml` добавить путь рядом с остальными block-маршрутами:

```yaml
  /programs/{id}/weeks/{week_id}/blocks/{block_id}/clients:
    parameters:
      - $ref: "#/components/parameters/ProgramId"
      - $ref: "#/components/parameters/WeekId"
      - $ref: "#/components/parameters/BlockId"
    put:
      tags: [programs]
      summary: Replace the client list of a day block
      description: >
        Full replace. An empty list makes the block shared with every client the
        program is assigned to. Only clients with an active assignment to this
        program are accepted.
      operationId: setProgramBlockClients
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/ProgramBlockClientsUpdate"
      responses:
        "200":
          description: Updated program detail
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProgramDetail"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          description: Forbidden (not the program owner)
        "404":
          description: Program, week or block not found
```

Добавить схему `ProgramBlockClientsUpdate` (массив uuid `client_user_ids`) и поле
`client_user_ids` в схему `ProgramDayBlock`. Имена параметров `WeekId` и `BlockId`
взять те, что уже используются соседними путями; если их нет — описать параметры
инлайном, как сделано у соседей.

- [ ] **Step 4: Запустить контрактные проверки**

Run: `go test ./internal/apicheck/... -count=1 && SKIP_SMOKE=1 ./postman/validate.sh`
Expected: PASS

- [ ] **Step 5: Добавить запрос в Postman**

В `postman/mentorix-backend.postman_collection.json` добавить запрос
`PUT {{base_url}}/programs/{{program_id}}/weeks/{{week_id}}/blocks/{{block_id}}/clients`
с телом `{"client_user_ids": []}` в ту же папку, где лежат остальные block-запросы.

Run: `SKIP_SMOKE=1 ./postman/validate.sh`
Expected: PASS

- [ ] **Step 6: Написать feature-док**

Создать `docs/features/program-block-visibility.md` (не длиннее 150 строк, по
образцу `docs/features/program-blocks.md`): назначение, таблица `program_block_clients`
и `block_key`, правило «пустой список = всем», инвариант дня и где он проверяется,
поведение при ungroup / extract / merge / снятии программы, чистка правил.
Не дублировать OpenAPI и правила именования — только ссылки.

- [ ] **Step 7: Связать док и статус**

В `docs/README.md` добавить ссылку на новый файл в список feature-доков.

В `docs/status.md` добавить в таблицу «Реализовано» строку с описанием
«Персональная видимость блоков дня (`block_key`, список клиентов на блоке)» и
ссылкой на `features/program-block-visibility.md`. Формат ссылки — такой же, как
у соседних строк таблицы.

- [ ] **Step 8: Обновить правила агента**

В `.claude/rules/database-naming.md` в разделе про program-деревья добавить
строку про `program_block_clients` и `block_key`.
В `.claude/rules/api-endpoints.md` в таблицу «Program week subtree» добавить
`PUT …/blocks/{block_id}/clients` в строку Block instance.

Следить за лимитом 120 строк на файл правил.

- [ ] **Step 9: Прогнать полный QA**

Run: `make check-ci`
Expected: `All checks passed.`

- [ ] **Step 10: Коммит**

```bash
git add api/openapi.yaml postman/ internal/apicheck/ docs/ .claude/rules/
git commit -m "docs(program): document per-client block visibility contract"
```

---

## После выполнения

Открыть PR в `develop` с описанием: что меняется, зачем и как проверено
(`make check` + прогон интеграционных тестов). Мержить `--squash`.
