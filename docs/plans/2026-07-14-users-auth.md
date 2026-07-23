# #4.1 Users/Auth — api-schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the `api-schema/` repository with OpenAPI 3.1 schemas for the Users/Auth service (MVP scope), following the structure of `~/projects/api/mws` and conventions of `~/projects/oapigenerator`.

**Architecture:** Single `api-schema/` repo at `~/zvonilka/backend/api-schema/`. Per-service directories (`users/`) with `src/openapi/{openapi.yaml,schemas/<domain>/,resources/v1/<group>/}` structure. Shared models in `common/src/openapi/schemas/<domain>/`. All cross-references via relative `$ref`. Global `generation_flags.yaml` enables `GOLANG_SPLIT_REQUEST_RESPONSE` and `USE_UTC_FOR_DATE_TIME`.

**Tech Stack:** OpenAPI 3.1, YAML. Validation: `python3 -c "import yaml; ..."` for syntax + `$ref` resolution check. Full OpenAPI structural validation — when generator (`~/projects/oapigenerator`) is available.

**Spec:** `backend/api-schema/docs/specs/2026-07-14-users-auth-design.md`

**Conventions (from spec):**
- `login`: 3-32 chars, `a-z0-9._-`, lowercased. Hidden (only in `/users/me`).
- `email`: required, unique, hidden.
- `nickname`: 1-32 chars, Unicode, NOT unique.
- `tag`: 7 chars from `A-Z0-9`, globally unique, server-generated.
- `identifier`: `email` or `login` (server detects by format).
- JWT claims: `sub`, `iss`, `aud`, `exp`, `iat` only.
- `UserRef` (public): `id`, `nickname`, `tag`. No `login`/`email`.
- `User` (full, `/users/me` only): `id`, `login`, `email`, `nickname`, `tag`, `status`, `email_verified`, `created_at`, `updated_at`.

**No commits:** User does all git commits. Plan stops at file creation + validation.

---

## File Structure (final state after plan)

```
backend/api-schema/
├── README.md
├── generation_flags.yaml                          # Task 1
├── common/
│   ├── generation_flags.yaml                      # Task 5
│   └── src/openapi/
│       ├── openapi.yaml                           # Task 5
│       ├── parameters/
│       │   └── list.yaml                          # Task 3
│       └── schemas/
│           ├── auth/
│           │   └── TokenPair.yaml                 # Task 4 (shared auth DTO)
│           ├── errors/
│           │   ├── Error.yaml                     # Task 2
│           │   └── ValidationError.yaml           # Task 2
│           ├── identifiers/
│           │   └── UUIDv7.yaml                    # Task 4 (shared identifier type)
│           ├── pagination/
│           │   ├── ListRequest.yaml               # Task 3
│           │   └── ListResponse.yaml              # Task 3
│           └── time/
│               └── Timestamp.yaml                 # Task 4
├── users/
│   ├── generation_flags.yaml                      # Task 12
│   └── src/openapi/
│       ├── openapi.yaml                           # Task 12
│       ├── schemas/
│       │   ├── auth/
│       │   │   ├── RegisterRequest.yaml           # Task 6
│       │   │   ├── RegisterResponse.yaml          # Task 6
│       │   │   ├── LoginRequest.yaml              # Task 6
│       │   │   ├── LoginResponse.yaml             # Task 6
│       │   │   ├── RefreshRequest.yaml            # Task 6
│       │   │   ├── RefreshResponse.yaml           # Task 6
│       │   │   └── LogoutRequest.yaml             # Task 6
│       │   ├── models/
│       │   │   ├── User.yaml                      # Task 5b
│       │   │   ├── UserRef.yaml                   # Task 5b
│       │   │   ├── UserStatus.yaml                # Task 5b
│       │   │   └── Session.yaml                   # Task 7
│       │   └── profile/
│       │       └── UpdateUserRequest.yaml         # Task 7
│       └── resources/v1/
│           ├── auth/
│           │   ├── register.yaml                  # Task 8
│           │   ├── login.yaml                     # Task 8
│           │   ├── refresh.yaml                   # Task 8
│           │   └── logout.yaml                    # Task 8
│           ├── profile/
│           │   ├── usersMe.yaml                   # Task 9
│           │   └── users.yaml                     # Task 9
│           ├── sessions/
│           │   ├── sessions.yaml                  # Task 10
│           │   └── session.yaml                   # Task 10
│           └── service/
│               ├── jwks.yaml                      # Task 11
│               └── health.yaml                    # Task 11
└── docs/
    ├── specs/2026-07-14-users-auth-design.md       # already exists
    └── plans/2026-07-14-users-auth.md              # this file
```

**Total: 35 files** (1 README + 1 global flags + 10 common + 12 users schemas + 12 users resources + 1 users openapi + 1 users flags... — точно: см. Task 13 Step 3 для checklist).

**`$ref` relative path conventions:**
- From `users/schemas/models/User.yaml` → common: `../../../../../common/src/openapi/schemas/time/Timestamp.yaml`
- From `users/schemas/models/User.yaml` → local (same dir): `./UserStatus.yaml`
- From `users/resources/v1/auth/register.yaml` → users schema: `../../../schemas/auth/RegisterRequest.yaml`
- From `users/resources/v1/auth/register.yaml` → common: `../../../../../../../common/src/openapi/schemas/errors/Error.yaml`

---

### Task 1: Repo skeleton + global generation flags + README

**Files:**
- Create: `backend/api-schema/README.md`
- Create: `backend/api-schema/generation_flags.yaml`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p backend/api-schema/{common/src/openapi/{schemas/{auth,errors,identifiers,pagination,time},parameters},users/src/openapi/{schemas/{auth,models,profile},resources/v1/{auth,profile,sessions,service}},generated}
mkdir -p backend/api-schema/docs/{specs,plans}
```

- [ ] **Step 2: Create `generation_flags.yaml`**

```yaml
# Global generation flags for oapigenerator.
# Per-service override: <service>/generation_flags.yaml.
# See ~/projects/oapigenerator README.md for flag semantics.

- name: GOLANG_SPLIT_REQUEST_RESPONSE
  description: "Split Request/Response models"
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}

- name: USE_UTC_FOR_DATE_TIME
  description: "Serialize date-time in UTC via model.UTCTime"
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}

- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  description: "Server request decoder does not call SetDefaults"
  enabled: false
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}

- name: USE_REQUIRED_V2
  description: "Use x-request-required/x-response-required"
  enabled: false
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}
```

- [ ] **Step 3: Create `README.md`**

```markdown
# Zvonilka API Schema

OpenAPI 3.1 спецификации для всех backend-сервисов zvonilka. Отсюда генератор
(`~/projects/oapigenerator`) генерирует Go-пакеты: модели, клиентские/серверные
интерфейсы, HTTP-клиент/сервер, моки, SDK.

## Структура

```
api-schema/
├── generation_flags.yaml            # глобальные дефолты
├── common/                          # общие модели
│   └── src/openapi/
│       ├── openapi.yaml             # paths: {} + components.schemas с $ref
│       ├── schemas/
│       │   ├── auth/                # общие auth DTO (TokenPair)
│       │   ├── errors/              # Error, ValidationError
│       │   ├── identifiers/         # UUIDv7 и др. ID типы
│       │   ├── pagination/          # ListRequest, ListResponse
│       │   └── time/               # Timestamp
│       └── parameters/             # общие query/header params
├── <service>/                       # один каталог на сервис
│   ├── generation_flags.yaml        # per-service override
│   └── src/openapi/
│       ├── openapi.yaml             # main spec, paths $ref resources
│       ├── schemas/
│       │   ├── auth/                # auth DTOs сервиса
│       │   ├── models/              # доменные модели
│       │   └── profile/             # profile update DTOs
│       └── resources/v1/
│           ├── auth/                # auth endpoints
│           ├── profile/             # profile endpoints
│           ├── sessions/            # session endpoints
│           └── service/             # service endpoints (jwks, health)
└── generated/                       # output codegen (per-service subdirs)
```

## Соглашения

- **OpenAPI 3.1** (не 3.0) — используем `type: object` + `additionalProperties: false` для strict объектов.
- **Одна схема = один файл** в `schemas/<domain>/`. Имя файла = имя компонента (e.g., `User.yaml` → `components.schemas.User`).
- **Cross-service refs** — через `common/`: `$ref: '../../../../../common/src/openapi/schemas/errors/Error.yaml'`.
- **Generation flags** — `GOLANG_SPLIT_REQUEST_RESPONSE: true`, `USE_UTC_FOR_DATE_TIME: true` (глобально).
- **x-validations** — используем для декларативной валидации (`Size >=N`, `app.EmailFormat`, и т.д.).
- **Resource files** — без префикса имени группы (e.g., `auth/register.yaml`, не `auth/authRegister.yaml`), т.к. папка уже указывает группу.

## Генерация кода (когда генератор будет передан)

```bash
# Для каждого сервиса:
go run ~/projects/oapigenerator/cmd/oapigen \
  -input ./users/src/openapi/openapi.yaml \
  -output ./generated/users \
  -import-prefix github.com/zvonilka/users/gen \
  -generation-flags-config-path ./generation_flags.yaml \
  -project-flags-path ./users/generation_flags.yaml
```

## Текущее состояние

