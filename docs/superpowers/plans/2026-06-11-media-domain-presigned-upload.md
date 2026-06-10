# Media-домен: presigned upload + хранение полного URL — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать media-домен generic-сервисом прямой загрузки в S3 (presigned POST с лимитом размера), а donate/settlement/news — хранить полный URL картинки с allowlist-валидацией (наш CDN + внешние хосты вроде imgur).

**Architecture:** Клиент получает в media-домене presigned POST (S3 энфорсит размер/тип), льёт файл напрямую, передаёт `public_url` в целевой домен; домен валидирует хост через общий `mediaurl.Validator` и сохраняет полный URL. Владение ресурсом проверяет целевой домен, media остаётся тупым. Бакеты — публичные, бутстрапятся в media.

**Tech Stack:** Go, gRPC + grpc-gateway, protobuf/buf, Uber fx, MongoDB (mongox), MinIO (minio-go v7), zap.

**Spec:** `docs/superpowers/specs/2026-06-11-media-domain-presigned-upload-design.md`

---

## File Structure

**Создаётся:**
- `internal/pkg/mediaurl/mediaurl.go` — `Validator` (allowlist хостов) + `Validate`.
- `internal/pkg/mediaurl/mediaurl_test.go` — тесты валидатора.
- `internal/media/internal/service/service_test.go` — тесты helper'ов media.

**Меняется:**
- `internal/pkg/config/config.go` — поле `MediaAllowedHosts`.
- `internal/pkg/storage/storage.go` — метод `PresignedPostObject`.
- `main.go` — провайдер `mediaurl.New`.
- `proto/media/v1/media.proto` + `internal/media/internal/service/{app,service,scope}.go` — purposes, POST-форма, per-purpose scope.
- `internal/media/fx.go` — бутстрап бакетов.
- `proto/settlement/v1/settlement.proto` + `internal/settlement/internal/service/{service,app}.go` — `data`→`url`.
- `proto/news/v1/news.proto` + `internal/news/internal/service/{service,app}.go` — `preview` bytes→string.
- `internal/donate/internal/service/{validate,app}.go` — allowlist-валидация.
- `internal/donate/fx.go`, `internal/settlement/fx.go`, `internal/news/fx.go` — удаление per-domain bucket bootstrap.

**Замечание про мёртвые зависимости:** после миграции поля `storage`/`cfg` в settlement/news и `storage` в donate перестают использоваться в хендлерах. Их fx-проводку НЕ удаляем в этой итерации (неиспользуемое поле структуры компилируется; удаление провайдеров — отдельная зачистка вне объёма). Удаляем только неиспользуемые **импорты** (иначе не соберётся).

---

### Task 1: Config — поле MediaAllowedHosts

**Files:**
- Modify: `internal/pkg/config/config.go:29`

- [ ] **Step 1: Добавить поле**

В `internal/pkg/config/config.go` после строки `CdnUrl string \`envconfig:"CDN_URL"\`` добавить:

```go
	CdnUrl string `envconfig:"CDN_URL"`

	// MediaAllowedHosts — хосты внешних ресурсов (помимо CDN), с которых
	// разрешено хранить ссылки на картинки (например i.imgur.com).
	MediaAllowedHosts []string `envconfig:"MEDIA_ALLOWED_HOSTS"`
```

- [ ] **Step 2: Сборка**

Run: `go build ./internal/pkg/config/...`
Expected: без ошибок.

- [ ] **Step 3: Commit**

```bash
git add internal/pkg/config/config.go
git commit -m "feat(config): add MediaAllowedHosts allowlist"
```

---

### Task 2: Пакет mediaurl — валидатор URL

**Files:**
- Create: `internal/pkg/mediaurl/mediaurl.go`
- Test: `internal/pkg/mediaurl/mediaurl_test.go`

- [ ] **Step 1: Написать падающий тест**

Создать `internal/pkg/mediaurl/mediaurl_test.go`:

