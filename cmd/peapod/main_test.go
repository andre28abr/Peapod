package main

import (
	"flag"
	"reflect"
	"testing"

	"peapod/internal/sandbox"
)

func TestSplitComma(t *testing.T) {
	if got := splitComma(""); got != nil {
		t.Errorf("splitComma(\"\") = %v, want nil", got)
	}
	if got := splitComma("   "); got != nil {
		t.Errorf("splitComma(blank) = %v, want nil", got)
	}
	if got := splitComma("a,b,c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("splitComma = %v", got)
	}
}

func TestParsePorts(t *testing.T) {
	got, err := parsePorts([]string{"8080:80", " 9090:90 ", ""})
	if err != nil {
		t.Fatalf("parsePorts: %v", err)
	}
	want := []sandbox.Port{{Host: 8080, Container: 80}, {Host: 9090, Container: 90}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePorts = %+v, want %+v", got, want)
	}
	if _, err := parsePorts([]string{"nope"}); err == nil {
		t.Error("parsePorts(\"nope\") should error")
	}
}

func TestParseMounts(t *testing.T) {
	ms := parseMounts([]string{"./src:/work", "/abs:/data", "garbage"}, "/base")
	want := []sandbox.Mount{
		{Host: "/base/src", Target: "/work"},
		{Host: "/abs", Target: "/data"},
	}
	if !reflect.DeepEqual(ms, want) {
		t.Errorf("parseMounts = %+v, want %+v", ms, want)
	}
}

// TestParseFlagsAnywhere guards the fix for Go's flag package stopping at the
// first positional — flags after the image must still be parsed.
func TestParseFlagsAnywhere(t *testing.T) {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "", "")
	net := fs.String("net", "none", "")
	parseFlagsAnywhere(fs, []string{"alpine", "--name", "x", "--net", "egress"})
	if *name != "x" {
		t.Errorf("name = %q, want x", *name)
	}
	if *net != "egress" {
		t.Errorf("net = %q, want egress", *net)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "alpine" {
		t.Errorf("positional args = %v, want [alpine]", fs.Args())
	}
}

func TestPreviewName(t *testing.T) {
	if got := previewName("/Users/a/Meu Repo", "feature/x"); got != "preview-Meu-Repo-feature-x" {
		t.Errorf("previewName = %q", got)
	}
}
