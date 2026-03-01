package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UsageEntry records token usage for a single LLM call.
type UsageEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	SessionKey   string    `json:"session_key"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	DurationMS   int64     `json:"duration_ms"`
}

// DailySummary holds aggregated usage for a day.
type DailySummary struct {
	Date          string                 `json:"date"` // YYYY-MM-DD
	TotalCalls    int                    `json:"total_calls"`
	TotalInput    int                    `json:"total_input_tokens"`
	TotalOutput   int                    `json:"total_output_tokens"`
	TotalTokens   int                    `json:"total_tokens"`
	TotalCostUSD  float64                `json:"total_cost_usd"`
	ByModel       map[string]*ModelUsage `json:"by_model"`
	BySession     map[string]int         `json:"by_session"` // session_key -> call count
	AvgDurationMS int64                  `json:"avg_duration_ms"`
}

// ModelUsage tracks usage per model.
type ModelUsage struct {
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// Tracker records and queries LLM usage data.
type Tracker struct {
	dir     string
	mu      sync.Mutex
	entries []UsageEntry // in-memory buffer for current day
	today   string       // current day key
}

// NewTracker creates a usage tracker that stores data in the given directory.
func NewTracker(workspaceDir string) *Tracker {
	dir := filepath.Join(workspaceDir, "usage")
	os.MkdirAll(dir, 0755)
	return &Tracker{
		dir:   dir,
		today: time.Now().Format("2006-01-02"),
	}
}

// Record adds a usage entry.
func (t *Tracker) Record(entry UsageEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.TotalTokens == 0 {
		entry.TotalTokens = entry.InputTokens + entry.OutputTokens
	}
	if entry.CostUSD == 0 {
		entry.CostUSD = estimateCost(entry.Model, entry.InputTokens, entry.OutputTokens)
	}

	day := entry.Timestamp.Format("2006-01-02")

	// Flush buffer if day changed
	if day != t.today {
		t.flushLocked()
		t.today = day
	}

	t.entries = append(t.entries, entry)

	// Flush every 50 entries
	if len(t.entries) >= 50 {
		t.flushLocked()
	}
}

// GetDailySummary returns usage summary for a specific day.
func (t *Tracker) GetDailySummary(date string) (*DailySummary, error) {
	t.mu.Lock()
	t.flushLocked()
	t.mu.Unlock()

	entries, err := t.loadDay(date)
	if err != nil {
		return nil, err
	}

	return summarize(date, entries), nil
}

// GetRangeSummary returns usage summary for a date range.
func (t *Tracker) GetRangeSummary(days int) ([]*DailySummary, error) {
	t.mu.Lock()
	t.flushLocked()
	t.mu.Unlock()

	summaries := make([]*DailySummary, 0, days)
	now := time.Now()

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		entries, err := t.loadDay(date)
		if err != nil {
			continue
		}
		if len(entries) > 0 {
			summaries = append(summaries, summarize(date, entries))
		}
	}

	return summaries, nil
}

// Flush writes buffered entries to disk.
func (t *Tracker) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flushLocked()
}

func (t *Tracker) flushLocked() {
	if len(t.entries) == 0 {
		return
	}

	// Group entries by day
	byDay := make(map[string][]UsageEntry)
	for _, e := range t.entries {
		day := e.Timestamp.Format("2006-01-02")
		byDay[day] = append(byDay[day], e)
	}

	for day, dayEntries := range byDay {
		path := t.dayFilePath(day)

		// Load existing entries
		existing, _ := t.loadDay(day)
		all := append(existing, dayEntries...)

		data, err := json.Marshal(all)
		if err != nil {
			continue
		}
		os.WriteFile(path, data, 0644)
	}

	t.entries = t.entries[:0]
}

func (t *Tracker) dayFilePath(date string) string {
	// Store as usage/YYYY-MM/DD.json
	parts := filepath.Join(t.dir, date[:7])
	os.MkdirAll(parts, 0755)
	return filepath.Join(parts, date[8:]+".json")
}

func (t *Tracker) loadDay(date string) ([]UsageEntry, error) {
	path := t.dayFilePath(date)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []UsageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func summarize(date string, entries []UsageEntry) *DailySummary {
	s := &DailySummary{
		Date:      date,
		ByModel:   make(map[string]*ModelUsage),
		BySession: make(map[string]int),
	}

	var totalDuration int64
	for _, e := range entries {
		s.TotalCalls++
		s.TotalInput += e.InputTokens
		s.TotalOutput += e.OutputTokens
		s.TotalTokens += e.TotalTokens
		s.TotalCostUSD += e.CostUSD
		totalDuration += e.DurationMS

		if _, ok := s.ByModel[e.Model]; !ok {
			s.ByModel[e.Model] = &ModelUsage{}
		}
		mu := s.ByModel[e.Model]
		mu.Calls++
		mu.InputTokens += e.InputTokens
		mu.OutputTokens += e.OutputTokens
		mu.TotalTokens += e.TotalTokens
		mu.CostUSD += e.CostUSD

		if e.SessionKey != "" {
			s.BySession[e.SessionKey]++
		}
	}

	if s.TotalCalls > 0 {
		s.AvgDurationMS = totalDuration / int64(s.TotalCalls)
	}

	return s
}

// estimateCost estimates the cost in USD based on model and token counts.
func estimateCost(model string, inputTokens, outputTokens int) float64 {
	// Price per 1M tokens (input/output) — approximate
	type pricing struct{ input, output float64 }
	prices := map[string]pricing{
		"claude-3-5-sonnet":  {3.0, 15.0},
		"claude-3-haiku":     {0.25, 1.25},
		"claude-3-opus":      {15.0, 75.0},
		"gpt-4o":             {5.0, 15.0},
		"gpt-4o-mini":        {0.15, 0.6},
		"gpt-4-turbo":        {10.0, 30.0},
		"gemini-1.5-pro":     {3.5, 10.5},
		"gemini-1.5-flash":   {0.35, 1.05},
		"glm-4":              {1.0, 1.0},
		"glm-4.7":            {1.0, 1.0},
		"deepseek-chat":      {0.14, 0.28},
		"deepseek-reasoner":  {0.55, 2.19},
	}

	p, ok := prices[model]
	if !ok {
		// Default fallback pricing
		p = pricing{1.0, 3.0}
	}

	cost := (float64(inputTokens) * p.input / 1_000_000) + (float64(outputTokens) * p.output / 1_000_000)
	// Round to 6 decimal places
	return float64(int64(cost*1_000_000)) / 1_000_000
}

// FormatCost formats a cost value for display.
func FormatCost(costUSD float64) string {
	if costUSD < 0.01 {
		return fmt.Sprintf("$%.4f", costUSD)
	}
	return fmt.Sprintf("$%.2f", costUSD)
}
