package downloader

import (
	"testing"
	"time"
)

func TestBackoffGrowthAndCap(t *testing.T) {
	if got := Backoff(1); got != 100*time.Millisecond {
		t.Fatalf("retry1 backoff=%v", got)
	}
	if got := Backoff(2); got != 200*time.Millisecond {
		t.Fatalf("retry2 backoff=%v", got)
	}
	if got := Backoff(10); got != 5*time.Second {
		t.Fatalf("cap backoff=%v", got)
	}
}
