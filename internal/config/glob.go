package config

import (
	"path"
	"strings"
)

// Selector decides which paths (smali class paths, resource paths, native libs)
// a config's include/exclude globs select. Globs support `*` (within a path
// segment), `**` (any number of segments) and a leading `!` to negate. Exclude
// always wins; with no include patterns everything is selected.
type Selector struct {
	includes []string
	excludes []string
}

// NewSelector builds a selector from include and exclude glob lists. A `!`-prefixed
// entry in either list is treated as an exclude.
func NewSelector(include, exclude []string) *Selector {
	s := &Selector{}
	add := func(p string, dst *[]string, neg *[]string) {
		if strings.HasPrefix(p, "!") {
			*neg = append(*neg, strings.TrimPrefix(p, "!"))
		} else {
			*dst = append(*dst, p)
		}
	}
	for _, p := range include {
		add(p, &s.includes, &s.excludes)
	}
	for _, p := range exclude {
		add(p, &s.excludes, &s.excludes)
	}
	return s
}

// Match reports whether path is selected: not matched by any exclude, and (if any
// includes are set) matched by at least one include.
func (s *Selector) Match(p string) bool {
	p = strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, "\\", "/")), "./")
	for _, e := range s.excludes {
		if matchGlob(e, p) {
			return false
		}
	}
	if len(s.includes) == 0 {
		return true
	}
	for _, i := range s.includes {
		if matchGlob(i, p) {
			return true
		}
	}
	return false
}

// matchGlob matches a `**`/`*` glob against a slash-separated path.
func matchGlob(pattern, name string) bool {
	return matchSegs(strings.Split(strings.Trim(pattern, "/"), "/"),
		strings.Split(strings.Trim(name, "/"), "/"))
}

func matchSegs(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true // trailing ** matches any remainder
			}
			for i := 0; i <= len(name); i++ {
				if matchSegs(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], name[0]); !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
