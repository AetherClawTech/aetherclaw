package brand

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBannerNotEmpty(t *testing.T) {
	assert.NotEmpty(t, Banner)
}

func TestIconNotEmpty(t *testing.T) {
	assert.NotEmpty(t, Icon)
}

func TestBannerWidth(t *testing.T) {
	for i, line := range strings.Split(Banner, "\n") {
		assert.LessOrEqual(t, len(line), 48, "line %d exceeds 48 chars: %q", i, line)
	}
}

func TestIconWidth(t *testing.T) {
	assert.LessOrEqual(t, len(Icon), 10, "icon exceeds 10 chars: %q", Icon)
}

func TestBannerASCIIOnly(t *testing.T) {
	for _, r := range Banner {
		assert.LessOrEqual(t, r, rune(127), "non-ASCII rune in banner: %q", r)
	}
}

func TestIconASCIIOnly(t *testing.T) {
	for _, r := range Icon {
		assert.LessOrEqual(t, r, rune(127), "non-ASCII rune in icon: %q", r)
	}
}

func TestPrintBanner(t *testing.T) {
	var buf bytes.Buffer
	PrintBanner(&buf)
	assert.Contains(t, buf.String(), "AetherClaw")
	assert.Contains(t, buf.String(), "/ / /")
}

func TestPrintIcon(t *testing.T) {
	var buf bytes.Buffer
	PrintIcon(&buf)
	assert.Equal(t, Icon, buf.String())
}
