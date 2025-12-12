package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type StructInfo struct {
	Name        string
	Description string
	Fields      []FieldInfo
	Package     string
	FilePath    string
}

type FieldInfo struct {
	Name        string
	Description string
	YAMLTag     string
	Type        string
	Embedded    bool
}

type Parser struct {
	structs map[string]StructInfo
}

func NewParser() *Parser {
	return &Parser{structs: make(map[string]StructInfo)}
}

func main() {
	dirPath := flag.String("dir", "", "Directory to parse (required)")
	structPattern := flag.String("struct", "", "Struct name or pattern (optional, supports * and ?)")
	dirFilter := flag.String("dir-filter", "", "Directory path filter (optional, supports * and ?)")
	allowCreateMod := flag.Bool("allow-create-go-mod", false, "Allow creating a temporary go.mod when none is found (intended for tests)")
	flag.Parse()

	if *dirPath == "" {
		fmt.Println("Please specify a directory using -dir flag")
		return
	}

	p := NewParser()
	if err := p.Load(*dirPath, *dirFilter, *allowCreateMod); err != nil {
		fmt.Printf("Error loading packages: %v\n", err)
		return
	}

	pattern := *structPattern
	names := sortedKeys(p.structs)

	var selected []StructInfo
	if pattern == "" {
		for _, n := range names {
			selected = append(selected, p.structs[n])
		}
	} else {
		for _, n := range names {
			if matchStructPattern(n, pattern) {
				selected = append(selected, p.structs[n])
			}
		}
		if len(selected) == 0 {
			fmt.Printf("No structs matched pattern '%s'.\n", pattern)
			return
		}
	}

	for _, s := range selected {
		printStructHeader(s)
		printYAML(p, s, 0, s.Name)
		fmt.Println()
	}
}

// --- Package Loading / AST Parsing ---

func (p *Parser) Load(dir, dirFilter string, allowCreate bool) error {
	// Try to locate go.mod from dir upward
	modPath, found := findGoModUp(dir)
	if !found {
		if allowCreate {
			tempPath := filepath.Join(dir, "go.mod")
			if err := os.WriteFile(
				tempPath,
				[]byte("module tempmod\n\ngo 1.21\n"),
				0600,
			); err != nil {
				return fmt.Errorf("failed to create temporary go.mod: %w", err)
			}
			modPath = tempPath
		} else {
			return fmt.Errorf("no go.mod found above '%s'; use -allow-create-go-mod for tests", dir)
		}
	}

	modDir := filepath.Dir(modPath)

	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedFiles,
		Dir:   modDir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {
			path := pkg.GoFiles[i]
			include := true
			if dirFilter != "" {
				absPath, _ := filepath.Abs(filepath.Dir(path))
				include = matchStructPattern(absPath, dirFilter)
			}
			if include {
				p.processFile(file, pkg.Name, path)
			}
		}
	}
	return nil
}

func (p *Parser) processFile(file *ast.File, pkgName, path string) {
	ast.Inspect(file, func(n ast.Node) bool {
		gen, ok := n.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			return true
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structName := typeSpec.Name.Name
			desc := extractDoc(gen.Doc, typeSpec.Doc)
			desc = cleanStructDoc(structName, desc)
			info := StructInfo{
				Name:        structName,
				Description: desc,
				Package:     pkgName,
				FilePath:    path,
			}
			if st, ok := typeSpec.Type.(*ast.StructType); ok {
				for _, f := range st.Fields.List {
					desc := extractDoc(f.Doc, nil)
					yamlTag := parseYAMLTag(f.Tag)
					fieldType := resolveType(f.Type)

					if len(f.Names) == 0 {
						// Anonymous embedded field
						info.Fields = append(info.Fields, FieldInfo{
							Name:        embeddedFieldNameFromType(fieldType),
							Description: desc,
							YAMLTag:     yamlTag,
							Type:        fieldType,
							Embedded:    true,
						})
						continue
					}

					for _, name := range f.Names {
						info.Fields = append(info.Fields, FieldInfo{
							Name:        name.Name,
							Description: desc,
							YAMLTag:     yamlTag,
							Type:        fieldType,
							Embedded:    false,
						})
					}
				}
			}
			p.structs[structName] = info
		}
		return true
	})
}

// --- Utilities ---

