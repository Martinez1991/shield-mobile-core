package obs

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRender(t *testing.T) {
	m := NewMetrics()
	m.IncCounter("shield_builds_total", "builds", 2, map[string]string{"tenant": "acme"})
	m.IncCounter("shield_builds_total", "builds", 1, map[string]string{"tenant": "acme"})
	m.SetGauge("shield_inflight", "in-flight builds", 3, nil)
	m.RegisterGaugeFunc("shield_queue_depth", "pending jobs", func() float64 { return 5 })
	m.ObserveHistogram("shield_stage_seconds", "stage latency", 0.2, map[string]string{"stage": "protect"})
	m.ObserveHistogram("shield_stage_seconds", "stage latency", 2, map[string]string{"stage": "protect"})

	out := m.render()
	checks := []string{
		"# TYPE shield_builds_total counter",
		`shield_builds_total{tenant="acme"} 3`,
		"# TYPE shield_inflight gauge",
		"shield_inflight 3",
		"shield_queue_depth 5", // sampled via the gauge func
		"# TYPE shield_stage_seconds histogram",
		`shield_stage_seconds_bucket{stage="protect",le="0.5"} 1`,
		`shield_stage_seconds_bucket{stage="protect",le="+Inf"} 2`,
		`shield_stage_seconds_count{stage="protect"} 2`,
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("metrics output missing %q:\n%s", c, out)
		}
	}
}

func TestMetricsHandler(t *testing.T) {
	m := NewMetrics()
	m.SetGauge("g", "", 1, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "g 1") {
		t.Errorf("body = %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("content-type = %q", ct)
	}
}
