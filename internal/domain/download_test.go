package domain

import "testing"

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from DownloadStatus
		to   DownloadStatus
		ok   bool
	}{
		{DownloadQueued, DownloadRunning, true},
		{DownloadRunning, DownloadPaused, true},
		{DownloadPaused, DownloadRunning, true},
		{DownloadRunning, DownloadCompleted, true},
		{DownloadCompleted, DownloadRunning, false},
		{DownloadCanceled, DownloadRunning, false},
	}

	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.ok {
			t.Fatalf("CanTransition(%s,%s)=%v want %v", tc.from, tc.to, got, tc.ok)
		}
	}
}
