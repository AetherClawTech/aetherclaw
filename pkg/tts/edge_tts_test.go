package tts

import (
	"strings"
	"testing"
)

func TestBuildSSML(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		voice    string
		pitch    string
		rate     string
		wantSub  string // must contain this substring
		wantEsc  string // XML-escaped version
	}{
		{
			name:    "basic text",
			text:    "Hello world",
			voice:   "en-US-AriaNeural",
			pitch:   "+0Hz",
			rate:    "+0%",
			wantSub: "Hello world",
		},
		{
			name:    "xml special chars",
			text:    "1 < 2 & 3 > 0",
			voice:   "en-US-AriaNeural",
			pitch:   "+0Hz",
			rate:    "+0%",
			wantEsc: "1 &lt; 2 &amp; 3 &gt; 0",
		},
		{
			name:    "quotes escaped",
			text:    `He said "hello" & 'bye'`,
			voice:   "en-US-GuyNeural",
			pitch:   "+0Hz",
			rate:    "+10%",
			wantEsc: "He said &quot;hello&quot; &amp; &apos;bye&apos;",
		},
		{
			name:    "voice set",
			text:    "test",
			voice:   "zh-CN-XiaoxiaoNeural",
			pitch:   "+0Hz",
			rate:    "+0%",
			wantSub: "zh-CN-XiaoxiaoNeural",
		},
		{
			name:    "rate set",
			text:    "fast",
			voice:   "en-US-AriaNeural",
			pitch:   "+0Hz",
			rate:    "+50%",
			wantSub: "rate='+50%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSSML(tt.text, tt.voice, tt.pitch, tt.rate)

			if !strings.Contains(got, "<speak") {
				t.Error("missing <speak> root element")
			}
			if !strings.Contains(got, "</speak>") {
				t.Error("missing </speak> closing tag")
			}

			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("missing substring %q in SSML:\n%s", tt.wantSub, got)
			}
			if tt.wantEsc != "" && !strings.Contains(got, tt.wantEsc) {
				t.Errorf("missing escaped substring %q in SSML:\n%s", tt.wantEsc, got)
			}

			// Must not contain raw special chars (except in XML tags)
			if tt.wantEsc != "" {
				// The raw text shouldn't appear unescaped
				if strings.Contains(got, tt.text) && strings.ContainsAny(tt.text, "<>&") {
					t.Errorf("text was not properly escaped in SSML")
				}
			}
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if len(id1) != 32 {
		t.Errorf("expected 32 hex chars, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("two consecutive IDs should not be equal")
	}
}

func TestEdgeTTSVoices(t *testing.T) {
	voices := EdgeTTSVoices()
	if len(voices) == 0 {
		t.Error("expected at least one voice")
	}

	// Check for common voices
	hasEnUS := false
	hasZhCN := false
	for _, v := range voices {
		if strings.HasPrefix(v, "en-US-") {
			hasEnUS = true
		}
		if strings.HasPrefix(v, "zh-CN-") {
			hasZhCN = true
		}
	}
	if !hasEnUS {
		t.Error("expected at least one en-US voice")
	}
	if !hasZhCN {
		t.Error("expected at least one zh-CN voice")
	}
}

func TestEdgeTTSBasicProps(t *testing.T) {
	e := NewEdgeTTS()
	if e.Name() != "edge" {
		t.Errorf("expected name 'edge', got %q", e.Name())
	}
	if !e.IsAvailable() {
		t.Error("EdgeTTS should always be available")
	}
}
