package engine

import (
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// forEachMethod rewrites each method body (`.method` .. `.end method`, inclusive)
// via fn, leaving everything else untouched.
func forEachMethod(c *smali.Class, fn func(block []string) []string) {
	var out []string
	i := 0
	for i < len(c.Lines) {
		if strings.HasPrefix(strings.TrimSpace(c.Lines[i]), ".method ") {
			j := i
			for j < len(c.Lines) && strings.TrimSpace(c.Lines[j]) != ".end method" {
				j++
			}
			end := j
			if end >= len(c.Lines) {
				end = len(c.Lines) - 1
			}
			out = append(out, fn(c.Lines[i:end+1])...)
			i = end + 1
			continue
		}
		out = append(out, c.Lines[i])
		i++
	}
	c.Lines = out
}

// firstBodyIndex returns the index (within block) of the first executable line
// (instruction or label), skipping directives (.registers/.locals/.param/
// .prologue/.line/...) and whole .annotation blocks. Returns -1 if the method
// has no body (abstract/native).
func firstBodyIndex(block []string) int {
	inAnnotation := false
	for i := 1; i < len(block); i++ { // block[0] is the .method line
		t := strings.TrimSpace(block[i])
		switch {
		case inAnnotation:
			if strings.HasPrefix(t, ".end annotation") {
				inAnnotation = false
			}
		case t == "" || strings.HasPrefix(t, "#"):
		case strings.HasPrefix(t, ".annotation"):
			inAnnotation = true
		case strings.HasPrefix(t, "."):
			// .registers/.locals/.param/.prologue/.line/.end param/...
		case strings.HasPrefix(t, ".end method"):
			return -1
		default:
			return i // label (":...") or instruction
		}
	}
	return -1
}

// localsAtEntry computes how many local registers are free at method entry.
// With `.locals N` locals are v0..v(N-1). With `.registers N` locals are the
// low N-P registers (params occupy the high P). Returns (locals, true) or
// (0, false) if the method has no register directive (abstract/native).
func localsAtEntry(block []string) (int, bool) {
	decl := block[0]
	params := paramRegisters(decl)
	for i := 1; i < len(block); i++ {
		t := strings.TrimSpace(block[i])
		if n, ok := intAfter(t, ".locals "); ok {
			return n, true
		}
		if n, ok := intAfter(t, ".registers "); ok {
			return n - params, true
		}
	}
	return 0, false
}

func intAfter(line, prefix string) (int, bool) {
	if !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	n := 0
	for _, r := range rest {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// paramRegisters counts the register width of a method's parameters, including
// the implicit `this` for non-static methods. long/double take two registers.
func paramRegisters(methodDecl string) int {
	static := methodHasFlag(methodDecl, "static")
	sig := methodSignature(methodDecl) // "(params)ret"
	open := strings.IndexByte(sig, '(')
	close := strings.IndexByte(sig, ')')
	regs := 0
	if !static {
		regs++ // this
	}
	if open < 0 || close < 0 || close < open {
		return regs
	}
	params := sig[open+1 : close]
	for i := 0; i < len(params); {
		switch params[i] {
		case 'J', 'D':
			regs += 2
			i++
		case 'L':
			for i < len(params) && params[i] != ';' {
				i++
			}
			i++ // skip ';'
			regs++
		case '[':
			for i < len(params) && params[i] == '[' {
				i++
			}
			if i < len(params) && params[i] == 'L' {
				for i < len(params) && params[i] != ';' {
					i++
				}
				i++
			} else {
				i++ // primitive element
			}
			regs++ // an array is one object reference
		default: // Z B S C I F V
			regs++
			i++
		}
	}
	return regs
}

// methodSignature returns the "name(params)ret" tail of a `.method` decl.
func methodSignature(decl string) string {
	t := strings.TrimSpace(decl)
	t = strings.TrimPrefix(t, ".method")
	t = strings.TrimSpace(t)
	if p := strings.IndexByte(t, '('); p >= 0 {
		// walk back to the token start (method name) — but callers only need the
		// paren section for param counting, so returning from '(' is enough here.
		return t[p:]
	}
	return t
}

func methodHasFlag(decl, flag string) bool {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(decl), ".method"))
	head := t
	if p := strings.IndexByte(t, '('); p >= 0 {
		head = t[:p]
	}
	for _, f := range strings.Fields(head) {
		if f == flag {
			return true
		}
	}
	return false
}
