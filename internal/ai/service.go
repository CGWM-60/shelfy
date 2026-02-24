package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/repo"
)

type Service struct {
	repo        repo.Repository
	embeddings  EmbeddingProvider
	llm         LLMProvider
	defaultTopK int
	mu          sync.Mutex
	metrics     metrics
}

type metrics struct {
	StartedAt time.Time

	IndexRuns        int64
	LastIndexedCount int64
	IndexedTotal     int64
	LastIndexedAt    *time.Time

	SearchRuns         int64
	SearchTotalLatency int64
	LastSearchLatency  int64
	LastSearchResults  int64

	AskRuns           int64
	AskSearchOnlyRuns int64
	AskTotalLatency   int64
	LastAskLatency    int64
	LastAskSources    int64

	EmbeddingCalls         int64
	EmbeddingTokensEst     int64
	LLMPromptTokensEst     int64
	LLMCompletionTokensEst int64

	LastError   string
	LastErrorAt *time.Time
}

type Report struct {
	EmbeddingProvider string `json:"embeddingProvider"`
	LLMProvider       string `json:"llmProvider"`

	StartedAt   time.Time  `json:"startedAt"`
	UptimeSec   int64      `json:"uptimeSec"`
	LastError   string     `json:"lastError,omitempty"`
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty"`

	IndexRuns        int64      `json:"indexRuns"`
	LastIndexedCount int64      `json:"lastIndexedCount"`
	IndexedTotal     int64      `json:"indexedTotal"`
	LastIndexedAt    *time.Time `json:"lastIndexedAt,omitempty"`

	SearchRuns          int64   `json:"searchRuns"`
	SearchAvgLatencyMs  float64 `json:"searchAvgLatencyMs"`
	LastSearchLatencyMs int64   `json:"lastSearchLatencyMs"`
	LastSearchResults   int64   `json:"lastSearchResults"`

	AskRuns           int64   `json:"askRuns"`
	AskSearchOnlyRuns int64   `json:"askSearchOnlyRuns"`
	AskAvgLatencyMs   float64 `json:"askAvgLatencyMs"`
	LastAskLatencyMs  int64   `json:"lastAskLatencyMs"`
	LastAskSources    int64   `json:"lastAskSources"`

	EmbeddingCalls               int64 `json:"embeddingCalls"`
	EmbeddingTokensEstimated     int64 `json:"embeddingTokensEstimated"`
	LLMPromptTokensEstimated     int64 `json:"llmPromptTokensEstimated"`
	LLMCompletionTokensEstimated int64 `json:"llmCompletionTokensEstimated"`
}

func NewService(r repo.Repository, embeddings EmbeddingProvider, llm LLMProvider) *Service {
	return &Service{
		repo:        r,
		embeddings:  embeddings,
		llm:         llm,
		defaultTopK: 5,
		metrics: metrics{
			StartedAt: time.Now(),
		},
	}
}

func (s *Service) Status() map[string]string {
	return map[string]string{"embeddingProvider": s.embeddings.Name(), "llmProvider": s.llm.Name()}
}

func (s *Service) Report() Report {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	avgSearch := 0.0
	if s.metrics.SearchRuns > 0 {
		avgSearch = float64(s.metrics.SearchTotalLatency) / float64(s.metrics.SearchRuns)
	}
	avgAsk := 0.0
	if s.metrics.AskRuns > 0 {
		avgAsk = float64(s.metrics.AskTotalLatency) / float64(s.metrics.AskRuns)
	}
	return Report{
		EmbeddingProvider:            s.embeddings.Name(),
		LLMProvider:                  s.llm.Name(),
		StartedAt:                    s.metrics.StartedAt,
		UptimeSec:                    int64(now.Sub(s.metrics.StartedAt).Seconds()),
		LastError:                    s.metrics.LastError,
		LastErrorAt:                  s.metrics.LastErrorAt,
		IndexRuns:                    s.metrics.IndexRuns,
		LastIndexedCount:             s.metrics.LastIndexedCount,
		IndexedTotal:                 s.metrics.IndexedTotal,
		LastIndexedAt:                s.metrics.LastIndexedAt,
		SearchRuns:                   s.metrics.SearchRuns,
		SearchAvgLatencyMs:           avgSearch,
		LastSearchLatencyMs:          s.metrics.LastSearchLatency,
		LastSearchResults:            s.metrics.LastSearchResults,
		AskRuns:                      s.metrics.AskRuns,
		AskSearchOnlyRuns:            s.metrics.AskSearchOnlyRuns,
		AskAvgLatencyMs:              avgAsk,
		LastAskLatencyMs:             s.metrics.LastAskLatency,
		LastAskSources:               s.metrics.LastAskSources,
		EmbeddingCalls:               s.metrics.EmbeddingCalls,
		EmbeddingTokensEstimated:     s.metrics.EmbeddingTokensEst,
		LLMPromptTokensEstimated:     s.metrics.LLMPromptTokensEst,
		LLMCompletionTokensEstimated: s.metrics.LLMCompletionTokensEst,
	}
}

