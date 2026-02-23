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
