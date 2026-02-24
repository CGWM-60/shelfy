package middlewares

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
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

type fakeHijackWriter struct {
	header   http.Header
	hijacked bool
}

func (f *fakeHijackWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *fakeHijackWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (f *fakeHijackWriter) WriteHeader(_ int) {}

func (f *fakeHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	server, client := net.Pipe()
	_ = client.Close()
	rw := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(nil)), bufio.NewWriter(io.Discard))
	return server, rw, nil
}

func TestLoggingResponseWriterForwardsHijack(t *testing.T) {
	t.Parallel()

	base := &fakeHijackWriter{}
	wrapped := &loggingResponseWriter{ResponseWriter: base, status: http.StatusOK}
	conn, _, err := wrapped.Hijack()
	if err != nil {
		t.Fatalf("hijack err=%v", err)
	}
	if conn != nil {
		_ = conn.Close()
	}
	if !base.hijacked {
		t.Fatalf("expected underlying hijack call")
	}
}
