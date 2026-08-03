# Bug report: oapicodegen v1.3.0 — splittable-converter эмитит undefined `anyToResponse`; cross-file `$ref` не линкуется к зарегистрированным схемам

**Репозиторий генератора:** github.com/ilovepitsa/oapicodegen
**Версия:** v1.3.0 (commit `af6fd6c`, feat t28: subpackage splitting for model/)
**Проект-потребитель:** github.com/zvonilkaRU/api-schema
**Дата:** 2026-07-31
**Результат:** обновление v1.2.1 → v1.3.0 заблокировано, откат к v1.2.1.

---

## TL;DR

Два связанных дефекта. Первый — **регрессия, ломающая сборку** (не давала обновиться).
Второй — **предсуществующая проблема типизации** (#69), которую v1.3.0 обещал починить, но не починил.

1. **Регрессия splittable-converter'а (блокер).** Новая в v1.3.0 логика конвертации splittable-полей
   эмитит `resp.<Field> = <TypeName>RequestToResponse(req.<Field>)`. Если поле резолвится в Go `any`
   (TypeName = `any`), генератор эмитит `anyToResponse(req.<Field>)`, но **функцию `anyToResponse`
   не определяет** ни в одном файле → `undefined: anyToResponse`, `go build` падает. В v1.2.1 для
   `any`-полей эмитился прямой копи `resp.<Field> = req.<Field>` — компилировалось.
2. **Cross-file `$ref` не линкуется к зарегистрированным схемам (предсуществующий, #69).** Схема
   `User`, зарегистрированная в `openapi.yaml` через `components.schemas.User: {$ref: "./schemas/models/User.yaml"}`,
   генерируется как типизированный struct (`UserRequest`/`UserResponse` + `UserRequestToResponse`).
   Но `$ref: "../models/User.yaml"` из соседнего файла (`schemas/auth/LoginResponse.yaml`) **не
   линкуется** к ней — поле падает в `any`. v1.3.0 обещал фикс `source_marking` для intra-service
   cross-file `$ref`, но для cross-subpackage случая (`schemas/auth/` → `schemas/models/`) он
   **не сработал** — `User` остался `any`. Дефект #1 — прямое следствие: `User` = `any` →
   splittable-converter эмитит `anyToResponse`.

---

## Анализ: проблема в генераторе или в наших спеках?

Проверялось эмпирически (regen с разными форматами `$ref` в `LoginResponse.yaml`, v1.3.0).

### Формат спек

zvonilka использует **one-schema-per-file** в **кастомном формате** (`name: <X>` + плоская схема,
не стандартный OpenAPI `<X>:` map). Схемы регистрируются в корневом `openapi.yaml`:
```yaml
components:
  schemas:
    User:
      $ref: "./schemas/models/User.yaml"
    LoginResponse:
      $ref: "./schemas/auth/LoginResponse.yaml"
```

Одна и та же схема `User.yaml` referenced из разных мест **тремя разными относительными путями**:
- `./schemas/models/User.yaml` — из `openapi.yaml` (регистрация в `components`). **Резолвится** — `User` генерируется как тип.
- `../models/User.yaml` — из `schemas/auth/LoginResponse.yaml` (cross-subpackage). **Не резолвится** → `any`.
- `../../../schemas/models/User.yaml` — из `resources/v1/auth/login.yaml` (resource → schema). Не проверялся, но та же природа.

### Эксперимент: стандартный internal `$ref`

Заменил в `LoginResponse.yaml`:
```yaml
user:
  $ref: "#/components/schemas/User"   # вместо "../models/User.yaml"
```
Результат v1.3.0:
```
ERROR  unable to locate reference anywhere in the rolodex
       reference: .../schemas/auth/LoginResponse.yaml#/components/schemas/User
```
Т.е. rolodex интерпретирует `#/components/schemas/User` как **фрагмент относительно текущего файла**
(`LoginResponse.yaml#/...`), а не относительно корневого `openapi.yaml`. Поле осталось `any`.

### Вывод о формате спек

- **`#/components/schemas/<Name>` (стандартный OpenAPI) НЕ работает** — rolodex резолвит `#/` против
  файла-источника, а не корня документа. Значит, даже миграция спек на стандартный формат internal-ref
  не поможет, пока rolodex не научится резолвить `#/` против корня.
- **Cross-file path `$ref` НЕ работает** — rolodex не дедуплицирует разные относительные пути к
  одному файлу (`./schemas/models/User.yaml` vs `../models/User.yaml`) и не линкует cross-file ref
  к уже зарегистрированной схеме.
- Схема `User` генерируется как тип **только** потому, что зарегистрирована в `openapi.yaml`. Все
  cross-file ссылки на неё из других schema-файлов → `any`.

Т.е. дефект #2 — это баг rolodex'а (неумение резолвить `#/` против корня + неумение дедуплицировать
пути), а НЕ формат наших спек. v1.3.0 не починил его для cross-subpackage случая. Однако **дефект #1
(регрессия конвертера) — безусловный баг генератора**, не зависящий от формата спек: генератор не
вправе эмитить undefined-функцию ни при каком входе.

### Эксперимент: подтверждение триггера `anyToResponse`

| `$ref` в `LoginResponse.yaml` | `User` тип | конвертер | сборка |
|---|---|---|---|
| `../models/User.yaml` (оригинал) | `any` | `resp.User = anyToResponse(req.User)` | ❌ undefined |
| `#/components/schemas/User` | `any` | `resp.User = req.User` | ✅ (но `User` нетипизирован) |

Разница в конвертере при одном и том же результирующем типе `any` означает: `anyToResponse`
триггерится когда генератор **частично** резолвит cross-file path `$ref` (находит схему, понимает
что она splittable, но не может типизировать поле) → берёт TypeName из незарезолвенного ref. Для
`#/...` (полностью неразрешимый фрагмент) генератор поле в `any` и эмитит прямой копи.

---

## Воспроизведение дефекта #1 (блокер)

### Спека

`users/src/openapi/schemas/auth/LoginResponse.yaml`:
```yaml
name: LoginResponse
type: object
required: [user, access_token, refresh_token, expires_in]
properties:
  user:
    $ref: "../models/User.yaml"   # cross-subpackage: schemas/auth/ -> schemas/models/
  access_token:   {type: string, x-validations: ["Size >=1"]}
  refresh_token:  {type: string, x-validations: ["Size >=1"]}
  expires_in:     {type: integer, format: int64, x-validations: [">0"]}
```

`RegisterResponse.yaml` — идентичный паттерн (`user: {$ref: "../models/User.yaml"}`).

### Сгенерированный код (v1.3.0, НЕ компилируется)

`generated/users/model/auth/login_response.gen.go`:
```go
type LoginResponseRequest struct {
	User         any    `json:"user"`          // ← any: cross-file $ref не резолвился
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
type LoginResponseResponse struct {
	User         any    `json:"user"`          // ← тоже any
	// ...
}
```

`generated/users/model/auth/login_response_converters.gen.go`:
```go
func LoginResponseRequestToResponse(req LoginResponseRequest) LoginResponseResponse {
	var resp LoginResponseResponse
	resp.User = anyToResponse(req.User)   // ← undefined: anyToResponse
	resp.AccessToken = req.AccessToken
	resp.RefreshToken = req.RefreshToken
	resp.ExpiresIn = req.ExpiresIn
	return resp
}
```

При этом `UserRequestToResponse` **определена** в `generated/users/model/models/user_converters.gen.go`
(`func UserRequestToResponse(req UserRequest) UserResponse`) — но auth-конвертеры зовут `anyToResponse`
вместо неё, потому что поле `User` потеряло тип.

### Compile-ошибка (из встроенной проверки `oapigen`)

```
oapigen: compile check: go build ./... failed (module root .): exit status 1
# github.com/zvonilkaRU/api-schema/generated/users/model/auth
generated/users/model/auth/login_response_converters.gen.go:7:14: undefined: anyToResponse
generated/users/model/auth/register_response_converters.gen.go:7:14: undefined: anyToResponse
exit status 1
```

Затронуты ровно 2 файла — оба содержат `user: {$ref: "../models/User.yaml"}`. Все прочие
`*RequestToResponse` конвертеры (User, Session, UserRef, Room, Channel, … — десятки) генерируются
корректно, т.к. их поля типизированы.

### Для сравнения: v1.2.1 (компилируется)

Типы те же (`User any` на обеих сторонах — дефект #2 существовал и в v1.2.1). Конвертер:
```go
func LoginResponseRequestToResponse(req LoginResponseRequest) LoginResponseResponse {
	var resp LoginResponseResponse
	resp.User = req.User            // ← прямой копи any -> any, OK
	resp.AccessToken = req.AccessToken
	// ...
	return resp
}
```

---

## Ожидаемое поведение

### Дефект #1 (минимальный фикс, разблокирует v1.3.0)

В `renderRequestToResponse` / `isSplittableField`: если поле резолвится в `any` (или Request- и
Response-типы совпадают), эмитить **прямой копи** `resp.<Field> = req.<Field>`, а не
`<TypeName>RequestToResponse(...)`. `isSplittableField` должен возвращать false для `any`-типов.
Это восстановит поведение v1.2.1 и сделает апгрейд безопасным (с любыми `any`-полями).

### Дефект #2 (корневая причина, разблокирует #69)

Различие intra-service vs cross-service уже есть (v1.3.0 source_marking), но cross-subpackage
intra-service ref всё ещё не линкуется. Нужны:
- дедупликация относительных путей к одному файлу (`./schemas/models/User.yaml` ≡ `../models/User.yaml` ≡ `../../../schemas/models/User.yaml`);
- ИЛИ поддержка `#/components/schemas/<Name>` с резолвцией фрагмента против корневого документа, а не файла-источника.

Предпочтителен второй вариант — он делает спеки переносимыми и соответствует стандарту OpenAPI.

---

## Окружение

- oapicodegen v1.3.0 (commit `af6fd6c`), Go 1.26, darwin/arm64
- Флаги генерации (`generation_flags.yaml`): `GOLANG_SPLIT_REQUEST_RESPONSE=true`,
  `GOLANG_SCHEMA_ANY=warn`, `USE_UTC_FOR_DATE_TIME=true`, `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS=true`,
  `USE_REQUIRED_V2=true`, `GOLANG_USE_OPTIONAL=true`
- Команда:
  ```
  go run github.com/ilovepitsa/oapicodegen/cmd/oapigen@v1.3.0 \
    -input ./zvonilkaRU -output ./generated \
    -import-prefix github.com/zvonilkaRU/api-schema/generated \
    -generation-flags-config-path ./generation_flags.yaml
  ```

---

## Связанное

- Предыдущий bug-report по v1.2.1 (rolodex one-schema-per-file, корень дефекта #2):
  `docs/2026-07-31-oapicodegen-rolodex-one-schema-per-file-bug.md`
- В v1.3.0 обещан фикс `source_marking` для intra-service cross-file `$ref`, но cross-subpackage
  случай (`schemas/auth/` → `schemas/models/`) остался нерабочим.
- Memory: `project_oapicodegen_v2_blocker` — хронология попыток апгрейда.
