# Страница статистики клиента по ссылке из Telegram — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Клиент нажимает в боте «📊 Статистика», получает подписанную ссылку на страницу фронта, а фронт по этой ссылке забирает аналитику клиента новым эндпоинтом `GET /client/analytics`.

**Architecture:** Ссылка — HMAC-подписанный URL с TTL 30 минут по образцу аватар-ссылок (`trainerclient/avatar_url.go`); подпись и проверка живут в `internal/analytics/client_link.go`. Эндпоинт без JWT: хендлер проверяет подпись, сервис отдаёт сводку по паре `(trainers.id, client_user_id)` через общую с тренерским путём функцию плюс последние 30 тренировок. Бот резолвит клиента и активного тренера уже существующими методами `trainerclient.Service` и строит ссылку через маленький интерфейс, реализованный в `analytics`.

**Tech Stack:** Go 1.x, Echo v4, pgx/v5, sqlc, PostgreSQL (схема `mentorix`), `go-telegram-bot-api/v5`, `crypto/hmac`.

**Spec:** [2026-09-16-client-analytics-link-design.md](../specs/2026-09-16-client-analytics-link-design.md)

## Global Constraints

- Ветка: `feature/client-analytics-link` от `develop`. Ничего не пушить в `develop` напрямую.
- Формат ссылки: `<CLIENT_ANALYTICS_PAGE_URL>?client_user_id=<uuid>&trainer_id=<uuid>&exp=<unix>&sig=<hex>`. Порядок параметров именно такой.
- Подпись: `hex(HMAC-SHA256(JWT_SECRET, "client_analytics:" + client_user_id + ":" + trainer_id + ":" + exp))`. Префикс `client_analytics:` — не `avatar:`.
- TTL ссылки — 30 минут, константа `clientAnalyticsLinkTTL`.
- `trainer_id` в ссылке и ответе — `trainers.id`, не `user_id` тренера.
- Лимит ленты в ответе — 30, константа `clientSelfRecentCompletionsLimit`.
- Эндпоинт `GET /client/analytics` — без `auth.JWTMiddleware`, без ролей. Коды: 400 (параметр отсутствует/не парсится), 401 (подпись/срок), 404 (нет связи или `blocked`), 200.
- Тренерский `GET /trainer/clients/{client_user_id}/analytics` после рефакторинга ведёт себя ровно как раньше; его тесты не трогать.
- JSON-ключи API — `snake_case`; в Go всегда явный тег `json:"..."`. Слайсы в ответе сериализуются массивом (`[]`), не `null`.
- Новых таблиц, миграций, SQL-запросов и Redis-ключей нет.
- `//nolint` не вводить. `any`-хаков и пустых `catch`-аналогов (`_ = err` для маскировки) — нет, кроме уже существующих паттернов в `sendText`.
- Каждая задача завершается коммитом; pre-commit прогоняет `make check-ci` (можно `PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit …`).
- Коммиты — Conventional Commits на английском, без trailers. `feat` только для user-facing поведения.
- Интеграционные тесты: `go test -tags integration ./internal/db/storetest/...` требуют `TEST_DATABASE_URL`. Если базы нет — так и написать в отчёте по задаче, не выдавать за прогнанное.

---

### Task 1: Конфиг — `CLIENT_ANALYTICS_PAGE_URL`

**Files:**
- Modify: `internal/config/config.go` (struct `Config`, `Load`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `cfg.ClientAnalyticsPageURL string` — пустая строка, если фича выключена; иначе абсолютный `http(s)` URL без query и fragment. Обрезаются только пробелы по краям: путь вроде `/stats` и завершающий `/` — часть адреса страницы, их не трогать.

- [ ] **Step 1: Создать ветку**

```bash
git checkout develop && git pull --ff-only && git checkout -b feature/client-analytics-link
```

- [ ] **Step 2: Написать падающие тесты**

В `internal/config/config_test.go` добавить в конец файла:

```go
func TestLoad_clientAnalyticsPageURL(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CLIENT_ANALYTICS_PAGE_URL", " https://app.example.com/stats ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientAnalyticsPageURL != "https://app.example.com/stats" {
		t.Errorf("url = %q", cfg.ClientAnalyticsPageURL)
	}
}

func TestLoad_clientAnalyticsPageURL_optional(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CLIENT_ANALYTICS_PAGE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientAnalyticsPageURL != "" {
		t.Errorf("url = %q, want empty", cfg.ClientAnalyticsPageURL)
	}
}

func TestLoad_clientAnalyticsPageURL_invalid(t *testing.T) {
	for _, raw := range []string{
		"app.example.com/stats",              // no scheme
		"ftp://app.example.com/stats",        // wrong scheme
		"https://app.example.com/stats?x=1",  // query is reserved for the link
		"https://app.example.com/stats#top",  // fragment
	} {
		t.Run(raw, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv("CLIENT_ANALYTICS_PAGE_URL", raw)
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %q", raw)
			}
		})
	}
}
```

- [ ] **Step 3: Убедиться, что тесты падают**

Run: `go test ./internal/config/ -run 'TestLoad_clientAnalyticsPageURL' -v`
Expected: FAIL — `cfg.ClientAnalyticsPageURL undefined`.

- [ ] **Step 4: Реализация**

В `internal/config/config.go`:

1. В импорты добавить `"net/url"`.
2. В `Config` после `BotWebhookSecret string` добавить:

```go
	// ClientAnalyticsPageURL is the frontend page the Telegram bot links to for
	// client self-analytics. Empty disables the "Статистика" button.
	ClientAnalyticsPageURL string
```

3. В `Load` в литерал `cfg := Config{…}` после `BotWebhookSecret: …,` добавить:

```go
		ClientAnalyticsPageURL: strings.TrimSpace(os.Getenv("CLIENT_ANALYTICS_PAGE_URL")),
```

4. После блока проверок `BotWebhookURL` (перед `return cfg, nil`) добавить:

```go
	if cfg.ClientAnalyticsPageURL != "" {
		if err := validatePageURL(cfg.ClientAnalyticsPageURL); err != nil {
			return Config{}, fmt.Errorf("config: CLIENT_ANALYTICS_PAGE_URL %w", err)
		}
	}
```

5. В конец файла добавить:

```go
// validatePageURL accepts only an absolute http(s) URL without query and
// fragment: the bot appends its own query string to it.
func validatePageURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must use http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("must be absolute")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must not contain query or fragment")
	}
	return nil
}
```

- [ ] **Step 5: Прогнать тесты**

Run: `go test ./internal/config/ -v`
Expected: PASS, включая три новых теста.

- [ ] **Step 6: Коммит**

```bash
git add internal/config/config.go internal/config/config_test.go
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "chore(config): add CLIENT_ANALYTICS_PAGE_URL"
```

---

### Task 2: Подпись ссылки — `internal/analytics/client_link.go`

**Files:**
- Create: `internal/analytics/client_link.go`
- Test: `internal/analytics/client_link_test.go`

**Interfaces:**
- Produces:
  - `func BuildClientAnalyticsLink(pageURL, secret string, clientUserID, trainerID uuid.UUID, now time.Time) string` — пустая строка, если `pageURL` или `secret` пуст.
  - `func VerifyClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID, exp int64, sig string, now time.Time) bool`
  - `type ClientLinkBuilder struct` + `func NewClientLinkBuilder(pageURL, secret string) *ClientLinkBuilder` + метод `BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string` (использует `time.Now()`). Его сигнатура — то, что бот ждёт от интерфейса `telegrambot.ClientAnalyticsLinker` (Task 6).
  - `const clientAnalyticsLinkTTL = 30 * time.Minute`

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/analytics/client_link_test.go`:

```go
package analytics

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/trainerclient"
)

