// Package analyze produces a static inventory + sensitive-code report over the
// smali IR (shield-platform.md sections 2.2 stages 2/4 and 9.1 "detecção
// automática de código sensível"). This is a heuristic MVP of what ai-svc does
// with models; here it is regex + Shannon entropy. No secret is stored in
// clear — only a masked preview (see the doc's responsibility note, section 9).
package analyze

import (
	"math"
	"regexp"

	"shield/internal/smali"
)

// Finding is one piece of sensitive data located in the binary.
type Finding struct {
	Class   string  `json:"class"`
	Kind    string  `json:"kind"`
	Preview string  `json:"preview"`
	Entropy float64 `json:"entropy"`
}

// Report is the analysis output (section 11.1 GET .../report shape, simplified).
type Report struct {
	Root     string    `json:"root"`
	Classes  int       `json:"classes"`
	Methods  int       `json:"methods"`
	Strings  int       `json:"strings"`
	Findings []Finding `json:"findings"`
}

var (
	methodRE      = regexp.MustCompile(`^\s*\.method\b`)
	constStringRE = regexp.MustCompile(`^\s*const-string(?:/jumbo)?\s+[vp]\d+,\s+"(.*)"\s*$`)

	secretRules = []struct {
		kind string
		re   *regexp.Regexp
	}{
		{"stripe-secret-key", regexp.MustCompile(`sk_live_[0-9A-Za-z]{10,}`)},
		{"aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
		{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}`)},
		{"private-key-block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`)},
	}
	highEntropyRE = regexp.MustCompile(`^[A-Za-z0-9+/=_\-]{20,}$`)
)

// Run analyzes the smali project rooted at root.
func Run(root string) (*Report, error) {
	classes, err := smali.LoadProject(root)
	if err != nil {
		return nil, err
	}
	rep := &Report{Root: root, Classes: len(classes)}
	for _, c := range classes {
		label := c.Descriptor
		if label == "" {
			label = c.Path
		}
		for _, ln := range c.Lines {
			if methodRE.MatchString(ln) {
				rep.Methods++
				continue
			}
			m := constStringRE.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			rep.Strings++
			val := m[1]
			if f, ok := classify(label, val); ok {
				rep.Findings = append(rep.Findings, f)
			}
		}
	}
	return rep, nil
}

// LooksSecret reports whether a string value matches a known secret pattern or
// is a high-entropy token — the same heuristic Run uses, exported so other passes
// (e.g. the risk scorer, issue #65) can score sensitive-string density.
func LooksSecret(val string) bool {
	_, ok := classify("", val)
	return ok
}

func classify(class, val string) (Finding, bool) {
	for _, r := range secretRules {
		if r.re.MatchString(val) {
			return Finding{Class: class, Kind: r.kind, Preview: mask(val), Entropy: shannon(val)}, true
		}
	}
	if highEntropyRE.MatchString(val) {
		if e := shannon(val); e >= 4.0 {
			return Finding{Class: class, Kind: "high-entropy-token", Preview: mask(val), Entropy: e}, true
		}
	}
	return Finding{}, false
}

// mask keeps a short prefix and hides the rest (never store secrets in clear).
func mask(s string) string {
	r := []rune(s)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:4]) + "…(" + itoa(len(r)) + " chars)"
}

// shannon returns the Shannon entropy (bits/byte) of s.
func shannon(s string) float64 {
	if s == "" {
		return 0
	}
	var freq [256]float64
	b := []byte(s)
	for _, c := range b {
		freq[c]++
	}
	n := float64(len(b))
	e := 0.0
	for _, f := range freq {
		if f == 0 {
			continue
		}
		p := f / n
		e -= p * math.Log2(p)
	}
	return e
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