```go
package mediaurl_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
)

func newValidator() *mediaurl.Validator {
	return mediaurl.New(config.Config{
		CdnUrl:            "https://cdn.test",
		MediaAllowedHosts: []string{"i.imgur.com"},
	})
}

func TestValidate(t *testing.T) {
	v := newValidator()
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"cdn host https", "https://cdn.test/donate-shop/a.webp", false},
		{"cdn host http", "http://cdn.test/donate-shop/a.webp", false},
		{"allowlisted host", "https://i.imgur.com/abc.png", false},
		{"disallowed host", "https://evil.example/x.png", true},
		{"bad scheme", "ftp://cdn.test/x", true},
		{"garbage", "not a url", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Validate(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.url, err)
			}
		})
	}
}
```

- [ ] **Step 2: Запустить тест — должен не собраться/упасть**

Run: `go test ./internal/pkg/mediaurl/...`
Expected: FAIL — `undefined: mediaurl.New` / `mediaurl.Validator`.

- [ ] **Step 3: Реализация**

Создать `internal/pkg/mediaurl/mediaurl.go`:

```go
package mediaurl

import (
	"errors"
	"net/url"
	"strings"

	"github.com/lasthearth/vsservice/internal/pkg/config"
)

// ErrInvalidURL — URL пуст, кривой, не http(s) или его хост не в allowlist.
var ErrInvalidURL = errors.New("media url is not allowed")

// Validator проверяет, что URL картинки ведёт на наш CDN или на разрешённый
// внешний хост.
type Validator struct {
	allowedHosts map[string]struct{}
}

// New собирает Validator из хоста CDN (из cfg.CdnUrl) и cfg.MediaAllowedHosts.
func New(cfg config.Config) *Validator {
	hosts := make(map[string]struct{})
	if u, err := url.Parse(cfg.CdnUrl); err == nil && u.Host != "" {
		hosts[u.Host] = struct{}{}
	}
	for _, h := range cfg.MediaAllowedHosts {
		if h = strings.TrimSpace(h); h != "" {
			hosts[h] = struct{}{}
		}
	}
	return &Validator{allowedHosts: hosts}
}

// Validate пропускает http(s) URL, чей хост — CDN или из allowlist.
func (v *Validator) Validate(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	if _, ok := v.allowedHosts[u.Host]; !ok {
		return ErrInvalidURL
	}
	return nil
}
```

- [ ] **Step 4: Запустить тест — должен пройти**

Run: `go test ./internal/pkg/mediaurl/...`
Expected: PASS (ok).

- [ ] **Step 5: Commit**

```bash
git add internal/pkg/mediaurl/
git commit -m "feat(mediaurl): add allowlist-based media URL validator"
```

---

### Task 3: Провайдер mediaurl в main.go

**Files:**
- Modify: `main.go:42-73`

- [ ] **Step 1: Импорт**

В `main.go` в блок импортов добавить:

```go
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
```

- [ ] **Step 2: Провайдер**

В `main.go` внутри `fx.Provide(...)` (после `config.New,` на строке ~43) добавить строку:

```go
			config.New,
			mediaurl.New,
```

- [ ] **Step 3: Сборка**

