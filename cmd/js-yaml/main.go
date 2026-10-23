// Copyright (c) 2011-2026 Vitaly Puzrin and js-yaml contributors.
// Copyright (c) 2026 Go port contributors.
// Licensed under the MIT license; see LICENSE.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zahgon/js-yaml-go/internal/yaml"
)

type orderedObject struct{ pairs []objectPair }
type objectPair struct {
	key   string
	value any
}

type options struct {
	mode, schema, filename, styles, quotingType string
	all, jsonCompat, legacy, noArrayIndent      bool
	skipInvalid, sortKeys, noRefs, noCompatMode bool
	condenseFlow, forceQuotes                   bool
	indent, flowLevel, lineWidth                int
}

func main() {
	op, err := parseArgs(os.Args[1:])
	if err != nil {
		fail(err)
	}
	in, err := readInput(op.filename)
	if err != nil {
		fail(err)
	}
	var out []byte
	if op.mode == "load" {
		out, err = load(in, op)
	} else {
		out, err = dump(in, op)
	}
	if err != nil {
		fail(err)
	}
	if _, err = os.Stdout.Write(out); err != nil {
		fail(err)
	}
}

func parseArgs(args []string) (options, error) {
	op := options{mode: "load", schema: "default", filename: "-", indent: 2, flowLevel: -1, lineWidth: 80, quotingType: "'"}
	if len(args) > 0 && (args[0] == "load" || args[0] == "dump") {
		op.mode = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("js-yaml", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&op.schema, "schema", op.schema, "")
	fs.BoolVar(&op.all, "all", false, "")
	fs.BoolVar(&op.jsonCompat, "json", false, "")
	fs.BoolVar(&op.legacy, "legacy", false, "")
	fs.IntVar(&op.indent, "indent", 2, "")
	fs.BoolVar(&op.noArrayIndent, "no-array-indent", false, "")
	fs.BoolVar(&op.skipInvalid, "skip-invalid", false, "")
	fs.IntVar(&op.flowLevel, "flow-level", -1, "")
	fs.StringVar(&op.styles, "styles", "", "")
	fs.BoolVar(&op.sortKeys, "sort-keys", false, "")
	fs.IntVar(&op.lineWidth, "line-width", 80, "")
	fs.BoolVar(&op.noRefs, "no-refs", false, "")
	fs.BoolVar(&op.noCompatMode, "no-compat-mode", false, "")
	fs.BoolVar(&op.condenseFlow, "condense-flow", false, "")
	fs.StringVar(&op.quotingType, "quoting-type", "'", "")
	fs.BoolVar(&op.forceQuotes, "force-quotes", false, "")
	if err := fs.Parse(args); err != nil {
		return op, err
	}
	if fs.NArg() > 1 {
		return op, errors.New("too many input files")
	}
	if fs.NArg() == 1 {
		op.filename = fs.Arg(0)
	}
	if op.indent < 1 {
		op.indent = 1
	}
	if op.quotingType != "'" && op.quotingType != "\"" {
		return op, errors.New("quoting-type must be ' or \"")
	}
	return op, nil
}

func readInput(name string) ([]byte, error) {
	if name == "-" || name == "" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(name)
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func load(src []byte, op options) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	values := []any{}
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, formatYAMLError(err, src)
		}
		if len(doc.Content) == 0 {
			continue
		}
		values = append(values, nodeToJS(doc.Content[0], op, map[*yaml.Node]any{}))
		if !op.all {
			break
		}
	}
	var value any = nil
	if op.all {
		value = values
	} else if len(values) > 0 {
		value = values[0]
	}
	b, err := marshalJS(value, "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func nodeToJS(n *yaml.Node, op options, seen map[*yaml.Node]any) any {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return nodeToJS(n.Alias, op, seen)
	}
	switch n.Kind {
	case yaml.SequenceNode:
		r := make([]any, len(n.Content))
		seen[n] = r
		for i, c := range n.Content {
			r[i] = nodeToJS(c, op, seen)
		}
		return r
	case yaml.MappingNode:
		if n.ShortTag() == "!!set" {
			return orderedObject{}
		}
		r := orderedObject{}
		index := map[string]int{}
		// js-yaml merge keys are inserted first; explicit keys always win.
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "<<" {
				mergeObject(&r, index, nodeToJS(n.Content[i+1], op, seen))
			}
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Value == "<<" {
				continue
			}
			key := jsKey(nodeToJS(k, op, seen))
			val := nodeToJS(n.Content[i+1], op, seen)
			if j, ok := index[key]; ok {
				r.pairs[j].value = val
			} else {
				index[key] = len(r.pairs)
				r.pairs = append(r.pairs, objectPair{key, val})
			}
		}
		return r
	case yaml.ScalarNode:
		return scalarToJS(n, op)
	}
	return nil
}

