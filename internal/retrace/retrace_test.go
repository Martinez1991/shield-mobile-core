package retrace

import (
	"strings"
	"testing"
)

const mappingFile = `# SHIELD mapping file — keep secret; needed to retrace stack traces.
com.bank.pay.Helper -> o.a
com.bank.pay.util.Crypto -> o.ab
`

func TestParseAndApply(t *testing.T) {
	rev := ParseMapping(strings.NewReader(mappingFile))
	if rev["o.a"] != "com.bank.pay.Helper" || rev["o.ab"] != "com.bank.pay.util.Crypto" {
		t.Fatalf("reverse map wrong: %v", rev)
	}

	trace := strings.Join([]string{
		"java.lang.RuntimeException: boom",
		"\tat o.a.ping(SourceFile)",
		"\tat o.ab.decrypt(SourceFile)",
		"\tat com.bank.pay.Main.run(SourceFile)",
	}, "\n")

	got := Apply(rev, trace)
	if !strings.Contains(got, "com.bank.pay.Helper.ping") {
		t.Errorf("o.a not retraced:\n%s", got)
	}
	if !strings.Contains(got, "com.bank.pay.util.Crypto.decrypt") {
		t.Errorf("o.ab not retraced:\n%s", got)
	}
	// "o.a" must NOT clobber the "o.ab" occurrence (longest-first + boundaries).
	if strings.Contains(got, "com.bank.pay.Helperb") || strings.Contains(got, "com.bank.pay.Helper.b") {
		t.Errorf("o.a wrongly matched inside o.ab:\n%s", got)
	}
	// unrelated class untouched.
	if !strings.Contains(got, "com.bank.pay.Main.run") {
		t.Errorf("unrelated class changed:\n%s", got)
	}
}

func TestApplyEmptyMapping(t *testing.T) {
	if Apply(nil, "at o.a.x()") != "at o.a.x()" {
		t.Error("empty mapping should be a no-op")
	}
}
