package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildIDDeterministic(t *testing.T) {
	a := BuildID("app.apk", "prod-high", "42")
	b := BuildID("app.apk", "prod-high", "42")
	if a != b {
		t.Fatal("BuildID must be deterministic")
	}
	if len(a) != 12 {
		t.Fatalf("BuildID len = %d, want 12", len(a))
	}
	if BuildID("app.apk", "prod-high", "43") == a {
		t.Error("different inputs should yield different ids")
	}
}

func TestJSONLoggerEmitsStructured(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "json", false)
	log.Info("obfuscation complete", "build_id", "abc123", "classes", 7)

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if m["build_id"] != "abc123" || m["msg"] != "obfuscation complete" {
		t.Errorf("missing structured fields: %v", m)
	}
}

func TestVerboseEnablesDebug(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, "text", true).Debug("hello", "k", "v")
	if !strings.Contains(buf.String(), "hello") {
		t.Error("verbose logger should emit debug lines")
	}
	buf.Reset()
	NewLogger(&buf, "text", false).Debug("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Error("non-verbose logger should suppress debug lines")
	}
}