func mergeObject(dst *orderedObject, index map[string]int, v any) {
	if a, ok := v.([]any); ok {
		for _, x := range a {
			mergeObject(dst, index, x)
		}
		return
	}
	o, ok := v.(orderedObject)
	if !ok {
		return
	}
	for _, p := range o.pairs {
		if _, exists := index[p.key]; !exists {
			index[p.key] = len(dst.pairs)
			dst.pairs = append(dst.pairs, p)
		}
	}
}
func jsKey(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return jsNumber(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func scalarToJS(n *yaml.Node, op options) any {
	tag := n.ShortTag()
	s := n.Value
	if tag == "!!binary" {
		if b, e := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, s)); e == nil {
			o := orderedObject{}
			for i, v := range b {
				o.pairs = append(o.pairs, objectPair{strconv.Itoa(i), float64(v)})
			}
			return o
		}
	}
	plain := n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) == 0
	if op.schema == "failsafe" && plain {
		return s
	}
	if plain {
		low := strings.ToLower(s)
		if op.schema == "default" {
			switch low {
			case "y", "yes", "on", "true":
				return true
			case "n", "no", "off", "false":
				return false
			case "null", "~":
				return nil
			}
		} else if low == "true" {
			return true
		} else if low == "false" {
			return false
		} else if low == "null" {
			return nil
		}
		if v, ok := resolvePlainNumber(s, op.schema); ok {
			return v
		}
		if op.schema == "default" {
			if t, ok := jsDate(s); ok {
				return t
			}
		}
		return s
	}
	switch tag {
	case "!!null":
		return nil
	case "!!bool":
		return strings.EqualFold(s, "true") || strings.EqualFold(s, "yes") || strings.EqualFold(s, "on")
	case "!!int":
		if v, ok := jsInt(s); ok {
			return v
		}
	case "!!float":
		if v, ok := jsFloat(s); ok {
			return v
		}
	case "!!timestamp":
		if op.schema == "default" {
			if t, ok := jsDate(s); ok {
				return t
			}
		}
	case "!!set":
		return orderedObject{}
	}
	return s
}

func resolvePlainNumber(s, schema string) (float64, bool) {
	x := strings.ReplaceAll(s, "_", "")
	low := strings.ToLower(x)
	if low == ".nan" || low == ".inf" || low == "+.inf" || low == "-.inf" {
		return jsFloat(x)
	}
	if schema == "default" { // YAML 1.1: 012 is octal, while 0o12 remains a string.
		sign := ""
		body := x
		if strings.HasPrefix(body, "+") || strings.HasPrefix(body, "-") {
			sign = body[:1]
			body = body[1:]
		}
		if strings.HasPrefix(body, "0o") {
			return 0, false
		}
		if len(body) > 1 && body[0] == '0' && !strings.ContainsAny(body, ".eE") {
			u, e := strconv.ParseUint(body[1:], 8, 64)
			if e == nil {
				v := float64(u)
				if sign == "-" {
					v = -v
				}
				return v, true
			}
		}
	}
	if strings.ContainsAny(x, ".eE") {
		return jsFloat(x)
	}
	return jsInt(x)
}

func jsInt(s string) (float64, bool) {
	x := strings.ReplaceAll(s, "_", "")
	sign := 1.0
	if strings.HasPrefix(x, "-") {
		sign = -1
		x = x[1:]
	} else if strings.HasPrefix(x, "+") {
		x = x[1:]
	}
	base := 10
	if strings.HasPrefix(x, "0b") {
		base = 2
		x = x[2:]
	} else if strings.HasPrefix(x, "0o") {
		base = 8
		x = x[2:]
	} else if strings.HasPrefix(x, "0x") {
		base = 16
		x = x[2:]
	}
	u, e := strconv.ParseUint(x, base, 64)
	if e != nil {
		return 0, false
	}
	return sign * float64(u), true
}
func jsFloat(s string) (float64, bool) {
	x := strings.ToLower(strings.ReplaceAll(s, "_", ""))
	switch x {
	case ".inf", "+.inf":
		return math.Inf(1), true
	case "-.inf":
		return math.Inf(-1), true
	case ".nan":
		return math.NaN(), true
	}
	v, e := strconv.ParseFloat(x, 64)
	return v, e == nil
}
func jsDate(s string) (string, bool) {
	layouts := []string{"2006-01-02", "2006-1-2", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05Z07:00", "2006-01-02T15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999 -07:00"}
	for _, l := range layouts {
		if t, e := time.Parse(l, s); e == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z"), true
		}
	}
	return "", false
}