func sortedKeys(m map[string]StructInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func matchStructPattern(name, pattern string) bool {
	if !strings.ContainsAny(pattern, "*?") {
		return name == pattern
	}

	var re strings.Builder
	re.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			re.WriteString(".*")
		case '?':
			re.WriteString(".")
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re.WriteString("$")

	ok, err := regexp.MatchString(re.String(), name)
	return err == nil && ok
}

func extractDoc(groups ...*ast.CommentGroup) string {
	var lines []string
	for _, g := range groups {
		if g == nil {
			continue
		}
		for _, c := range g.List {
			txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if txt != "" {
				lines = append(lines, txt)
			}
		}
	}
	return strings.Join(lines, " ")
}

func parseYAMLTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	re := regexp.MustCompile(`yaml:"([^"]+)"`)
	m := re.FindStringSubmatch(tag.Value)
	if len(m) < 2 {
		return ""
	}
	return strings.Split(m[1], ",")[0]
}

func resolveType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + resolveType(t.X)
	case *ast.ArrayType:
		return "[]" + resolveType(t.Elt)
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return pkg.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	default:
		return "unknown"
	}
}

func embeddedFieldNameFromType(typ string) string {
	typ = strings.TrimPrefix(typ, "*")
	parts := strings.Split(typ, ".")
	return strings.ToLower(parts[len(parts)-1])
}

// --- Printing / Recursion ---

func printStructHeader(s StructInfo) {
	rel, _ := filepath.Rel(".", s.FilePath)
	fmt.Printf("### %s#%s\n\n", rel, s.Name)

	if s.Description != "" {
		fmt.Printf("%s\n\n", s.Description)
	}
}

func printYAML(p *Parser, s StructInfo, level int, seen ...string) {
	indent := strings.Repeat("  ", level)

	// track seen types for recursion guard
	seenMap := make(map[string]bool, len(seen))
	for _, v := range seen {
		seenMap[v] = true
	}

	for _, f := range s.Fields {
		if f.Description != "" {
			fmt.Printf("%s# %s\n", indent, f.Description)
		}
		name := f.YAMLTag
		if name == "" {
			name = strings.ToLower(f.Name)
		}

		typeName := strings.TrimPrefix(f.Type, "*")
		displayType := fmt.Sprintf("<%s>", f.Type)

		if strings.HasPrefix(f.Type, "[]") {
			elem := strings.TrimPrefix(strings.TrimPrefix(f.Type, "[]"), "*")
			fmt.Printf("%s%s: <%s>\n", indent, name, f.Type)
			if nested, ok := findStruct(p, elem); ok && len(nested.Fields) > 0 {
				// avoid cycles for slices too
				if seenMap[nested.Name] {
					fmt.Printf("%s  # (recursive reference to %s)\n", indent, nested.Name)
				} else {
					fmt.Printf("%s-\n", indent)
					printYAML(p, nested, level+1, append(seen, nested.Name)...)
				}
			}
			continue
		}

		if nested, ok := findStruct(p, typeName); ok && len(nested.Fields) > 0 {
			fmt.Printf("%s%s: %s\n", indent, name, displayType)
			if seenMap[nested.Name] {
				fmt.Printf("%s  # (recursive reference to %s)\n", indent, nested.Name)
			} else {
				printYAML(p, nested, level+1, append(seen, nested.Name)...)
			}
		} else {
			fmt.Printf("%s%s: %s\n", indent, name, displayType)
		}
	}
}

func findStruct(p *Parser, typeName string) (StructInfo, bool) {
	if s, ok := p.structs[typeName]; ok {
		return s, true
	}
	if strings.Contains(typeName, ".") {
		parts := strings.Split(typeName, ".")
		if len(parts) == 2 {
			if s, ok := p.structs[parts[1]]; ok {
				return s, true
			}
		}
	}
	return StructInfo{}, false
}

// findGoModUp walks upward from the given directory until it finds a go.mod file.
// Returns the path to the go.mod and whether it was found.
func findGoModUp(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			return modPath, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return "", false
}

func cleanStructDoc(structName, doc string) string {
	doc = strings.TrimSpace(doc)
	if strings.HasPrefix(doc, structName) {
		rest := strings.TrimSpace(strings.TrimPrefix(doc, structName))
		if rest != "" {
			firstRune := []rune(rest)[0]
			// Trim only if next word starts with uppercase (description style)
			if firstRune >= 'A' && firstRune <= 'Z' {
				return strings.TrimSpace(rest)
			}
		}
	}
	return doc
}
