package config

import (
	"fmt"
	"strconv"
	"strings"
)

// A deliberately minimal, dependency-free YAML reader for SHIELD's flat config
// schema (see Config). It supports: `key: value`, nested maps by 2-space
// indentation, `- item` sequences, `#` comments, and quoted scalars. It does NOT
// support flow style ({}/[]), anchors, multi-line scalars or tabs — those error
// clearly. Keeping it stdlib-only preserves the zero-dependency core; the same
// discipline as the hand-rolled aapt2/protobuf manifest parser.

type yline struct {
	indent int
	text   string
}

func yamlToMap(data []byte) (map[string]any, error) {
	var lines []yline
	for n, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.Contains(leading(raw), "\t") {
			return nil, fmt.Errorf("yaml line %d: tabs are not allowed for indentation", n+1)
		}
		text := stripComment(raw)
		if strings.TrimSpace(text) == "" {
			continue
		}
		indent := len(text) - len(strings.TrimLeft(text, " "))
		lines = append(lines, yline{indent: indent, text: strings.TrimLeft(text, " ")})
	}
	m, _, err := parseMap(lines, 0, 0)
	return m, err
}

func parseMap(lines []yline, i, indent int) (map[string]any, int, error) {
	m := map[string]any{}
	for i < len(lines) {
		ln := lines[i]
		if ln.indent < indent {
			break
		}
		if ln.indent > indent {
			return nil, i, fmt.Errorf("yaml: unexpected indent at %q", ln.text)
		}
		if strings.HasPrefix(ln.text, "- ") || ln.text == "-" {
			return nil, i, fmt.Errorf("yaml: unexpected sequence item at map level: %q", ln.text)
		}
		key, val, ok := strings.Cut(ln.text, ":")
		if !ok {
			return nil, i, fmt.Errorf("yaml: expected 'key: value' at %q", ln.text)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if val != "" {
			m[key] = scalar(val)
			i++
			continue
		}
		// Nested block: peek the next content line's indent.
		if i+1 >= len(lines) || lines[i+1].indent <= indent {
			m[key] = nil
			i++
			continue
		}
		child := lines[i+1].indent
		if strings.HasPrefix(lines[i+1].text, "- ") || lines[i+1].text == "-" {
			seq, ni, err := parseSeq(lines, i+1, child)
			if err != nil {
				return nil, ni, err
			}
			m[key] = seq
			i = ni
		} else {
			sub, ni, err := parseMap(lines, i+1, child)
			if err != nil {
				return nil, ni, err
			}
			m[key] = sub
			i = ni
		}
	}
	return m, i, nil
}

func parseSeq(lines []yline, i, indent int) ([]any, int, error) {
	var out []any
	for i < len(lines) && lines[i].indent == indent {
		t := lines[i].text
		if !strings.HasPrefix(t, "-") {
			break
		}
		item := strings.TrimSpace(strings.TrimPrefix(t, "-"))
		if item == "" {
			return nil, i, fmt.Errorf("yaml: empty sequence item")
		}
		out = append(out, scalar(item))
		i++
	}
	return out, i, nil
}

func scalar(v string) any {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	switch v {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	case "null", "~":
		return nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return v
}

func stripComment(s string) string {
	inS, inD := false, false
	for i, r := range s {
		switch r {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case '#':
			if !inS && !inD && (i == 0 || s[i-1] == ' ') {
				return strings.TrimRight(s[:i], " ")
			}
		}
	}
	return strings.TrimRight(s, " ")
}

func leading(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}
