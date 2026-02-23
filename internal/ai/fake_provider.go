package ai

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
)

type FakeEmbeddingProvider struct{}

type FakeLLMProvider struct{}

func (FakeEmbeddingProvider) Name() string { return "fake" }

func (FakeEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, text := range texts {
		vec := make([]float64, 16)
		for _, token := range strings.Fields(strings.ToLower(text)) {
			h := fnv.New64a()
			_, _ = h.Write([]byte(token))
			idx := int(h.Sum64() % uint64(len(vec)))
			vec[idx]++
		}
		out = append(out, vec)
	}
	return out, nil
}

func (FakeLLMProvider) Name() string { return "fake" }

func (FakeLLMProvider) Ask(ctx context.Context, question string, contextSnippets []string) (string, error) {
	if len(contextSnippets) == 0 {
		return "Je n'ai pas trouvé de source pertinente.", nil
	}
	return fmt.Sprintf("Réponse synthétique: %s", strings.Join(contextSnippets, " | ")), nil
}