func (s *Service) SetDefaultTopK(k int) {
	if k > 0 {
		s.defaultTopK = k
	}
}

func (s *Service) Index(ctx context.Context) (int, error) {
	started := time.Now()
	media, err := s.repo.ListMedia(ctx, "", "")
	if err != nil {
		s.recordError(err)
		return 0, err
	}
	downloads, err := s.repo.ListDownloads(ctx)
	if err != nil {
		s.recordError(err)
		return 0, err
	}
	if len(media) == 0 && len(downloads) == 0 {
		if err := s.repo.DeleteAllAIChunks(ctx); err != nil {
			s.recordError(err)
			return 0, err
		}
		s.recordIndex(0, time.Since(started))
		return 0, nil
	}

	if err := s.repo.DeleteAllAIChunks(ctx); err != nil {
		s.recordError(err)
		return 0, err
	}

	type document struct {
		mediaID string
		key     string
		content string
	}
	docs := make([]document, 0, len(media)+len(downloads))
	seen := map[string]struct{}{}
	addDoc := func(mediaID, key, content string) {
		key = strings.TrimSpace(strings.ToLower(key))
		content = strings.TrimSpace(content)
		if key == "" {
			key = mediaID
		}
		if content == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		docs = append(docs, document{
			mediaID: mediaID,
			key:     key,
			content: content,
		})
	}

	for _, m := range media {
		addDoc(m.ID, mediaKey(m), strings.TrimSpace(m.Title+" "+strings.Join(m.Tags, " ")))
	}
	for _, d := range downloads {
		id := "download:" + d.ID
		content := strings.TrimSpace(d.FileName + " " + d.SourceLink + " " + d.DestinationPath)
		addDoc(id, downloadKey(d), content)
	}
	if len(docs) == 0 {
		s.recordIndex(0, time.Since(started))
		return 0, nil
	}

	texts := make([]string, 0, len(docs))
	for _, doc := range docs {
		texts = append(texts, doc.content)
	}
	vectors, err := s.embeddings.Embed(ctx, texts)
	if err != nil {
		s.recordError(err)
		return 0, err
	}
	s.recordEmbeddingUsage(texts)
	for i, doc := range docs {
		raw, _ := json.Marshal(vectors[i])
		chunkID := chunkIDForKey(doc.key)
		if err := s.repo.UpsertAIChunk(ctx, chunkID, doc.mediaID, doc.content, string(raw)); err != nil {
			s.recordError(err)
			return 0, err
		}
	}
	s.recordIndex(len(docs), time.Since(started))
	return len(docs), nil
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]domain.AISearchResult, error) {
	started := time.Now()
	if limit <= 0 {
		limit = s.defaultTopK
	}
	qVecs, err := s.embeddings.Embed(ctx, []string{query})
	if err != nil {
		s.recordError(err)
		return nil, err
	}
	s.recordEmbeddingUsage([]string{query})
	chunks, err := s.repo.ListAIChunks(ctx)
	if err != nil {
		s.recordError(err)
		return nil, err
	}
	mediaList, err := s.repo.ListMedia(ctx, "", "")
	if err != nil {
		s.recordError(err)
		return nil, err
	}
	mediaTitle := map[string]string{}
	for _, item := range mediaList {
		mediaTitle[item.ID] = item.Title
	}
	best := make(map[string]domain.AISearchResult, len(chunks))
	for _, chunk := range chunks {
		var vec []float64
		if err := json.Unmarshal([]byte(chunk.Embedding), &vec); err != nil {
			continue
		}
		score := cosine(qVecs[0], vec)
		result := domain.AISearchResult{
			MediaID: chunk.MediaID,
			Snippet: chunk.Content,
			Score:   score,
			Title:   resolveTitle(chunk.MediaID, mediaTitle),
		}
		key := strings.ToLower(strings.TrimSpace(chunk.MediaID + "|" + chunk.Content))
		if existing, ok := best[key]; !ok || result.Score > existing.Score {
			best[key] = result
		}
	}
	results := make([]domain.AISearchResult, 0, len(best))
	for _, row := range best {
		results = append(results, row)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	s.recordSearch(len(results), time.Since(started))
	return results, nil
}

func (s *Service) Ask(ctx context.Context, question string, searchOnly bool) (domain.AIAskResponse, error) {
	started := time.Now()
	s.mu.Lock()
	s.metrics.AskRuns++
	if searchOnly {
		s.metrics.AskSearchOnlyRuns++
	}
	s.mu.Unlock()

	search, err := s.Search(ctx, question, 5)
	if err != nil {
		s.recordError(err)
		return domain.AIAskResponse{}, err
	}
	if searchOnly {
		s.recordAsk(len(search), time.Since(started), 0, 0)
		return domain.AIAskResponse{Answer: "Mode recherche: aucune génération.", Sources: search}, nil
	}
	snippets := make([]string, 0, len(search))
	for _, row := range search {
		snippets = append(snippets, row.Snippet)
	}
	promptTokenEstimate := estimateTokenCount(question)
	for _, snippet := range snippets {
		promptTokenEstimate += estimateTokenCount(snippet)
	}
	answer, err := s.llm.Ask(ctx, question, snippets)
	if err != nil {
		s.recordError(err)
		return domain.AIAskResponse{}, err
	}
	completionTokenEstimate := estimateTokenCount(answer)
	s.recordAsk(len(search), time.Since(started), promptTokenEstimate, completionTokenEstimate)
	return domain.AIAskResponse{Answer: answer, Sources: search}, nil
}

func (s *Service) recordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.metrics.LastError = err.Error()
	s.metrics.LastErrorAt = &now
}

