package brand

import (
	"fmt"
	"io"
)

// Icon is the compact ASCII icon for log prefixes and inline usage.
const Icon = "(///)"

// Banner is the ASCII art banner for CLI startup display.
const Banner = `     .-------.
    /  / / /  \
   |  / / /    |
   | / / /     |
    \         /
     '-------'
    AetherClaw`

// PrintBanner writes the ASCII banner to w.
func PrintBanner(w io.Writer) {
	fmt.Fprintln(w, Banner)
}

// PrintIcon writes the compact ASCII icon to w.
func PrintIcon(w io.Writer) {
	fmt.Fprint(w, Icon)
}
