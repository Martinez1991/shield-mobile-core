package risk

import (
	"fmt"
	"math"
	"sort"
)

// Assessment is a method's risk verdict: a score in [0,1) with the explaining
// reasons and the raw features.
type Assessment struct {
	Features MethodFeatures `json:"features"`
	Score    float64        `json:"score"`
	Reasons  []string       `json:"reasons"`
}

// signal is one weighted risk contributor.
type signal struct {
	label  string
	value  float64
	weight float64
}

// squashK controls how fast the score saturates toward 1 (diminishing returns).
const squashK = 5.0

// Score combines features into a risk score in [0,1) plus the top contributing
// reasons (explainability). It is monotonic — every feature can only raise the
// score — and deterministic. Sensitive calls and secrets dominate; complexity
// and size are weak tie-breakers.
func Score(f MethodFeatures) (float64, []string) {
	signals := []signal{
		{"chamada(s) sensível(is)", float64(f.SensitiveCalls), 3.0},
		{"segredo(s)/token(s)", float64(f.SecretHits), 2.5},
		{"branch(es)", float64(f.Branches), 0.4},
		{"chamada(s)", float64(f.Calls), 0.15},
		{"string(s)", float64(f.ConstStrings), 0.1},
		{"instruç(ões)", float64(f.Instructions), 0.02},
	}
	var raw float64
	for _, s := range signals {
		raw += s.value * s.weight
	}
	score := 1 - math.Exp(-raw/squashK)

	// Reasons: the signals that actually contributed, strongest first.
	sort.SliceStable(signals, func(i, j int) bool {
		return signals[i].value*signals[i].weight > signals[j].value*signals[j].weight
	})
	var reasons []string
	for _, s := range signals {
		if s.value > 0 && len(reasons) < 3 {
			reasons = append(reasons, fmt.Sprintf("%d %s", int(s.value), s.label))
		}
	}
	return score, reasons
}

// Assess extracts features from a method block and scores it.
func Assess(block []string) Assessment {
	f := Analyze(block)
	score, reasons := Score(f)
	return Assessment{Features: f, Score: score, Reasons: reasons}
}
