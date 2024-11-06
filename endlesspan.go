package endlesspan

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"github.com/gostaticanalysis/analysisutil"
	"github.com/gostaticanalysis/comment"
	"github.com/gostaticanalysis/comment/passes/commentmap"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const (
	doc        = "endlesspan detects missing span.End() calls in OpenTelemetry tracing spans."
	importPath = "go.opentelemetry.io/otel/trace"
)

// Analyzer detects missing span.End() calls in OpenTelemetry tracing spans.
var Analyzer = &analysis.Analyzer{
	Name: "endlesspan",
	Doc:  doc,
	Run:  run,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		commentmap.Analyzer,
	},
}

func run(pass *analysis.Pass) (any, error) {
	spanType := analysisutil.TypeOf(pass, importPath, "Span")
	//config := &packages.Config{
	//	Mode: packages.NeedTypes | packages.NeedDeps,
	//}
	//pkgs, err := packages.Load(config, importPath)
	//if err != nil {
	//	return nil, err
	//}
	//spanType := types.NewPointer(pkgs[0].Types.Scope().Lookup("Span").Type())

	endMethod := analysisutil.MethodOf(spanType, "End")

	funcs := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA).SrcFuncs
	cmaps := pass.ResultOf[commentmap.Analyzer].(comment.Maps)

	fileMap := map[*ast.File]bool{}

FUNC_LOOP:
	for _, f := range funcs {
		// skip if f returns Span interface
		results := f.Signature.Results()
		if results != nil {
			for i := 0; i < results.Len(); i++ {
				if types.Identical(results.At(i).Type(), spanType) {
					continue FUNC_LOOP
				}
			}
		}

		if !isItsFileImportingTrace(pass, f, fileMap) { // skip this
			continue
		}
		instructions := analysisutil.NotCalledIn(f, spanType, endMethod)
		for _, inst := range instructions {
			pos := inst.Pos()
			if pos == token.NoPos {
				continue
			}
			line := pass.Fset.File(pos).Line(pos)
			if !cmaps.IgnoreLine(pass.Fset, line, "endlesspan") {
				pass.Reportf(pos, "span must be ended")
			}
		}
	}

	return nil, nil
}

func isItsFileImportingTrace(pass *analysis.Pass, f *ssa.Function, fileMap map[*ast.File]bool) (ret bool) {
	obj := f.Object()
	if obj == nil {
		return true
	}
	file := analysisutil.File(pass, obj.Pos())
	if file == nil {
		return true
	}
	if skip, has := fileMap[file]; has {
		return skip
	}
	defer func() {
		fileMap[file] = ret
	}()

	for _, impt := range file.Imports {
		path, err := strconv.Unquote(impt.Path.Value)
		if err != nil {
			continue
		}
		path = analysisutil.RemoveVendor(path)
		if path == importPath {
			return true
		}
	}

	return false
}