Run: `go build ./...`
Expected: без ошибок.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat(media): provide mediaurl validator via fx"
```

---

### Task 4: storage.PresignedPostObject

**Files:**
- Modify: `internal/pkg/storage/storage.go`

- [ ] **Step 1: Добавить метод**

В конец `internal/pkg/storage/storage.go` добавить (импорты `context`, `time`, `minio` уже есть):

```go
// PresignedPostObject возвращает presigned POST URL и поля формы для прямой
// загрузки в S3. Размер ограничивается через POST-policy (S3 отклоняет
// превышение). Если contentType пуст — разрешается любой image/*.
func (s *Storage) PresignedPostObject(
	ctx context.Context,
	bucketName, objectName string,
	expiry time.Duration,
	maxSize int64,
	contentType string,
) (string, map[string]string, error) {
	policy := minio.NewPostPolicy()
	if err := policy.SetBucket(bucketName); err != nil {
		return "", nil, err
	}
	if err := policy.SetKey(objectName); err != nil {
		return "", nil, err
	}
	if err := policy.SetExpires(time.Now().UTC().Add(expiry)); err != nil {
		return "", nil, err
	}
	if err := policy.SetContentLengthRange(1, maxSize); err != nil {
		return "", nil, err
	}
	if contentType != "" {
		if err := policy.SetContentType(contentType); err != nil {
			return "", nil, err
		}
	} else if err := policy.SetContentTypeStartsWith("image/"); err != nil {
		return "", nil, err
	}

	u, formData, err := s.client.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return "", nil, err
	}
	return u.String(), formData, nil
}
```

> Нет юнит-теста: `PresignedPostPolicy` требует живой MinIO. Покрывается сборкой + ручной проверкой.

- [ ] **Step 2: Сборка**

Run: `go build ./internal/pkg/storage/...`
Expected: без ошибок.

- [ ] **Step 3: Commit**

```bash
git add internal/pkg/storage/storage.go
git commit -m "feat(storage): add PresignedPostObject with content-length-range"
```

---

### Task 5: Media-сервис — proto, codegen, handler

Этот таск атомарен: после `buf generate` старый handler не соберётся, поэтому proto + gen + переписанный сервис коммитятся вместе.

**Files:**
- Modify: `proto/media/v1/media.proto`
- Modify: `internal/media/internal/service/app.go`
- Modify: `internal/media/internal/service/service.go`
- Modify: `internal/media/internal/service/scope.go`
- Test: `internal/media/internal/service/service_test.go`

- [ ] **Step 1: Proto — purposes, content_type, POST-форма**

В `proto/media/v1/media.proto` заменить enum, request и `UploadTarget`:

```proto
enum UploadPurpose {
  UPLOAD_PURPOSE_UNSPECIFIED = 0;
  UPLOAD_PURPOSE_DONATE_SHOP = 1;
  UPLOAD_PURPOSE_SETTLEMENT = 2;
  UPLOAD_PURPOSE_NEWS = 3;
}

message CreateUploadUrlsRequest {
  UploadPurpose purpose = 1;
  int32 count = 2;
  // Опционально. MIME загружаемого файла; должен входить в список, разрешённый
  // для purpose. Пусто → разрешается любой image/*, объект получает .webp.
  string content_type = 3;
}

