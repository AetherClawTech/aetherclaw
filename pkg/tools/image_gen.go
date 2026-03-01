package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ImageGenTool generates images using OpenAI DALL-E API.
type ImageGenTool struct {
	apiKey string
}

func NewImageGenTool(apiKey string) *ImageGenTool {
	if apiKey == "" {
		return nil
	}
	return &ImageGenTool{apiKey: apiKey}
}

func (t *ImageGenTool) Name() string { return "image_gen" }
func (t *ImageGenTool) Description() string {
	return "Generate an image from a text description using DALL-E. Returns the image URL."
}

func (t *ImageGenTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Text description of the image to generate",
			},
			"size": map[string]any{
				"type":        "string",
				"description": "Image size: 1024x1024, 1792x1024, or 1024x1792 (default: 1024x1024)",
				"enum":        []string{"1024x1024", "1792x1024", "1024x1792"},
			},
			"quality": map[string]any{
				"type":        "string",
				"description": "Image quality: standard or hd (default: standard)",
				"enum":        []string{"standard", "hd"},
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *ImageGenTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("prompt is required")
	}

	size, _ := args["size"].(string)
	if size == "" {
		size = "1024x1024"
	}

	quality, _ := args["quality"].(string)
	if quality == "" {
		quality = "standard"
	}

	reqBody := map[string]any{
		"model":   "dall-e-3",
		"prompt":  prompt,
		"n":       1,
		"size":    size,
		"quality": quality,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal request: %v", err))
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.openai.com/v1/images/generations",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("API request failed: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(respBody)))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse response: %v", err))
	}

	if len(result.Data) == 0 {
		return ErrorResult("no image generated")
	}

	output := map[string]string{
		"url":    result.Data[0].URL,
		"prompt": prompt,
	}
	if result.Data[0].RevisedPrompt != "" {
		output["revised_prompt"] = result.Data[0].RevisedPrompt
	}

	data, _ := json.Marshal(output)
	return &ToolResult{
		ForLLM:  string(data),
		ForUser: fmt.Sprintf("Image generated: %s", result.Data[0].URL),
	}
}
