package tts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	edgeTTSWSURL       = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeTTSTrustedToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeTTSOrigin       = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"
)

// EdgeTTS implements TTS using Microsoft Edge's free speech service.
// No API key required — uses the same endpoint as the Edge browser's Read Aloud.
type EdgeTTS struct {
	voice string // default voice
}

// NewEdgeTTS creates a new Edge TTS provider.
func NewEdgeTTS() *EdgeTTS {
	return &EdgeTTS{
		voice: "en-US-AriaNeural",
	}
}

func (t *EdgeTTS) Name() string        { return "edge" }
func (t *EdgeTTS) IsAvailable() bool   { return true }

func (t *EdgeTTS) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*AudioResult, error) {
	voice := opts.Voice
	if voice == "" {
		voice = t.voice
	}

	pitch := "+0Hz"
	rate := "+0%"
	if opts.Speed > 0 && opts.Speed != 1.0 {
		pct := int((opts.Speed - 1.0) * 100)
		if pct >= 0 {
			rate = fmt.Sprintf("+%d%%", pct)
		} else {
			rate = fmt.Sprintf("%d%%", pct)
		}
	}

	reqID := generateRequestID()
	ssml := buildSSML(text, voice, pitch, rate)

	audioData, err := t.synthesizeWS(ctx, reqID, ssml)
	if err != nil {
		return nil, fmt.Errorf("edge tts synthesis: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "aetherclaw-tts-edge-*.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	n, err := tmpFile.Write(audioData)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to write audio: %w", err)
	}

	return &AudioResult{
		FilePath: tmpFile.Name(),
		Format:   "mp3",
		Size:     int64(n),
	}, nil
}

func (t *EdgeTTS) synthesizeWS(ctx context.Context, reqID, ssml string) ([]byte, error) {
	wsURL := fmt.Sprintf(
		"%s?TrustedClientToken=%s&ConnectionId=%s",
		edgeTTSWSURL, edgeTTSTrustedToken, reqID,
	)

	header := http.Header{
		"Origin":     {edgeTTSOrigin},
		"User-Agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("websocket connect: %w", err)
	}
	defer conn.Close()

	// Send speech config
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	configMsg := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},`+
			`"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`,
		timestamp,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(configMsg)); err != nil {
		return nil, fmt.Errorf("send config: %w", err)
	}

	// Send SSML
	ssmlMsg := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%s\r\nPath:ssml\r\n\r\n%s",
		reqID, timestamp, ssml,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("send ssml: %w", err)
	}

	// Read audio frames
	var audioData []byte
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			// If we already have audio data, this might just be the server closing
			if len(audioData) > 0 {
				break
			}
			return nil, fmt.Errorf("read message: %w", err)
		}

		switch msgType {
		case websocket.BinaryMessage:
			// Binary frames: 2-byte header length + header + audio data
			if len(data) < 2 {
				continue
			}
			headerLen := int(data[0])<<8 | int(data[1])
			if len(data) > 2+headerLen {
				audioData = append(audioData, data[2+headerLen:]...)
			}
		case websocket.TextMessage:
			text := string(data)
			if strings.Contains(text, "turn.end") {
				return audioData, nil
			}
		}
	}

	if len(audioData) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return audioData, nil
}

func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func buildSSML(text, voice, pitch, rate string) string {
	// Escape XML special characters in text
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "'", "&apos;")
	text = strings.ReplaceAll(text, "\"", "&quot;")

	return fmt.Sprintf(
		`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>`+
			`<voice name='%s'>`+
			`<prosody pitch='%s' rate='%s'>%s</prosody>`+
			`</voice></speak>`,
		voice, pitch, rate, text,
	)
}

// EdgeTTSVoices returns commonly available Edge TTS voices.
func EdgeTTSVoices() []string {
	return []string{
		"en-US-AriaNeural",
		"en-US-GuyNeural",
		"en-US-JennyNeural",
		"en-GB-SoniaNeural",
		"zh-CN-XiaoxiaoNeural",
		"zh-CN-YunxiNeural",
		"ja-JP-NanamiNeural",
		"ko-KR-SunHiNeural",
		"fr-FR-DeniseNeural",
		"de-DE-KatjaNeural",
		"es-ES-ElviraNeural",
		"pt-BR-FranciscaNeural",
	}
}
