// Copyright 2020 The protobuf-tools Authors.
// SPDX-License-Identifier: BSD-3-Clause

package facts

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type IsDeprecated struct{ Msg string }

func (*IsDeprecated) AFact()           {}
func (d *IsDeprecated) String() string { return "Deprecated: " + d.Msg }

type DeprecatedResult struct {
	Objects  map[types.Object]*IsDeprecated
	Packages map[*types.Package]*IsDeprecated
}

var Deprecated = &analysis.Analyzer{
	Name:       "fact.deprecated",
	Doc:        "Mark deprecated objects",
	Run:        deprecated,
	FactTypes:  []analysis.Fact{(*IsDeprecated)(nil)},
	ResultType: reflect.TypeOf(DeprecatedResult{}),
}

func deprecated(pass *analysis.Pass) (interface{}, error) {
	var names []*ast.Ident

	extractDeprecatedMessage := func(docs []*ast.CommentGroup) string {
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			parts := strings.Split(doc.Text(), "\n\n")
			for _, part := range parts {
				if !strings.HasPrefix(part, "Deprecated: ") {
					continue
				}
				alt := part[len("Deprecated: "):]
				alt = strings.Replace(alt, "\n", " ", -1)
				return alt
			}
		}
		return ""
	}
	doDocs := func(names []*ast.Ident, docs []*ast.CommentGroup) {
		alt := extractDeprecatedMessage(docs)
		if alt == "" {
			return
		}

		for _, name := range names {
			obj := pass.TypesInfo.ObjectOf(name)
			pass.ExportObjectFact(obj, &IsDeprecated{alt})
		}
	}

	var docs []*ast.CommentGroup
	for _, f := range pass.Files {
		docs = append(docs, f.Doc)
	}
	if alt := extractDeprecatedMessage(docs); alt != "" {
		pass.ExportPackageFact(&IsDeprecated{alt})
	}

	docs = docs[:0]
	for _, f := range pass.Files {
		fn := func(node ast.Node) bool {
			if node == nil {
				return true
			}
			var ret bool
			switch node := node.(type) {
			case *ast.GenDecl:
				switch node.Tok {
				case token.TYPE, token.CONST, token.VAR:
					docs = append(docs, node.Doc)
					return true
				default:
					return false
				}
			case *ast.FuncDecl:
				docs = append(docs, node.Doc)
				names = []*ast.Ident{node.Name}
				ret = false
			case *ast.TypeSpec:
				docs = append(docs, node.Doc)
				names = []*ast.Ident{node.Name}
				ret = true
			case *ast.ValueSpec:
				docs = append(docs, node.Doc)
				names = node.Names
				ret = false
			case *ast.File:
				return true
			case *ast.StructType:
				for _, field := range node.Fields.List {
					doDocs(field.Names, []*ast.CommentGroup{field.Doc})
				}
				return false
			case *ast.InterfaceType:
				for _, field := range node.Methods.List {
					doDocs(field.Names, []*ast.CommentGroup{field.Doc})
				}
				return false
			default:
				return false
			}
			if len(names) == 0 || len(docs) == 0 {
				return ret
			}
			doDocs(names, docs)

			docs = docs[:0]
			names = nil
			return ret
		}
		ast.Inspect(f, fn)
	}

	out := DeprecatedResult{
		Objects:  map[types.Object]*IsDeprecated{},
		Packages: map[*types.Package]*IsDeprecated{},
	}

	for _, fact := range pass.AllObjectFacts() {
		out.Objects[fact.Object] = fact.Fact.(*IsDeprecated)
	}
	for _, fact := range pass.AllPackageFacts() {
		out.Packages[fact.Package] = fact.Fact.(*IsDeprecated)
	}

	return out, nil
}
