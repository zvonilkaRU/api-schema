# Bug report: oapicodegen v1.2.1 — rolodex некорректно резолвит cross-file $ref для one-schema-per-file layout

**Репозиторий генератора:** github.com/ilovepitsa/oapicodegen
**Версия:** v1.2.1 (парсер на libopenapi rolodex)
**Проект-потребитель:** github.com/zvonilkaRU/api-schema
**Дата:** 2026-07-31

## Кратко

При layout **one-schema-per-file** (каждая OpenAPI-схема в отдельном файле, подключённая через
`components/schemas/<Name>: {$ref: './path/to/<Name>.yaml'}`) rolodex libopenapi **некорректно
резолвит cross-file `$ref`** внутри schema-файлов: BasePath противоречив, refs падают в `any`.
В testdata генератора используется **multi-schema-per-file** (несколько схем в одном `models.yaml`),
где этой проблемы нет — поэтому баг не покрыт CI генератора.

## Воспроизведение

### Спека (one-schema-per-file)

`users/src/openapi/schemas/models/UserStatus.yaml`:
```yaml
UserStatus:
  type: string
  enum: [active, suspended, deleted]
```

`users/src/openapi/schemas/models/User.yaml` (соседний файл, та же директория `schemas/models/`):
```yaml
User:
  type: object
  properties:
    status:
      $ref: "./UserStatus.yaml#/UserStatus"   # relative к текущему файлу
```

`users/src/openapi/openapi.yaml`:
```yaml
components:
  schemas:
    UserStatus:
      $ref: "./schemas/models/UserStatus.yaml#/UserStatus"
    User:
      $ref: "./schemas/models/User.yaml#/User"
```

### Симптом 1: `./UserStatus.yaml` резолвится относительно НЕ директории файла

`$ref: "./UserStatus.yaml#/UserStatus"` внутри `User.yaml` (лежит в `schemas/models/`)
ожидаемо резолвится в `schemas/models/UserStatus.yaml`. Вместо этого генератор ищет:

```
warn: schema-any [users] User.properties.status: unresolved external $ref:
  zvonilkaRU/users/src/openapi/UserStatus.yaml#/UserStatus
```

То есть rolodex резолвит относительно `users/src/openapi/` (BasePath корневого документа), а не
относительно `schemas/models/` (директории файла-источника). Поле падает в `any`.

### Симптом 2: явный путь от openapi.yaml даёт ДВОЙНОЙ путь

Если указать `$ref` относительно корневого документа
(`$ref: "./schemas/models/UserStatus.yaml#/UserStatus"` внутри `User.yaml`), rolodex резолвит
его относительно директории файла (`schemas/models/`) — и получается двойной путь:

debug-лог (`-log-level debug`):
```
ERROR unable to open the rolodex file, check specification references and base path
  file="/Users/.../zvonilkaRU/users/src/openapi/schemas/models/schemas/models/UserStatus.yaml"
  error="open .../schemas/models/schemas/models/UserStatus.yaml: no such file or directory"
oapigen: cannot resolve reference `./schemas/models/UserStatus.yaml#/UserStatus`,
  it's missing: $schemas.models['UserStatus.yaml$'].UserStatus [36:7]
```

То есть **один и тот же класс `$ref` резолвится с разным BasePath** в разных проходах rolodex
(один раз — относительно корня документа, другой раз — относительно директории файла).
Для one-schema-per-file layout нет `$ref`-формата, который rolodex резолвит корректно.

## Почему не пойман CI генератора

`testdata/integration` использует **multi-schema-per-file**:
`testdata/integration/users/src/openapi/schemas/users/models.yaml` содержит `User`, `UserRole`,
`UserStatus`, `Pagination` в одном файле. Internal refs — `#/UserStatus` (без файла), cross-file —
`./../auth/address.yaml#/Address` (между файлами-группами). Для этого layout rolodex работает
(внутри файла refs не требуют file-path резолвинга).

`testdata/project/minimal` — все схемы inline в `components/schemas` одного openapi.yaml.

One-schema-per-file layout нигде в testdata не представлен → регрессия не ловится.

## Предлагаемое исправление

1. Унифицировать BasePath rolodex для cross-file `$ref` внутри schema-файлов: всегда резолвить
   относительно **директории файла-источника** (как ожидается по OpenAPI-семантике `$ref`), а не
   относительно BasePath корневого документа. Либо задокументировать, что one-schema-per-file
   не поддерживается и rolodex требует multi-schema-per-file.
2. Покрыть в testdata: golden-кейс с one-schema-per-file layout (каждая схема в отдельном файле,
   подключённая через `components/schemas/<Name>: {$ref: './<Name>.yaml'}`, internal cross-file
   `$ref: './Sibling.yaml#/Sibling'`), чтобы регрессия ловилась CI.

## Окружение потребителя

- oapicodegen v1.2.1, Go 1.26, GOPROXY=direct
- zvonilkaRU/api-schema: 42 schema-файла в one-schema-per-file layout, 217 cross-file `$ref`
- Текущий workaround: оставить `GOLANG_SCHEMA_ANY=warn`, поля резолвятся в `any` (код собирается,
  но теряется типизация id/timestamps/enums). Миграция в multi-schema-per-file layout возможна, но
  это переработка структуры спек всех сервисов — отложена до решения по подходу (fix upstream vs
  migrate layout).
