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
│       │   └── time/                # Timestamp
│       └── parameters/              # общие query/header params
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
├── docs/                            # спецификации и планы
│   ├── specs/                       # design specs
│   └── plans/                       # implementation plans
└── generated/                       # output codegen (per-service subdirs)
```

## Соглашения

- **OpenAPI 3.1** (не 3.0) — используем `type: object` + `additionalProperties: false` для strict объектов.
- **Одна схема = один файл** в `schemas/<domain>/`. Имя файла = имя компонента (e.g., `User.yaml` → `components.schemas.User`).
- **Cross-service refs** — через `common/`: `$ref: '../../../../common/src/openapi/schemas/errors/Error.yaml'`.
- **Generation flags** — `GOLANG_SPLIT_REQUEST_RESPONSE: true`, `USE_UTC_FOR_DATE_TIME: true` (глобально).
- **x-validations** — используем для декларативной валидации (`Size >=N`, `app.EmailFormat`, и т.д.).
- **Resource files** — без префикса имени группы (e.g., `auth/register.yaml`, не `auth/authRegister.yaml`), т.к. папка уже указывает группу.

## Генерация кода

Команда — ровно как в CI (`.github/workflows/ci.yml`, `pr-tag.yml`); CI на PR
перегенерирует и падает, если закоммичено не то, что генерируется:

```bash
GOPRIVATE=github.com/zvonilkaRU,github.com/ilovepitsa \
  go run github.com/ilovepitsa/oapicodegen/cmd/oapigen@v1.3.6 \
  -input ./zvonilkaRU \
  -output ./generated \
  -import-prefix github.com/zvonilkaRU/api-schema/generated \
  -generation-flags-config-path ./generation_flags.yaml
```

Конфиг флагов один — корневой `generation_flags.yaml`; пер-сервисные
`{service}/generation_flags.yaml` могут точечно переопределять (файл может
отсутствовать). Проектного уровня `zvonilkaRU/generation_flags.yaml` нет —
генератор его не читал (каталог без своей спеки не считается проектом).

## Текущее состояние

- **users/** (#4.1 Users/Auth) — MVP: registration, login, refresh rotation, logout, profile, sessions, JWKS, health. См. `docs/specs/2026-07-14-users-auth-design.md`.

## Валидация

YAML syntax check (быстрая):
```bash
find . -name '*.yaml' -exec python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" {} \;
```

Полная OpenAPI-валидация — через генератор (`-dry-run`) когда он будет передан.
