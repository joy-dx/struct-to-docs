package main

import (
	"bytes"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Helpers -------------------------------------------------------

func initTempModule(t *testing.T, dir string) {
	t.Helper()
	mod := []byte("module example.com/testdata\n\ngo 1.21\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), mod, 0600); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
}

func writeTempGoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed writing temp Go file: %v", err)
	}
	return path
}

func captureOutput(f func()) string {
	var buf bytes.Buffer
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	buf.Write(out)
	return buf.String()
}

// --- Example Struct Definitions -----------------------------------

const exampleStructs = `package testdata

// Tool describes a software tool definition
type Tool struct {
	// Name Human readable title representing the tool name
	Name string ` + "`yaml:\"name\"`" + `
	// Ref A human readable but, machine safe identifier for the tool
	Ref ToolRef ` + "`yaml:\"ref\"`" + `
	// Summary a short description highlighting the purpose of the tool
	Summary string ` + "`yaml:\"summary\"`" + `
	// Description Long form description of the tool
	Description string ` + "`yaml:\"description\"`" + `
	// HomePage URL pointing to the official home page of the tool
	HomePage string ` + "`yaml:\"home_page\"`" + `
	// IconURL Custom URL pointing to an image that can be used where icons are presented to the user
	IconURL string ` + "`yaml:\"icon_url\"`" + `
	// License URL pointing to the tool license agreement
	License string ` + "`yaml:\"license\"`" + `
	// Dependencies A slice of tool references that need to also be available before tool installation / use
	Dependencies []ToolRef ` + "`yaml:\"dependencies\"`" + `
	// Binaries A slice of path references to executable files that should be made available to the environments
	Binaries []string ` + "`yaml:\"binaries\"`" + `
	// Environment System environment variable declarations to be included at operating time
	Environment []string ` + "`yaml:\"environment\"`" + `
	// Tags A slice of custom taxonomy that can be used to characterise the tool
	Tags []string ` + "`yaml:\"tags\"`" + `
	// Installs collection of install records letting the system know what is available for reuse
	Installs []*ToolInstall ` + "`yaml:\"installs\"`" + `
}

// ToolRef represents a unique reference identifier for tools
type ToolRef string

// ToolInstall represents a single installation record
type ToolInstall struct {
	// ToolRef A human readable but machine safe identifier for the tool
	ToolRef string ` + "`yaml:\"tool_ref\"`" + `
	// Version Semantic version string
	Version string ` + "`yaml:\"version\"`" + `
	// Platform What operating system this release record is relevant to
	Platform string ` + "`yaml:\"platform\"`" + `
}
`

// --- Tests --------------------------------------------------------

func TestParser_Load_SimpleStructs(t *testing.T) {
	tmpdir := t.TempDir()
	initTempModule(t, tmpdir)
	writeTempGoFile(t, tmpdir, "tool.go", exampleStructs)

	p := NewParser()
	if err := p.Load(tmpdir, "", true); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Basic checks
	if len(p.structs) == 0 {
		t.Fatalf("expected some structs, got none")
	}
	wantStructs := []string{"Tool", "ToolRef", "ToolInstall"}
	for _, w := range wantStructs {
		if _, ok := p.structs[w]; !ok {
			t.Errorf("expected struct %s to be parsed", w)
		}
	}
}

func TestParser_ExtractsDocsAndTags(t *testing.T) {
	tmpdir := t.TempDir()
	initTempModule(t, tmpdir)
	writeTempGoFile(t, tmpdir, "tool.go", exampleStructs)
	p := NewParser()
	if err := p.Load(tmpdir, "", true); err != nil {
		t.Fatal(err)
	}

	tool := p.structs["Tool"]
	if tool.Description != "Tool describes a software tool definition" {
		t.Errorf("unexpected struct doc: %q", tool.Description)
	}
	foundName := false
	for _, f := range tool.Fields {
		if f.Name == "Name" {
			foundName = true
			if !strings.Contains(f.Description, "Human readable title") {
				t.Errorf("expected doc comment for Name field, got %q", f.Description)
			}
			if f.YAMLTag != "name" {
				t.Errorf("expected yaml tag 'name', got %q", f.YAMLTag)
			}
		}
	}
	if !foundName {
		t.Error("Name field not found")
	}
}

