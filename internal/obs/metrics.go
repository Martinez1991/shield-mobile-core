package obs

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Metrics is a tiny, dependency-free metrics registry that renders the
// Prometheus text exposition format (shield-platform.md section 21, issues #18
// and #21). It covers counters, gauges and simple histograms — enough for
// per-pipeline-stage latency/failure metrics and the queue-depth autoscaling
// signal — without pulling in the Prometheus client library (the engine and its
// tooling stay stdlib-only).
type Metrics struct {
	mu         sync.Mutex
	counters   map[string]*series
	gauges     map[string]*series
	histograms map[string]*histogram
	gaugeFns   map[string]func() float64
}

type series struct {
	help   string
	values map[string]float64 // label-set key -> value
}

type histogram struct {
	help    string
	buckets []float64
	counts  map[string][]uint64 // label-set key -> per-bucket counts (+Inf last)
	sums    map[string]float64
	totals  map[string]uint64
}

// NewMetrics creates an empty registry.
func NewMetrics() *Metrics {
	return &Metrics{
		counters:   map[string]*series{},
		gauges:     map[string]*series{},
		histograms: map[string]*histogram{},
		gaugeFns:   map[string]func() float64{},
	}
}

func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%q", k, labels[k])
	}
	return b.String()
}

// IncCounter adds delta to a counter series (created on first use).
func (m *Metrics) IncCounter(name, help string, delta float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.counters[name]
	if s == nil {
		s = &series{help: help, values: map[string]float64{}}
		m.counters[name] = s
	}
	s.values[labelKey(labels)] += delta
}

// SetGauge sets a gauge series value.
func (m *Metrics) SetGauge(name, help string, v float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.gauges[name]
	if s == nil {
		s = &series{help: help, values: map[string]float64{}}
		m.gauges[name] = s
	}
	s.values[labelKey(labels)] = v
}

// RegisterGaugeFunc registers a callback sampled at scrape time (e.g. queue
// depth), so callers don't have to push updates.
func (m *Metrics) RegisterGaugeFunc(name, help string, fn func() float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gauges[name] == nil {
		m.gauges[name] = &series{help: help, values: map[string]float64{}}
	}
	m.gaugeFns[name] = fn
}

// DefaultBuckets are seconds-scale latency buckets for pipeline-stage timing.
var DefaultBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60}

// ObserveHistogram records a value (created on first use with DefaultBuckets).
func (m *Metrics) ObserveHistogram(name, help string, v float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.histograms[name]
	if h == nil {
		h = &histogram{help: help, buckets: DefaultBuckets, counts: map[string][]uint64{}, sums: map[string]float64{}, totals: map[string]uint64{}}
		m.histograms[name] = h
	}
	key := labelKey(labels)
	if h.counts[key] == nil {
		h.counts[key] = make([]uint64, len(h.buckets)+1)
	}
	for i, ub := range h.buckets {
		if v <= ub {
			h.counts[key][i]++
		}
	}
	h.counts[key][len(h.buckets)]++ // +Inf
	h.sums[key] += v
	h.totals[key]++
}

// Handler renders the registry in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(m.render()))
	})
}

func braces(key string) string {
	if key == "" {
		return ""
	}
	return "{" + key + "}"
}

func (m *Metrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder

	writeSeries := func(kind string, byName map[string]*series) {
		names := sortedKeys(byName)
		for _, name := range names {
			s := byName[name]
			if s.help != "" {
				fmt.Fprintf(&b, "# HELP %s %s\n", name, s.help)
			}
			fmt.Fprintf(&b, "# TYPE %s %s\n", name, kind)
			if fn := m.gaugeFns[name]; fn != nil {
				s.values[""] = fn()
			}
			for _, key := range sortedKeys(s.values) {
				fmt.Fprintf(&b, "%s%s %g\n", name, braces(key), s.values[key])
			}
		}
	}
	writeSeries("counter", m.counters)
	writeSeries("gauge", m.gauges)

	for _, name := range sortedKeys(m.histograms) {
		h := m.histograms[name]
		if h.help != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", name, h.help)
		}
		fmt.Fprintf(&b, "# TYPE %s histogram\n", name)
		for _, key := range sortedKeysU(h.counts) {
			counts := h.counts[key]
			for i, ub := range h.buckets {
				lbl := withLE(key, fmt.Sprintf("%g", ub))
				fmt.Fprintf(&b, "%s_bucket%s %d\n", name, braces(lbl), counts[i])
			}
			fmt.Fprintf(&b, "%s_bucket%s %d\n", name, braces(withLE(key, "+Inf")), counts[len(h.buckets)])
			fmt.Fprintf(&b, "%s_sum%s %g\n", name, braces(key), h.sums[key])
			fmt.Fprintf(&b, "%s_count%s %d\n", name, braces(key), h.totals[key])
		}
	}
	return b.String()
}

func withLE(key, le string) string {
	le = fmt.Sprintf("le=%q", le)
	if key == "" {
		return le
	}
	return key + "," + le
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysU(m map[string][]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
