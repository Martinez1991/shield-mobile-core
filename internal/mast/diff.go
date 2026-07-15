package mast

// Differential is the core of the analyze → protect → verify loop: scan an app,
// protect it, scan again, and show what SHIELD's protection resolved. It is the
// risk-plane analogue of the golden/ART correctness gate (which proves the app
// still works); this proves the app got safer.
type Differential struct {
	Before        Report    `json:"-"`
	After         Report    `json:"-"`
	ScoreDelta    int       `json:"scoreDelta"`    // After.SecurityScore - Before.SecurityScore
	ResolvedHigh  []Finding `json:"resolvedHigh"`  // high findings present before, gone after
	RemainingHigh []Finding `json:"remainingHigh"` // high findings still present
}

// Diff compares a pre- and post-protection report by finding title.
func Diff(before, after Report) Differential {
	afterTitles := map[string]bool{}
	for _, f := range after.High {
		afterTitles[f.Title] = true
	}
	d := Differential{
		Before:     before,
		After:      after,
		ScoreDelta: after.SecurityScore - before.SecurityScore,
	}
	for _, f := range before.High {
		if afterTitles[f.Title] {
			d.RemainingHigh = append(d.RemainingHigh, f)
		} else {
			d.ResolvedHigh = append(d.ResolvedHigh, f)
		}
	}
	return d
}