message UploadTarget {
  // POST-эндпоинт: клиент шлёт multipart/form-data (fields + файл).
  string post_url = 1;
  // Поля формы, которые надо отправить вместе с файлом (policy, signature, key…).
  map<string, string> fields = 2;
  // Публичный URL объекта после загрузки — клиент передаёт его в целевой домен.
  string public_url = 3;
}
```

- [ ] **Step 2: Codegen**

Run: `buf generate`
Expected: обновляются `gen/media/v1/*.pb.go` (поля `PostUrl`, `Fields`, `PublicUrl`, `ContentType`; константы `UploadPurpose_UPLOAD_PURPOSE_SETTLEMENT`, `..._NEWS`).

- [ ] **Step 3: app.go — purposeConfig, helper'ы, Storage, checkScope**

Заменить весь `internal/media/internal/service/app.go` на:

```go
package service

import (
	"context"
	"slices"
	"strings"
	"time"

	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ mediav1.MediaServiceServer = (*Service)(nil)

const presignExpiry = 15 * time.Minute

// purposeConfig описывает, куда и с какими ограничениями грузится purpose.
type purposeConfig struct {
	bucket       string
	maxSize      int64    // лимит размера, зашивается в POST-policy
	contentTypes []string // допустимые MIME (если запрос задал content_type)
	scope        string   // требуемый JWT scope; "" = любой аутентифицированный
}

// purpose → конфиг. Имена бакетов совпадают с существующими публичными
// бакетами доменов, чтобы не осиротить уже сохранённые URL.
var purposes = map[mediav1.UploadPurpose]purposeConfig{
	mediav1.UploadPurpose_UPLOAD_PURPOSE_DONATE_SHOP: {
		bucket: "donate-shop", maxSize: 2 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "donate:shop:create",
	},
	mediav1.UploadPurpose_UPLOAD_PURPOSE_SETTLEMENT: {
		bucket: "settlementsreq", maxSize: 5 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "",
	},
	mediav1.UploadPurpose_UPLOAD_PURPOSE_NEWS: {
		bucket: "news", maxSize: 5 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "news:create",
	},
}

// extFromContentType возвращает расширение объекта по MIME (default .webp).
func extFromContentType(ct string) string {
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ".webp"
	}
}

// checkScope проверяет JWT scope для purpose, требующих его.
func (s *Service) checkScope(ctx context.Context, cfg purposeConfig) error {
	if cfg.scope == "" {
		return nil
	}
	claims, err := interceptor.GetClaims(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, "missing claims")
	}
	if !slices.Contains(strings.Fields(claims.Scope), cfg.scope) {
		return status.Error(codes.PermissionDenied, "no permission for this upload purpose")
	}
	return nil
}

// Storage — подмножество pkg/storage.Storage, нужное media-сервису.
type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	CreateBucket(ctx context.Context, bucketName string) error
	MakeBucketPublic(ctx context.Context, bucketName string) error
	PresignedPostObject(
		ctx context.Context,
		bucketName, objectName string,
		expiry time.Duration,
		maxSize int64,
		contentType string,
	) (string, map[string]string, error)
}

var _ Storage = (*storage.Storage)(nil)

type Service struct {
	storage Storage
	cfg     config.Config
	log     logger.Logger
}

type Opts struct {
	fx.In
	Storage Storage
	Config  config.Config
	Logger  logger.Logger
}

func New(opts Opts) *Service {
	return &Service{storage: opts.Storage, cfg: opts.Config, log: opts.Logger}
}
```

- [ ] **Step 4: service.go — handler на presigned POST**

Заменить весь `internal/media/internal/service/service.go` на:

```go
package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) CreateUploadUrls(ctx context.Context, req *mediav1.CreateUploadUrlsRequest) (*mediav1.CreateUploadUrlsResponse, error) {
	l := s.log.With(zap.String("method", "CreateUploadUrls"))

	if req.Count < 1 || req.Count > 20 {
		return nil, status.Error(codes.InvalidArgument, "count must be between 1 and 20")
	}

	cfg, ok := purposes[req.Purpose]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown upload purpose")
	}

	if err := s.checkScope(ctx, cfg); err != nil {
		return nil, err
	}

	contentType := req.ContentType
	if contentType != "" && !slices.Contains(cfg.contentTypes, contentType) {
		return nil, status.Error(codes.InvalidArgument, "unsupported content_type")
	}
	ext := extFromContentType(contentType)

	targets := make([]*mediav1.UploadTarget, 0, req.Count)
	for i := int32(0); i < req.Count; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			l.Error("failed to generate uuid", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to generate object name")
		}
		objectName := id.String() + ext

		postURL, fields, err := s.storage.PresignedPostObject(ctx, cfg.bucket, objectName, presignExpiry, cfg.maxSize, contentType)
		if err != nil {
			l.Error("failed to generate presigned post", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to generate upload url")
		}

		publicURL := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, cfg.bucket, objectName)
		targets = append(targets, &mediav1.UploadTarget{
			PostUrl:   postURL,
			Fields:    fields,
			PublicUrl: publicURL,
		})
	}

	return &mediav1.CreateUploadUrlsResponse{Targets: targets}, nil
}
```

- [ ] **Step 5: scope.go — снять метод-scope (per-purpose теперь в хендлере)**

Заменить тело `Scope()` в `internal/media/internal/service/scope.go`:

```go
package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper. Метод требует только аутентификацию;
// проверка прав — per-purpose внутри CreateUploadUrls (см. checkScope).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	return map[interceptor.Method]interceptor.Scope{}
}
```

- [ ] **Step 6: Тест helper'ов**

Создать `internal/media/internal/service/service_test.go`:

```go
package service

import "testing"

func TestExtFromContentType(t *testing.T) {
	cases := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/webp": ".webp",
		"":           ".webp",
		"text/plain": ".webp",
	}
	for ct, want := range cases {
		if got := extFromContentType(ct); got != want {
			t.Errorf("extFromContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestPurposesConfigured(t *testing.T) {
	for purpose, cfg := range purposes {
		if cfg.bucket == "" {
			t.Errorf("purpose %v: empty bucket", purpose)
		}
		if cfg.maxSize <= 0 {
			t.Errorf("purpose %v: non-positive maxSize", purpose)
		}
		if len(cfg.contentTypes) == 0 {
			t.Errorf("purpose %v: no content types", purpose)
		}
	}
}
```

- [ ] **Step 7: Сборка + тесты**

Run: `go build ./... && go test ./internal/media/...`
Expected: сборка ок; тесты PASS.

- [ ] **Step 8: Commit**

```bash
git add proto/media/v1/media.proto gen/media/ internal/media/internal/service/ docs/v1/openapi.yaml
git commit -m "feat(media): presigned POST, settlement/news purposes, per-purpose scope"
```

---

### Task 6: Бутстрап бакетов в media + удаление из доменов

**Files:**
- Modify: `internal/media/fx.go`
- Modify: `internal/donate/fx.go`
- Modify: `internal/settlement/fx.go`
- Modify: `internal/news/fx.go`

- [ ] **Step 1: media.fx — создание публичных бакетов**

В `internal/media/fx.go` внутри `fx.Module("media", ...)` добавить `fx.Invoke` (после блока `fx.Provide(...)`), импортировав `"context"` и `"go.uber.org/fx"` (fx уже есть):

```go
		fx.Invoke(func(lc fx.Lifecycle, s service.Storage) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					for _, bucket := range []string{"donate-shop", "settlementsreq", "news"} {
						exists, err := s.BucketExists(ctx, bucket)
						if err != nil {
							return err
						}
						if exists {
							continue
						}
						if err := s.CreateBucket(ctx, bucket); err != nil {
							return err
						}
						if err := s.MakeBucketPublic(ctx, bucket); err != nil {
							return err
						}
					}
					return nil
				},
			})
		}),
```

- [ ] **Step 2: Убрать bucket-bootstrap из donate**

В `internal/donate/fx.go` удалить весь `fx.Invoke(func(lc fx.Lifecycle, s service.Storage) {...})` блок (строки ~48-65). Если после этого импорт `"context"` стал неиспользуемым — удалить его.

- [ ] **Step 3: Убрать bucket-bootstrap из settlement**

В `internal/settlement/fx.go` удалить `fx.Invoke(func(lc fx.Lifecycle, storage service.Storage) {...})` блок (бутстрап `settlementsreq`). Если `"context"` стал неиспользуемым — удалить.

- [ ] **Step 4: Убрать bucket-bootstrap из news**

В `internal/news/fx.go` удалить `fx.Invoke(func(lc fx.Lifecycle, storage service.Storage) {...})` блок (бутстрап `news`). Если `"context"` стал неиспользуемым — удалить.

- [ ] **Step 5: Сборка**

Run: `go build ./...`
Expected: без ошибок (неиспользуемые провайдеры `service.Storage` в доменах допустимы — fx ленивый).

- [ ] **Step 6: Commit**

```bash
git add internal/media/fx.go internal/donate/fx.go internal/settlement/fx.go internal/news/fx.go
git commit -m "refactor(media): own public bucket bootstrap, drop per-domain creation"
```

---

### Task 7: donate — allowlist-валидация image_url

**Files:**
- Modify: `internal/donate/internal/service/validate.go`
- Modify: `internal/donate/internal/service/app.go`

- [ ] **Step 1: app.go — добавить mediaurl, убрать bucketName**

В `internal/donate/internal/service/app.go`:
- удалить строку `const bucketName = "donate-shop"`;
- добавить импорт `"github.com/lasthearth/vsservice/internal/pkg/mediaurl"`;
- в `Service` добавить поле `mediaUrl *mediaurl.Validator`;
- в `Opts` добавить поле `MediaURL *mediaurl.Validator`;
- в `New` добавить `mediaUrl: opts.MediaURL,`.

Результат:

```go
type Service struct {
	repo     DonateRepository
	storage  Storage
	cfg      config.Config
	log      logger.Logger
	mapper   Mapper
	mediaUrl *mediaurl.Validator
}

type Opts struct {
	fx.In

	Repo     DonateRepository
	Storage  Storage
	Config   config.Config
	Logger   logger.Logger
	MediaURL *mediaurl.Validator
}

func New(opts Opts) *Service {
	return &Service{
		repo:     opts.Repo,
		storage:  opts.Storage,
		cfg:      opts.Config,
		log:      opts.Logger,
		mapper:   &sermapper.MapperImpl{},
		mediaUrl: opts.MediaURL,
	}
}
```

- [ ] **Step 2: validate.go — заменить prefix-check на allowlist**

Заменить весь `internal/donate/internal/service/validate.go` на:

```go
package service

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// validateImageURL проверяет, что URL непустой и его хост разрешён
// (наш CDN или allowlist внешних хостов).
func (s *Service) validateImageURL(u string) error {
	if u == "" {
		return status.Error(codes.InvalidArgument, "image_url is required")
	}
	if err := s.mediaUrl.Validate(u); err != nil {
		return status.Error(codes.InvalidArgument, "image_url host is not allowed")
	}
	return nil
}
```

- [ ] **Step 3: Сборка**

Run: `go build ./internal/donate/...`
Expected: без ошибок. Если ругается на неиспользуемый `bucketName` где-то ещё — `grep -rn "bucketName" internal/donate/internal/service/` и убрать.

- [ ] **Step 4: Commit**

```bash
git add internal/donate/internal/service/validate.go internal/donate/internal/service/app.go
git commit -m "feat(donate): validate image_url via mediaurl allowlist"
```

---

### Task 8: settlement — приём URL вместо байтов

**Files:**
- Modify: `proto/settlement/v1/settlement.proto:265-267`
- Modify: `internal/settlement/internal/service/service.go:1-105`
- Modify: `internal/settlement/internal/service/app.go`

- [ ] **Step 1: Proto — SubmitAttachment.data → url**

В `proto/settlement/v1/settlement.proto` заменить `SubmitAttachment`:

```proto
  message SubmitAttachment {
    string url = 1 [(google.api.field_behavior) = REQUIRED];
    string description = 2 [(google.api.field_behavior) = REQUIRED];
  }
```

- [ ] **Step 2: Codegen**

Run: `buf generate`
Expected: `gen/settlement/v1/*.pb.go` — поле `Url string` в `SubmitRequest_SubmitAttachment`.

- [ ] **Step 3: app.go — добавить mediaurl**

В `internal/settlement/internal/service/app.go`:
- импорт `"github.com/lasthearth/vsservice/internal/pkg/mediaurl"`;
- в `Opts` добавить `MediaURL *mediaurl.Validator`;
- в `Service` добавить `mediaUrl *mediaurl.Validator`;
- в `New` добавить `mediaUrl: opts.MediaURL,`.

- [ ] **Step 4: service.go — переписать Submit + почистить импорты**

В `internal/settlement/internal/service/service.go` заменить блок импортов на (убраны `bytes`, `fmt`, `mime`, `uuid`, `image`):

```go
import (
	"context"
	"errors"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)
```

Заменить начало `Submit` (от сигнатуры до конца цикла по attachments, т.е. старые строки 22-90) на:

```go
// Submit implements settlementv1.SettlementServiceServer
func (s *Service) Submit(ctx context.Context, req *settlementv1.SubmitRequest) (*settlementv1.SubmitResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Attachments) == 0 {
		return nil, status.Error(codes.InvalidArgument, "attachments cannot be empty")
	}

	s.log.Info("submitting settlement request",
		zap.String("leader_id", userID),
		zap.String("settlement_name", req.Name),
		zap.Int("attachments", len(req.Attachments)))

	stype, err := TypeFromReqProto(req.Type)
	if err != nil {
		return nil, err
	}

	if err := s.dbRepo.IsMemberOrLeader(ctx, "", userID); err != nil {
		s.log.Error("user validation failed", zap.Error(err), zap.String("user_id", userID))
		if err != ierror.ErrAlreadyMember {
			return nil, err
		}
	}

	attachs := make([]model.Attachment, len(req.Attachments))
	for i, attachment := range req.Attachments {
		if err := s.mediaUrl.Validate(attachment.Url); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid attachment url")
		}
		attachs[i] = model.Attachment{
			Url:  attachment.Url,
			Desc: attachment.Description,
		}
	}
```

Всё, что после (`opts := SettlementOpts{...}` и далее), оставить без изменений.

- [ ] **Step 5: Проверить отсутствие ссылок на старое поле + сборка**

Run: `grep -rn "\.Data" internal/settlement && go build ./internal/settlement/...`
Expected: grep ничего не находит в коде хендлеров; сборка ок. Если `lo`/`errors` оказались неиспользуемы — убрать соответствующий импорт.

- [ ] **Step 6: Commit**

```bash
git add proto/settlement/v1/settlement.proto gen/settlement/ internal/settlement/internal/service/ docs/v1/openapi.yaml
git commit -m "feat(settlement): accept attachment URLs with allowlist validation"
```

---

### Task 9: news — превью как URL

**Files:**
- Modify: `proto/news/v1/news.proto:75`
- Modify: `internal/news/internal/service/service.go:1-78`
- Modify: `internal/news/internal/service/app.go`

- [ ] **Step 1: Proto — preview bytes → string**

В `proto/news/v1/news.proto` в `CreateNewsRequest` заменить `bytes preview = 3;` на:

```proto
  string preview = 3;
```

- [ ] **Step 2: Codegen**

Run: `buf generate`
Expected: `gen/news/v1/*.pb.go` — поле `Preview string` в `CreateNewsRequest`.

- [ ] **Step 3: app.go — добавить mediaurl**

В `internal/news/internal/service/app.go`:
- импорт `"github.com/lasthearth/vsservice/internal/pkg/mediaurl"`;
- в `Opts` добавить `MediaURL *mediaurl.Validator`;
- в `Service` добавить `mediaUrl *mediaurl.Validator`;
- в `New` добавить `mediaUrl: opts.MediaURL,`.

- [ ] **Step 4: service.go — переписать CreateNews + почистить импорты**

В `internal/news/internal/service/service.go` заменить блок импортов на (убраны `bytes`, `mime`, `uuid`; добавлены `codes`, `status`):

```go
import (
	"context"
	"fmt"

	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)
```

> Если после правок `zap` или `emptypb` окажутся неиспользуемы в файле — убрать их импорт (проверит сборка).

Заменить `CreateNews` (старые строки 19-78) на:

```go
// CreateNews implements newsv1.NewsServiceServer.
func (s *Service) CreateNews(ctx context.Context, req *newsv1.CreateNewsRequest) (*newsv1.News, error) {
	if err := s.mediaUrl.Validate(req.Preview); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preview url")
	}

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	news := &model.News{
		Title:     req.Title,
		Content:   req.Content,
		Preview:   req.Preview,
		CreatedBy: userID,
	}

	if err := s.validator.Struct(news); err != nil {
		return nil, err
	}

	created, err := s.repo.CreateNews(ctx, news)
	if err != nil {
		return nil, err
	}

	if err := s.cnuc.CreateNotification(
		ctx,
		"Новая новость",
		fmt.Sprintf("Новость: %s", req.Title),
		notificationuc.WithBroadcast(),
	); err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*created), nil
}
```

- [ ] **Step 5: Сборка**

Run: `go build ./internal/news/...`
Expected: без ошибок. Если ругается на неиспользуемый импорт (`zap`/`emptypb`) — убрать.

- [ ] **Step 6: Commit**

```bash
git add proto/news/v1/news.proto gen/news/ internal/news/internal/service/ docs/v1/openapi.yaml
git commit -m "feat(news): accept preview URL with allowlist validation"
```

---

### Task 10: Финальная проверка

- [ ] **Step 1: Полная сборка**

Run: `go build ./...`
Expected: без ошибок.

- [ ] **Step 2: Все тесты**

Run: `go test ./...`
Expected: PASS (как минимум `mediaurl` и `media` — зелёные; остальные не сломаны).

- [ ] **Step 3: Vet**

Run: `go vet ./internal/media/... ./internal/pkg/mediaurl/... ./internal/donate/... ./internal/settlement/... ./internal/news/...`
Expected: без замечаний.

---

## Out of scope
- player-аватар (остаётся server-proxied с вариантами X96/X48).
- Удаление мёртвых `storage`/`cfg`-зависимостей из donate/settlement/news (только импорты чистим).
- Клиентская интеграция (multipart POST), HEAD-проверка размера внешних ссылок, асинхронный ресайз.
