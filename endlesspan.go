package endlesspan

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

const doc = "endlesspan detects missing span.End() calls in OpenTelemetry tracing spans."

// Analyzer detects missing span.End() calls in OpenTelemetry tracing spans.
var Analyzer = &analysis.Analyzer{
	Name: "endlesspan",
	Doc:  doc,
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspector := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Load OpenTelemetry trace package to get Span interface info
	config := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedDeps,
	}
	pkgs, err := packages.Load(config, "g")
	if err != nil {
		return nil, err
	}

	spanType := types.NewPointer(pkgs[0].Types.Scope().Lookup("Span").Type())

	// Track function-level span variables and their End() calls
	type funcInfo struct {
		spans    map[types.Object]bool // track if span variable has End() called
		hasDefer map[types.Object]bool // track if End() is called with defer
		returned map[types.Object]bool // track if span is returned
	}

	funcStack := []*funcInfo{}

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.FuncLit)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.ReturnStmt)(nil),
		(*ast.DeferStmt)(nil),
	}

	inspector.Preorder(nodeFilter, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			// Push new function context
			funcStack = append(funcStack, &funcInfo{
				spans:    make(map[types.Object]bool),
				hasDefer: make(map[types.Object]bool),
				returned: make(map[types.Object]bool),
			})

		case *ast.AssignStmt:
			if len(funcStack) == 0 {
				return
			}
			currentFunc := funcStack[len(funcStack)-1]

			// Check for tracer.Start assignments
			if len(n.Lhs) != 2 || len(n.Rhs) != 1 {
				return
			}

			// Check if the right side is a tracer.Start call
			call, ok := n.Rhs[0].(*ast.CallExpr)
			if !ok {
				return
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Start" {
				return
			}

			// Get the span variable
			spanIdent, ok := n.Lhs[1].(*ast.Ident)
			if !ok {
				return
			}

			spanObj := pass.TypesInfo.ObjectOf(spanIdent)
			if spanObj == nil {
				return
			}

			// Verify the type is trace.Span
			if types.AssignableTo(spanObj.Type(), spanType) {
				currentFunc.spans[spanObj] = false
			}

		case *ast.DeferStmt:
			if len(funcStack) == 0 {
				return
			}
			currentFunc := funcStack[len(funcStack)-1]

			call, ok := n.Call.Fun.(*ast.SelectorExpr)
			if !ok || call.Sel.Name != "End" {
				return
			}

			// Check if the receiver is a span
			if id, ok := call.X.(*ast.Ident); ok {
				if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
					if _, exists := currentFunc.spans[obj]; exists {
						currentFunc.spans[obj] = true
						currentFunc.hasDefer[obj] = true
					}
				}
			}

		case *ast.ReturnStmt:
			if len(funcStack) == 0 {
				return
			}
			currentFunc := funcStack[len(funcStack)-1]

			for _, result := range n.Results {
				if id, ok := result.(*ast.Ident); ok {
					if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
						if _, exists := currentFunc.spans[obj]; exists {
							currentFunc.returned[obj] = true
						}
					}
				}
			}

			// Check for non-deferred End() calls
			for _, expr := range n.Results {
				if call, ok := expr.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if sel.Sel.Name == "End" {
							if id, ok := sel.X.(*ast.Ident); ok {
								if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
									if _, exists := currentFunc.spans[obj]; exists {
										currentFunc.spans[obj] = true
										if !currentFunc.hasDefer[obj] {
											pass.Reportf(call.Pos(), "you may want to defer the span.End() call")
										}
									}
								}
							}
						}
					}
				}
			}

		}

		// When leaving a function, check for spans that were never ended
		if _, ok := n.(*ast.FuncDecl); ok {
			if len(funcStack) > 0 {
				currentFunc := funcStack[len(funcStack)-1]
				for span, ended := range currentFunc.spans {
					if !ended && !currentFunc.returned[span] {
						pass.Reportf(span.Pos(), "missing span.End() call in the scope")
					}
				}
				// Pop function context
				funcStack = funcStack[:len(funcStack)-1]
			}
		}
	})

	return nil, nil
}
