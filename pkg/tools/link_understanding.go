package tools

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/AetherClawTech/aetherclaw/pkg/logger"
)

// urlRegex matches HTTP/HTTPS URLs in text.
var urlRegex = regexp.MustCompile(`https?://[^\s<>\[\]()'"` + "`" + `]+`)

// ExtractURLs finds all HTTP/HTTPS URLs in the given text.
func ExtractURLs(text string) []string {
	matches := urlRegex.FindAllString(text, -1)
	// Deduplicate
	seen := make(map[string]bool)
	unique := make([]string, 0, len(matches))
	for _, url := range matches {
		// Trim trailing punctuation that's likely not part of the URL
		url = strings.TrimRight(url, ".,;:!?)")
		if !seen[url] {
			seen[url] = true
			unique = append(unique, url)
		}
	}
	return unique
}

// LinkContent holds extracted content from a URL.
type LinkContent struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// FetchLinkContent fetches and extracts text content from a URL.
// Returns extracted title and main content text.
func FetchLinkContent(url string, maxBytes int) *LinkContent {
	if maxBytes <= 0 {
		maxBytes = 10000
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &LinkContent{URL: url, Error: err.Error()}
	}
	req.Header.Set("User-Agent", "AetherClaw/1.0 (link-understanding)")
	req.Header.Set("Accept", "text/html, application/json, text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return &LinkContent{URL: url, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LinkContent{URL: url, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes*2)))
	if err != nil {
		return &LinkContent{URL: url, Error: err.Error()}
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)

	lc := &LinkContent{URL: url}

	if strings.Contains(contentType, "text/html") {
		lc.Title = extractHTMLTitle(text)
		lc.Content = extractHTMLText(text)
	} else if strings.Contains(contentType, "application/json") {
		lc.Content = text
	} else {
		lc.Content = text
	}

	// Truncate content
	if len(lc.Content) > maxBytes {
		lc.Content = lc.Content[:maxBytes] + "... (truncated)"
	}

	return lc
}

// extractHTMLTitle extracts the <title> content from HTML.
func extractHTMLTitle(html string) string {
	titleRe := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractHTMLText strips HTML tags and extracts readable text.
func extractHTMLText(html string) string {
	// Remove script and style blocks
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = scriptRe.ReplaceAllString(html, "")
	html = styleRe.ReplaceAllString(html, "")

	// Remove HTML comments
	commentRe := regexp.MustCompile(`(?s)<!--.*?-->`)
	html = commentRe.ReplaceAllString(html, "")

	// Remove nav, header, footer elements
	for _, tag := range []string{"nav", "header", "footer"} {
		tagRe := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = tagRe.ReplaceAllString(html, "")
	}

	// Strip all remaining HTML tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	text := tagRe.ReplaceAllString(html, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Collapse whitespace
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// EnrichMessageWithLinks detects URLs in a message and appends extracted content.
// Returns the enriched message content.
func EnrichMessageWithLinks(content string, maxPerLink int) string {
	urls := ExtractURLs(content)
	if len(urls) == 0 {
		return content
	}

	// Limit to 3 URLs to avoid overwhelming the context
	if len(urls) > 3 {
		urls = urls[:3]
	}

	if maxPerLink <= 0 {
		maxPerLink = 5000
	}

	var enrichments []string
	for _, url := range urls {
		lc := FetchLinkContent(url, maxPerLink)
		if lc.Error != "" {
			logger.DebugCF("link", "Failed to fetch link content",
				map[string]any{"url": url, "error": lc.Error})
			continue
		}
		if lc.Content == "" {
			continue
		}

		var parts []string
		if lc.Title != "" {
			parts = append(parts, fmt.Sprintf("Title: %s", lc.Title))
		}
		parts = append(parts, fmt.Sprintf("Content: %s", lc.Content))
		enrichments = append(enrichments, fmt.Sprintf("[Link: %s]\n%s", url, strings.Join(parts, "\n")))
	}

	if len(enrichments) == 0 {
		return content
	}

	return content + "\n\n---\n[Auto-extracted link content]\n" + strings.Join(enrichments, "\n\n")
}
