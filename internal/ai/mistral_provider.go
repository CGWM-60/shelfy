package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type MistralEmbeddingProvider struct {
	client HTTPClient
	apiKey string
	model  string
}

type MistralLLMProvider struct {
	client HTTPClient
	apiKey string
	model  string
}

func NewMistralEmbeddingProvider(client HTTPClient, apiKey, model string) *MistralEmbeddingProvider {
	return &MistralEmbeddingProvider{client: client, apiKey: apiKey, model: model}
}

func NewMistralLLMProvider(client HTTPClient, apiKey, model string) *MistralLLMProvider {
	return &MistralLLMProvider{client: client, apiKey: apiKey, model: model}
}

func (p *MistralEmbeddingProvider) Name() string { return "mistral" }

func (p *MistralEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if p.apiKey == "" {
		return nil, errors.New("MISTRAL_API_KEY missing")
	}
	payload, _ := json.Marshal(map[string]interface{}{"model": p.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mistral.ai/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mistral embeddings status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([][]float64, 0, len(body.Data))
	for _, row := range body.Data {
		out = append(out, row.Embedding)
	}
	return out, nil
}

func (p *MistralLLMProvider) Name() string { return "mistral" }

func (p *MistralLLMProvider) Ask(ctx context.Context, question string, snippets []string) (string, error) {
	if p.apiKey == "" {
		return "", errors.New("MISTRAL_API_KEY missing")
	}
	messages := []map[string]string{{"role": "system", "content": "Réponds en français de manière claire et concise."}}
	prompt := "Question: " + question + "\n\nContexte:\n"
	for _, snippet := range snippets {
		prompt += "- " + snippet + "\n"
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})

	payload, _ := json.Marshal(map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mistral.ai/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("mistral chat status %d", resp.StatusCode)
	}
	var body struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if len(body.Choices) == 0 {
		return "", errors.New("mistral empty choices")
	}
	return body.Choices[0].Message.Content, nil
}
