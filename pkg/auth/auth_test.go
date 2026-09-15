package auth

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestVerifier_RefreshLoop_StopsOnClose (api-schema#15): refreshLoop обязан
// выходить по Close — раньше goroutine жила вечно (утечка при shutdown).
func TestVerifier_RefreshLoop_StopsOnClose(t *testing.T) {
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// Координаты — произвольные 32 байта: fetchJWKS их не математит, только
		// декодирует base64url и собирает ecdsa.PublicKey.
		_, _ = w.Write([]byte(`{"keys":[{"crv":"P-256","x":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","y":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`))
	}))
	defer srv.Close()

	v := &Verifier{
		jwksURL:         srv.URL,
		refreshInterval: 5 * time.Millisecond,
		stopCh:          make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		v.refreshLoop()
	}()

	// Хотя бы один refresh до Close.
	deadline := time.Now().Add(2 * time.Second)
	for fetches.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("refresh не произошёл до Close")
		}

		time.Sleep(time.Millisecond)
	}

	v.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refreshLoop не остановился по Close")
	}

	// После Close (интервал 5мс, ждём 50мс) новых fetch быть не должно.
	before := fetches.Load()
	time.Sleep(50 * time.Millisecond)
	if got := fetches.Load(); got != before {
		t.Errorf("после Close были fetch-и: %d -> %d", before, got)
	}

	// Close идемпотентен — повторный вызов не паникует на закрытом канале.
	v.Close()
}
