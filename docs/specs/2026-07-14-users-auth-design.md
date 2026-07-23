
# #4.1 Users/Auth — Design Spec

**Date:** 2026-07-14
**Subproject:** #4.1 (первый сервис подпроекта #4 — backend-сервисы)
**Status:** Approved (brainstormed 2026-07-14)
**Depends on:** #4.0 Backend infra (PostgreSQL в infra/), api-schema/ репо (создаётся в этой задаче)
**Blocks:** #4.2 IAM (потребует JWT/JWKS от Users/Auth), #4.6 Notifications (не блокируется, но Users/Auth будет слать события в стрим)

## 1. Цель

Первый backend-сервис платформы zvonilka: управление профилями пользователей и аутентификация. Выдаёт JWT access-токены, которые другие сервисы (#4.2 IAM, #4.3 Rooms, ...) верифицируют локально через JWKS. Не занимается авторизацией (роли, permissions) — это IAM. Не занимается отправкой email/SMS — это Notifications (#4.6).

## 2. Границы

### Входит в #4.1

- **A. Profile management** — CRUD пользователей (создание, чтение, обновление, soft-delete), `GET /users/me`.
- **B. Registration** — `login` + `email` + `password` + `nickname`. Без email verification (это задача Notifications).
- **C. Login / logout** — аутентификация по `identifier` (может быть `email` или `login`) + `password`. Logout (инвалидация refresh-цепочки).
- **D. Token management** — JWT access (ES256, 15min) + refresh (7 дней, rotation + reuse detection).
- **H. Sessions** — список активных сессий текущего пользователя, отзыв конкретной сессии.

### Не входит (отдельные задачи)

- **E. Password reset** — второй проход; зависит от #4.6 Notifications (отправка ссылки).
- **F. Email verification** — уходит в #4.6 Notifications как одна из категорий сообщений. Users/Auth только кладёт событие в стрим «отправь verification email такому-то».
- **G. OAuth / SSO** (Google, GitHub) — второй проход.
- **I. 2FA / MFA** (TOTP, SMS) — второй проход.
- **J. Roles / permissions** — в #4.2 IAM.

### Взаимодействие с другими сервисами

```
Клиент ──[JWT]──► Rooms (или любой сервис)
                    │
                    1. Верифицирует JWT подпись локально через JWKS
                    2. Извлекает user_id, email из claims
                    3. Идёт в IAM: "user X хочет action Y на resource Z"
                    │
                    ► IAM (stateless policy engine)
                      - bindings: user→role→permissions
                      - возвращает allow/deny
                    │
                    4. Rooms выполняет или 403
```

Users/Auth **не** знает про роли и permissions. JWT claims: `sub` (user_id), `iss`, `aud`, `exp`, `iat`. `login`, `email`, `nickname`, `tag` **не** включаются в JWT — другие сервисы при необходимости тянут `UserRef` через `GET /users/{id}`. Ролей в JWT тоже нет — иначе при смене роли пришлось бы перевыпускать все активные токены.

## 3. Технические решения

### 3.1. Стек

- **Go** (версия — на усмотрение имплементатора, ≥1.22)
- **PostgreSQL 16** (schema `users`, созданная через `infra/scripts/create-service-db.sh users`)
- **HTTP layer** — генерируется из OpenAPI 3.1 через `~/projects/oapigenerator` (генератор будет предоставлен позже)
- **Migrations** — `goose` (github.com/pressly/goose), SQL-based up/down
- **Primary keys** — UUID **v7** (time-ordered, sortable)

### 3.2. Аутентификация

| Параметр | Решение |
|---|---|
| Password hashing | Argon2id, RFC 9106 first recommendation: `m=64MiB, t=3, p=2`. `golang.org/x/crypto/argon2` |
| JWT signing | ES256 (ECDSA P-256). Приватный ключ только в Users/Auth; публичный отдаётся по JWKS |
| JWKS endpoint | `GET /users/v1/.well-known/jwks.json` |
| JWT library | `github.com/lestrrat-go/jwx` или `github.com/go-jose/go-jose` (на выбор имплементатора) |
| Access token TTL | 15 минут |
| Refresh token TTL | 7 дней |
| Refresh storage | PostgreSQL, таблица `users.refresh_tokens`. Храним SHA-256 hash, не сам токен |
| Refresh rotation | Да. Каждый `/auth/refresh` выдаёт новый refresh, старый помечается `revoked_at`. `chain_id` связывает всю цепочку |
| Reuse detection | Если представлен revoked/used refresh → инвалидируем всю цепочку (`UPDATE refresh_tokens SET revoked_at = now() WHERE chain_id = ? AND revoked_at IS NULL`) |
| Logout | Инвалидация текущей refresh-цепочки (или всех цепочек пользователя при logout-all) |

### 3.3. Секреты

- Argon2id-параметры — константы в коде (не секрет).
- ES256 private key — SealedSecret в namespace `users`, монтируется как файл. Бэкап ключа — вне скоупа этого спека (отдельная задача инфры).
- TOTP secrets / backup codes — НЕ в MVP (2FA отложен).

## 4. Схема PostgreSQL

Schema `users` (создаётся `infra/scripts/create-service-db.sh users`). Все PK — UUID v7.

### 4.1. Таблица `users`

| Колонка | Тип | Описание |
|---|---|---|
| `id` | UUID PK | UUID v7 |
| `login` | TEXT UNIQUE NOT NULL | уникальный, скрытый идентификатор для аутентификации. 3-32 символа, charset `a-z0-9._-`, lowercased при нормализации |
| `email` | TEXT UNIQUE NOT NULL | lowercased. Требуется для регистрации и password reset (E) |
| `nickname` | TEXT NOT NULL | отображаемое имя, 1-32 символа, Unicode. Может повторяться у разных пользователей |
| `tag` | TEXT UNIQUE NOT NULL | 7 символов из charset `A-Z0-9` (36 chars, все заглавные), генерируется сервером. Глобально уникальный. Для disambiguation при совпадении nicknames |
| `password_hash` | TEXT NOT NULL | argon2id encoded string |
| `password_changed_at` | TIMESTAMPTZ | для invalidation старых сессий при смене пароля (future) |
| `email_verified` | BOOLEAN DEFAULT false | пока всегда false (верификация — #4.6) |
| `status` | TEXT NOT NULL DEFAULT 'active' | enum: `active` \| `suspended` \| `deleted` |
| `last_login_at` | TIMESTAMPTZ | обновляется при login |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `deleted_at` | TIMESTAMPTZ NULL | soft delete |

Indexes: `users_login_idx UNIQUE (login) WHERE deleted_at IS NULL`, `users_email_idx UNIQUE (email) WHERE deleted_at IS NULL`, `users_tag_idx UNIQUE (tag) WHERE deleted_at IS NULL`, `users_status_idx (status)`.

**Tag generation:** сервер генерирует тег при регистрации в цикле: random 7 chars → проверка `users_tag_idx` → при коллизии перегенерить. Charset: `ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789` (36 символов, все заглавные). 36^7 ≈ 78 млрд комбинаций, коллизия маловероятна, но проверка обязательна. Все символы заглавные — чтобы не путать `l`/`1`/`I` и `0`/`O`/`o` между собой.

**Login hidden:** `login` возвращается только в `GET /users/v1/users/me` для собственного профиля. Не возвращается в `UserRef`, не включается в JWT claims, не логируется. Другие сервисы видят только `nickname` + `tag`.

### 4.2. Таблица `refresh_tokens`

| Колонка | Тип | Описание |
|---|---|---|
| `id` | UUID PK | UUID v7 |
| `user_id` | UUID NOT NULL FK → users.id | |
| `token_hash` | TEXT UNIQUE NOT NULL | SHA-256 hex of raw refresh token |
| `parent_id` | UUID NULL FK → refresh_tokens.id | предыдущий токен в цепочке (для rotation) |
| `chain_id` | UUID NOT NULL | вся цепочка инвалидируется при reuse |
| `user_agent` | TEXT | из `User-Agent` header |
| `ip` | INET | IP клиента |
| `expires_at` | TIMESTAMPTZ NOT NULL | |
| `revoked_at` | TIMESTAMPTZ NULL | NULL = active |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

Indexes: `refresh_tokens_token_hash_idx UNIQUE (token_hash)`, `refresh_tokens_user_id_idx (user_id, revoked_at)`, `refresh_tokens_chain_id_idx (chain_id)`.

### 4.3. Таблицы вне MVP

Зарезервированы на будущее, в миграциях MVP **не создаются**:

- `oauth_accounts` — SSO-привязки (для G).
- `totp_factors` — 2FA TOTP (для I).
- `password_resets` — сброс пароля (для E).

## 5. API-контракт (OpenAPI 3.1)

Все пути — под префиксом `/users/v1/`. Версия в URL — v1.

### 5.1. Auth endpoints

| Метод | Path | Описание |
|---|---|---|
| Метод | Path | Описание |
|---|---|---|
| `POST` | `/users/v1/auth/register` | Регистрация: `login` + `email` + `password` + `nickname`. Сервер генерирует `tag`. Создаёт user, выдаёт access + refresh. |
| `POST` | `/users/v1/auth/login` | Логин по `identifier` (может быть `email` или `login`) + `password`. Выдаёт access + refresh. |
| `POST` | `/users/v1/auth/refresh` | Refresh rotation. Принимает refresh, выдаёт новый access + новый refresh. Старый инвалидируется. Reuse → инвалидация цепочки. |
| `POST` | `/users/v1/auth/logout` | Инвалидация текущей refresh-цепочки. Требует access токен (в `Authorization` header) + `refresh_token` в body. Logout-all (все цепочки пользователя) — будущие расширения (см. §9). |

### 5.2. Profile endpoints

| Метод | Path | Описание |
|---|---|---|
| `GET` | `/users/v1/users/me` | Текущий профиль (по JWT). Возвращает полный `User` включая скрытые `login`, `email`. |
| `PATCH` | `/users/v1/users/me` | Обновить `nickname`, опционально `login` (смена login — отдельная операция с подтверждением, в MVP разрешена). Email/password — отдельные эндпоинты (вне MVP). |
| `GET` | `/users/v1/users/{id}` | Чтение чужого профиля (для других сервисов, по JWT-авторизации через IAM). Возвращает `UserRef` (`id`, `nickname`, `tag`) — БЕЗ `login` и `email`. |

### 5.3. Sessions endpoints

| Метод | Path | Описание |
|---|---|---|
| `GET` | `/users/v1/sessions` | Список активных сессий текущего пользователя. |
| `DELETE` | `/users/v1/sessions/{id}` | Отзыв конкретной сессии (по `refresh_tokens.id`). |

### 5.4. Service endpoints

| Метод | Path | Описание |
|---|---|---|
| `GET` | `/users/v1/.well-known/jwks.json` | JWKS для верификации JWT другими сервисами. Без аутентификации. |
| `GET` | `/users/v1/health` | Liveness/readiness probe. Без аутентификации. |

### 5.5. Модели (ключевые)

- `User` — полный профиль текущего пользователя (`id`, `login`, `email`, `nickname`, `tag`, `status`, `email_verified`, `created_at`, `updated_at`). Возвращается только из `/users/me`. `password_hash` НЕ отдаётся наружу никогда.
- `UserRef` — компактная ссылка для других пользователей/сервисов (`id`, `nickname`, `tag`). БЕЗ `login` и `email`. Используется в `GET /users/{id}`.
- `RegisterRequest` — `login`, `email`, `password`, `nickname`.
- `RegisterResponse` — `user` (full `User`), `access_token`, `refresh_token`.
- `LoginRequest` — `identifier` (email или login), `password`.
- `LoginResponse` — `user` (full `User`), `access_token`, `refresh_token`.
- `RefreshRequest` — `refresh_token`.
- `RefreshResponse` — `access_token`, `refresh_token`.
- `LogoutRequest` — `refresh_token`.
- `UpdateUserRequest` — `nickname` (optional), `login` (optional).
- `Session` — `id`, `user_agent`, `ip`, `created_at`, `expires_at`, `current` (boolean, для текущей).
- `Error` (в `common/`) — стандартная ошибка: `code`, `message`, `details`.

### 5.6. Коды ответов

- `200` — успех.
- `201` — создано (register).
- `204` — успех без body (logout, delete session).
- `400` — невалидный запрос (bad email format, короткий пароль, невалидный login charset).
- `401` — невалидный/просроченный access токен.
- `403` — доступ запрещён (для endpoints, требующих IAM — пока не используется в MVP, т.к. `/users/{id}` доступен любому аутентифицированному).
- `404` — не найдено (user, session).
- `409` — конфликт (`login` уже занят, `email` уже занят, `tag` collision — сервер перегенерирует автоматически, но при исчерпании попыток — 500; refresh token reuse detected → цепочка инвалидирована, клиент должен заново логиниться).
- `422` — semantic validation error (прим.: 409 используется для reuse и коллизий, т.к. это конфликт состояния, не формат-ошибка).
- `500` — внутренняя ошибка.

### 5.7. Authorization (MVP)

В MVP IAM (#4.2) ещё нет. Авторизационные правила:

- `/users/me`, `/sessions` — только владелец (по `sub` из JWT).
- `/users/{id}` — любой аутентифицированный пользователь может читать минимальный профиль (`UserRef`: `id`, `nickname`, `tag`). `login` и `email` скрыты. После подключения IAM — закрыть через policy «читать чужой профиль только при наличии role/permission».
- `/sessions/{id}` — только владелец сессии (проверка `refresh_tokens.user_id == JWT.sub`).
- `.well-known/jwks.json`, `/health` — без аутентификации.

### 5.8. Валидация

Используем `x-validations` расширение генератора:

- `login`: `Size >=3`, `Size <=32`, `app.LoginFormat` (named validator: regex `^[a-z0-9._-]+$`, lowercase enforced via normalization перед валидацией).
- `email`: `app.EmailFormat` (named validator, реализован в сервисе).
- `password`: `Size >=8`, `Size <=128`.
- `nickname`: `Size >=1`, `Size <=32`, `app.NoControlChars` (named validator: запрет управляющих Unicode-символов, trim leading/trailing whitespace).
- `tag`: НЕ валидируется на input — генерируется сервером. На output: `Size ==7`, charset `A-Z0-9`.
- `identifier` (в `LoginRequest`): `Size >=3`, `Size <=320` (max email length), `app.IdentifierFormat` (named validator: принимает email или login format).

## 6. Структура репозитория api-schema/

```
api-schema/
├── README.md
├── generation_flags.yaml                # глобальные дефолты
├── common/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml                 # paths: {} + components.schemas с $ref
│       ├── schemas/
│       │   ├── errors/
│       │   │   ├── Error.yaml
│       │   │   ├── ValidationError.yaml
│       │   │   └── ...
│       │   ├── pagination/
│       │   │   ├── ListRequest.yaml     # page_size, page_token
│       │   │   ├── ListResponse.yaml    # next_page_token
│       │   │   └── ...
│       │   └── time/
│       │       └── Timestamp.yaml       # date-time RFC3339
│       └── parameters/
│           └── list.yaml                # pageSize, pageToken, ...
├── users/                               # #4.1 Users/Auth
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml                 # paths $ref resources/v1/*
│       ├── schemas/
│       │   ├── User.yaml                # full profile (with login, email — only for /me)
│       │   ├── UserRef.yaml             # public profile (id, nickname, tag — no login/email)
│       │   ├── UserStatus.yaml          # enum: active|suspended|deleted
│       │   ├── RegisterRequest.yaml
│       │   ├── RegisterResponse.yaml
│       │   ├── LoginRequest.yaml        # identifier (email|login) + password
│       │   ├── LoginResponse.yaml
│       │   ├── RefreshRequest.yaml
│       │   ├── RefreshResponse.yaml
│       │   ├── LogoutRequest.yaml
│       │   ├── UpdateUserRequest.yaml
│       │   ├── Session.yaml
│       │   └── TokenPair.yaml           # access_token + refresh_token, reused
│       └── resources/v1/
│           ├── authRegister.yaml
│           ├── authLogin.yaml
│           ├── authRefresh.yaml
│           ├── authLogout.yaml
│           ├── usersMe.yaml             # GET, PATCH
│           ├── users.yaml               # GET /users/{id}
│           ├── sessions.yaml            # GET /sessions
│           ├── session.yaml             # DELETE /sessions/{id}
│           ├── jwks.yaml                # GET /.well-known/jwks.json
│           └── health.yaml              # GET /health
└── generated/                           # output codegen (per-service subdirs)
```

### 6.1. Глобальные generation flags

```yaml
# api-schema/generation_flags.yaml
- name: GOLANG_SPLIT_REQUEST_RESPONSE
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}
- name: USE_UTC_FOR_DATE_TIME
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}
```

Split Request/Response — раздельные модели для запросов и ответов (без `readOnly`/`writeOnly` overlap). UTC datetime — все `date-time` поля сериализуются в UTC через `model.UTCTime`.

### 6.2. Per-service override

В MVP все сервисы используют глобальные дефолты. Per-service `generation_flags.yaml` — пустой или копия глобального (для будущих override).

## 7. MVP scope

### Что делаем в этой задаче (#4.1 MVP)

1. Создаём репозиторий `api-schema/` со структурой из §6.
2. Пишем OpenAPI 3.1 схемы для Users/Auth (все эндпоинты из §5).
3. Общие модели (`common/`): `Error`, `ValidationError`, `ListRequest`, `ListResponse`, `Timestamp`.
4. README в `api-schema/` с инструкцией по запуску генератора (когда он будет передан).

### Что **не** делаем в этой задаче

- Сам Go-сервис (репозиторий `users/` будет создан отдельно, после получения генератора).
- Миграции goose (в репо сервиса).
- Kubernetes-манифесты (в `infra/` — когда сервис будет готов к деплою).
- Деплой.

### Критерий готовности

- OpenAPI-схема валидируется (через `libopenapi` или онлайн-валидатор).
- Все эндпоинты из §5 описаны.
- Общие модели в `common/` и ссылаются из `users/` через `$ref`.
- `generation_flags.yaml` настроены.
- README описывает структуру и как запустить генератор.

## 8. Trade-offs и риски

### 8.1. JWT без ролей в claims

**Решение:** роли хранит IAM, JWT содержит только `sub` + `email`.
**Trade-off:** при каждом запросе сервис идёт в IAM за решением (extra hop). Но: смена роли мгновенно применяется (не нужно ждать истечения access токена). Альтернатива — роли в JWT — отвергнута из-за перевыпуска при каждом change role.

### 8.2. Refresh rotation с reuse detection

**Решение:** каждый refresh выдаёт новый токен, старый инвалидируется; reuse → инвалидация цепочки.
**Trade-off:** при параллельных запросах с одного устройства можно порвать цепочку (treat as reuse). Mitigation: клиент должен сериализовать refresh-запросы. Это стандартный trade-off OAuth 2.0 BCP.

### 8.3. PostgreSQL для refresh, не Redis

**Решение:** refresh tokens в PostgreSQL.
**Trade-off:** каждый refresh = запрос в БД. Но refresh происходит раз в 15 мин на пользователя — нагрузка минимальна. Зато: durable, SQL-аналитика, logout-all тривиален.

### 8.4. Email verification отложена

**Решение:** `email_verified` всегда `false` в MVP, верификация — в #4.6.
**Trade-off:** в MVP можно зарегистрироваться с любым email. Acceptable для внутреннего тестирования, не для production. После #4.6 — включаем верификацию.

### 8.5. ES256 vs RS256

**Решение:** ES256 (ECDSA P-256).
**Trade-off:** компактнее, быстрее, но менее распространён в legacy-системах. Для greenfield-проекта — предпочтителен.

### 8.6. OpenAPI-схема до генератора

**Решение:** пишем OpenAPI сейчас, генератор получим позже.
**Риск:** если генератор не поддержит что-то в наших схемах — придётся адаптировать. Mitigation: генератор уже поддерживает стандартный OpenAPI 3.x + `x-validations`, мы используем именно это. Нестандартных расширений нет.

### 8.7. Login скрыт, email скрыт, tag публичен

**Решение:** `login` и `email` видит только сам пользователь через `/users/me`. Другим пользователям/сервисам отдаётся `UserRef` (`id`, `nickname`, `tag`). `tag` генерируется сервером, глобально уникальный, используется для disambiguation при совпадении `nickname`.
**Trade-off:** пользователь может логиниться по `email` или по `login` (оба скрыты от других). `nickname` может повторяться — disambiguation через `tag`. Альтернатива — уникальный `nickname` — отвергнута, т.к. ограничивает выбор отображаемого имени.

## 9. Будующие расширения (после MVP)

В порядке приоритета:

1. **E. Password reset** — после #4.6 Notifications.
2. **G. OAuth / SSO** — Google, GitHub.
3. **I. 2FA / MFA** — TOTP + backup codes.
4. Рефреш-токен в httpOnly cookie (вместо JSON-body) — для web-клиентов.
5. Rate limiting на auth-эндпоинты (через Redis token bucket).
6. Audit log всех auth-событий (login, logout, refresh) — отдельный audit-сервис или таблица.

## 10. Open questions

Нет. Все ключевые решения приняты в brainstorming-сессии 2026-07-14.
