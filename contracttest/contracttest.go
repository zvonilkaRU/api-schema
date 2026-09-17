// Package contracttest — конформанс-тесты контракта (api-schema#9, инкремент 2).
//
// Схема: generated-echoserver монтируется на echo с fake-имплементацией
// интерфейса Server; бьём в него generated-httpclient-ом (или raw HTTP для
// транспорных деталей). Ловит расхождения OpenAPI ↔ generated: пути, методы,
// коды ответов, JSON-теги в обе стороны, strict-binding (unknown field → 400),
// форму ошибки валидации.
//
// Fake имплементирует интерфейс напрямую: компилятор держит тест в точном
// соответствии с контрактом — новая операция в спеке без сценария не даст
// собрать пакет, пока fake не расширен (а значит, и тест не обновлён).
package contracttest

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	validator "github.com/ilovepitsa/oapicodegen/pkg/validator"
)

// Serve поднимает httptest-сервер вокруг echo с переданной регистрацией
// маршрутов (сюда передают ServerHTTP.Register). Закрывается с тестом.
func Serve(t *testing.T, register func(e *echo.Echo)) *httptest.Server {
	t.Helper()

	e := echo.New()
	e.HideBanner = true
	register(e)

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	return srv
}

// stubValidator — именованный валидатор, делегирующий решение функции.
type stubValidator struct {
	name string
	fn   func(value any) error
}

func (v stubValidator) Name() string             { return v.name }
func (v stubValidator) Validate(value any) error { return v.fn(value) }

// StubRegistry строит Registry со стаб-валидаторами ровно на имена из
// expected (AssertExact страхует от дрейфа набора). decide вызывается на
// каждую именованную валидацию: вернула ошибку — поле невалидно; nil — валидно.
// decide == nil — все значения валидны (для happy-path сценариев).
func StubRegistry(
	t *testing.T,
	expected []string,
	decide func(name string, value any) error,
) *validator.Registry {
	t.Helper()

	reg := validator.New()
	// С Go 1.22 переменная цикла своя на каждой итерации — копия не нужна.
	for _, name := range expected {
		reg.Register(stubValidator{name: name, fn: func(value any) error {
			if decide == nil {
				return nil
			}

			return decide(name, value)
		}})
	}
	if err := reg.AssertExact(expected); err != nil {
		t.Fatalf("stub registry не совпал с expected: %v", err)
	}

	return reg
}
