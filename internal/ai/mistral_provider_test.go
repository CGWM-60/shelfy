package ai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMistralProviders(t *testing.T) {
	client := fakeHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/embeddings") {
			return jsonResponse(`{"data":[{"embedding":[0.1,0.2]}]}`), nil
		}
		if strings.HasSuffix(req.URL.Path, "/chat/completions") {
			return jsonResponse(`{"choices":[{"message":{"content":"Réponse"}}]}`), nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"error":"not found"}`)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
	}}

	embed := NewMistralEmbeddingProvider(client, "api-key", "mistral-embed")
	llm := NewMistralLLMProvider(client, "api-key", "mistral-small-latest")

	vectors, err := embed.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("embed error: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 {
		t.Fatalf("unexpected vectors: %+v", vectors)
	}
	answer, err := llm.Ask(context.Background(), "question", []string{"ctx"})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if answer == "" {
		t.Fatalf("empty answer")
	}
}

type fakeHTTPClient struct {
	handler func(req *http.Request) (*http.Response, error)
}

func (c fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.handler(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