func TestParser_RecursiveYAMLPrinting(t *testing.T) {
	tmpdir := t.TempDir()
	initTempModule(t, tmpdir)
	writeTempGoFile(t, tmpdir, "tool.go", exampleStructs)

	p := NewParser()
	if err := p.Load(tmpdir, "", true); err != nil {
		t.Fatal(err)
	}

	tool := p.structs["Tool"]
	out := captureOutput(func() {
		printStructHeader(tool)
		printYAML(p, tool, 0)
	})

	// Verify nested ToolInstall expansion
	if !strings.Contains(out, "tool_ref: <string>") {
		t.Errorf("expected nested ToolInstall content in output:\n%s", out)
	}
	if !strings.Contains(out, "dependencies: <[]ToolRef>") {
		t.Errorf("expected slice field in output:\n%s", out)
	}
}

func TestMatchStructPattern(t *testing.T) {
	tests := []struct {
		name, pattern string
		want          bool
	}{
		{"Tool", "Tool", true},
		{"ToolInstall", "Tool*", true},
		{"ToolInstall", "?ool*", true},
		{"Random", "Tool*", false},
	}
	for _, tt := range tests {
		if got := matchStructPattern(tt.name, tt.pattern); got != tt.want {
			t.Errorf("matchStructPattern(%q,%q)=%v, want %v", tt.name, tt.pattern, got, tt.want)
		}
	}
}

func TestEmbeddedStructHandling(t *testing.T) {
	tmpdir := t.TempDir()
	initTempModule(t, tmpdir)
	source := `package testdata
type Meta struct {
	// Common field
	ID string ` + "`yaml:\"id\"`" + `
}
type FullItem struct {
	// Embedded metadata
	Meta
	// CustomName Another field
	CustomName string ` + "`yaml:\"custom_name\"`" + `
}`
	writeTempGoFile(t, tmpdir, "embedded.go", source)

	p := NewParser()
	err := p.Load(tmpdir, "", true)
	if err != nil {
		t.Fatal(err)
	}

	item := p.structs["FullItem"]
	foundEmbedded := false
	for _, f := range item.Fields {
		if f.Embedded && f.Name == "meta" {
			foundEmbedded = true
		}
	}
	if !foundEmbedded {
		t.Error("expected embedded field detected")
	}
}

// Clean up or coverage for edge cases
func TestEmptyYAMLTagParsing(t *testing.T) {
	badTag := &ast.BasicLit{Value: "`json:\"foo\"`"}
	got := parseYAMLTag(badTag)
	if got != "" {
		t.Errorf("expected empty yaml tag, got %q", got)
	}
}

func TestResolveType(t *testing.T) {
	id := &ast.Ident{Name: "string"}
	got := resolveType(id)
	if got != "string" {
		t.Errorf("expected string, got %q", got)
	}
}

func TestStructDocPrinted(t *testing.T) {
	tmpdir := t.TempDir()
	initTempModule(t, tmpdir)

	source := `package testdata
// CoreArchiveExtractConfig Decompress a wide variety of archives
type CoreArchiveExtractConfig struct {
	Ref string ` + "`yaml:\"ref\"`" + `
}`
	writeTempGoFile(t, tmpdir, "archive.go", source)

	p := NewParser()
	if err := p.Load(tmpdir, "", true); err != nil {
		t.Fatal(err)
	}

	s := p.structs["CoreArchiveExtractConfig"]
	out := captureOutput(func() {
		printStructHeader(s)
		printYAML(p, s, 0)
	})

	if strings.Contains(out, "CoreArchiveExtractConfig Decompress") {
		t.Errorf("struct name should have been trimmed from description:\n%s", out)
	}
	if !strings.Contains(out, "Decompress a wide variety of archives") {
		t.Errorf("expected clean description in output:\n%s", out)
	}
}
