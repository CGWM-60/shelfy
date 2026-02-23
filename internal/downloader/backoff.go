package downloader

import "time"

func Backoff(retry int) time.Duration {
	if retry < 1 {
		retry = 1
	}
	d := time.Duration(1<<uint(retry-1)) * 100 * time.Millisecond
	if d > 5*time.Second {
		return 5 * time.Second
	}
	return d
}
