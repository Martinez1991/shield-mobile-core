// Package ir is a Go-native, typed intermediate representation for smali methods.
//
// It promotes the ad-hoc, VM-local proto-IR (internal/engine/vm.go's method
// parser + register file) into a first-class, reusable model: a method is parsed
// into structured instructions with labels, and register types are reconstructed
// by dataflow analysis (see types.go). This is the foundation for #20 — enabling
// type-directed transforms (VM invoke marshalling, flattening) without leaving
// the pure-Go, zero-dependency engine. See docs/adr/0001-typed-ir.md.
package ir

import (
	"regexp"
	"strconv"
	"strings"
)

// Insn is one decoded smali instruction. Args are the raw operand tokens in
// order (registers like "v3"/"p1", literals, and type/method/field references).
type Insn struct {
	Op   string
	Args []string
}

// Method is a parsed smali method: its signature, register layout, instruction
// stream and label table (label name, including the leading ':', -> index of the
// instruction it precedes).
type Method struct {
	Decl      string
	Static    bool
	Name      string
	Params    string // raw parameter descriptor string, e.g. "IJLjava/lang/String;"
	Ret       string // return type descriptor, e.g. "I", "J", "Ljava/lang/String;"
	Regs      int    // total register count
	ParamBase int    // index of the first parameter register
	Insns     []Insn
	Labels    map[string]int
}

var (
	methodDeclRE = regexp.MustCompile(`^\s*\.method\s+((?:[\w-]+\s+)*)([\w$<>]+)\((.*)\)(\S+)\s*$`)
	regsRE       = regexp.MustCompile(`^\s*\.(registers|locals)\s+(\d+)\s*$`)
)

// ParseMethod parses a method block (from ".method ..." to ".end method") into a
// Method. It returns ok=false if the declaration or the register directive is
// missing/malformed.
func ParseMethod(block []string) (*Method, bool) {
	if len(block) == 0 {
		return nil, false
	}
	m := methodDeclRE.FindStringSubmatch(block[0])
	if m == nil {
		return nil, false
	}
	flags, name, params, ret := m[1], m[2], m[3], m[4]
	meth := &Method{
		Decl:   block[0],
		Static: strings.Contains(" "+flags, " static "),
		Name:   name,
		Params: params,
		Ret:    ret,
		Labels: map[string]int{},
	}

	regKind, regCount, ok := findRegs(block)
	if !ok {
		return nil, false
	}
	regWidth := paramRegWidth(params, meth.Static)
	if regKind == "locals" {
		meth.Regs = regCount + regWidth
		meth.ParamBase = regCount
	} else {
		meth.Regs = regCount
		meth.ParamBase = regCount - regWidth
	}
	if meth.Regs < 0 || meth.ParamBase < 0 {
		return nil, false
	}

	for _, ln := range methodBody(block) {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, ":") { // label binds to the next real instruction
			meth.Labels[t] = len(meth.Insns)
			continue
		}
		if strings.HasPrefix(t, ".") { // directive (.line, .prologue, .param, ...)
			continue
		}
		fields := splitFields(t)
		meth.Insns = append(meth.Insns, Insn{Op: fields[0], Args: fields[1:]})
	}
	return meth, true
}

func findRegs(block []string) (string, int, bool) {
	for _, ln := range block {
		if m := regsRE.FindStringSubmatch(ln); m != nil {
			n, _ := strconv.Atoi(m[2])
			return m[1], n, true
		}
	}
	return "", 0, false
}

func methodBody(block []string) []string {
	end := len(block)
	for i := len(block) - 1; i >= 0; i-- {
		if strings.TrimSpace(block[i]) == ".end method" {
			end = i
			break
		}
	}
	if len(block) < 2 || end < 1 {
		return nil
	}
	return block[1:end]
}

// splitFields tokenizes an instruction line: commas become spaces, then split on
// whitespace. (Type/method references contain no spaces, so this is safe.)
func splitFields(t string) []string {
	return strings.Fields(strings.ReplaceAll(t, ",", " "))
}

// paramRegWidth returns how many registers the parameters occupy, including the
// implicit `this` for instance methods (long/double take two).
func paramRegWidth(params string, static bool) int {
	w := 0
	if !static {
		w++ // this
	}
	for _, d := range SplitDescriptors(params) {
		if d == "J" || d == "D" {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// SplitDescriptors splits a bare descriptor list ("IJLjava/lang/String;[I") into
// individual type descriptors. Best-effort on malformed input.
func SplitDescriptors(desc string) []string {
	var out []string
	for i := 0; i < len(desc); {
		start := i
		for i < len(desc) && desc[i] == '[' {
			i++
		}
		if i >= len(desc) {
			out = append(out, desc[start:])
			break
		}
		if desc[i] == 'L' {
			j := strings.IndexByte(desc[i:], ';')
			if j < 0 {
				out = append(out, desc[start:])
				break
			}
			i += j + 1
		} else {
			i++
		}
		out = append(out, desc[start:i])
	}
	return out
}
