# AetherClaw Brand Assets

## Files

| File | Purpose |
|------|---------|
| `aetherclaw-mark.svg` | Primary mark using `currentColor` — adapts to light/dark contexts. Use as favicon, docs header, or web icon. |
| `aetherclaw-mark-mono.svg` | Pure black mark on transparent background. Use for print or monochrome contexts. |
| `terminal-banner.txt` | ASCII banner for CLI startup. Width <= 48 chars, ASCII-only for cross-platform compatibility. |
| `terminal-icon.txt` | Compact ASCII icon `(///)` for log prefixes and inline CLI usage. |

## Design

- **Mark**: Thin halo ring + three diagonal slashes (`///`) inside
- **ViewBox**: 0 0 256 256 — renders cleanly at 16x16 through 512x512
- **Colors**: No gradients, no shadows, no blur — single stroke color only

## Usage

### Favicon
```html
<link rel="icon" href="aetherclaw-mark.svg" type="image/svg+xml">
```

### CLI
The `internal/brand` Go package embeds these as constants. See `brand.go`.

### Docs / README
```markdown
![AetherClaw](assets/brand/aetherclaw-mark.svg)
```
