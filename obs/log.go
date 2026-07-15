// Package obs provides minimal structured observability for the CLI/engine
// (shield-platform.md section 21; issue #17). Logs are structured (text or JSON)
// and correlated by a deterministic build_id, laying the groundwork for the full
// OTel/Prometheus stack.
package obs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"strings"
)

// NewLogger builds a slog.Logger writing to w. format is "text" or "json";
// verbose raises the level to Debug.
func NewLogger(w io.Writer, format string, verbose bool) *slog.Logger {
	lvl := slog.LevelInfo
	if verbose {
		lvl = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

// BuildID derives a short, deterministic correlation id from the build inputs
// (same input + policy + seed => same id, consistent with P2 determinism).
func BuildID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:12]
}
