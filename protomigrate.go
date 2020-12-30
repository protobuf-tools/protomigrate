// Copyright 2020 The protobuf-tools Authors.
// SPDX-License-Identifier: BSD-3-Clause

// package protomigrate migrates Go protobuf v1 usage to protobuf v2.
package protomigrate

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"honnef.co/go/tools/analysis/report"
	"honnef.co/go/tools/knowledge"

	"github.com/protobuf-tools/protomigrate/facts"
)

var spewer = &spew.ConfigState{
	Indent:                  "  ",
	SortKeys:                true, // maps should be spewed in a deterministic order
	DisablePointerAddresses: true, // don't spew the addresses of pointers
	DisableCapacities:       true, // don't spew capacities of collections
	ContinueOnMethod:        true, // recursion should continue once a custom error or Stringer interface is invoked
	SpewKeys:                true, // if unable to sort map keys then spew keys to strings and sort those
	MaxDepth:                2,    // maximum number of levels to descend into nested data structures.
}

const doc = "protomigrate migrate Go protobuf v1 usage to protobuf v2"

// Analyzer describes protomigrate analysis function detector.
var Analyzer = &analysis.Analyzer{
	Name: "protomigrate",
	Doc:  doc,
	Run:  checkDeprecated,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		inspect.Analyzer,
		facts.Deprecated,
		facts.Generated,
	},
}

var protoV1Packages = map[string]bool{
	"github.com/golang/protobuf/descriptor":       true,
	"github.com/golang/protobuf/jsonpb":           true,
	"github.com/golang/protobuf/proto":            true,
	"github.com/golang/protobuf/ptypes":           true,
	"github.com/golang/protobuf/ptypes/any":       true,
	"github.com/golang/protobuf/ptypes/duration":  true,
	"github.com/golang/protobuf/ptypes/empty":     true,
	"github.com/golang/protobuf/ptypes/struct":    true,
	"github.com/golang/protobuf/ptypes/timestamp": true,
	"github.com/golang/protobuf/ptypes/wrappers":  true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	fmt.Printf("pass: %s\n", spewer.Sdump(pass))

	for _, file := range pass.Files {
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			fmt.Printf("path: %s\n", path)
			if protoV1Packages[path] {
				fmt.Printf("hit: %s\n", path)
			}
		}
	}

	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.BasicLit)(nil),
		(*ast.BlockStmt)(nil),
		(*ast.CallExpr)(nil),
		(*ast.Comment)(nil),
		(*ast.CommentGroup)(nil),
		(*ast.DeclStmt)(nil),
		(*ast.ExprStmt)(nil),
		(*ast.FieldList)(nil),
		(*ast.FuncDecl)(nil),
		(*ast.GenDecl)(nil),
		(*ast.GenDecl)(nil),
		(*ast.GenDecl)(nil),
		(*ast.Ident)(nil),
		(*ast.ImportSpec)(nil),
		(*ast.SelectorExpr)(nil),
		(*ast.ValueSpec)(nil),
	}
	ins.Preorder(nodeFilter, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.Ident:
			fmt.Printf("n: %T = %#v\n", n, n)
			if n.Name == "gopher" {
				pass.Reportf(n.Pos(), "identifier is gopher")
			}
		default:
			fmt.Printf("n: %T = %#v\n", n, n)
		}
	})

	s := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	for _, f := range s.SrcFuncs {
		fmt.Println(f)
		for _, b := range f.Blocks {
			fmt.Printf("\tBlock %d\n", b.Index)
			for _, instr := range b.Instrs {
				fmt.Printf("\t\t%[1]T\t%[1]T = %[1]v(%[1]p)\n", instr)
				for _, v := range instr.Operands(nil) {
					if v != nil {
						fmt.Printf("\t\t\t%[1]T\t%[1]v(%[1]p)\n", *v)
					}
				}
			}
		}
	}

	return nil, nil
}

