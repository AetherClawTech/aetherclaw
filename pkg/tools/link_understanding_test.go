package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"no urls here", 0},
		{"check https://example.com please", 1},
		{"visit http://foo.com and https://bar.com", 2},
		{"same url twice: https://a.com and https://a.com", 1}, // dedup
		{"url with trailing punctuation: https://example.com.", 1},
		{"url in parens (https://example.com)", 1},
	}

	for _, tt := range tests {
		urls := ExtractURLs(tt.input)
		if len(urls) != tt.expected {
			t.Errorf("ExtractURLs(%q) = %d URLs, want %d (got: %v)", tt.input, len(urls), tt.expected, urls)
		}
	}
}

func TestExtractURLs_TrailingPunctuation(t *testing.T) {
	urls := ExtractURLs("See https://example.com/page.")
	if len(urls) != 1 {
		t.Fatalf("Expected 1 URL, got %d", len(urls))
	}
	if urls[0] != "https://example.com/page" {
		t.Errorf("Expected trailing period removed, got: %s", urls[0])
	}
}

func TestFetchLinkContent_HTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Test Title</title></head><body><h1>Hello</h1><p>World</p></body></html>"))
	}))
	defer server.Close()

	lc := FetchLinkContent(server.URL, 10000)
	if lc.Error != "" {
		t.Fatalf("Unexpected error: %s", lc.Error)
	}
	if lc.Title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got: %s", lc.Title)
	}
	if lc.Content == "" {
		t.Error("Expected non-empty content")
	}
}

func TestFetchLinkContent_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer server.Close()

	lc := FetchLinkContent(server.URL, 10000)
	if lc.Error != "" {
		t.Fatalf("Unexpected error: %s", lc.Error)
	}
	if lc.Content != `{"key":"value"}` {
		t.Errorf("Expected JSON content, got: %s", lc.Content)
	}
}

func TestFetchLinkContent_Truncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 0; i < 1000; i++ {
			w.Write([]byte("this is a long content line\n"))
		}
	}))
	defer server.Close()

	lc := FetchLinkContent(server.URL, 100)
	if lc.Error != "" {
		t.Fatalf("Unexpected error: %s", lc.Error)
	}
	if len(lc.Content) > 130 { // 100 + "... (truncated)"
		t.Errorf("Expected truncated content, got length: %d", len(lc.Content))
	}
}

func TestFetchLinkContent_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	lc := FetchLinkContent(server.URL, 10000)
	if lc.Error == "" {
		t.Error("Expected error for 404 response")
	}
}

func TestEnrichMessageWithLinks_NoURLs(t *testing.T) {
	input := "Hello, how are you?"
	result := EnrichMessageWithLinks(input, 5000)
	if result != input {
		t.Errorf("Expected unchanged message, got: %s", result)
	}
}

func TestEnrichMessageWithLinks_WithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Test</title></head><body><p>Extracted</p></body></html>"))
	}))
	defer server.Close()

	input := "Check this: " + server.URL
	result := EnrichMessageWithLinks(input, 5000)
	if result == input {
		t.Error("Expected enriched message with link content")
	}
}

func TestExtractHTMLTitle(t *testing.T) {
	tests := []struct {
		html     string
		expected string
	}{
		{"<html><head><title>Test</title></head></html>", "Test"},
		{"<html><head><TITLE>Upper</TITLE></head></html>", "Upper"},
		{"<html><body>no title</body></html>", ""},
	}

	for _, tt := range tests {
		got := extractHTMLTitle(tt.html)
		if got != tt.expected {
			t.Errorf("extractHTMLTitle(%q) = %q, want %q", tt.html, got, tt.expected)
		}
	}
}
