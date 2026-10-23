// Copyright (c) 2011-2026 Vitaly Puzrin and js-yaml contributors.
// Licensed under the MIT license; see LICENSE.
package main

import (
	"strings"
	"testing"
)

func TestLoadComposed(t *testing.T) {
	out, err := load([]byte("base: &b {yes: 012, text: café}\nout: {<<: *b, yes: 9}\n"), options{schema: "default"})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"base\": {\n    \"true\": 10,\n    \"text\": \"café\"\n  },\n  \"out\": {\n    \"true\": 9,\n    \"text\": \"café\"\n  }\n}\n"
	if string(out) != want {
		t.Fatalf("got %q", out)
	}
}

func TestLoadAllAndErrors(t *testing.T) {
	out, err := load([]byte("---\na: 1\n---\n- null\n"), options{schema: "default", all: true})
	if err != nil || !strings.Contains(string(out), "\"a\": 1") {
		t.Fatalf("%s %v", out, err)
	}
	_, err = load([]byte("x: [1, 2\n"), options{schema: "default"})
	if err == nil {
		t.Fatal("malformed YAML accepted")
	}
}

func TestSchemas(t *testing.T) {
	for _, tc := range []struct{ schema, input, contains string }{
		{"default", "yes\n", "true"}, {"core", "yes\n", "\"yes\""},
		{"json", "true\n", "true"}, {"failsafe", "123\n", "\"123\""},
	} {
		out, err := load([]byte(tc.input), options{schema: tc.schema})
		if err != nil || !strings.Contains(string(out), tc.contains) {
			t.Errorf("%s: %q %v", tc.schema, out, err)
		}
	}
}

func TestDumpOptions(t *testing.T) {
	out, err := dump([]byte(`{"b":"yes","a":[1,true,null]}`), options{indent: 2, flowLevel: -1, lineWidth: 80, sortKeys: true, quotingType: "'"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "a:\n") || !strings.Contains(string(out), "b:") {
		t.Fatalf("%q", out)
	}
}

func TestJavaScriptNumberFormatting(t *testing.T) {
	for _, tc := range []struct {
		v float64
		s string
	}{{0, "0"}, {1e21, "1e+21"}, {-2.5, "-2.5"}} {
		if got := jsNumber(tc.v); got != tc.s {
			t.Errorf("%v: %s", tc.v, got)
		}
	}
}