func checkDeprecated(pass *analysis.Pass) (interface{}, error) {
	deprs := pass.ResultOf[facts.Deprecated].(facts.DeprecatedResult)

	// Selectors can appear outside of function literals, e.g. when
	// declaring package level variables.

	var tfn types.Object
	stack := 0
	fn := func(node ast.Node, push bool) bool {
		if !push {
			stack--
			return false
		}
		stack++
		if stack == 1 {
			tfn = nil
		}
		if fn, ok := node.(*ast.FuncDecl); ok {
			tfn = pass.TypesInfo.ObjectOf(fn.Name)
		}
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		obj := pass.TypesInfo.ObjectOf(sel.Sel)
		if obj.Pkg() == nil {
			return true
		}
		if pass.Pkg == obj.Pkg() || obj.Pkg().Path()+"_test" == pass.Pkg.Path() {
			// Don't flag stuff in our own package
			return true
		}
		if depr, ok := deprs.Objects[obj]; ok {
			std, ok := knowledge.StdlibDeprecations[SelectorName(pass, sel)]
			if ok {
				switch std.AlternativeAvailableSince {
				case knowledge.DeprecatedNeverUse:
					// This should never be used, regardless of the
					// targeted Go version. Examples include insecure
					// cryptography or inherently broken APIs.
					//
					// We always want to flag these.
				case knowledge.DeprecatedUseNoLonger:
					// This should no longer be used. Using it with
					// older Go versions might still make sense.
					if !IsGoVersion(pass, std.DeprecatedSince) {
						return true
					}
				default:
					if std.AlternativeAvailableSince < 0 {
						panic(fmt.Sprintf("unhandled case %d", std.AlternativeAvailableSince))
					}
					// Look for the first available alternative, not the first
					// version something was deprecated in. If a function was
					// deprecated in Go 1.6, an alternative has been available
					// already in 1.0, and we're targeting 1.2, it still
					// makes sense to use the alternative from 1.0, to be
					// future-proof.
					if !IsGoVersion(pass, std.AlternativeAvailableSince) {
						return true
					}
				}
			}
			if ok && !IsGoVersion(pass, std.AlternativeAvailableSince) {
				return true
			}

			if tfn != nil {
				if _, ok := deprs.Objects[tfn]; ok {
					// functions that are deprecated may use deprecated
					// symbols
					return true
				}
			}

			if ok {
				if std.AlternativeAvailableSince == knowledge.DeprecatedNeverUse {
					report.Report(pass, sel, fmt.Sprintf("%s has been deprecated since Go 1.%d because it shouldn't be used: %s", report.Render(pass, sel), std.DeprecatedSince, depr.Msg))
				} else if std.AlternativeAvailableSince == std.DeprecatedSince || std.AlternativeAvailableSince == knowledge.DeprecatedUseNoLonger {
					report.Report(pass, sel, fmt.Sprintf("%s has been deprecated since Go 1.%d: %s", report.Render(pass, sel), std.DeprecatedSince, depr.Msg))
				} else {
					report.Report(pass, sel, fmt.Sprintf("%s has been deprecated since Go 1.%d and an alternative has been available since Go 1.%d: %s", report.Render(pass, sel), std.DeprecatedSince, std.AlternativeAvailableSince, depr.Msg))
				}
			} else {
				report.Report(pass, sel, fmt.Sprintf("%s is deprecated: %s", report.Render(pass, sel), depr.Msg))
			}
			return true
		}
		return true
	}

	fn2 := func(node ast.Node) {
		spec := node.(*ast.ImportSpec)
		var imp *types.Package
		if spec.Name != nil {
			imp = pass.TypesInfo.ObjectOf(spec.Name).(*types.PkgName).Imported()
		} else {
			imp = pass.TypesInfo.Implicits[spec].(*types.PkgName).Imported()
		}

		p := spec.Path.Value
		path := p[1 : len(p)-1]
		if depr, ok := deprs.Packages[imp]; ok {
			if path == "github.com/golang/protobuf/proto" {
				gen, ok := Generator(pass, spec.Path.Pos())
				if ok && gen == facts.ProtocGenGo {
					return
				}
			}
			report.Report(pass, spec, fmt.Sprintf("package %s is deprecated: %s", path, depr.Msg))
		}
	}
	pass.ResultOf[inspect.Analyzer].(*inspector.Inspector).Nodes(nil, fn)
	Preorder(pass, fn2, (*ast.ImportSpec)(nil))
	return nil, nil
}

func Generator(pass *analysis.Pass, pos token.Pos) (facts.Generator, bool) {
	file := pass.Fset.PositionFor(pos, false).Filename
	m := pass.ResultOf[facts.Generated].(map[string]facts.Generator)
	g, ok := m[file]
	return g, ok
}

func Preorder(pass *analysis.Pass, fn func(ast.Node), typs ...ast.Node) {
	pass.ResultOf[inspect.Analyzer].(*inspector.Inspector).Preorder(typs, fn)
}

func IsGoVersion(pass *analysis.Pass, minor int) bool {
	f, ok := pass.Analyzer.Flags.Lookup("go").Value.(flag.Getter)
	if !ok {
		panic("requested Go version, but analyzer has no version flag")
	}
	version := f.Get().(int)
	return version >= minor
}

func SelectorName(pass *analysis.Pass, expr *ast.SelectorExpr) string {
	info := pass.TypesInfo
	sel := info.Selections[expr]
	if sel == nil {
		if x, ok := expr.X.(*ast.Ident); ok {
			pkg, ok := info.ObjectOf(x).(*types.PkgName)
			if !ok {
				// This shouldn't happen
				return fmt.Sprintf("%s.%s", x.Name, expr.Sel.Name)
			}
			return fmt.Sprintf("%s.%s", pkg.Imported().Path(), expr.Sel.Name)
		}
		panic(fmt.Sprintf("unsupported selector: %v", expr))
	}
	return fmt.Sprintf("(%s).%s", sel.Recv(), sel.Obj().Name())
}
