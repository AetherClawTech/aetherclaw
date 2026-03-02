package media

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/AetherClawTech/aetherclaw/pkg/providers"
)

const maxMediaFileSize = 20 * 1024 * 1024 // 20MB

// DetectMIME detects the MIME type from file header magic bytes.
// Supports JPEG, PNG, GIF, WebP.
func DetectMIME(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	header := make([]byte, 16)
	n, err := f.Read(header)
	if err != nil {
		return "", fmt.Errorf("read header: %w", err)
	}
	header = header[:n]

	return detectMIMEFromHeader(header)
}

func detectMIMEFromHeader(header []byte) (string, error) {
	if len(header) < 4 {
		return "", fmt.Errorf("file too small to detect MIME type")
	}

	// JPEG: FF D8 FF
	if header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return "image/jpeg", nil
	}

	// PNG: 89 50 4E 47
	if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 {
		return "image/png", nil
	}

	// GIF: 47 49 46 38
	if header[0] == 0x47 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x38 {
		return "image/gif", nil
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(header) >= 12 &&
		header[0] == 0x52 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x46 &&
		header[8] == 0x57 && header[9] == 0x45 && header[10] == 0x42 && header[11] == 0x50 {
		return "image/webp", nil
	}

	return "", fmt.Errorf("unsupported image format (header: %x)", header[:4])
}

// FileToContentPart reads a file, detects its MIME type, base64-encodes it,
// and returns a ContentPart suitable for multimodal LLM messages.
func FileToContentPart(path string) (*providers.ContentPart, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > maxMediaFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxMediaFileSize)
	}

	mimeType, err := DetectMIME(path)
	if err != nil {
		return nil, fmt.Errorf("detect mime: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	return &providers.ContentPart{
		Type: "image",
		Source: &providers.ImageSource{
			Type:      "base64",
			MediaType: mimeType,
			Data:      encoded,
			FilePath:  path,
		},
	}, nil
}
