package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Chunk represents a memory chunk with text and optional embedding.
type Chunk struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Source    string    `json:"source,omitempty"` // where this memory came from
	Tags      []string  `json:"tags,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"` // vector embedding
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchResult holds a search result with relevance score.
type SearchResult struct {
	Chunk  *Chunk  `json:"chunk"`
	Score  float64 `json:"score"`
	Method string  `json:"method"` // "keyword", "vector", "hybrid"
}

// EmbeddingProvider generates vector embeddings for text.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	Name() string
}

// Index manages memory chunks with keyword and vector search.
type Index struct {
	dir       string
	mu        sync.RWMutex
	chunks    map[string]*Chunk
	embedding EmbeddingProvider
}

// NewIndex creates a memory index at the given directory.
func NewIndex(dir string, embedding EmbeddingProvider) *Index {
	os.MkdirAll(dir, 0o755)

	idx := &Index{
		dir:       dir,
		chunks:    make(map[string]*Chunk),
		embedding: embedding,
	}

	idx.load()
	return idx
}

// Store adds or updates a memory chunk.
func (idx *Index) Store(ctx context.Context, content, source string, tags []string) (*Chunk, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	id := fmt.Sprintf("mem-%d", time.Now().UnixNano())
	now := time.Now()

	chunk := &Chunk{
		ID:        id,
		Content:   content,
		Source:    source,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Generate embedding if provider available
	if idx.embedding != nil {
		embedding, err := idx.embedding.Embed(ctx, content)
		if err == nil {
			chunk.Embedding = embedding
		}
	}

	idx.chunks[id] = chunk
	idx.saveLocked()

	return chunk, nil
}

// Search performs hybrid search (keyword + vector) and returns ranked results.
func (idx *Index) Search(ctx context.Context, query string, limit int) []*SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	results := make(map[string]*SearchResult)

	// 1. Keyword search (BM25-inspired)
	queryTerms := tokenize(query)
	for _, chunk := range idx.chunks {
		score := bm25Score(queryTerms, chunk.Content)
		if score > 0 {
			results[chunk.ID] = &SearchResult{
				Chunk:  chunk,
				Score:  score,
				Method: "keyword",
			}
		}
	}

	// 2. Vector search (if embedding provider available)
	if idx.embedding != nil {
		queryEmbed, err := idx.embedding.Embed(ctx, query)
		if err == nil && len(queryEmbed) > 0 {
			for _, chunk := range idx.chunks {
				if len(chunk.Embedding) == 0 {
					continue
				}
				sim := cosineSimilarity(queryEmbed, chunk.Embedding)
				if sim > 0.3 { // threshold
					if existing, ok := results[chunk.ID]; ok {
						// Hybrid: combine scores
						existing.Score = existing.Score*0.4 + sim*0.6
						existing.Method = "hybrid"
					} else {
						results[chunk.ID] = &SearchResult{
							Chunk:  chunk,
							Score:  sim,
							Method: "vector",
						}
					}
				}
			}
		}
	}

	// Sort by score descending
	sorted := make([]*SearchResult, 0, len(results))
	for _, r := range results {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	return sorted
}

// Delete removes a memory chunk by ID.
func (idx *Index) Delete(id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, ok := idx.chunks[id]; !ok {
		return fmt.Errorf("chunk not found: %s", id)
	}

	delete(idx.chunks, id)
	idx.saveLocked()
	return nil
}

// List returns all chunks, optionally filtered by tags.
func (idx *Index) List(tags []string, limit int) []*Chunk {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	result := make([]*Chunk, 0)
	for _, chunk := range idx.chunks {
		if len(tags) > 0 && !hasAnyTag(chunk.Tags, tags) {
			continue
		}
		result = append(result, chunk)
		if len(result) >= limit {
			break
		}
	}

	// Sort by most recent
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result
}

// Count returns the number of stored chunks.
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.chunks)
}

// Persistence

func (idx *Index) load() {
	path := filepath.Join(idx.dir, "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var chunks []*Chunk
	if err := json.Unmarshal(data, &chunks); err != nil {
		return
	}

	for _, c := range chunks {
		idx.chunks[c.ID] = c
	}
}

func (idx *Index) saveLocked() {
	chunks := make([]*Chunk, 0, len(idx.chunks))
	for _, c := range idx.chunks {
		chunks = append(chunks, c)
	}

	data, err := json.Marshal(chunks)
	if err != nil {
		return
	}

	path := filepath.Join(idx.dir, "index.json")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return
	}
	os.Rename(tmpPath, path)
}

// Text processing helpers

func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Split on non-alphanumeric characters
	tokens := make([]string, 0)
	current := strings.Builder{}
	for _, ch := range text {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			current.WriteRune(ch)
		} else if current.Len() > 0 {
			token := current.String()
			if len(token) > 1 { // skip single chars
				tokens = append(tokens, token)
			}
			current.Reset()
		}
	}
	if current.Len() > 1 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// bm25Score computes a simplified BM25 score.
func bm25Score(queryTerms []string, document string) float64 {
	docTerms := tokenize(document)
	if len(docTerms) == 0 || len(queryTerms) == 0 {
		return 0
	}

	// Term frequency map
	tf := make(map[string]int)
	for _, t := range docTerms {
		tf[t]++
	}

	k1 := 1.2
	b := 0.75
	avgDL := 100.0 // assumed average doc length
	dl := float64(len(docTerms))

	score := 0.0
	for _, term := range queryTerms {
		freq := float64(tf[term])
		if freq == 0 {
			continue
		}
		// Simplified BM25
		numerator := freq * (k1 + 1)
		denominator := freq + k1*(1-b+b*dl/avgDL)
		score += numerator / denominator
	}

	return score
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func hasAnyTag(chunkTags, filterTags []string) bool {
	for _, ft := range filterTags {
		for _, ct := range chunkTags {
			if ct == ft {
				return true
			}
		}
	}
	return false
}

// Embedding providers

// OpenAIEmbedding generates embeddings via OpenAI's API.
type OpenAIEmbedding struct {
	apiKey string
	model  string
}

func NewOpenAIEmbedding(apiKey string) *OpenAIEmbedding {
	if apiKey == "" {
		return nil
	}
	return &OpenAIEmbedding{
		apiKey: apiKey,
		model:  "text-embedding-3-small",
	}
}

func (e *OpenAIEmbedding) Name() string { return "openai" }

func (e *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := map[string]any{
		"model": e.model,
		"input": text,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.openai.com/v1/embeddings",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI embeddings API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

// GeminiEmbedding generates embeddings via Google Gemini API.
type GeminiEmbedding struct {
	apiKey string
}

func NewGeminiEmbedding(apiKey string) *GeminiEmbedding {
	if apiKey == "" {
		return nil
	}
	return &GeminiEmbedding{apiKey: apiKey}
}

func (e *GeminiEmbedding) Name() string { return "gemini" }

func (e *GeminiEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := map[string]any{
		"model": "models/embedding-001",
		"content": map[string]any{
			"parts": []map[string]string{
				{"text": text},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/embedding-001:embedContent?key=%s",
		e.apiKey,
	)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini embeddings API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Embedding.Values, nil
}
