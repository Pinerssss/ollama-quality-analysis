package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) != 2 {
		panic("usage: split_routes DISPOSABLE_REPO")
	}
	root := os.Args[1]
	name := filepath.Join(root, "server/routes.go")
	src, err := os.ReadFile(name)
	must(err)
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, name, src, parser.ParseComments)
	must(err)
	groups := map[string][]ast.Decl{"routes.go": {}, "routes_generate.go": {}, "routes_chat.go": {}, "routes_blob.go": {}, "routes_host.go": {}}
	target := map[string]string{}
	for _, s := range strings.Fields("GenerateHandler") {
		target[s] = "routes_generate.go"
	}
	for _, s := range strings.Fields("ChatHandler writeChatResponse prepareNativeChatRequest handleNativeChat truncateNativeChatMessages countChatImages") {
		target[s] = "routes_chat.go"
	}
	for _, s := range strings.Fields("HeadBlobHandler CreateBlobHandler") {
		target[s] = "routes_blob.go"
	}
	for _, s := range strings.Fields("isLocalIP allowedHost allowedHostsMiddleware") {
		target[s] = "routes_host.go"
	}
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		dst := "routes.go"
		if fn, ok := d.(*ast.FuncDecl); ok {
			if s, ok := target[fn.Name.Name]; ok {
				dst = s
			}
		}
		groups[dst] = append(groups[dst], d)
	}

	for dst, decls := range groups {
		used := map[string]bool{}
		for _, d := range decls {
			ast.Inspect(d, func(n ast.Node) bool {
				if s, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := s.X.(*ast.Ident); ok {
						used[id.Name] = true
					}
				}
				return true
			})
		}
		var out bytes.Buffer
		fmt.Fprintln(&out, "package server\n\nimport (")
		for _, im := range f.Imports {
			p, err := strconv.Unquote(im.Path.Value)
			must(err)
			alias := path.Base(p)
			if im.Name != nil {
				alias = im.Name.Name
			}
			if used[alias] || alias == "_" || alias == "." {
				fmt.Fprint(&out, "\t")
				if im.Name != nil {
					fmt.Fprint(&out, alias, " ")
				}
				fmt.Fprintln(&out, im.Path.Value)
			}
		}
		fmt.Fprintln(&out, ")")
		for _, d := range decls {
			start := d.Pos()
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Doc != nil {
					start = d.Doc.Pos()
				}
			case *ast.GenDecl:
				if d.Doc != nil {
					start = d.Doc.Pos()
				}
			}
			fmt.Fprint(&out, "\n", string(src[fs.Position(start).Offset:fs.Position(d.End()).Offset]), "\n")
		}
		result, err := format.Source(out.Bytes())
		must(err)
		must(os.WriteFile(filepath.Join(root, "server", dst), result, 0644))
	}
}
