package usage

import (
	"os"
	"testing"
	"time"
)

func TestTracker_Record(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "usage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewTracker(tmpDir)

	tracker.Record(UsageEntry{
		SessionKey:   "test-session",
		Model:        "gpt-4o",
		Provider:     "openai",
		InputTokens:  100,
		OutputTokens: 50,
		DurationMS:   500,
	})

	tracker.Record(UsageEntry{
		SessionKey:   "test-session",
		Model:        "gpt-4o",
		Provider:     "openai",
		InputTokens:  200,
		OutputTokens: 100,
		DurationMS:   800,
	})

	tracker.Flush()

	// Get daily summary
	today := time.Now().Format("2006-01-02")
	summary, err := tracker.GetDailySummary(today)
	if err != nil {
		t.Fatalf("GetDailySummary failed: %v", err)
	}

	if summary.TotalCalls != 2 {
		t.Errorf("Expected 2 calls, got %d", summary.TotalCalls)
	}
	if summary.TotalInput != 300 {
		t.Errorf("Expected 300 input tokens, got %d", summary.TotalInput)
	}
	if summary.TotalOutput != 150 {
		t.Errorf("Expected 150 output tokens, got %d", summary.TotalOutput)
	}
}

func TestTracker_GetRangeSummary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "usage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewTracker(tmpDir)

	tracker.Record(UsageEntry{
		SessionKey:  "session-1",
		Model:       "gpt-4o",
		InputTokens: 100,
	})

	tracker.Flush()

	summaries, err := tracker.GetRangeSummary(7)
	if err != nil {
		t.Fatalf("GetRangeSummary failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("Expected 1 day summary, got %d", len(summaries))
	}
}

func TestTracker_ByModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "usage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewTracker(tmpDir)

	tracker.Record(UsageEntry{Model: "gpt-4o", InputTokens: 100})
	tracker.Record(UsageEntry{Model: "claude-3-5-sonnet", InputTokens: 200})
	tracker.Record(UsageEntry{Model: "gpt-4o", InputTokens: 150})
	tracker.Flush()

	today := time.Now().Format("2006-01-02")
	summary, _ := tracker.GetDailySummary(today)

	if len(summary.ByModel) != 2 {
		t.Errorf("Expected 2 models, got %d", len(summary.ByModel))
	}

	gpt4 := summary.ByModel["gpt-4o"]
	if gpt4 == nil {
		t.Fatal("Expected gpt-4o model usage")
	}
	if gpt4.Calls != 2 {
		t.Errorf("Expected 2 gpt-4o calls, got %d", gpt4.Calls)
	}
}

func TestEstimateCost(t *testing.T) {
	cost := estimateCost("gpt-4o", 1000, 500)
	if cost <= 0 {
		t.Errorf("Expected positive cost, got %f", cost)
	}

	// Unknown model should use default pricing
	unknownCost := estimateCost("unknown-model", 1000, 500)
	if unknownCost <= 0 {
		t.Errorf("Expected positive cost for unknown model, got %f", unknownCost)
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost     float64
		expected string
	}{
		{0.001, "$0.0010"},
		{0.50, "$0.50"},
		{1.23, "$1.23"},
	}

	for _, tt := range tests {
		got := FormatCost(tt.cost)
		if got != tt.expected {
			t.Errorf("FormatCost(%f) = %q, want %q", tt.cost, got, tt.expected)
		}
	}
}

func TestTracker_EmptyDay(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "usage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewTracker(tmpDir)

	summary, err := tracker.GetDailySummary("2020-01-01")
	if err != nil {
		t.Fatalf("Expected no error for empty day: %v", err)
	}
	if summary.TotalCalls != 0 {
		t.Errorf("Expected 0 calls for empty day, got %d", summary.TotalCalls)
	}
}
