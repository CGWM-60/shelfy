package domain

type AISearchResult struct {
	MediaID string  `json:"mediaId"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Title   string  `json:"title"`
}

type AIAskResponse struct {
	Answer  string           `json:"answer"`
	Sources []AISearchResult `json:"sources"`
}