func (s *Service) recordEmbeddingUsage(texts []string) {
	totalTokens := int64(0)
	for _, text := range texts {
		totalTokens += int64(estimateTokenCount(text))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics.EmbeddingCalls++
	s.metrics.EmbeddingTokensEst += totalTokens
}

func (s *Service) recordIndex(indexed int, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.metrics.IndexRuns++
	s.metrics.LastIndexedCount = int64(indexed)
	s.metrics.IndexedTotal += int64(indexed)
	s.metrics.LastIndexedAt = &now
}

func (s *Service) recordSearch(results int, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms := elapsed.Milliseconds()
	s.metrics.SearchRuns++
	s.metrics.SearchTotalLatency += ms
	s.metrics.LastSearchLatency = ms
	s.metrics.LastSearchResults = int64(results)
}

func (s *Service) recordAsk(sources int, elapsed time.Duration, promptTokens, completionTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms := elapsed.Milliseconds()
	s.metrics.AskTotalLatency += ms
	s.metrics.LastAskLatency = ms
	s.metrics.LastAskSources = int64(sources)
	s.metrics.LLMPromptTokensEst += int64(promptTokens)
	s.metrics.LLMCompletionTokensEst += int64(completionTokens)
}

func estimateTokenCount(text string) int {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}
	// Heuristic for quick monitoring without external tokenizer.
	approx := int(math.Ceil(float64(len(runes)) / 4.0))
	if approx < 1 {
		return 1
	}
	return approx
}

func chunkIDForKey(key string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("chunk:%x", h.Sum64())
}

func normalizePath(raw string) string {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "." || cleaned == "" {
		return ""
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return strings.ToLower(cleaned)
	}
	return strings.ToLower(abs)
}

func mediaKey(m domain.MediaItem) string {
	if path := normalizePath(m.Path); path != "" {
		return "path:" + path
	}
	return "media:" + m.ID
}

func downloadKey(d domain.DownloadJob) string {
	if path := normalizePath(d.DestinationPath); path != "" {
		return "path:" + path
	}
	if link := strings.TrimSpace(strings.ToLower(d.SourceLink)); link != "" {
		return "url:" + link
	}
	return "download:" + d.ID
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	size := len(a)
	if len(b) < size {
		size = len(b)
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := 0; i < size; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func resolveTitle(id string, mediaTitle map[string]string) string {
	if title, ok := mediaTitle[id]; ok && title != "" {
		return title
	}
	if strings.HasPrefix(id, "download:") {
		return strings.TrimPrefix(id, "download:")
	}
	return id
}
