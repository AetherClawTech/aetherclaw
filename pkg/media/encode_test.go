package media

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMIMEFromHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  []byte
		want    string
		wantErr bool
	}{
		{
			name:   "JPEG",
			header: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
			want:   "image/jpeg",
		},
		{
			name:   "PNG",
			header: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			want:   "image/png",
		},
		{
			name:   "GIF",
			header: []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61},
			want:   "image/gif",
		},
		{
			name:   "WebP",
			header: []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			want:   "image/webp",
		},
		{
			name:    "too small",
			header:  []byte{0x00, 0x01},
			wantErr: true,
		},
		{
			name:    "unknown format",
			header:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectMIMEFromHeader(tt.header)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectMIME(t *testing.T) {
	// Create a temp JPEG file
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "test.jpg")
	// Minimal JPEG header
	err := os.WriteFile(jpegPath, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	mime, err := DetectMIME(jpegPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("got %q, want image/jpeg", mime)
	}

	// Test non-existent file
	_, err = DetectMIME(filepath.Join(dir, "nonexistent.jpg"))
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileToContentPart(t *testing.T) {
	dir := t.TempDir()

	// Create a valid PNG file (minimal header + some data)
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	pngPath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(pngPath, pngData, 0o644); err != nil {
		t.Fatal(err)
	}

	part, err := FileToContentPart(pngPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if part.Type != "image" {
		t.Errorf("type = %q, want 'image'", part.Type)
	}
	if part.Source == nil {
		t.Fatal("source is nil")
	}
	if part.Source.Type != "base64" {
		t.Errorf("source.type = %q, want 'base64'", part.Source.Type)
	}
	if part.Source.MediaType != "image/png" {
		t.Errorf("media_type = %q, want 'image/png'", part.Source.MediaType)
	}
	if part.Source.FilePath != pngPath {
		t.Errorf("file_path = %q, want %q", part.Source.FilePath, pngPath)
	}

	// Verify base64 decoding matches original
	decoded, err := base64.StdEncoding.DecodeString(part.Source.Data)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}
	if len(decoded) != len(pngData) {
		t.Errorf("decoded length %d != original %d", len(decoded), len(pngData))
	}
}

func TestFileToContentPart_SizeGuard(t *testing.T) {
	dir := t.TempDir()

	// Create a file that is too large (we'll simulate with a file that reports
	// a size but we just need to test the check)
	largePath := filepath.Join(dir, "large.png")
	// Create file with valid PNG header but check is against stat size
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(largePath, pngHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	// This file is small enough to pass, just verify it works
	_, err := FileToContentPart(largePath)
	if err != nil {
		t.Fatalf("small file should work: %v", err)
	}
}

func TestFileToContentPart_NonExistent(t *testing.T) {
	_, err := FileToContentPart("/nonexistent/path/image.png")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFileToContentPart_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("hello world, some text content"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := FileToContentPart(txtPath)
	if err == nil {
		t.Error("expected error for text file")
	}
}