const testLinkSecret = "test-jwt-secret-at-least-32-chars-long"

func parseLink(t *testing.T, link string) (clientUserID, trainerID uuid.UUID, exp int64, sig string) {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	q := u.Query()
	clientUserID, err = uuid.Parse(q.Get("client_user_id"))
	if err != nil {
		t.Fatalf("client_user_id: %v", err)
	}
	trainerID, err = uuid.Parse(q.Get("trainer_id"))
	if err != nil {
		t.Fatalf("trainer_id: %v", err)
	}
	exp, err = strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil {
		t.Fatalf("exp: %v", err)
	}
	return clientUserID, trainerID, exp, q.Get("sig")
}

func TestClientAnalyticsLink_roundTrip(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()

	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	if !strings.HasPrefix(link, "https://app.example.com/stats?client_user_id=") {
		t.Fatalf("link = %q", link)
	}
	gotClient, gotTrainer, exp, sig := parseLink(t, link)
	if gotClient != clientID || gotTrainer != trainerID {
		t.Fatalf("ids = %s / %s", gotClient, gotTrainer)
	}
	if want := now.Add(clientAnalyticsLinkTTL).Unix(); exp != want {
		t.Fatalf("exp = %d, want %d", exp, want)
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now) {
		t.Fatal("fresh link must verify")
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now.Add(29*time.Minute)) {
		t.Fatal("link must verify before expiry")
	}
}

func TestClientAnalyticsLink_expired(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()
	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	_, _, exp, sig := parseLink(t, link)

	if VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now.Add(31*time.Minute)) {
		t.Fatal("expired link must not verify")
	}
}

func TestClientAnalyticsLink_tamper(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()
	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	_, _, exp, sig := parseLink(t, link)

	// Flip the last hex digit so the tampered signature always differs.
	flipped := sig[:len(sig)-1] + "0"
	if sig[len(sig)-1] == '0' {
		flipped = sig[:len(sig)-1] + "1"
	}

	cases := map[string]bool{
		"other client":  VerifyClientAnalyticsLink(testLinkSecret, uuid.New(), trainerID, exp, sig, now),
		"other trainer": VerifyClientAnalyticsLink(testLinkSecret, clientID, uuid.New(), exp, sig, now),
		"other exp":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp+1, sig, now),
		"other sig":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, flipped, now),
		"other secret":  VerifyClientAnalyticsLink("another-secret-at-least-32-chars-long!!", clientID, trainerID, exp, sig, now),
		"empty sig":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, "", now),
		"empty secret":  VerifyClientAnalyticsLink("", clientID, trainerID, exp, sig, now),
		"zero exp":      VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, 0, sig, now),
	}
	for name, ok := range cases {
		if ok {
			t.Errorf("%s: must not verify", name)
		}
	}
}

// An avatar signature over the same client id must not open the analytics page:
// the two link kinds live in different HMAC domains.
func TestClientAnalyticsLink_avatarSignatureRejected(t *testing.T) {
	clientID, trainerID := uuid.New(), uuid.New()
	avatar := trainerclient.BuildAvatarURL(clientID, "photos/x.jpg", testLinkSecret)
	u, err := url.Parse(avatar)
	if err != nil {
		t.Fatalf("parse avatar: %v", err)
	}
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if err != nil {
		t.Fatalf("avatar exp: %v", err)
	}
	if VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, u.Query().Get("sig"), time.Now()) {
		t.Fatal("avatar signature must not verify as analytics link")
	}
}

func TestBuildClientAnalyticsLink_disabled(t *testing.T) {
	now := time.Now()
	if got := BuildClientAnalyticsLink("", testLinkSecret, uuid.New(), uuid.New(), now); got != "" {
		t.Fatalf("empty page url: got %q", got)
	}
	if got := BuildClientAnalyticsLink("https://app.example.com/stats", "", uuid.New(), uuid.New(), now); got != "" {
		t.Fatalf("empty secret: got %q", got)
	}
}

