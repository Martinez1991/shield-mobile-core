package mast

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeMobSF stands in for a MobSF instance: it checks the API key and serves
// canned upload/scan responses so the client is testable offline.
func fakeMobSF(t *testing.T, apiKey string, report reportEnvelope) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, _, err := r.FormFile("file"); err != nil {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(Uploaded{FileName: "app.apk", Hash: "abc123", ScanType: "apk"})
	})
	scan := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.FormValue("hash") != "abc123" {
			http.Error(w, "unknown hash", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(report)
	}
	mux.HandleFunc("/api/v1/scan", scan)
	mux.HandleFunc("/api/v1/report_json", scan)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sampleReport(score int, high ...string) reportEnvelope {
	var fs []Finding
	for _, h := range high {
		fs = append(fs, Finding{Title: h, Section: "code"})
	}
	return reportEnvelope{AppSec: &Report{SecurityScore: score, High: fs}}
}

func TestScanFile(t *testing.T) {
	srv := fakeMobSF(t, "SECRET-KEY", sampleReport(42, "Hardcoded secret", "Weak crypto"))
	c := New(srv.URL, "SECRET-KEY")

	path := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(path, []byte("PK\x03\x04 fake apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := c.ScanFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Hash != "abc123" || rep.SecurityScore != 42 {
		t.Errorf("report = %+v", rep)
	}
	if len(rep.High) != 2 || rep.High[0].Title != "Hardcoded secret" {
		t.Errorf("high findings = %+v", rep.High)
	}
}

func TestBadAPIKey(t *testing.T) {
	srv := fakeMobSF(t, "RIGHT", sampleReport(50))
	c := New(srv.URL, "WRONG")
	path := filepath.Join(t.TempDir(), "app.apk")
	_ = os.WriteFile(path, []byte("x"), 0o600)
	if _, err := c.ScanFile(context.Background(), path); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 unauthorized, got %v", err)
	}
}

func TestDiff(t *testing.T) {
	before := Report{SecurityScore: 40, High: []Finding{
		{Title: "Hardcoded secret"}, {Title: "Weak crypto"}, {Title: "No RASP"},
	}}
	after := Report{SecurityScore: 78, High: []Finding{{Title: "Weak crypto"}}}

	d := Diff(before, after)
	if d.ScoreDelta != 38 {
		t.Errorf("ScoreDelta = %d, want 38", d.ScoreDelta)
	}
	if len(d.ResolvedHigh) != 2 {
		t.Errorf("ResolvedHigh = %+v, want 2 (secret + RASP)", d.ResolvedHigh)
	}
	if len(d.RemainingHigh) != 1 || d.RemainingHigh[0].Title != "Weak crypto" {
		t.Errorf("RemainingHigh = %+v", d.RemainingHigh)
	}
}
