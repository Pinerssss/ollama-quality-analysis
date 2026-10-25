package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Function struct {
	Name       string `json:"name"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Complexity int    `json:"complexity"`
}
type Interface struct {
	Name     string   `json:"name"`
	Start    int      `json:"start"`
	Methods  []string `json:"methods"`
	Embedded []string `json:"embedded"`
}
type File struct {
	Path            string      `json:"path"`
	PhysicalLines   int         `json:"physical_lines"`
	CodeLines       int         `json:"code_lines"`
	InternalImports []string    `json:"internal_imports"`
	Functions       []Function  `json:"functions"`
	Interfaces      []Interface `json:"interfaces"`
}

func fail(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) < 3 {
		panic("usage: metrics ROOT file.go ...")
	}
	result := struct {
		Version string `json:"version"`
		Files   []File `json:"files"`
	}{Version: "1.0.0"}
	for _, rel := range os.Args[2:] {
		data, err := os.ReadFile(filepath.Join(os.Args[1], rel))
		fail(err)
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, rel, data, parser.ParseComments)
		fail(err)
		out := File{Path: rel, PhysicalLines: len(strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")), InternalImports: []string{}, Functions: []Function{}, Interfaces: []Interface{}}
		// Count physical lines containing any non-comment, non-inserted token.
		// Include every physical line occupied by a multiline string token.
		tf := token.NewFileSet().AddFile(rel, -1, len(data))
		var s scanner.Scanner
		s.Init(tf, data, func(p token.Position, msg string) { panic(fmt.Sprint(p, msg)) }, scanner.ScanComments)
		lines := map[int]bool{}
		for {
			p, t, lit := s.Scan()
			if t == token.EOF {
				break
			}
			if t == token.COMMENT || (t == token.SEMICOLON && lit == "\n") {
				continue
			}
			n := tf.Position(p).Line
			for i := 0; i <= strings.Count(lit, "\n"); i++ {
				lines[n+i] = true
			}
		}
		out.CodeLines = len(lines)
		for _, i := range f.Imports {
			p, err := strconv.Unquote(i.Path.Value)
			fail(err)
			if strings.HasPrefix(p, "github.com/ollama/ollama/") {
				out.InternalImports = append(out.InternalImports, p)
			}
		}
		sort.Strings(out.InternalImports)
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil {
				name := fn.Name.Name
				if fn.Recv != nil {
					name = fmt.Sprint(fs.Position(fn.Pos()).Line, ":", name)
				}
				cc := 1
				// Same decision rules as gocyclo: if, for/range, each non-default
				// case/select clause and each && or ||. Nested literals included.
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
						cc++
					case *ast.CaseClause:
						if x.List != nil {
							cc++
						}
					case *ast.CommClause:
						if x.Comm != nil {
							cc++
						}
					case *ast.BinaryExpr:
						if x.Op == token.LAND || x.Op == token.LOR {
							cc++
						}
					}
					return true
				})
				out.Functions = append(out.Functions, Function{name, fs.Position(fn.Pos()).Line, fs.Position(fn.End()).Line, cc})
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			v := Interface{Name: ts.Name.Name, Start: fs.Position(ts.Pos()).Line, Methods: []string{}, Embedded: []string{}}
			for _, m := range it.Methods.List {
				if len(m.Names) == 0 {
					v.Embedded = append(v.Embedded, string(data[fs.Position(m.Pos()).Offset:fs.Position(m.End()).Offset]))
				}
				for _, name := range m.Names {
					v.Methods = append(v.Methods, name.Name)
				}
			}
			out.Interfaces = append(out.Interfaces, v)
			return true
		})
		result.Files = append(result.Files, out)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	fail(enc.Encode(result))
}
