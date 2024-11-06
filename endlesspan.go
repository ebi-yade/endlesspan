package endlesspan

import (
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
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

// spanUsage は1つのスパン変数の使用状況を追跡
type spanUsage struct {
	name     string
	pos      token.Pos
	hasEnd   bool
	deferred bool
	obj      types.Object // スパン変数の types.Object
}

// scopeInfo は関数スコープの情報を保持
type scopeInfo struct {
	funcType *ast.FuncType
	spans    map[types.Object]*spanUsage
	parent   *scopeInfo
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// スパンを返す関数の一覧
	spanReturningFuncs := make(map[*ast.FuncType]bool)

	// 現在のスコープスタック
	var currentScope *scopeInfo

	inspect.Preorder([]ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.FuncLit)(nil),
	}, func(n ast.Node) {
		var funcType *ast.FuncType
		switch fn := n.(type) {
		case *ast.FuncDecl:
			funcType = fn.Type
			slog.Debug("analyzing function", "name", fn.Name.Name)
		case *ast.FuncLit:
			funcType = fn.Type
			slog.Debug("analyzing anonymous function")
		}

		// スパンを返す関数かどうかをチェック
		if funcType.Results != nil {
			for _, ret := range funcType.Results.List {
				if t := pass.TypesInfo.TypeOf(ret.Type); t != nil {
					if hasEndMethod(pass.TypesInfo, t) {
						spanReturningFuncs[funcType] = true
						slog.Debug("found span-returning function")
						return
					}
				}
			}
		}

		// 新しいスコープを作成
		scope := &scopeInfo{
			funcType: funcType,
			spans:    make(map[types.Object]*spanUsage),
			parent:   currentScope,
		}
		currentScope = scope

		// 関数本体を解析
		ast.Inspect(n, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Start" {
						handleStart(pass, node, scope)
					} else if sel.Sel.Name == "End" {
						handleEnd(pass, node, scope)
					}
				}
			case *ast.DeferStmt:
				handleDefer(pass, node, scope)
			}
			return true
		})

		// スコープを抜ける前にチェック
		if !spanReturningFuncs[scope.funcType] {
			for _, usage := range scope.spans {
				slog.Debug("checking span status",
					"name", usage.name,
					"hasEnd", usage.hasEnd,
					"deferred", usage.deferred)
				if !usage.hasEnd {
					pass.Reportf(usage.pos, "span missing End call in the scope")
				}
			}
		}

		// スコープを戻す
		currentScope = scope.parent
	})

	return nil, nil
}

func handleStart(pass *analysis.Pass, node *ast.CallExpr, scope *scopeInfo) {
	if t := pass.TypesInfo.TypeOf(node); t != nil {
		if tuple, ok := t.(*types.Tuple); ok && tuple.Len() >= 2 {
			spanType := tuple.At(1).Type()
			slog.Debug("checking Start return type", "type", spanType.String())

			// 親の assign 文を探す
			if parent := findAssignParent(node); parent != nil {
				if len(parent.Lhs) >= 2 {
					if id, ok := parent.Lhs[1].(*ast.Ident); ok {
						if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
							slog.Debug("found span variable",
								"name", id.Name,
								"pos", pass.Fset.Position(id.Pos()))
							scope.spans[obj] = &spanUsage{
								name: id.Name,
								pos:  id.Pos(),
								obj:  obj,
							}
						}
					}
				}
			}
		}
	}
}

func handleEnd(pass *analysis.Pass, node *ast.CallExpr, scope *scopeInfo) {
	if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			if obj := pass.TypesInfo.Uses[id]; obj != nil {
				if usage, exists := scope.spans[obj]; exists {
					slog.Debug("found End call", "span", id.Name)
					usage.hasEnd = true
					pass.Reportf(node.Pos(), "you may want to defer the End call of the span")
				}
			}
		}
	}
}

func handleDefer(pass *analysis.Pass, node *ast.DeferStmt, scope *scopeInfo) {
	if call, ok := node.Call.Fun.(*ast.SelectorExpr); ok {
		if call.Sel.Name == "End" {
			if id, ok := call.X.(*ast.Ident); ok {
				if obj := pass.TypesInfo.Uses[id]; obj != nil {
					if usage, exists := scope.spans[obj]; exists {
						slog.Debug("found deferred End", "span", id.Name)
						usage.hasEnd = true
						usage.deferred = true
					}
				}
			}
		}
	}
}

// hasEndMethod は型が End メソッドを持つかどうかを判定
func hasEndMethod(info *types.Info, t types.Type) bool {
	if t == nil {
		return false
	}

	if named, ok := t.(*types.Named); ok {
		for i := 0; i < named.NumMethods(); i++ {
			if named.Method(i).Name() == "End" {
				return true
			}
		}
		t = named.Underlying()
	}

	if iface, ok := t.(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			if iface.Method(i).Name() == "End" {
				return true
			}
		}
	}

	return false
}

// findAssignParent は CallExpr の親の AssignStmt を探す
func findAssignParent(node ast.Node) *ast.AssignStmt {
	parent := node
	for {
		if parent == nil {
			return nil
		}
		if assign, ok := parent.(*ast.AssignStmt); ok {
			return assign
		}
		parent = parent.(ast.Node).Parent()
	}
}
