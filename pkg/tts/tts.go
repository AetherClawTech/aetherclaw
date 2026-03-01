package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Provider defines the TTS provider interface.
type Provider interface {
	Name() string
	Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*AudioResult, error)
	IsAvailable() bool
}

// SynthesizeOptions configures TTS synthesis.
type SynthesizeOptions struct {
	Voice  string  // voice identifier
	Speed  float64 // speech speed (0.25-4.0, default 1.0)
	Format string  // output format: "mp3", "opus", "aac", "flac"
}

// AudioResult holds the synthesized audio.
type AudioResult struct {
	FilePath string  // path to the saved audio file
	Format   string  // audio format
	Duration float64 // duration in seconds (estimated)
	Size     int64   // file size in bytes
}

// OpenAITTS implements TTS via OpenAI's API.
type OpenAITTS struct {
	apiKey string
	model  string // "tts-1" or "tts-1-hd"
}

func NewOpenAITTS(apiKey string) *OpenAITTS {
	if apiKey == "" {
		return nil
	}
	return &OpenAITTS{
		apiKey: apiKey,
		model:  "tts-1",
	}
}

func (t *OpenAITTS) Name() string { return "openai" }

func (t *OpenAITTS) IsAvailable() bool { return t.apiKey != "" }

func (t *OpenAITTS) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*AudioResult, error) {
	voice := opts.Voice
	if voice == "" {
		voice = "alloy" // default voice
	}

	speed := opts.Speed
	if speed <= 0 {
		speed = 1.0
	}

	format := opts.Format
	if format == "" {
		format = "mp3"
	}

	reqBody := map[string]any{
		"model":           t.model,
		"input":           text,
		"voice":           voice,
		"speed":           speed,
		"response_format": format,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.openai.com/v1/audio/speech",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	// Save to temp file
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, fmt.Sprintf("aetherclaw-tts-*.%s", format))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	size, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}

	return &AudioResult{
		FilePath: tmpFile.Name(),
		Format:   format,
		Size:     size,
	}, nil
}

// ElevenLabsTTS implements TTS via ElevenLabs API.
type ElevenLabsTTS struct {
	apiKey string
}

func NewElevenLabsTTS(apiKey string) *ElevenLabsTTS {
	if apiKey == "" {
		return nil
	}
	return &ElevenLabsTTS{apiKey: apiKey}
}

func (t *ElevenLabsTTS) Name() string { return "elevenlabs" }

func (t *ElevenLabsTTS) IsAvailable() bool { return t.apiKey != "" }

func (t *ElevenLabsTTS) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*AudioResult, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel (default)
	}

	reqBody := map[string]any{
		"text":     text,
		"model_id": "eleven_monolingual_v1",
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xi-Api-Key", t.apiKey)
	req.Header.Set("Accept", "audio/mpeg")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "aetherclaw-tts-*.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	size, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}

	return &AudioResult{
		FilePath: tmpFile.Name(),
		Format:   "mp3",
		Size:     size,
	}, nil
}

// Manager manages multiple TTS providers with fallback.
type Manager struct {
	providers []Provider
	outputDir string
}

// NewManager creates a TTS manager with available providers.
// Pass only non-nil providers (check before calling).
func NewManager(outputDir string, providers ...Provider) *Manager {
	available := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p != nil && !isNilProvider(p) && p.IsAvailable() {
			available = append(available, p)
		}
	}

	os.MkdirAll(outputDir, 0o755)

	return &Manager{
		providers: available,
		outputDir: outputDir,
	}
}

// isNilProvider checks if a Provider interface holds a nil underlying value.
func isNilProvider(p Provider) bool {
	if p == nil {
		return true
	}
	// Use reflect-free approach: check each known concrete type
	switch v := p.(type) {
	case *OpenAITTS:
		return v == nil
	case *ElevenLabsTTS:
		return v == nil
	}
	return false
}

// IsAvailable returns true if at least one TTS provider is configured.
func (m *Manager) IsAvailable() bool {
	return len(m.providers) > 0
}

// Synthesize generates speech from text using the first available provider.
func (m *Manager) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*AudioResult, error) {
	if len(m.providers) == 0 {
		return nil, fmt.Errorf("no TTS providers configured")
	}

	var lastErr error
	for _, p := range m.providers {
		result, err := p.Synthesize(ctx, text, opts)
		if err != nil {
			lastErr = err
			continue
		}

		// Move to output directory if different
		if m.outputDir != "" {
			newPath := filepath.Join(m.outputDir, filepath.Base(result.FilePath))
			if err := os.Rename(result.FilePath, newPath); err == nil {
				result.FilePath = newPath
			}
		}

		return result, nil
	}

	return nil, fmt.Errorf("all TTS providers failed, last error: %w", lastErr)
}

// ListVoices returns available voices for each provider.
func (m *Manager) ListVoices() map[string][]string {
	voices := map[string][]string{
		"openai": {"alloy", "echo", "fable", "onyx", "nova", "shimmer"},
		"elevenlabs": {
			"Rachel (21m00Tcm4TlvDq8ikWAM)",
			"Domi",
			"Bella",
			"Antoni",
			"Elli",
			"Josh",
			"Arnold",
			"Adam",
			"Sam",
		},
	}
	return voices
}
