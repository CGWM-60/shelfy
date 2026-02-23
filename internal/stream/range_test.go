package stream

import (
	"net/http/httptest"
	"testing"
)

func TestParseRangeHeader(t *testing.T) {
	rng, err := ParseRangeHeader("bytes=10-19", 100)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rng.Start != 10 || rng.End != 19 || !rng.Valid {
		t.Fatalf("unexpected range: %+v", rng)
	}
}

func TestWriteRangeHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteRangeHeaders(rr, ByteRange{Start: 0, End: 9, Valid: true}, 100, "video/mp4")
	if rr.Code != 206 {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Content-Range") != "bytes 0-9/100" {
		t.Fatalf("content-range=%s", rr.Header().Get("Content-Range"))
	}
}