func marshalJS(v any, indent string) ([]byte, error) {
	var b bytes.Buffer
	if err := writeJSON(&b, v, 0, indent); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func writeJSON(w *bytes.Buffer, v any, depth int, indent string) error {
	switch x := v.(type) {
	case orderedObject:
		w.WriteByte('{')
		for i, p := range x.pairs {
			if i > 0 {
				w.WriteByte(',')
			}
			w.WriteByte('\n')
			w.WriteString(strings.Repeat(indent, depth+1))
			kb, _ := json.Marshal(p.key)
			w.Write(kb)
			w.WriteString(": ")
			writeJSON(w, p.value, depth+1, indent)
		}
		if len(x.pairs) > 0 {
			w.WriteByte('\n')
			w.WriteString(strings.Repeat(indent, depth))
		}
		w.WriteByte('}')
	case []any:
		w.WriteByte('[')
		for i, q := range x {
			if i > 0 {
				w.WriteByte(',')
			}
			w.WriteByte('\n')
			w.WriteString(strings.Repeat(indent, depth+1))
			writeJSON(w, q, depth+1, indent)
		}
		if len(x) > 0 {
			w.WriteByte('\n')
			w.WriteString(strings.Repeat(indent, depth))
		}
		w.WriteByte(']')
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			w.WriteString("null")
		} else {
			w.WriteString(jsNumber(x))
		}
	default:
		b, e := json.Marshal(x)
		if e != nil {
			return e
		}
		w.Write(b)
	}
	return nil
}
func jsNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if i := strings.IndexByte(s, 'e'); i >= 0 {
		mant, exp := s[:i], s[i+1:]
		neg := strings.HasPrefix(exp, "-")
		exp = strings.TrimLeft(exp, "+-0")
		if exp == "" {
			exp = "0"
		}
		if neg {
			exp = "-" + exp
		} else {
			exp = "+" + exp
		}
		s = mant + "e" + exp
	}
	return s
}

func dump(src []byte, op options) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("unexpected value after JSON input")
		}
		return nil, err
	}
	n := jsToNode(v, op, 0, false)
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{n}}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(op.indent)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	out := b.String()
	if op.noArrayIndent {
		out = removeArrayIndent(out)
	}
	return []byte(out), nil
}
func jsToNode(v any, op options, depth int, isKey bool) *yaml.Node {
	switch x := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(x)}
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(x.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: x.String()}
	case string:
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: x}
		if op.forceQuotes && !isKey {
			if op.quotingType == "\"" {
				n.Style = yaml.DoubleQuotedStyle
			} else {
				n.Style = yaml.SingleQuotedStyle
			}
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if op.flowLevel >= 0 && depth >= op.flowLevel {
			n.Style = yaml.FlowStyle
		}
		for _, q := range x {
			n.Content = append(n.Content, jsToNode(q, op, depth+1, false))
		}
		return n
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if op.flowLevel >= 0 && depth >= op.flowLevel {
			n.Style = yaml.FlowStyle
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		if op.sortKeys {
			sort.Strings(keys)
		} else {
			sort.SliceStable(keys, func(i, j int) bool { return jsKeyLess(keys[i], keys[j]) })
		}
		for _, k := range keys {
			n.Content = append(n.Content, jsToNode(k, op, depth+1, true), jsToNode(x[k], op, depth+1, false))
		}
		return n
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
}
func jsKeyLess(a, b string) bool {
	ai, ae := strconv.ParseUint(a, 10, 32)
	bi, be := strconv.ParseUint(b, 10, 32)
	if ae == nil && be == nil {
		return ai < bi
	}
	if ae == nil {
		return true
	}
	if be == nil {
		return false
	}
	return a < b
}
func removeArrayIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		trim := strings.TrimLeft(l, " ")
		if strings.HasPrefix(trim, "- ") || trim == "-" {
			if len(l) >= 2 {
				lines[i] = l[2:]
			}
		}
	}
	return strings.Join(lines, "\n")
}
func formatYAMLError(err error, src []byte) error {
	text := string(src)
	// js-yaml reports the first following line whose indentation proves that an
	// unterminated flow collection cannot continue. The scanner foundation
	// reports the unmatched bracket itself, so locate that proof line here.
	if strings.Contains(err.Error(), "expected ',' or ']'") && strings.Contains(text, "[") {
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		openLine := -1
		for i, sourceLine := range lines {
			if strings.LastIndex(sourceLine, "[") > strings.LastIndex(sourceLine, "]") {
				openLine = i
			}
		}
		proofLine := openLine + 1
		for proofLine < len(lines) && strings.TrimSpace(lines[proofLine]) == "" {
			proofLine++
		}
		if proofLine >= len(lines) {
			proofLine = len(lines) - 1
		}
		line := proofLine + 1
		col := len(lines[proofLine]) - len(strings.TrimLeft(lines[proofLine], " ")) + 1
		var b strings.Builder
		fmt.Fprintf(&b, "YAMLException: deficient indentation (%d:%d)\n\n", line, col)
		start := line - 3
		if start < 1 {
			start = 1
		}
		for i := start; i <= line && i <= len(lines); i++ {
			fmt.Fprintf(&b, " %d | %s\n", i, lines[i-1])
		}
		b.WriteString(strings.Repeat("-", col+4) + "^")
		return errors.New(b.String())
	}
	return fmt.Errorf("YAMLException: %s", err)
}
