package middlewares

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoggingIncludesStatusCode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	log := zerolog.New(&out)
	mw := Logging(log)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", res.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode log: %v raw=%s", err, out.String())
	}
	if got, ok := payload["status"].(float64); !ok || int(got) != http.StatusInternalServerError {
		t.Fatalf("expected status=500 in log, got=%v raw=%s", payload["status"], out.String())
	}
}