func TestClientLinkBuilder(t *testing.T) {
	b := NewClientLinkBuilder("https://app.example.com/stats", testLinkSecret)
	clientID, trainerID := uuid.New(), uuid.New()
	link := b.BuildClientAnalyticsLink(clientID, trainerID)
	gotClient, gotTrainer, exp, sig := parseLink(t, link)
	if gotClient != clientID || gotTrainer != trainerID {
		t.Fatalf("ids = %s / %s", gotClient, gotTrainer)
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, time.Now()) {
		t.Fatal("builder link must verify")
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./internal/analytics/ -run 'ClientAnalyticsLink|ClientLinkBuilder' -v`
Expected: FAIL — `undefined: BuildClientAnalyticsLink`.

- [ ] **Step 3: Реализация**

Создать `internal/analytics/client_link.go`:

```go
package analytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// clientAnalyticsLinkTTL bounds how long a link from the Telegram bot opens
// the client's analytics page. The link is a bearer secret: keep it short.
const clientAnalyticsLinkTTL = 30 * time.Minute

// clientAnalyticsLinkDomain separates these signatures from avatar URLs that
// share the same secret.
const clientAnalyticsLinkDomain = "client_analytics:"

// BuildClientAnalyticsLink returns the frontend page URL with a signed query
// string, or "" when the feature is not configured.
func BuildClientAnalyticsLink(pageURL, secret string, clientUserID, trainerID uuid.UUID, now time.Time) string {
	if pageURL == "" || secret == "" {
		return ""
	}
	exp := now.Add(clientAnalyticsLinkTTL).Unix()
	sig := signClientAnalyticsLink(secret, clientUserID, trainerID, exp)
	return fmt.Sprintf("%s?client_user_id=%s&trainer_id=%s&exp=%d&sig=%s", pageURL, clientUserID, trainerID, exp, sig)
}

// VerifyClientAnalyticsLink checks the signature and expiry of query
// parameters forwarded by the frontend.
func VerifyClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID, exp int64, sig string, now time.Time) bool {
	if secret == "" || sig == "" || exp <= 0 {
		return false
	}
	if now.Unix() > exp {
		return false
	}
	expected := signClientAnalyticsLink(secret, clientUserID, trainerID, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

func signClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID, exp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(clientAnalyticsLinkDomain))
	_, _ = mac.Write([]byte(clientUserID.String()))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(trainerID.String()))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ClientLinkBuilder is what the Telegram bot uses to issue links; it hides the
// page URL and secret from the bot package.
type ClientLinkBuilder struct {
	pageURL string
	secret  string
}

func NewClientLinkBuilder(pageURL, secret string) *ClientLinkBuilder {
	return &ClientLinkBuilder{pageURL: pageURL, secret: secret}
}

func (b *ClientLinkBuilder) BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string {
	return BuildClientAnalyticsLink(b.pageURL, b.secret, clientUserID, trainerID, time.Now())
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./internal/analytics/ -run 'ClientAnalyticsLink|ClientLinkBuilder' -v`
Expected: PASS (6 тестов).

- [ ] **Step 5: Коммит**

```bash
git add internal/analytics/client_link.go internal/analytics/client_link_test.go
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "chore(analytics): sign and verify client analytics links"
```

---

### Task 3: Сервис — `ClientSelfAnalytics` и общая часть с тренерским путём

**Files:**
- Modify: `internal/analytics/model.go` (после `ClientAnalytics`/`ClientInfo`)
- Modify: `internal/analytics/store.go` (новый метод)
- Modify: `internal/analytics/service.go` (`ClientAnalytics`, `ClientCompletions`, новые функции)
- Test: `internal/db/storetest/analytics_integration_test.go` (integration)

**Interfaces:**
- Consumes: `Store.ClientHeader`, `Store.ListClientCompletions`, `Service.commentsByCompletionIDs`, sqlc `GetTrainerUserDisplayName`.
- Produces:
  - `type ClientSelfAnalytics struct { Client ClientSelfInfo; Trainer ClientSelfTrainer; CurrentAssignment *AssignmentAnalytics; Activity ActivityStats; RecentCompletions []CompletionItem; ExpiresAt time.Time }`
  - `func (s *Service) ClientSelfAnalytics(ctx context.Context, clientUserID, trainerID uuid.UUID) (ClientSelfAnalytics, error)` — `ExpiresAt` оставляет нулевым, его ставит хендлер (Task 4).
  - `const clientSelfRecentCompletionsLimit = 30`

- [ ] **Step 1: Написать падающий интеграционный тест**

В `internal/db/storetest/analytics_integration_test.go` после `TestAnalyticsService_ClientCompletions` добавить:

```go
func TestAnalyticsService_ClientSelfAnalytics(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-self")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	got, err := svc.ClientSelfAnalytics(ctx, fx.clientUserID, fx.trainerID)
	if err != nil {
		t.Fatalf("ClientSelfAnalytics: %v", err)
	}
	if got.Client.ClientUserID != fx.clientUserID {
		t.Fatalf("client = %+v", got.Client)
	}
	if got.Trainer.TrainerID != fx.trainerID {
		t.Fatalf("trainer = %+v", got.Trainer)
	}
	if got.CurrentAssignment == nil || got.CurrentAssignment.ProgramID != fx.programID {
		t.Fatalf("current_assignment = %+v", got.CurrentAssignment)
	}
	if got.CurrentAssignment.Progress.CompletedDays != 2 || got.CurrentAssignment.Progress.TotalTrainingDays != 3 {
		t.Fatalf("progress = %+v", got.CurrentAssignment.Progress)
	}
	if got.Activity.TotalCompletions != 3 {
		t.Fatalf("activity = %+v", got.Activity)
	}
	if len(got.RecentCompletions) != 3 {
		t.Fatalf("recent = %d items", len(got.RecentCompletions))
	}
	for i := 1; i < len(got.RecentCompletions); i++ {
		if got.RecentCompletions[i].CompletedAt.After(got.RecentCompletions[i-1].CompletedAt) {
			t.Fatalf("recent completions must be sorted desc: %+v", got.RecentCompletions)
		}
	}
	for _, item := range got.RecentCompletions {
		if item.Comments == nil {
			t.Fatalf("comments must be [] not null: %+v", item)
		}
	}
	if !got.ExpiresAt.IsZero() {
		t.Fatalf("expires_at is set by the handler, got %v", got.ExpiresAt)
	}

	// Trainer the client is not linked to → not found.
	if _, err := svc.ClientSelfAnalytics(ctx, fx.clientUserID, uuid.New()); !errors.Is(err, analytics.ErrClientNotFound) {
		t.Fatalf("foreign trainer err = %v", err)
	}
	// Unknown client → not found.
	if _, err := svc.ClientSelfAnalytics(ctx, uuid.New(), fx.trainerID); !errors.Is(err, analytics.ErrClientNotFound) {
		t.Fatalf("unknown client err = %v", err)
	}

	// Blocked link → not found: the trainer revoked access.
	if _, err := pool.Exec(ctx, `
		UPDATE mentorix.trainer_clients SET status = 'blocked'
		WHERE trainer_id = $1 AND client_user_id = $2
	`, fx.trainerID, fx.clientUserID); err != nil {
		t.Fatalf("block: %v", err)
	}
	if _, err := svc.ClientSelfAnalytics(ctx, fx.clientUserID, fx.trainerID); !errors.Is(err, analytics.ErrClientNotFound) {
		t.Fatalf("blocked err = %v", err)
	}
}
```

- [ ] **Step 2: Убедиться, что тест не компилируется**

Run: `go test -tags integration ./internal/db/storetest/ -run TestAnalyticsService_ClientSelfAnalytics -count=1`
Expected: FAIL — `svc.ClientSelfAnalytics undefined`. (Без `TEST_DATABASE_URL` компиляция всё равно пройдёт до этой ошибки — этого достаточно для красной фазы.)

- [ ] **Step 3: Модели**

В `internal/analytics/model.go` после структуры `ClientInfo` добавить:

```go
// ClientSelfAnalytics is what the client sees on the page opened from the
// Telegram bot: the same figures the trainer sees for this client, scoped to
// one trainer, plus the latest completions. No avatar, no link status.
type ClientSelfAnalytics struct {
	Client            ClientSelfInfo       `json:"client"`
	Trainer           ClientSelfTrainer    `json:"trainer"`
	CurrentAssignment *AssignmentAnalytics `json:"current_assignment"`
	Activity          ActivityStats        `json:"activity"`
	RecentCompletions []CompletionItem     `json:"recent_completions"`
	// ExpiresAt is when the signed link stops working; set by the handler.
	ExpiresAt time.Time `json:"expires_at"`
}

type ClientSelfInfo struct {
	ClientUserID uuid.UUID `json:"client_user_id"`
	DisplayName  string    `json:"display_name"`
}

type ClientSelfTrainer struct {
	TrainerID   uuid.UUID `json:"trainer_id"`
	DisplayName string    `json:"display_name"`
}
```

- [ ] **Step 4: Store**

В `internal/analytics/store.go` после `ClientHeader` добавить:

```go
func (s *Store) TrainerDisplayName(ctx context.Context, trainerID uuid.UUID) (string, error) {
	return s.q.GetTrainerUserDisplayName(ctx, pgconv.ToPGUUID(trainerID))
}
```

- [ ] **Step 5: Сервис — выделить общую часть**

В `internal/analytics/service.go`:

1. Заменить начало `ClientAnalytics` так, чтобы после получения `trainerID` вся работа шла в новой функции:

```go
func (s *Service) ClientAnalytics(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (ClientAnalytics, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return ClientAnalytics{}, err
	}
	return s.clientAnalyticsByTrainerID(ctx, trainerID, clientUserID)
}

// clientAnalyticsByTrainerID is shared by the trainer endpoint (trainer resolved
// from the JWT) and the client self page (trainer id taken from the signed link).
func (s *Service) clientAnalyticsByTrainerID(ctx context.Context, trainerID, clientUserID uuid.UUID) (ClientAnalytics, error) {
	header, err := s.store.ClientHeader(ctx, trainerID, clientUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientAnalytics{}, ErrClientNotFound
		}
		return ClientAnalytics{}, fmt.Errorf("client header: %w", err)
	}
	// … далее тело прежней ClientAnalytics без изменений, начиная с `now := s.now()`
	// и до `return result, nil`.
}
```

Тело от `now := s.now()` до конца переносится в `clientAnalyticsByTrainerID` как есть.

2. В `ClientCompletions` вынести построение элементов в отдельную функцию. Заменить участок от `rows, err := s.store.ListClientCompletions(...)` до `return CompletionsResult{…}` на:

```go
	items, err := s.completionItems(ctx, trainerID, clientUserID, params)
	if err != nil {
		return CompletionsResult{}, err
	}
	return CompletionsResult{
		Items:      items,
		Pagination: paginationMeta(params.Page, params.Limit, total),
	}, nil
```

и после `ClientCompletions` добавить:

```go
// completionItems loads one page of the client's journal with trainer replies.
func (s *Service) completionItems(ctx context.Context, trainerID, clientUserID uuid.UUID, params CompletionsParams) ([]CompletionItem, error) {
	rows, err := s.store.ListClientCompletions(ctx, trainerID, clientUserID, params)
	if err != nil {
		return nil, fmt.Errorf("list completions: %w", err)
	}
	comments, err := s.commentsByCompletionIDs(ctx, completionIDsFromClientRows(rows))
	if err != nil {
		return nil, err
	}
	items := make([]CompletionItem, 0, len(rows))
	for _, row := range rows {
		id := pgconv.FromPGUUID(row.ID)
		itemComments := comments[id]
		if itemComments == nil {
			itemComments = []CompletionComment{}
		}
		items = append(items, CompletionItem{
			ID:             id,
			CompletedAt:    row.CompletedAt.UTC(),
			ProgramID:      uuidPtrFromPG(row.ProgramID),
			ProgramName:    row.ProgramName,
			ProgramNameRu:  row.ProgramNameRu,
			WeekNumber:     int(row.WeekNumber),
			DayNumber:      int(row.DayNumber),
			ResultText:     row.ResultText,
			IsCurrentCycle: row.IsCurrentCycle,
			Comments:       itemComments,
		})
	}
	return items, nil
}
```

Старый цикл `for _, row := range rows { … }` из `ClientCompletions` удалить — он переехал сюда.

- [ ] **Step 6: Сервис — `ClientSelfAnalytics`**

В `internal/analytics/service.go` после `completionItems` добавить:

```go
const clientSelfRecentCompletionsLimit = 30

const clientLinkStatusBlocked = "blocked"

// ClientSelfAnalytics builds the client's own page for one trainer. Access was
// already proven by the signed link; here we only check the link is still alive.
func (s *Service) ClientSelfAnalytics(ctx context.Context, clientUserID, trainerID uuid.UUID) (ClientSelfAnalytics, error) {
	base, err := s.clientAnalyticsByTrainerID(ctx, trainerID, clientUserID)
	if err != nil {
		return ClientSelfAnalytics{}, err
	}
	if base.Client.Status == clientLinkStatusBlocked {
		return ClientSelfAnalytics{}, ErrClientNotFound
	}

	trainerName, err := s.store.TrainerDisplayName(ctx, trainerID)
	if err != nil {
		return ClientSelfAnalytics{}, fmt.Errorf("trainer display name: %w", err)
	}

	recent, err := s.completionItems(ctx, trainerID, clientUserID, CompletionsParams{
		Page:  1,
		Limit: clientSelfRecentCompletionsLimit,
	})
	if err != nil {
		return ClientSelfAnalytics{}, err
	}

	return ClientSelfAnalytics{
		Client: ClientSelfInfo{
			ClientUserID: base.Client.ClientUserID,
			DisplayName:  base.Client.DisplayName,
		},
		Trainer: ClientSelfTrainer{
			TrainerID:   trainerID,
			DisplayName: trainerName,
		},
		CurrentAssignment: base.CurrentAssignment,
		Activity:          base.Activity,
		RecentCompletions: recent,
	}, nil
}
```

Проверить, что `CompletionsParams.Offset()` при `Page: 1` даёт `0` (см. `params.go:59`); если формула другая — подставить `Page` так, чтобы offset был 0.

- [ ] **Step 7: Прогнать юнит и интеграционные тесты**

Run: `go test ./internal/analytics/ -count=1`
Expected: PASS (старые тесты сервиса и хендлеров не сломаны).

Run: `go test -tags integration ./internal/db/storetest/ -run 'TestAnalyticsService_' -count=1 -v`
Expected: PASS для `ClientAnalytics`, `ClientCompletions`, `ClientSelfAnalytics` и остальных. Если `TEST_DATABASE_URL` не задан — тесты будут пропущены/упадут на подключении: зафиксировать это в отчёте как «интеграция не прогнана», не считать задачу проверенной.

- [ ] **Step 8: gofmt, vet, lint**

Run: `gofmt -l ./internal/analytics && go vet ./internal/analytics/ && golangci-lint run ./internal/analytics/...`
Expected: пустой вывод gofmt, без ошибок.

- [ ] **Step 9: Коммит**

```bash
git add internal/analytics/model.go internal/analytics/store.go internal/analytics/service.go internal/db/storetest/analytics_integration_test.go
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "refactor(analytics): share client analytics core and add client self view"
```

---

### Task 4: Хендлер `GET /client/analytics`

**Files:**
- Modify: `internal/analytics/handlers.go` (`Mount`, новый хендлер)
- Test: `internal/analytics/handlers_test.go`

**Interfaces:**
- Consumes: `VerifyClientAnalyticsLink` (Task 2), `Service.ClientSelfAnalytics` (Task 3), `HTTPErrorFrom`.
- Produces: маршрут `GET /client/analytics`, хендлер `(*Handlers).GetClientSelfAnalytics`.

- [ ] **Step 1: Написать падающие тесты**

В `internal/analytics/handlers_test.go`:

1. В `TestHandlers_Mount_registersRoutes` в слайс `want` добавить строку `"GET /client/analytics",`.
2. В импорты добавить `"fmt"`, `"net/url"`, `"strconv"`, `"time"`.
3. В конец файла добавить:

```go
func clientLinkQuery(t *testing.T, clientID, trainerID uuid.UUID, now time.Time) url.Values {
	t.Helper()
	link := analytics.BuildClientAnalyticsLink("https://app.example.com/stats", "test-jwt-secret-at-least-32-chars-long", clientID, trainerID, now)
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	return u.Query()
}

func TestGetClientSelfAnalytics_badRequest(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	clientID, trainerID := uuid.New(), uuid.New()
	valid := clientLinkQuery(t, clientID, trainerID, time.Now())

	cases := map[string]url.Values{}
	for _, drop := range []string{"client_user_id", "trainer_id", "exp", "sig"} {
		q := url.Values{}
		for k, v := range valid {
			if k != drop {
				q[k] = v
			}
		}
		cases["missing "+drop] = q
	}
	bad := url.Values{}
	for k, v := range valid {
		bad[k] = v
	}
	bad.Set("client_user_id", "not-a-uuid")
	cases["bad client_user_id"] = bad
	badExp := url.Values{}
	for k, v := range valid {
		badExp[k] = v
	}
	badExp.Set("exp", "soon")
	cases["bad exp"] = badExp

	for name, q := range cases {
		c, _ := testContext(e, "/client/analytics?"+q.Encode(), uuid.Nil, nil)
		err := h.GetClientSelfAnalytics(c)
		he, ok := err.(*echo.HTTPError)
		if !ok || he.Code != http.StatusBadRequest {
			t.Fatalf("%s: err = %v, want 400", name, err)
		}
	}
}

func TestGetClientSelfAnalytics_unauthorized(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	clientID, trainerID := uuid.New(), uuid.New()

	// Tampered signature.
	q := clientLinkQuery(t, clientID, trainerID, time.Now())
	q.Set("trainer_id", uuid.New().String())
	c, _ := testContext(e, "/client/analytics?"+q.Encode(), uuid.Nil, nil)
	if he, ok := h.GetClientSelfAnalytics(c).(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("tampered: want 401, got %v", he)
	}

	// Expired link (issued an hour ago, TTL is 30 minutes).
	q = clientLinkQuery(t, clientID, trainerID, time.Now().Add(-time.Hour))
	c, _ = testContext(e, "/client/analytics?"+q.Encode(), uuid.Nil, nil)
	if he, ok := h.GetClientSelfAnalytics(c).(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("expired: want 401, got %v", he)
	}

	// exp in the future but signed with another secret.
	exp := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	q = url.Values{}
	q.Set("client_user_id", clientID.String())
	q.Set("trainer_id", trainerID.String())
	q.Set("exp", exp)
	q.Set("sig", fmt.Sprintf("%064x", 0))
	c, _ = testContext(e, "/client/analytics?"+q.Encode(), uuid.Nil, nil)
	if he, ok := h.GetClientSelfAnalytics(c).(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("foreign sig: want 401, got %v", he)
	}
}
```

Хендлер с валидной подписью в юнит-тестах не проверяется: у `testHandlers()` пул `nil`, а путь 200/404 покрыт интеграционным тестом сервиса (Task 3) и Postman (Task 5).

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./internal/analytics/ -run 'TestGetClientSelfAnalytics|TestHandlers_Mount' -v`
Expected: FAIL — `h.GetClientSelfAnalytics undefined`.

- [ ] **Step 3: Реализация**

В `internal/analytics/handlers.go`:

1. В импорты добавить `"time"`.
2. В `Mount` после блока `g := e.Group("/trainer", …)` и его маршрутов добавить:

```go
	// Client self page: access is proven by the signed link, not by a JWT.
	e.GET("/client/analytics", h.GetClientSelfAnalytics)
```

3. После `GetClientAnalytics` добавить:

```go
// GetClientSelfAnalytics serves the page the Telegram bot links to. The four
// query parameters come verbatim from the link; the signature covers all of them.
func (h *Handlers) GetClientSelfAnalytics(c echo.Context) error {
	clientUserID, err := uuid.Parse(c.QueryParam("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid client_user_id")
	}
	trainerID, err := uuid.Parse(c.QueryParam("trainer_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid trainer_id")
	}
	exp, err := strconv.ParseInt(c.QueryParam("exp"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid exp")
	}
	sig := c.QueryParam("sig")
	if sig == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing sig")
	}
	if !VerifyClientAnalyticsLink(h.jwtSecret, clientUserID, trainerID, exp, sig, time.Now()) {
		return echo.NewHTTPError(http.StatusUnauthorized, "link expired or invalid")
	}

	result, err := h.svc.ClientSelfAnalytics(c.Request().Context(), clientUserID, trainerID)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	result.ExpiresAt = time.Unix(exp, 0).UTC()
	return c.JSON(http.StatusOK, result)
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./internal/analytics/ -count=1 -v -run 'TestGetClientSelfAnalytics|TestHandlers_'`
Expected: PASS. `TestHandlers_unauthorized` не трогаем: новый хендлер не требует JWT и в его список не входит.

- [ ] **Step 5: Коммит**

```bash
git add internal/analytics/handlers.go internal/analytics/handlers_test.go
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "feat(analytics): client self analytics endpoint behind signed link"
```

---

### Task 5: Контракт — OpenAPI, apicheck, Postman

**Files:**
- Modify: `api/openapi.yaml` (paths после `/trainer/clients/{client_user_id}/completions`; `components/schemas` после `ClientAnalytics`)
- Modify: `internal/apicheck/schema.go` (`schemaBindings`)
- Modify: `postman/mentorix-backend.postman_collection.json` (папка `Optional (не в walkthrough)`)

**Interfaces:**
- Consumes: Go-типы `ClientSelfAnalytics`, `ClientSelfInfo`, `ClientSelfTrainer` (Task 3), маршрут `/client/analytics` (Task 4).

- [ ] **Step 1: Убедиться, что контракт сейчас красный**

Run: `go test ./internal/apicheck/... -count=1`
Expected: FAIL — `GET /client/analytics` есть в Echo, но отсутствует в OpenAPI и Postman.

- [ ] **Step 2: OpenAPI — путь**

В `api/openapi.yaml` перед строкой `  /trainer/programs/analytics:` вставить:

```yaml
  /client/analytics:
    get:
      tags: [analytics]
      summary: Client self analytics (signed link)
      description: >
        The page a client opens from the Telegram bot button «Статистика».
        No Bearer token: access is proven by the signed query string the bot
        issued (`client_user_id`, `trainer_id`, `exp`, `sig`; HMAC over all
        four, valid 30 minutes). The frontend forwards the link's query string
        verbatim. Data is scoped to one trainer (the client's active trainer in
        the bot) and mirrors the trainer's client analytics plus the latest 30
        completions with trainer replies.
      operationId: getClientSelfAnalytics
      parameters:
        - name: client_user_id
          in: query
          required: true
          schema:
            type: string
            format: uuid
        - name: trainer_id
          in: query
          required: true
          schema:
            type: string
            format: uuid
          description: trainers.id of the client's active trainer
        - name: exp
          in: query
          required: true
          schema:
            type: integer
            format: int64
          description: Unix expiry timestamp from the link
        - name: sig
          in: query
          required: true
          schema:
            type: string
          description: HMAC signature from the link
      responses:
        "200":
          description: Client self analytics
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ClientSelfAnalytics"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          description: Link signature invalid or expired
        "404":
          description: Client is not linked to the trainer or the link is blocked

```

- [ ] **Step 3: OpenAPI — схемы**

В `components/schemas` перед `    AnalyticsClientInfo:` вставить:

```yaml
    ClientSelfAnalytics:
      type: object
      required: [client, trainer, current_assignment, activity, recent_completions, expires_at]
      properties:
        client:
          $ref: "#/components/schemas/ClientSelfInfo"
        trainer:
          $ref: "#/components/schemas/ClientSelfTrainer"
        current_assignment:
          $ref: "#/components/schemas/AnalyticsAssignment"
          nullable: true
        activity:
          $ref: "#/components/schemas/AnalyticsActivity"
        recent_completions:
          type: array
          description: Latest completions with this trainer, newest first, at most 30
          items:
            $ref: "#/components/schemas/ClientCompletionItem"
        expires_at:
          type: string
          format: date-time
          description: When the signed link stops working

    ClientSelfInfo:
      type: object
      required: [client_user_id, display_name]
      properties:
        client_user_id:
          type: string
          format: uuid
        display_name:
          type: string

    ClientSelfTrainer:
      type: object
      required: [trainer_id, display_name]
      properties:
        trainer_id:
          type: string
          format: uuid
          description: trainers.id
        display_name:
          type: string

```

- [ ] **Step 4: apicheck**

В `internal/apicheck/schema.go` в `schemaBindings()` после строки с `"AnalyticsClientInfo"` добавить:

```go
		{name: "ClientSelfAnalytics", typ: reflect.TypeOf(analytics.ClientSelfAnalytics{})},
		{name: "ClientSelfInfo", typ: reflect.TypeOf(analytics.ClientSelfInfo{})},
		{name: "ClientSelfTrainer", typ: reflect.TypeOf(analytics.ClientSelfTrainer{})},
```

- [ ] **Step 5: Postman**

В `postman/mentorix-backend.postman_collection.json` в папке `"Optional (не в walkthrough)"` сразу после элемента `"GET /trainer/clients/:client_user_id/avatar"` добавить элемент (соблюдая запятые JSON):

```json
        {
          "name": "GET /client/analytics",
          "request": {
            "method": "GET",
            "header": [],
            "url": "{{base_url}}/client/analytics?client_user_id={{client_user_id}}&trainer_id={{trainer_id}}&exp={{client_analytics_exp}}&sig={{client_analytics_sig}}",
            "description": "Client self page opened from the Telegram bot. No Bearer — the four query params come from the signed link the bot sends (valid 30 minutes)."
          },
          "response": []
        }
```

- [ ] **Step 6: Прогнать контракт**

Run: `SKIP_SMOKE=1 ./postman/validate.sh`
Expected: `=== Contract alignment …` → `ok`, в конце `OK`. Если apicheck ругается на порядок/имена схем — поправить YAML, не тест.

- [ ] **Step 7: Коммит**

```bash
git add api/openapi.yaml internal/apicheck/schema.go postman/mentorix-backend.postman_collection.json
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "docs(api): describe GET /client/analytics in OpenAPI and Postman"
```

---

### Task 6: Бот — кнопка «📊 Статистика»

**Files:**
- Modify: `internal/telegrambot/keyboard.go`
- Modify: `internal/telegrambot/bot.go` (интерфейсы, `Bot`, `New`, `handleMessage`, новый хендлер, `helpText`)
- Modify: `internal/telegrambot/workout.go` (только замена `mainMenuKeyboard()` → `b.menu`)
- Modify: `internal/telegrambot/format.go` (`menuErrorText`, новая `formatStatsMessage`)
- Test: `internal/telegrambot/bot_test.go`

**Interfaces:**
- Consumes: `trainerclient.Service.GetTelegramActiveTrainer(ctx, telegramUserID string) (*trainerclient.ActiveTrainerResponse, error)` и `ClientUserIDByTelegram` (оба уже существуют); `trainerclient.ErrActiveTrainerNotSet`, `trainerclient.ErrTelegramUserNotFound`.
- Produces:
  - `type ClientAnalyticsLinker interface { BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string }` — ему удовлетворяет `*analytics.ClientLinkBuilder` (Task 2).
  - `func WithClientAnalyticsLink(l ClientAnalyticsLinker) BotOption`
  - `const btnStats = "📊 Статистика"`
  - `func mainMenuKeyboard(withStats bool) tgbotapi.ReplyKeyboardMarkup`

- [ ] **Step 1: Написать падающие тесты**

В `internal/telegrambot/bot_test.go`:

1. Заменить `TestMainMenuKeyboard_buttons` на:

```go
func TestMainMenuKeyboard_buttons(t *testing.T) {
	kb := mainMenuKeyboard(false)
	if len(kb.Keyboard) != 2 {
		t.Fatalf("rows = %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 2 || len(kb.Keyboard[1]) != 1 {
		t.Fatalf("unexpected layout: %+v", kb.Keyboard)
	}

	withStats := mainMenuKeyboard(true)
	if len(withStats.Keyboard) != 2 || len(withStats.Keyboard[1]) != 2 {
		t.Fatalf("unexpected layout with stats: %+v", withStats.Keyboard)
	}
	if withStats.Keyboard[1][1].Text != btnStats {
		t.Fatalf("second row = %+v", withStats.Keyboard[1])
	}
}
```

2. В `fakeTrainerClient` добавить поля и метод:

```go
	activeTrainer    *trainerclient.ActiveTrainerResponse
	activeTrainerErr error
```

```go
func (f *fakeTrainerClient) GetTelegramActiveTrainer(context.Context, string) (*trainerclient.ActiveTrainerResponse, error) {
	if f.activeTrainerErr != nil {
		return nil, f.activeTrainerErr
	}
	if f.activeTrainer != nil {
		return f.activeTrainer, nil
	}
	return &trainerclient.ActiveTrainerResponse{
		TrainerID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		DisplayName: "Anna",
	}, nil
}
```

3. В конец файла добавить:

```go
type fakeLinker struct {
	link  string
	calls [][2]uuid.UUID
}

func (f *fakeLinker) BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string {
	f.calls = append(f.calls, [2]uuid.UUID{clientUserID, trainerID})
	return f.link
}

func statsMessage() *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 42},
		Text: btnStats,
	}
}

func TestBot_handleStats_sendsLink(t *testing.T) {
	api := &fakeTelegramAPI{}
	linker := &fakeLinker{link: "https://app.example.com/stats?client_user_id=x&trainer_id=y&exp=1&sig=z"}
	clientID := uuid.New()
	bot := New(api, &fakeTrainerClient{clientUserID: clientID}, WithClientAnalyticsLink(linker))

	bot.handleMessage(context.Background(), statsMessage())

	if len(api.sent) != 1 {
		t.Fatalf("sent = %+v", api.sent)
	}
	msg := api.sent[0]
	if !strings.Contains(msg.Text, "Anna") || !strings.Contains(msg.Text, "30 минут") {
		t.Fatalf("text = %q", msg.Text)
	}
	if strings.Contains(msg.Text, linker.link) {
		t.Fatalf("link must live in the button, not in the text: %q", msg.Text)
	}
	inline, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok || len(inline.InlineKeyboard) != 1 || len(inline.InlineKeyboard[0]) != 1 {
		t.Fatalf("reply markup = %#v", msg.ReplyMarkup)
	}
	btn := inline.InlineKeyboard[0][0]
	if btn.URL == nil || *btn.URL != linker.link {
		t.Fatalf("button = %+v", btn)
	}
	if len(linker.calls) != 1 || linker.calls[0][0] != clientID || linker.calls[0][1] != uuid.MustParse("22222222-2222-2222-2222-222222222222") {
		t.Fatalf("linker calls = %+v", linker.calls)
	}
}

func TestBot_handleStats_noActiveTrainer(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{activeTrainerErr: trainerclient.ErrActiveTrainerNotSet},
		WithClientAnalyticsLink(&fakeLinker{link: "https://app.example.com/stats?x=1"}))

	bot.handleMessage(context.Background(), statsMessage())

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "Выберите тренера") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleStats_unknownUser(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{clientUserEr: trainerclient.ErrTelegramUserNotFound},
		WithClientAnalyticsLink(&fakeLinker{link: "https://app.example.com/stats?x=1"}))

	bot.handleMessage(context.Background(), statsMessage())

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "нет тренеров") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleStats_disabled(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})

	bot.handleMessage(context.Background(), statsMessage())

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "недоступна") {
		t.Fatalf("sent = %+v", api.sent)
	}
	if _, ok := api.sent[0].ReplyMarkup.(tgbotapi.InlineKeyboardMarkup); ok {
		t.Fatal("disabled feature must not send an inline button")
	}
}

func TestBot_menu_hidesStatsWhenDisabled(t *testing.T) {
	enabled := New(&fakeTelegramAPI{}, &fakeTrainerClient{}, WithClientAnalyticsLink(&fakeLinker{}))
	if len(enabled.menu.Keyboard[1]) != 2 {
		t.Fatalf("enabled menu = %+v", enabled.menu.Keyboard)
	}
	disabled := New(&fakeTelegramAPI{}, &fakeTrainerClient{})
	if len(disabled.menu.Keyboard[1]) != 1 {
		t.Fatalf("disabled menu = %+v", disabled.menu.Keyboard)
	}
}

func TestHelpText_mentionsStatsOnlyWhenEnabled(t *testing.T) {
	if !strings.Contains(helpText(true), btnStats) {
		t.Fatal("enabled help must mention stats button")
	}
	if strings.Contains(helpText(false), btnStats) {
		t.Fatal("disabled help must not mention stats button")
	}
}
```

4. Заменить существующий `TestHelpText` на:

```go
func TestHelpText(t *testing.T) {
	if helpText(false) == "" {
		t.Fatal("expected help text")
	}
}
```

- [ ] **Step 2: Убедиться, что тесты не компилируются**

Run: `go test ./internal/telegrambot/ -count=1`
Expected: FAIL — `undefined: btnStats`, `WithClientAnalyticsLink`, аргументы `mainMenuKeyboard`.

- [ ] **Step 3: Клавиатура**

Заменить содержимое `internal/telegrambot/keyboard.go` на:

```go
package telegrambot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	btnProgram  = "📅 Программа"
	btnTrainers = "👤 Тренеры"
	btnHelp     = "❓ Помощь"
	btnStats    = "📊 Статистика"
)

// mainMenuKeyboard builds the reply keyboard; the stats button is shown only
// when the client analytics page is configured.
func mainMenuKeyboard(withStats bool) tgbotapi.ReplyKeyboardMarkup {
	bottom := []tgbotapi.KeyboardButton{tgbotapi.NewKeyboardButton(btnHelp)}
	if withStats {
		bottom = append(bottom, tgbotapi.NewKeyboardButton(btnStats))
	}
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnProgram),
			tgbotapi.NewKeyboardButton(btnTrainers),
		),
		tgbotapi.NewKeyboardButtonRow(bottom...),
	)
}
```

- [ ] **Step 4: Bot — интерфейсы, поле меню, опция**

В `internal/telegrambot/bot.go`:

1. В интерфейс `trainerClient` добавить строку:

```go
	GetTelegramActiveTrainer(ctx context.Context, telegramUserID string) (*trainerclient.ActiveTrainerResponse, error)
```

2. После `telegramAPI` добавить:

```go
// ClientAnalyticsLinker issues signed links to the client analytics page.
// Implemented by analytics.ClientLinkBuilder; nil means the feature is off.
type ClientAnalyticsLinker interface {
	BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string
}
```

3. В `Bot` добавить поля:

```go
	links    ClientAnalyticsLinker
	menu     tgbotapi.ReplyKeyboardMarkup
```

4. После `WithWorkoutCompletions` добавить:

```go
// WithClientAnalyticsLink enables the «Статистика» button.
func WithClientAnalyticsLink(l ClientAnalyticsLinker) BotOption {
	return func(b *Bot) {
		b.links = l
	}
}
```

5. В `New` после цикла по `opts` добавить `b.menu = mainMenuKeyboard(b.links != nil)`:

```go
func New(api telegramAPI, clients trainerClient, opts ...BotOption) *Bot {
	b := &Bot{api: api, clients: clients}
	for _, opt := range opts {
		opt(b)
	}
	b.menu = mainMenuKeyboard(b.links != nil)
	return b
}
```

- [ ] **Step 5: Bot — заменить все `mainMenuKeyboard()` на `b.menu`**

```bash
sed -i '' 's/mainMenuKeyboard()/b.menu/g' internal/telegrambot/bot.go internal/telegrambot/workout.go
grep -n "mainMenuKeyboard()" internal/telegrambot/*.go
```

Expected: grep ничего не находит вне тестов. Убедиться, что в `keyboard.go` определение функции не задето (там `mainMenuKeyboard(withStats bool)`, sed его не тронет).

- [ ] **Step 6: Bot — обработчик и help**

В `internal/telegrambot/bot.go`:

1. В `handleMessage`:
   - в `case "menu", "help":` заменить `helpText()` на `helpText(b.links != nil)`;
   - в первом `switch strings.TrimSpace(msg.Text)` расширить `case btnProgram, btnTrainers, btnHelp:` до `case btnProgram, btnTrainers, btnHelp, btnStats:`;
   - во втором `switch` перед `case btnHelp:` добавить:

```go
	case btnStats:
		b.handleStats(ctx, msg.Chat.ID, tgID)
		return
```

   - в `case btnHelp:` заменить `helpText()` на `helpText(b.links != nil)`.

2. После `handleTrainers` добавить:

```go
// handleStats sends a signed link to the client's analytics page for the
// active trainer. The link itself sits in an inline URL button so it does not
// show up as text in the chat history.
func (b *Bot) handleStats(ctx context.Context, chatID int64, telegramUserID string) {
	if b.links == nil {
		b.sendText(chatID, formatStatsUnavailable(), b.menu)
		return
	}
	clientUserID, err := b.clients.ClientUserIDByTelegram(ctx, telegramUserID)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), b.menu)
		return
	}
	trainer, err := b.clients.GetTelegramActiveTrainer(ctx, telegramUserID)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), b.menu)
		return
	}
	link := b.links.BuildClientAnalyticsLink(clientUserID, trainer.TrainerID)
	if link == "" {
		b.sendText(chatID, formatStatsUnavailable(), b.menu)
		return
	}
	inline := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonURL("Открыть статистику", link),
	))
	b.sendTextWithInline(chatID, formatStatsMessage(trainer.DisplayName), b.menu, inline)
}
```

3. После `sendMarkdownWithInline` добавить плоский вариант без Markdown (имя тренера — произвольный текст, экранировать нечего):

```go
func (b *Bot) sendTextWithInline(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup, inline tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = inline
	if _, err := b.api.Send(msg); err != nil {
		b.sendText(chatID, text, keyboard)
	}
}
```

4. Заменить `helpText` на:

```go
func helpText(withStats bool) string {
	text := "Mentorix — программы тренировок от вашего тренера.\n\n" +
		"📅 Программа — выбор недели и дня\n" +
		"👤 Тренеры — ваши тренеры\n"
	if withStats {
		text += "📊 Статистика — ссылка на страницу с вашей статистикой\n"
	}
	return text + "\nКоманды: /menu — показать меню"
}
```

- [ ] **Step 7: format.go — тексты и новая ветка ошибки**

В `internal/telegrambot/format.go`:

1. Заменить `menuErrorText` на:

```go
func menuErrorText(err error) string {
	switch {
	case errors.Is(err, trainerclient.ErrActiveTrainerNotSet):
		return "Выберите тренера в разделе «Тренеры»."
	case errors.Is(err, trainerclient.ErrTelegramUserNotFound):
		return "У вас пока нет тренеров. Откройте ссылку-приглашение от тренера."
	}
	return "Не удалось загрузить данные. Попробуйте позже."
}
```

2. В конец файла добавить:

```go
func formatStatsMessage(trainerDisplayName string) string {
	return fmt.Sprintf("📊 Ваша статистика у тренера %s\n\nСсылка действует 30 минут.", trainerDisplayName)
}

func formatStatsUnavailable() string {
	return "Статистика пока недоступна."
}
```

Проверить, что `fmt` и `errors` уже импортированы в `format.go` (да: `fmt.Fprintf`, `errors.Is` там есть).

- [ ] **Step 8: Прогнать тесты пакета**

Run: `go test ./internal/telegrambot/ -count=1 -v -run 'TestBot_handleStats|TestBot_menu|TestHelpText|TestMainMenuKeyboard|TestBot_handleMessage'`
Expected: PASS. Затем `go test ./internal/telegrambot/ -count=1` — PASS целиком (проверка, что замена `b.menu` в `workout.go` ничего не сломала).

- [ ] **Step 9: gofmt, vet, lint**

Run: `gofmt -l ./internal/telegrambot && go vet ./internal/telegrambot/ && golangci-lint run ./internal/telegrambot/...`
Expected: чисто.

- [ ] **Step 10: Коммит**

```bash
git add internal/telegrambot/
PRE_COMMIT_CHECK_FLAGS=--no-coverage git commit -m "feat(telegrambot): stats button with signed link to client analytics page"
```

---

### Task 7: Wiring, env, документация

**Files:**
- Modify: `cmd/api/main.go` (создание бота)
- Modify: `render.yaml` (envVars)
- Modify: `.env.example` (новая переменная; файл читать через `Read`, если Bash его не отдаёт)
- Create: `docs/features/client-analytics.md`
- Modify: `docs/README.md`, `docs/status.md`, `docs/features/telegram-bot.md`, `docs/features/trainer-analytics.md`, `docs/environments.md`

**Interfaces:**
- Consumes: `cfg.ClientAnalyticsPageURL` (Task 1), `analytics.NewClientLinkBuilder` (Task 2), `telegrambot.WithClientAnalyticsLink` (Task 6).

**Обозначение в шаблонах ниже:** `LINK(text|path)` — обычная markdown-ссылка:
текст в квадратных скобках, затем путь в круглых. В плане она записана так,
чтобы `docs-check` не проверял ссылки из шаблонов относительно самого плана.
При записи файлов заменить на настоящую ссылку.

- [ ] **Step 1: Wiring**

В `cmd/api/main.go` заменить создание бота:

```go
		if cfg.BotToken != "" && cfg.BotWebhookURL != "" {
			botOpts := []telegrambot.BotOption{
				telegrambot.WithWorkoutCompletions(workoutSvc, workoutPending),
			}
			if cfg.ClientAnalyticsPageURL != "" {
				botOpts = append(botOpts, telegrambot.WithClientAnalyticsLink(
					analytics.NewClientLinkBuilder(cfg.ClientAnalyticsPageURL, cfg.JWTSecret),
				))
			} else {
				logger.Info("client analytics page not configured; stats button disabled")
			}
			tgBot, err := telegrambot.NewFromToken(cfg.BotToken, trainerClientSvc, botOpts...)
			if err != nil {
				logger.Error("telegram webhook bot init failed", "error", err)
				os.Exit(1)
			}
```

Остальная часть блока (регистрация webhook) без изменений.

- [ ] **Step 2: Сборка**

Run: `go build ./... && go vet ./cmd/...`
Expected: без ошибок.

- [ ] **Step 3: Env**

1. В `render.yaml` после элемента `TRAINER_INVITE_TTL_DAYS` добавить:

```yaml
      - key: CLIENT_ANALYTICS_PAGE_URL
        sync: false
```

2. В `.env.example` рядом с блоком `BOT_*` добавить:

```dotenv
# Frontend page for client self-analytics (Telegram bot button). Empty = button hidden.
CLIENT_ANALYTICS_PAGE_URL=
```

3. В `docs/environments.md` строку `Dashboard \`sync: false\`` дополнить: `…, \`TRAINER_INVITE_TTL_DAYS\`, \`CLIENT_ANALYTICS_PAGE_URL\``.

- [ ] **Step 4: Новый feature doc**

Создать `docs/features/client-analytics.md`:

```markdown
# Client analytics page

**Статус:** реализовано  
**Код:** `internal/analytics/client_link.go`, `internal/analytics/` (`ClientSelfAnalytics`), `internal/telegrambot/` (кнопка «📊 Статистика»)

## Назначение

Клиент из Telegram-бота открывает в браузере страницу со своей статистикой у активного тренера. Бот выдаёт подписанную ссылку, фронт пересылает её query-строку на `GET /client/analytics` и рендерит ответ. Входа в веб у клиента нет: ссылка сама является пропуском на 30 минут.

## Безопасность

- Ссылка: `<CLIENT_ANALYTICS_PAGE_URL>?client_user_id&trainer_id&exp&sig`; `sig` — HMAC-SHA256 на `JWT_SECRET` с доменом `client_analytics:` (отдельно от аватар-ссылок). Подделать или подменить `client_user_id` / `trainer_id` нельзя.
- TTL 30 минут; отзыва конкретной ссылки нет. Пока ссылка жива, её можно переслать — принято осознанно, страница только на чтение.
- Связь `trainer_clients` и статус `blocked` проверяются при каждом запросе, не при выдаче.
- В сообщении бота ссылка спрятана в inline URL-кнопку.

## API

Контракт: `api/openapi.yaml` — `GET /client/analytics`. Без JWT. `400` — параметр отсутствует/не парсится, `401` — подпись или срок, `404` — клиент не связан с тренером или заблокирован.

Ответ: `client` (id, имя), `trainer` (`trainers.id`, имя), `current_assignment` и `activity` — те же схемы, что в тренерской аналитике (LINK(trainer-analytics.md|trainer-analytics.md)), `recent_completions` — последние 30 тренировок с ответами тренера, `expires_at` — срок ссылки.

## Бот

Кнопка «📊 Статистика» есть только при заданном `CLIENT_ANALYTICS_PAGE_URL`. Тренер — активный, как для «Программа»; нет активного → «Выберите тренера в разделе «Тренеры»». Подробности меню — LINK(telegram-bot.md|telegram-bot.md).

## Env

| Переменная | Обязательна | Назначение |
| ---------- | ----------- | ---------- |
| `CLIENT_ANALYTICS_PAGE_URL` | нет | Абсолютный `http(s)` URL страницы фронта без query; пусто — кнопки нет |

Origin страницы должен быть в `CORS_ALLOW_ORIGINS`.

## См. также

- LINK(trainer-analytics.md|trainer-analytics.md) — те же расчёты со стороны тренера
- LINK(telegram-bot.md|telegram-bot.md) — меню бота
```

- [ ] **Step 5: Ссылки и строки в существующих доках**

1. `docs/README.md` — в таблицу «Фичи» после строки `trainer-analytics.md` добавить:

```markdown
| LINK(features/client-analytics.md|features/client-analytics.md) | Страница статистики клиента по ссылке из Telegram |
```

2. `docs/status.md` — после строки «Аналитика тренера …» добавить:

```markdown
| Статистика клиента по подписанной ссылке из Telegram (`GET /client/analytics`, кнопка «📊 Статистика») | LINK(features/client-analytics.md|features/client-analytics.md) |
```

3. `docs/features/telegram-bot.md`:
   - в «Решения» строку `Меню: reply keyboard (…)` дополнить: `+ «Статистика» при заданном CLIENT_ANALYTICS_PAGE_URL`;
   - в таблицу «Меню» добавить строку `| Статистика | inline URL-кнопка на страницу статистики у активного тренера, ссылка живёт 30 минут — LINK(client-analytics.md|client-analytics.md) |`;
   - в таблицу «Env» добавить `| \`CLIENT_ANALYTICS_PAGE_URL\` | нет | кнопка «Статистика»; пусто — кнопки нет |`.

4. `docs/features/trainer-analytics.md` — в «См. также» добавить `- LINK(client-analytics.md|client-analytics.md) — та же сводка глазами клиента`.

- [ ] **Step 6: docs-check и полный CI-прогон**

Run: `./scripts/docs-check.sh`
Expected: `docs-check passed.`

Run: `make check-ci`
Expected: все шаги зелёные (gofmt, vet, unit, build, sqlc drift, lint, contract, docs-check, integration + coverage). Если интеграция не может подключиться к БД — зафиксировать в отчёте, какие шаги прошли, а какие нет.

- [ ] **Step 7: Коммит**

```bash
git add cmd/api/main.go render.yaml .env.example docs/
git commit -m "feat(telegrambot): wire client analytics link and document the feature"
```

- [ ] **Step 8: Финал**

Ветка готова к PR в `develop`. Push и PR — только по запросу владельца:

```bash
git push -u origin feature/client-analytics-link
gh pr create --base develop --title "feat: client analytics page via signed link from Telegram" --body-file <(cat <<'EOF'
## What
- `GET /client/analytics` — client self analytics behind an HMAC-signed, 30-minute link (no JWT).
- Telegram bot: «📊 Статистика» button sends the link as an inline URL button; shown only when `CLIENT_ANALYTICS_PAGE_URL` is set.
- Trainer client analytics core extracted to a shared function keyed by `trainers.id`.

## Why
Clients asked for a browser page with their progress without a separate web login. Spec: docs/superpowers/specs/2026-09-16-client-analytics-link-design.md

## Verification
- `make check-ci` (unit, lint, contract, docs-check, integration + coverage).
- New tests: link signing/verify/tamper, handler 400/401, bot button/menu/help, service integration incl. blocked link.
EOF
)
```
