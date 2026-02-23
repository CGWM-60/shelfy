package httpclient

import (
	"net/http"
	"time"
)

type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

func New(timeout time.Duration) Client {
	return &http.Client{Timeout: timeout}
}

// NewStreaming returns an HTTP client suitable for large downloads:
// it limits connection/header wait time but does not enforce a global request timeout.
func NewStreaming(timeout time.Duration) Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if timeout > 0 {
		transport.ResponseHeaderTimeout = timeout
	}
	return &http.Client{
		Transport: transport,
		Timeout:   0,
	}
}
