package ai

import "context"

type EmbeddingProvider interface {
	Name() string
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type LLMProvider interface {
	Name() string
	Ask(ctx context.Context, question string, contextSnippets []string) (string, error)
}
