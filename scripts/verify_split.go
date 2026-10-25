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
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func declarations(root string, files []string) map[string]string {
	out := map[string]string{}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, "server", name))
		check(err)
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, name, data, 0)
		check(err)
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				continue
			}
			start, end := d.Pos(), d.End()
			key := ""
			switch n := d.(type) {
			case *ast.FuncDecl:
				key = "func:" + n.Name.Name
				if n.Name.Name == "optionsForPrompt" {
					start = n.Body.Pos()
				}
			case *ast.GenDecl:
				var names []string
				for _, sp := range n.Specs {
					switch s := sp.(type) {
					case *ast.TypeSpec:
						names = append(names, s.Name.Name)
					case *ast.ValueSpec:
						for _, id := range s.Names {
							names = append(names, id.Name)
						}
					}
				}
				key = n.Tok.String() + ":" + strings.Join(names, ",")
			}
			if key == "type:contextLengthProvider" {
				continue
			}
			src := data[fs.Position(start).Offset:fs.Position(end).Offset]
			tf := token.NewFileSet().AddFile("x", -1, len(src))
			var sc scanner.Scanner
			sc.Init(tf, src, nil, 0)
			var tokens []string
			for {
				_, t, l := sc.Scan()
				if t == token.EOF {
					break
				}
				tokens = append(tokens, fmt.Sprintf("%d:%s", t, l))
			}
			if _, exists := out[key]; exists {
				panic("duplicate declaration " + key)
			}
			out[key] = strings.Join(tokens, "|")
		}
	}
	return out
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: verify_split BASELINE PROTOTYPE")
	}
	a := declarations(os.Args[1], []string{"routes.go"})
	b := declarations(os.Args[2], []string{"routes.go", "routes_generate.go", "routes_chat.go", "routes_blob.go", "routes_host.go"})
	if len(a) != len(b) {
		panic("declaration counts differ")
	}
	for k, v := range a {
		if b[k] != v {
			panic("changed or missing: " + k)
		}
	}
	result := map[string]any{"unchanged_declarations": len(a), "status": "PASS", "exception": "optionsForPrompt signature intentionally narrowed; its body is token-identical; one interface added", "limitation": "does not build or run the server package"}
	check(json.NewEncoder(os.Stdout).Encode(result))
}
