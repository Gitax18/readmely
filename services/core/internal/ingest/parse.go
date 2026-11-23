package ingest

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// Symbol is a function, class or type found in a source file.
type Symbol struct {
	Name string
	Kind string // function, method, class, type, interface, constant
	Line int
}

// Parsed is what reading one source file yields.
type Parsed struct {
	Symbols []Symbol
	Imports []string
	IsMain  bool // a Go file that declares package main with func main
}

const (
	maxSymbolsPerFile = 300
	maxImportsPerFile = 200
)

// Parse reads a source file. Go uses the standard library's parser; the other languages are read
// line by line with patterns, which is less exact (a snippet inside a string can look like a
// declaration) but needs no native code and never fails on a file that does not compile.
func Parse(lang string, content []byte) Parsed {
	switch lang {
	case "Go":
		return parseGo(content)
	case "Python":
		return parsePython(content)
	case "JavaScript", "TypeScript":
		return parseJS(content)
	}
	return Parsed{}
}

func parseGo(content []byte) Parsed {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if f == nil {
		return Parsed{}
	}
	var p Parsed
	add := func(name, kind string, pos token.Pos) {
		if len(p.Symbols) < maxSymbolsPerFile {
			p.Symbols = append(p.Symbols, Symbol{Name: name, Kind: kind, Line: fset.Position(pos).Line})
		}
	}
	for _, imp := range f.Imports {
		if s, err := strconv.Unquote(imp.Path.Value); err == nil && len(p.Imports) < maxImportsPerFile {
			p.Imports = append(p.Imports, s)
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "function"
			if d.Recv != nil {
				kind = "method"
			}
			if f.Name.Name == "main" && d.Name.Name == "main" && d.Recv == nil {
				p.IsMain = true
			}
			add(d.Name.Name, kind, d.Pos())
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					if _, ok := s.Type.(*ast.InterfaceType); ok {
						kind = "interface"
					}
					add(s.Name.Name, kind, s.Pos())
				case *ast.ValueSpec:
					if d.Tok == token.CONST {
						for _, n := range s.Names {
							if n.IsExported() {
								add(n.Name, "constant", n.Pos())
							}
						}
					}
				}
			}
		}
	}
	return p
}

var (
	pyDef    = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_]\w*)`)
	pyClass  = regexp.MustCompile(`^class\s+([A-Za-z_]\w*)`)
	pyImport = regexp.MustCompile(`^\s*import\s+([A-Za-z_][\w.]*(?:\s*,\s*[A-Za-z_][\w.]*)*)`)
	pyFrom   = regexp.MustCompile(`^\s*from\s+(\.*[A-Za-z_]?[\w.]*)\s+import\b`)
)

func lines(content []byte, fn func(n int, line string)) {
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		fn(n, sc.Text())
	}
}

func parsePython(content []byte) Parsed {
	var p Parsed
	lines(content, func(n int, line string) {
		if len(p.Symbols) < maxSymbolsPerFile {
			if m := pyDef.FindStringSubmatch(line); m != nil {
				kind := "function"
				if m[1] != "" {
					kind = "method"
				}
				p.Symbols = append(p.Symbols, Symbol{Name: m[2], Kind: kind, Line: n})
			} else if m := pyClass.FindStringSubmatch(line); m != nil {
				p.Symbols = append(p.Symbols, Symbol{Name: m[1], Kind: "class", Line: n})
			}
		}
		if len(p.Imports) >= maxImportsPerFile {
			return
		}
		if m := pyFrom.FindStringSubmatch(line); m != nil {
			p.Imports = append(p.Imports, m[1])
		} else if m := pyImport.FindStringSubmatch(line); m != nil {
			for _, name := range strings.Split(m[1], ",") {
				p.Imports = append(p.Imports, strings.TrimSpace(name))
			}
		}
	})
	return p
}

var (
	jsFunc      = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`)
	jsClass     = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`)
	jsArrow     = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*(?::[^=]+)?=>`)
	jsInterface = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][\w$]*)`)
	jsType      = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][\w$]*)\s*(?:<[^=]*>)?\s*=`)
	jsFrom      = regexp.MustCompile(`(?:^|\s)(?:import|export)\b[^'"]*?\bfrom\s+['"]([^'"]+)['"]`)
	jsBare      = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
	jsRequire   = regexp.MustCompile(`\brequire\(\s*['"]([^'"]+)['"]\s*\)`)
	jsDynamic   = regexp.MustCompile(`\bimport\(\s*['"]([^'"]+)['"]\s*\)`)
)

func parseJS(content []byte) Parsed {
	var p Parsed
	lines(content, func(n int, line string) {
		if len(line) > 2000 { // minified or data: skip rather than scan
			return
		}
		if len(p.Symbols) < maxSymbolsPerFile {
			for _, c := range []struct {
				re   *regexp.Regexp
				kind string
			}{{jsFunc, "function"}, {jsClass, "class"}, {jsInterface, "interface"}, {jsType, "type"}, {jsArrow, "function"}} {
				if m := c.re.FindStringSubmatch(line); m != nil {
					p.Symbols = append(p.Symbols, Symbol{Name: m[1], Kind: c.kind, Line: n})
					break
				}
			}
		}
		if len(p.Imports) >= maxImportsPerFile {
			return
		}
		for _, re := range []*regexp.Regexp{jsFrom, jsBare, jsRequire, jsDynamic} {
			if m := re.FindStringSubmatch(line); m != nil {
				p.Imports = append(p.Imports, m[1])
				break
			}
		}
	})
	return p
}
