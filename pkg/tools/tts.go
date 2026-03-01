package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AetherClawTech/aetherclaw/pkg/tts"
)

// TTSTool provides text-to-speech synthesis to the agent.
type TTSTool struct {
	manager *tts.Manager
}

func NewTTSTool(manager *tts.Manager) *TTSTool {
	if manager == nil || !manager.IsAvailable() {
		return nil
	}
	return &TTSTool{manager: manager}
}

func (t *TTSTool) Name() string { return "tts" }
func (t *TTSTool) Description() string {
	return "Convert text to speech audio. Generates an audio file that can be sent via messaging channels. Supports multiple voices."
}

func (t *TTSTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to convert to speech",
			},
			"voice": map[string]interface{}{
				"type":        "string",
				"description": "Voice to use (e.g., 'alloy', 'echo', 'nova' for OpenAI; voice ID for ElevenLabs)",
			},
			"speed": map[string]interface{}{
				"type":        "number",
				"description": "Speech speed (0.25-4.0, default 1.0)",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'speak' to generate audio, 'voices' to list available voices",
				"enum":        []string{"speak", "voices"},
			},
		},
		"required": []string{"text"},
	}
}

func (t *TTSTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		action = "speak"
	}

	if action == "voices" {
		voices := t.manager.ListVoices()
		data, _ := json.MarshalIndent(voices, "", "  ")
		return SilentResult(string(data))
	}

	text, _ := args["text"].(string)
	if text == "" {
		return ErrorResult("text is required")
	}

	// Limit text length
	if len(text) > 4096 {
		text = text[:4096]
	}

	opts := tts.SynthesizeOptions{}
	if voice, ok := args["voice"].(string); ok {
		opts.Voice = voice
	}
	if speed, ok := args["speed"].(float64); ok {
		opts.Speed = speed
	}

	result, err := t.manager.Synthesize(ctx, text, opts)
	if err != nil {
		return ErrorResult(fmt.Sprintf("TTS failed: %v", err))
	}

	output := map[string]interface{}{
		"file_path": result.FilePath,
		"format":    result.Format,
		"size":      result.Size,
	}
	data, _ := json.Marshal(output)
	return &ToolResult{
		ForLLM:  string(data),
		ForUser: fmt.Sprintf("Audio generated: %s (%d bytes)", result.FilePath, result.Size),
	}
}