- **users/** (#4.1 Users/Auth) — MVP: registration, login, refresh rotation, logout, profile, sessions, JWKS, health. См. `docs/specs/2026-07-14-users-auth-design.md`.

## Валидация

YAML syntax check (быстрая):
```bash
find . -name '*.yaml' -exec python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" {} \;
```

Полная OpenAPI-валидация — через генератор (`-dry-run`) когда он будет передан.
```

- [ ] **Step 4: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/generation_flags.yaml'))"`
Expected: no output (success).

- [ ] **Step 5: User commits**

```bash
git add backend/api-schema/{README.md,generation_flags.yaml}
git commit -m "feat(api-schema): repo skeleton + global generation flags"
```

---

### Task 2: common/ — error schemas

**Files:**
- Create: `backend/api-schema/common/src/openapi/schemas/errors/Error.yaml`
- Create: `backend/api-schema/common/src/openapi/schemas/errors/ValidationError.yaml`

- [ ] **Step 1: Create `Error.yaml`**

```yaml
name: Error
description: Стандартная ошибка API. Возвращается во всех не-2xx ответах.
type: object
required:
  - code
  - message
properties:
  code:
    type: string
    description: Машинный код ошибки (e.g., "INVALID_ARGUMENT", "NOT_FOUND", "PERMISSION_DENIED").
    x-validations: ["Size >=1", "Size <=64"]
  message:
    type: string
    description: Человекочитаемое сообщение.
    x-validations: ["Size >=1", "Size <=512"]
  details:
    type: array
    description: Дополнительные детали (могут быть специфичны для кода ошибки).
    items:
      type: object
      additionalProperties: true
  request_id:
    type: string
    description: Trace ID запроса (для корреляции с логами).
    x-validations: ["Size <=64"]
additionalProperties: false
```

- [ ] **Step 2: Create `ValidationError.yaml`**

```yaml
name: ValidationError
description: Детали ошибки валидации конкретного поля. Вкладывается в Error.details.
type: object
required:
  - field
  - message
properties:
  field:
    type: string
    description: Имя поля (dot-path для nested, e.g., "user.login" или "items[0].id").
    x-validations: ["Size >=1", "Size <=128"]
  message:
    type: string
    description: Что не так с полем.
    x-validations: ["Size >=1", "Size <=256"]
  rule:
    type: string
    description: Имя нарушенного правила (e.g., "Size", "app.EmailFormat").
    x-validations: ["Size <=64"]
additionalProperties: false
```

- [ ] **Step 3: Validate YAML syntax**

Run:
```bash
for f in Error ValidationError; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/common/src/openapi/schemas/errors/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/common/src/openapi/schemas/errors/
git commit -m "feat(api-schema/common): error schemas (Error, ValidationError)"
```

---

### Task 3: common/ — pagination schemas + shared parameters

**Files:**
- Create: `backend/api-schema/common/src/openapi/schemas/pagination/ListRequest.yaml`
- Create: `backend/api-schema/common/src/openapi/schemas/pagination/ListResponse.yaml`
- Create: `backend/api-schema/common/src/openapi/parameters/list.yaml`

- [ ] **Step 1: Create `ListRequest.yaml`**

```yaml
name: ListRequest
description: Общие query-параметры для list-эндпоинтов (пагинация, фильтрация).
type: object
properties:
  page_size:
    type: integer
    format: int32
    description: Количество элементов на странице. Max 100.
    default: 20
    minimum: 1
    maximum: 100
    x-validations: [">=1", "<=100"]
  page_token:
    type: string
    description: Opaque токен страницы. Извлекается из ListResponse.next_page_token.
    x-validations: ["Size <=256"]
  filter:
    type: string
    description: Фильтр в SQL-подобном синтаксисе (service-specific).
    x-validations: ["Size <=1024"]
  order_by:
    type: string
    description: Поле сортировки с опциональным направлением (e.g., "created_at desc").
    x-validations: ["Size <=128"]
additionalProperties: false
```

- [ ] **Step 2: Create `ListResponse.yaml`**

```yaml
name: ListResponse
description: Обёртка для list-ответов с пагинацией. Наследуется конкретными response-схемами через allOf.
type: object
required:
  - items
properties:
  next_page_token:
    type: string
    description: Токен следующей страницы. Пустой или отсутствует — это последняя страница.
    x-validations: ["Size <=256"]
additionalProperties: false
```

- [ ] **Step 3: Create `parameters/list.yaml`**

```yaml
# Общие query-параметры для list-эндпоинтов.
# Используются через $ref в resource-файлах.

pageSizeParam:
  name: page_size
  in: query
  description: Количество элементов на странице.
  required: false
  schema:
    type: integer
    format: int32
    default: 20
    minimum: 1
    maximum: 100

pageTokenParam:
  name: page_token
  in: query
  description: Opaque токен страницы.
  required: false
  schema:
    type: string
    maxLength: 256

filterParam:
  name: filter
  in: query
  description: SQL-подобный фильтр.
  required: false
  schema:
    type: string
    maxLength: 1024

orderByParam:
  name: order_by
  in: query
  description: Поле сортировки (e.g., "created_at desc").
  required: false
  schema:
    type: string
    maxLength: 128
```

- [ ] **Step 4: Validate YAML syntax**

Run:
```bash
for f in backend/api-schema/common/src/openapi/schemas/pagination/ListRequest.yaml \
         backend/api-schema/common/src/openapi/schemas/pagination/ListResponse.yaml \
         backend/api-schema/common/src/openapi/parameters/list.yaml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))"
done
```
Expected: no output (success).

- [ ] **Step 5: User commits**

```bash
git add backend/api-schema/common/src/openapi/{schemas/pagination,parameters}
git commit -m "feat(api-schema/common): pagination schemas + shared query parameters"
```

---

### Task 4: common/ — time, identifiers, auth (TokenPair) schemas

**Files:**
- Create: `backend/api-schema/common/src/openapi/schemas/time/Timestamp.yaml`
- Create: `backend/api-schema/common/src/openapi/schemas/identifiers/UUIDv7.yaml`
- Create: `backend/api-schema/common/src/openapi/schemas/auth/TokenPair.yaml`

- [ ] **Step 1: Create `Timestamp.yaml`**

```yaml
name: Timestamp
description: RFC3339 timestamp в UTC. С USE_UTC_FOR_DATE_TIME флагом генерируется как model.UTCTime.
type: string
format: date-time
x-validations: ["app.UTCDateTime"]
```

- [ ] **Step 2: Create `UUIDv7.yaml`**

```yaml
name: UUIDv7
description: UUID v7 (time-ordered). Используется для всех primary keys в zvonilka.
type: string
format: uuid
pattern: '^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
x-validations: ["app.UUIDv7"]
```

- [ ] **Step 3: Create `TokenPair.yaml`**

```yaml
name: TokenPair
description: Пара access + refresh токенов. Возвращается из auth-эндпоинтов. Общая для всех сервисов, использующих Users/Auth.
type: object
required:
  - access_token
  - refresh_token
  - expires_in
properties:
  access_token:
    type: string
    description: JWT (ES256) access токен. TTL 15min.
    x-validations: ["Size >=1"]
  refresh_token:
    type: string
    description: Opaque refresh токен. TTL 7 дней. Хранить на клиенте (httpOnly cookie для web).
    x-validations: ["Size >=1"]
  expires_in:
    type: integer
    format: int64
    description: TTL access токена в секундах (900 = 15min).
    x-validations: [">0"]
additionalProperties: false
```

- [ ] **Step 4: Validate YAML syntax**

Run:
```bash
for f in time/Timestamp identifiers/UUIDv7 auth/TokenPair; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/common/src/openapi/schemas/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 5: User commits**

```bash
git add backend/api-schema/common/src/openapi/schemas/{time,identifiers,auth}
git commit -m "feat(api-schema/common): time, identifiers (UUIDv7), auth (TokenPair) schemas"
```

---

### Task 5: common/ — openapi.yaml + per-service generation flags

**Files:**
- Create: `backend/api-schema/common/src/openapi/openapi.yaml`
- Create: `backend/api-schema/common/generation_flags.yaml`

- [ ] **Step 1: Create `common/generation_flags.yaml`**

```yaml
# Per-service override for common/ — inherits global defaults, no overrides.
# Listed explicitly for documentation.

GOLANG_SPLIT_REQUEST_RESPONSE: true
USE_UTC_FOR_DATE_TIME: true
```

- [ ] **Step 2: Create `common/src/openapi/openapi.yaml`**

```yaml
openapi: 3.1.0
info:
  title: Zvonilka Common Schemas
  description: Общие модели для всех сервисов zvonilka. Paths пустые — только компоненты.
  version: 0.1.0
servers:
  - url: http://localhost:8080/
paths: {}
components:
  schemas:
    Error:
      $ref: "./schemas/errors/Error.yaml"
    ValidationError:
      $ref: "./schemas/errors/ValidationError.yaml"
    ListRequest:
      $ref: "./schemas/pagination/ListRequest.yaml"
    ListResponse:
      $ref: "./schemas/pagination/ListResponse.yaml"
    Timestamp:
      $ref: "./schemas/time/Timestamp.yaml"
    UUIDv7:
      $ref: "./schemas/identifiers/UUIDv7.yaml"
    TokenPair:
      $ref: "./schemas/auth/TokenPair.yaml"
  parameters:
    pageSizeParam:
      $ref: "./parameters/list.yaml#/pageSizeParam"
    pageTokenParam:
      $ref: "./parameters/list.yaml#/pageTokenParam"
    filterParam:
      $ref: "./parameters/list.yaml#/filterParam"
    orderByParam:
      $ref: "./parameters/list.yaml#/orderByParam"
```

- [ ] **Step 3: Validate YAML syntax + resolve $ref paths**

Run:
```bash
python3 << 'PYEOF'
import yaml, os
base = 'backend/api-schema/common/src/openapi'
with open(f'{base}/openapi.yaml') as f:
    spec = yaml.safe_load(f)
for name, ref in spec['components']['schemas'].items():
    path = ref['$ref'].replace('./', f'{base}/')
    assert os.path.exists(path), f'Missing ref: {path} for schema {name}'
print(f'All {len(spec["components"]["schemas"])} common schemas resolve.')
PYEOF
```
Expected: `All 7 common schemas resolve.`

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/common/{generation_flags.yaml,src/openapi/openapi.yaml}
git commit -m "feat(api-schema/common): openapi.yaml + per-service generation flags"
```

---

### Task 5b: users/ — models (UserStatus, User, UserRef)

**Files:**
- Create: `backend/api-schema/users/src/openapi/schemas/models/UserStatus.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/models/User.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/models/UserRef.yaml`

- [ ] **Step 1: Create `UserStatus.yaml`**

```yaml
name: UserStatus
description: Статус учётной записи пользователя.
type: string
enum:
  - active
  - suspended
  - deleted
x-validations: ["app.UserStatusValue"]
```

- [ ] **Step 2: Create `User.yaml`**

```yaml
name: User
description: Полный профиль пользователя. Возвращается ТОЛЬКО из /users/me (владельцу). login и email скрыты от других пользователей.
type: object
required:
  - id
  - login
  - email
  - nickname
  - tag
  - status
  - email_verified
  - created_at
  - updated_at
properties:
  id:
    $ref: "../../../../../common/src/openapi/schemas/identifiers/UUIDv7.yaml"
  login:
    type: string
    description: Уникальный скрытый идентификатор для аутентификации. 3-32 символа, a-z0-9._-, lowercased.
    x-validations: ["Size >=3", "Size <=32", "app.LoginFormat"]
  email:
    type: string
    format: email
    description: Email пользователя. Скрытый (видит только владелец).
    x-validations: ["app.EmailFormat"]
  nickname:
    type: string
    description: Отображаемое имя. 1-32 Unicode-символа. Может повторяться.
    x-validations: ["Size >=1", "Size <=32", "app.NoControlChars"]
  tag:
    type: string
    description: 7 символов A-Z0-9, генерируется сервером. Глобально уникальный. Для disambiguation при совпадении nickname.
    readOnly: true
    x-validations: ["Size ==7", "app.TagFormat"]
  status:
    $ref: "./UserStatus.yaml"
    readOnly: true
  email_verified:
    type: boolean
    description: Подтверждён ли email. В MVP всегда false (верификация — #4.6).
    readOnly: true
  created_at:
    $ref: "../../../../../common/src/openapi/schemas/time/Timestamp.yaml"
    readOnly: true
  updated_at:
    $ref: "../../../../../common/src/openapi/schemas/time/Timestamp.yaml"
    readOnly: true
additionalProperties: false
```

- [ ] **Step 3: Create `UserRef.yaml`**

```yaml
name: UserRef
description: Публичный профиль пользователя. Без login и email. Возвращается из /users/{id} другим пользователям/сервисам.
type: object
required:
  - id
  - nickname
  - tag
properties:
  id:
    $ref: "../../../../../common/src/openapi/schemas/identifiers/UUIDv7.yaml"
  nickname:
    type: string
    description: Отображаемое имя.
    x-validations: ["Size >=1", "Size <=32", "app.NoControlChars"]
  tag:
    type: string
    description: 7 символов A-Z0-9. Для disambiguation.
    readOnly: true
    x-validations: ["Size ==7", "app.TagFormat"]
additionalProperties: false
```

- [ ] **Step 4: Validate YAML syntax**

Run:
```bash
for f in UserStatus User UserRef; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/schemas/models/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 5: User commits**

```bash
git add backend/api-schema/users/src/openapi/schemas/models/
git commit -m "feat(api-schema/users): models (UserStatus, User, UserRef)"
```

---

### Task 6: users/ — auth schemas (Register, Login, Refresh, Logout)

**Files:**
- Create: `backend/api-schema/users/src/openapi/schemas/auth/RegisterRequest.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/RegisterResponse.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/LoginRequest.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/LoginResponse.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/RefreshRequest.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/RefreshResponse.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/auth/LogoutRequest.yaml`

- [ ] **Step 1: Create `RegisterRequest.yaml`**

```yaml
name: RegisterRequest
description: Тело запроса POST /auth/register.
type: object
required:
  - login
  - email
  - password
  - nickname
properties:
  login:
    type: string
    description: Уникальный скрытый идентификатор. 3-32 символа, a-z0-9._-, lowercased.
    x-validations: ["Size >=3", "Size <=32", "app.LoginFormat"]
  email:
    type: string
    format: email
    x-validations: ["app.EmailFormat"]
  password:
    type: string
    format: password
    description: 8-128 символов.
    x-validations: ["Size >=8", "Size <=128"]
  nickname:
    type: string
    description: Отображаемое имя. 1-32 Unicode.
    x-validations: ["Size >=1", "Size <=32", "app.NoControlChars"]
additionalProperties: false
```

- [ ] **Step 2: Create `RegisterResponse.yaml`**

```yaml
name: RegisterResponse
description: Ответ POST /auth/register. Возвращает полный профиль + токены.
type: object
required:
  - user
  - access_token
  - refresh_token
  - expires_in
properties:
  user:
    $ref: "../models/User.yaml"
  access_token:
    type: string
    x-validations: ["Size >=1"]
  refresh_token:
    type: string
    x-validations: ["Size >=1"]
  expires_in:
    type: integer
    format: int64
    x-validations: [">0"]
additionalProperties: false
```

- [ ] **Step 3: Create `LoginRequest.yaml`**

```yaml
name: LoginRequest
description: Тело запроса POST /auth/login. Identifier может быть email или login.
type: object
required:
  - identifier
  - password
properties:
  identifier:
    type: string
    description: Email или login. Сервер определяет формат (наличие '@' → email).
    x-validations: ["Size >=3", "Size <=320", "app.IdentifierFormat"]
  password:
    type: string
    format: password
    x-validations: ["Size >=8", "Size <=128"]
additionalProperties: false
```

- [ ] **Step 4: Create `LoginResponse.yaml`**

```yaml
name: LoginResponse
description: Ответ POST /auth/login.
type: object
required:
  - user
  - access_token
  - refresh_token
  - expires_in
properties:
  user:
    $ref: "../models/User.yaml"
  access_token:
    type: string
    x-validations: ["Size >=1"]
  refresh_token:
    type: string
    x-validations: ["Size >=1"]
  expires_in:
    type: integer
    format: int64
    x-validations: [">0"]
additionalProperties: false
```

- [ ] **Step 5: Create `RefreshRequest.yaml`**

```yaml
name: RefreshRequest
description: Тело запроса POST /auth/refresh.
type: object
required:
  - refresh_token
properties:
  refresh_token:
    type: string
    description: Ранее выданный refresh токен.
    x-validations: ["Size >=1"]
additionalProperties: false
```

- [ ] **Step 6: Create `RefreshResponse.yaml`**

```yaml
name: RefreshResponse
description: Ответ POST /auth/refresh. Новый access + новый refresh. Старый refresh инвалидируется.
type: object
required:
  - access_token
  - refresh_token
  - expires_in
properties:
  access_token:
    type: string
    x-validations: ["Size >=1"]
  refresh_token:
    type: string
    x-validations: ["Size >=1"]
  expires_in:
    type: integer
    format: int64
    x-validations: [">0"]
additionalProperties: false
```

- [ ] **Step 7: Create `LogoutRequest.yaml`**

```yaml
name: LogoutRequest
description: Тело запроса POST /auth/logout. Инвалидирует refresh-цепочку.
type: object
required:
  - refresh_token
properties:
  refresh_token:
    type: string
    description: Refresh токен, который нужно инвалидировать (вместе со всей цепочкой).
    x-validations: ["Size >=1"]
additionalProperties: false
```

- [ ] **Step 8: Validate YAML syntax**

Run:
```bash
for f in RegisterRequest RegisterResponse LoginRequest LoginResponse \
         RefreshRequest RefreshResponse LogoutRequest; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/schemas/auth/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 9: User commits**

```bash
git add backend/api-schema/users/src/openapi/schemas/auth/
git commit -m "feat(api-schema/users): auth request/response schemas"
```

---

### Task 7: users/ — profile (UpdateUserRequest) + models (Session)

**Files:**
- Create: `backend/api-schema/users/src/openapi/schemas/profile/UpdateUserRequest.yaml`
- Create: `backend/api-schema/users/src/openapi/schemas/models/Session.yaml`

- [ ] **Step 1: Create `UpdateUserRequest.yaml`**

```yaml
name: UpdateUserRequest
description: Тело запроса PATCH /users/me. Все поля optional.
type: object
properties:
  nickname:
    type: string
    description: Новое отображаемое имя.
    x-validations: ["Size >=1", "Size <=32", "app.NoControlChars"]
  login:
    type: string
    description: Новый login. В MVP разрешена смена без подтверждения.
    x-validations: ["Size >=3", "Size <=32", "app.LoginFormat"]
additionalProperties: false
```

- [ ] **Step 2: Create `Session.yaml`**

```yaml
name: Session
description: Активная сессия пользователя (refresh token entry).
type: object
required:
  - id
  - user_agent
  - ip
  - created_at
  - expires_at
  - current
properties:
  id:
    $ref: "../../../../../common/src/openapi/schemas/identifiers/UUIDv7.yaml"
  user_agent:
    type: string
    description: User-Agent клиента при создании сессии.
    x-validations: ["Size <=512"]
  ip:
    type: string
    description: IP-адрес клиента при создании сессии.
    x-validations: ["app.IPAddress"]
  created_at:
    $ref: "../../../../../common/src/openapi/schemas/time/Timestamp.yaml"
  expires_at:
    $ref: "../../../../../common/src/openapi/schemas/time/Timestamp.yaml"
  current:
    type: boolean
    description: true если это сессия текущего запроса (по JWT).
additionalProperties: false
```

- [ ] **Step 3: Validate YAML syntax**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/schemas/profile/UpdateUserRequest.yaml'))"
python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/schemas/models/Session.yaml'))"
```
Expected: no output (success).

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/users/src/openapi/schemas/{profile,models/}
git commit -m "feat(api-schema/users): UpdateUserRequest, Session schemas"
```

---

### Task 8: users/ — auth resources (register, login, refresh, logout)

**Files:**
- Create: `backend/api-schema/users/src/openapi/resources/v1/auth/register.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/auth/login.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/auth/refresh.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/auth/logout.yaml`

- [ ] **Step 1: Create `register.yaml`**

```yaml
post:
  tags:
    - auth
  summary: Register new user
  description: Создаёт пользователя по login + email + password + nickname. Сервер генерирует tag. Возвращает access + refresh.
  operationId: registerUser
  requestBody:
    required: true
    content:
      application/json:
        schema:
          $ref: "../../../schemas/auth/RegisterRequest.yaml"
  responses:
    "201":
      description: User created.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/auth/RegisterResponse.yaml"
    "400":
      description: Invalid request body.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "409":
      description: Login or email already taken.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 2: Create `login.yaml`**

```yaml
post:
  tags:
    - auth
  summary: Login
  description: Аутентификация по identifier (email или login) + password. Возвращает access + refresh.
  operationId: loginUser
  requestBody:
    required: true
    content:
      application/json:
        schema:
          $ref: "../../../schemas/auth/LoginRequest.yaml"
  responses:
    "200":
      description: Login successful.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/auth/LoginResponse.yaml"
    "400":
      description: Invalid request body.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "401":
      description: Invalid credentials.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 3: Create `refresh.yaml`**

```yaml
post:
  tags:
    - auth
  summary: Refresh access token
  description: Принимает refresh токен, выдаёт новый access + новый refresh. Старый refresh инвалидируется. При reuse отозванного токена — инвалидация всей цепочки (409).
  operationId: refreshToken
  requestBody:
    required: true
    content:
      application/json:
        schema:
          $ref: "../../../schemas/auth/RefreshRequest.yaml"
  responses:
    "200":
      description: New token pair.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/auth/RefreshResponse.yaml"
    "401":
      description: Invalid or expired refresh token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "409":
      description: Refresh token reuse detected. Entire chain invalidated. Client must re-login.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 4: Create `logout.yaml`**

```yaml
post:
  tags:
    - auth
  summary: Logout
  description: Инвалидирует текущую refresh-цепочку. Требует access токен в Authorization header + refresh_token в body.
  operationId: logoutUser
  security:
    - bearerAuth: []
  requestBody:
    required: true
    content:
      application/json:
        schema:
          $ref: "../../../schemas/auth/LogoutRequest.yaml"
  responses:
    "204":
      description: Logout successful. No content.
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 5: Validate YAML syntax**

Run:
```bash
for f in register login refresh logout; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/resources/v1/auth/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 6: User commits**

```bash
git add backend/api-schema/users/src/openapi/resources/v1/auth/
git commit -m "feat(api-schema/users): auth resource endpoints"
```

---

### Task 9: users/ — profile resources (usersMe, users)

**Files:**
- Create: `backend/api-schema/users/src/openapi/resources/v1/profile/usersMe.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/profile/users.yaml`

- [ ] **Step 1: Create `usersMe.yaml`**

```yaml
get:
  tags:
    - users
  summary: Get current user profile
  description: Возвращает полный профиль текущего пользователя (по JWT), включая скрытые login и email.
  operationId: getCurrentUser
  security:
    - bearerAuth: []
  responses:
    "200":
      description: Current user profile.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/models/User.yaml"
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"

patch:
  tags:
    - users
  summary: Update current user profile
  description: Обновляет nickname и/или login текущего пользователя. Email/password — отдельные эндпоинты (вне MVP).
  operationId: updateCurrentUser
  security:
    - bearerAuth: []
  requestBody:
    required: true
    content:
      application/json:
        schema:
          $ref: "../../../schemas/profile/UpdateUserRequest.yaml"
  responses:
    "200":
      description: Updated user profile.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/models/User.yaml"
    "400":
      description: Invalid request body.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "409":
      description: Login already taken.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 2: Create `users.yaml`**

```yaml
get:
  tags:
    - users
  summary: Get user by ID
  description: Возвращает публичный профиль (UserRef: id, nickname, tag). БЕЗ login и email. В MVP доступен любому аутентифицированному. После #4.2 IAM — закрыть через policy.
  operationId: getUserById
  security:
    - bearerAuth: []
  parameters:
    - name: id
      in: path
      required: true
      schema:
        type: string
        format: uuid
      x-validations: ["app.UUIDv7"]
  responses:
    "200":
      description: User public profile.
      content:
        application/json:
          schema:
            $ref: "../../../schemas/models/UserRef.yaml"
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "404":
      description: User not found.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error (invalid UUID).
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 3: Validate YAML syntax**

Run:
```bash
for f in usersMe users; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/resources/v1/profile/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/users/src/openapi/resources/v1/profile/
git commit -m "feat(api-schema/users): profile resource endpoints (me, by-id)"
```

---

### Task 10: users/ — session resources (sessions, session)

**Files:**
- Create: `backend/api-schema/users/src/openapi/resources/v1/sessions/sessions.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/sessions/session.yaml`

- [ ] **Step 1: Create `sessions.yaml`**

```yaml
get:
  tags:
    - sessions
  summary: List active sessions
  description: Возвращает список активных сессий (refresh token entries) текущего пользователя.
  operationId: listSessions
  security:
    - bearerAuth: []
  parameters:
    - $ref: "../../../../../../../common/src/openapi/parameters/list.yaml#/pageSizeParam"
    - $ref: "../../../../../../../common/src/openapi/parameters/list.yaml#/pageTokenParam"
  responses:
    "200":
      description: List of active sessions.
      content:
        application/json:
          schema:
            type: object
            required:
              - items
            properties:
              items:
                type: array
                items:
                  $ref: "../../../schemas/models/Session.yaml"
              next_page_token:
                type: string
                x-validations: ["Size <=256"]
            additionalProperties: false
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 2: Create `session.yaml`**

```yaml
delete:
  tags:
    - sessions
  summary: Revoke session by ID
  description: Отзыв конкретной сессии текущего пользователя по её ID. Отзывать можно только свои сессии.
  operationId: deleteSession
  security:
    - bearerAuth: []
  parameters:
    - name: id
      in: path
      required: true
      schema:
        type: string
        format: uuid
      x-validations: ["app.UUIDv7"]
  responses:
    "204":
      description: Session revoked. No content.
    "401":
      description: Invalid or expired access token.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "403":
      description: Session belongs to another user.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "404":
      description: Session not found.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "422":
      description: Validation error (invalid UUID).
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 3: Validate YAML syntax**

Run:
```bash
for f in sessions session; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/resources/v1/sessions/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/users/src/openapi/resources/v1/sessions/
git commit -m "feat(api-schema/users): session resource endpoints"
```

---

### Task 11: users/ — service resources (jwks, health)

**Files:**
- Create: `backend/api-schema/users/src/openapi/resources/v1/service/jwks.yaml`
- Create: `backend/api-schema/users/src/openapi/resources/v1/service/health.yaml`

- [ ] **Step 1: Create `jwks.yaml`**

```yaml
get:
  tags:
    - service
  summary: Get JWKS (JSON Web Key Set)
  description: Возвращает публичные ключи для верификации JWT access токенов. Без аутентификации. Другие сервисы polled этот endpoint для локальной верификации JWT.
  operationId: getJwks
  security: []
  responses:
    "200":
      description: JWKS (RFC 7517).
      content:
        application/json:
          schema:
            type: object
            required:
              - keys
            properties:
              keys:
                type: array
                items:
                  type: object
                  required:
                    - kty
                    - kid
                    - use
                    - alg
                    - crv
                    - x
                    - y
                  properties:
                    kty:
                      type: string
                      enum: ["EC"]
                    kid:
                      type: string
                      description: Key ID. Совпадает с kid в JWT header.
                    use:
                      type: string
                      enum: ["sig"]
                    alg:
                      type: string
                      enum: ["ES256"]
                    crv:
                      type: string
                      enum: ["P-256"]
                    x:
                      type: string
                      description: Base64url-encoded X coordinate.
                    y:
                      type: string
                      description: Base64url-encoded Y coordinate.
                  additionalProperties: false
            additionalProperties: false
    "500":
      description: Internal error.
      content:
        application/json:
          schema:
            $ref: "../../../../../../../common/src/openapi/schemas/errors/Error.yaml"
```

- [ ] **Step 2: Create `health.yaml`**

```yaml
get:
  tags:
    - service
  summary: Health check
  description: Liveness/readiness probe. Без аутентификации.
  operationId: healthCheck
  security: []
  responses:
    "200":
      description: Service is healthy.
      content:
        application/json:
          schema:
            type: object
            required:
              - status
            properties:
              status:
                type: string
                enum: ["ok"]
              version:
                type: string
                x-validations: ["Size <=64"]
            additionalProperties: false
    "503":
      description: Service is unhealthy.
      content:
        application/json:
          schema:
            type: object
            required:
              - status
            properties:
              status:
                type: string
                enum: ["degraded"]
              error:
                type: string
                x-validations: ["Size <=256"]
            additionalProperties: false
```

- [ ] **Step 3: Validate YAML syntax**

Run:
```bash
for f in jwks health; do
  python3 -c "import yaml; yaml.safe_load(open('backend/api-schema/users/src/openapi/resources/v1/service/$f.yaml'))"
done
```
Expected: no output (success).

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/users/src/openapi/resources/v1/service/
git commit -m "feat(api-schema/users): service resource endpoints (jwks, health)"
```

---

### Task 12: users/ — openapi.yaml + generation_flags.yaml

**Files:**
- Create: `backend/api-schema/users/src/openapi/openapi.yaml`
- Create: `backend/api-schema/users/generation_flags.yaml`

- [ ] **Step 1: Create `users/generation_flags.yaml`**

```yaml
# Per-service override for users/ — inherits global defaults, no overrides.
# Listed explicitly for documentation.

GOLANG_SPLIT_REQUEST_RESPONSE: true
USE_UTC_FOR_DATE_TIME: true
```

- [ ] **Step 2: Create `users/src/openapi/openapi.yaml`**

```yaml
openapi: 3.1.0
info:
  title: Zvonilka Users/Auth API
  description: |
    Users & Authentication service for zvonilka.

    ## Identity model
    - `login` — unique, HIDDEN (only in /users/me). Used for auth.
    - `email` — unique, required, HIDDEN. For password reset + verification.
    - `nickname` — display name, NOT unique.
    - `tag` — 7 chars A-Z0-9, server-generated, globally unique.

    ## Auth
    - Access token: JWT ES256, 15min TTL. Verified locally by other services via JWKS.
    - Refresh token: 7 days, rotation + reuse detection. Stored in PostgreSQL.
    - Login endpoint accepts `identifier` (email or login).

    ## Visibility
    - `/users/me` → full User (with login, email).
    - `/users/{id}` → UserRef (id, nickname, tag). NO login/email.
    - JWT claims: sub, iss, aud, exp, iat only.
  version: 1.0.0
servers:
  - url: https://users.zvonilka.space
    description: Production
  - url: http://localhost:8080
    description: Local dev
paths:
  /users/v1/auth/register:
    $ref: "./resources/v1/auth/register.yaml#/post"
  /users/v1/auth/login:
    $ref: "./resources/v1/auth/login.yaml#/post"
  /users/v1/auth/refresh:
    $ref: "./resources/v1/auth/refresh.yaml#/post"
  /users/v1/auth/logout:
    $ref: "./resources/v1/auth/logout.yaml#/post"
  /users/v1/users/me:
    $ref: "./resources/v1/profile/usersMe.yaml"
  /users/v1/users/{id}:
    $ref: "./resources/v1/profile/users.yaml#/get"
  /users/v1/sessions:
    $ref: "./resources/v1/sessions/sessions.yaml#/get"
  /users/v1/sessions/{id}:
    $ref: "./resources/v1/sessions/session.yaml#/delete"
  /users/v1/.well-known/jwks.json:
    $ref: "./resources/v1/service/jwks.yaml#/get"
  /users/v1/health:
    $ref: "./resources/v1/service/health.yaml#/get"
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: JWT access token (ES256). Obtained from /auth/login or /auth/refresh.
```

- [ ] **Step 3: Validate YAML syntax + resolve all $ref paths**

Run:
```bash
python3 << 'PYEOF'
import yaml, os

base = 'backend/api-schema/users/src/openapi'
with open(f'{base}/openapi.yaml') as f:
    spec = yaml.safe_load(f)

for path, ref_obj in spec['paths'].items():
    ref = ref_obj['$ref']
    file_part = ref.split('#')[0].replace('./', f'{base}/')
    assert os.path.exists(file_part), f'Path {path}: missing file {file_part}'
    if '#' in ref:
        method = ref.split('#/')[1]
        with open(file_part) as rf:
            resource = yaml.safe_load(rf)
        assert method in resource, f'Path {path}: method {method} not found in {file_part}'

print(f'All {len(spec["paths"])} paths resolve.')
PYEOF
```
Expected: `All 10 paths resolve.`

- [ ] **Step 4: User commits**

```bash
git add backend/api-schema/users/{generation_flags.yaml,src/openapi/openapi.yaml}
git commit -m "feat(api-schema/users): main openapi.yaml + per-service generation flags"
```

---

### Task 13: Final validation — all YAML + all $ref cross-references

**Files:**
- No new files. Validation only.

- [ ] **Step 1: Validate YAML syntax of all files**

Run:
```bash
find backend/api-schema -name '*.yaml' -print0 | while IFS= read -r -d '' f; do
  python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" "$f" || echo "FAIL: $f"
done
echo "YAML syntax check complete."
```
Expected: no `FAIL:` lines, `YAML syntax check complete.` at the end.

- [ ] **Step 2: Validate all $ref cross-references resolve**

Run:
```bash
python3 << 'PYEOF'
import yaml, os, re

base = 'backend/api-schema'
ref_pattern = re.compile(r'\$ref:\s*["\']?([^"\']+?)["\']?(?:\s|$)')

errors = []
checked = 0

for root, dirs, files in os.walk(base):
    if '/docs/' in root or '/generated/' in root:
        continue
    for fname in files:
        if not fname.endswith('.yaml'):
            continue
        fpath = os.path.join(root, fname)
        with open(fpath) as f:
            content = f.read()
        for match in ref_pattern.finditer(content):
            ref = match.group(1)
            if ref.startswith('#/'):
                continue
            file_part = ref.split('#')[0]
            resolved = os.path.normpath(os.path.join(os.path.dirname(fpath), file_part))
            if not os.path.exists(resolved):
                errors.append(f'{fpath}: missing $ref {ref} -> {resolved}')
            checked += 1

print(f'Checked {checked} $refs.')
if errors:
    print('ERRORS:')
    for e in errors:
        print(f'  {e}')
    exit(1)
print('All $refs resolve.')
PYEOF
```
Expected: `All $refs resolve.`

- [ ] **Step 3: Verify all expected files exist**

Run:
```bash
python3 << 'PYEOF'
import os

expected = [
    # Root
    'backend/api-schema/README.md',
    'backend/api-schema/generation_flags.yaml',
    # common/
    'backend/api-schema/common/generation_flags.yaml',
    'backend/api-schema/common/src/openapi/openapi.yaml',
    'backend/api-schema/common/src/openapi/parameters/list.yaml',
    'backend/api-schema/common/src/openapi/schemas/auth/TokenPair.yaml',
    'backend/api-schema/common/src/openapi/schemas/errors/Error.yaml',
    'backend/api-schema/common/src/openapi/schemas/errors/ValidationError.yaml',
    'backend/api-schema/common/src/openapi/schemas/identifiers/UUIDv7.yaml',
    'backend/api-schema/common/src/openapi/schemas/pagination/ListRequest.yaml',
    'backend/api-schema/common/src/openapi/schemas/pagination/ListResponse.yaml',
    'backend/api-schema/common/src/openapi/schemas/time/Timestamp.yaml',
    # users/ schemas
    'backend/api-schema/users/src/openapi/schemas/auth/RegisterRequest.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/RegisterResponse.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/LoginRequest.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/LoginResponse.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/RefreshRequest.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/RefreshResponse.yaml',
    'backend/api-schema/users/src/openapi/schemas/auth/LogoutRequest.yaml',
    'backend/api-schema/users/src/openapi/schemas/models/User.yaml',
    'backend/api-schema/users/src/openapi/schemas/models/UserRef.yaml',
    'backend/api-schema/users/src/openapi/schemas/models/UserStatus.yaml',
    'backend/api-schema/users/src/openapi/schemas/models/Session.yaml',
    'backend/api-schema/users/src/openapi/schemas/profile/UpdateUserRequest.yaml',
    # users/ resources
    'backend/api-schema/users/src/openapi/resources/v1/auth/register.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/auth/login.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/auth/refresh.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/auth/logout.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/profile/usersMe.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/profile/users.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/sessions/sessions.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/sessions/session.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/service/jwks.yaml',
    'backend/api-schema/users/src/openapi/resources/v1/service/health.yaml',
    # users/ top-level
    'backend/api-schema/users/generation_flags.yaml',
    'backend/api-schema/users/src/openapi/openapi.yaml',
]

missing = [f for f in expected if not os.path.exists(f)]
if missing:
    print(f'MISSING {len(missing)} files:')
    for f in missing:
        print(f'  {f}')
    exit(1)
print(f'All {len(expected)} expected files exist.')
PYEOF
```
Expected: `All 36 expected files exist.`

- [ ] **Step 4: User final commit**

```bash
git add backend/api-schema/
git commit -m "feat(api-schema): complete Users/Auth MVP OpenAPI schemas"
```

---

## Self-review checklist (after implementation)

- [ ] All 36 expected files created (Task 13 Step 3).
- [ ] All YAML files syntactically valid (Task 13 Step 1).
- [ ] All `$ref` cross-references resolve (Task 13 Step 2).
- [ ] `common/openapi.yaml` has empty `paths:` and all 7 schemas in `components.schemas` (Task 5).
- [ ] `users/openapi.yaml` has all 10 paths with `$ref` to resource files in subfolders (Task 12).
- [ ] `User.yaml` includes `login`, `email` (hidden fields). `UserRef.yaml` does NOT include them (Task 5b).
- [ ] `LoginRequest.yaml` uses `identifier` field, not `email`/`login` separately (Task 6).
- [ ] `refresh.yaml` returns `409` on reuse detection (Task 8).
- [ ] `logout.yaml` has `security: bearerAuth` (requires access token) (Task 8).
- [ ] `jwks.yaml` and `health.yaml` have `security: []` (no auth) (Task 11).
- [ ] `generation_flags.yaml` (global) has `GOLANG_SPLIT_REQUEST_RESPONSE` and `USE_UTC_FOR_DATE_TIME` enabled (Task 1).
- [ ] Tag charset is `A-Z0-9` (all uppercase, 36 chars) (Task 5b, User.yaml + UserRef.yaml).
- [ ] `TokenPair.yaml` lives in `common/schemas/auth/` (shared, not in users/) (Task 4).
- [ ] `UUIDv7.yaml` lives in `common/schemas/identifiers/` (Task 4).
- [ ] Users resources are in subfolders (`auth/`, `profile/`, `sessions/`, `service/`) — no `authRegister.yaml` prefixes (Task 8-11).

## Post-implementation notes

- **Generator run:** when `~/projects/oapigenerator` is handed over, run it with `-dry-run` first to catch any OpenAPI structural issues the YAML syntax check misses.
- **Go service repo:** separate repo `~/zvonilka/backend/users/` (or similar) will consume `api-schema/` as a git submodule or vendored dependency. Out of scope for this plan.
- **Migrations:** goose migrations for PostgreSQL schema (see spec §4) live in the Go service repo, not here.
- **K8s manifests:** in `infra/` repo, created when the Go service is ready to deploy.
