package present

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestExplanationCoversProductionCodes(t *testing.T) {
	codes := map[string]bool{"quota_exceeded": true, "permission_denied": true, "unknown_remote_outcome": true}
	files := []string{"../state/model.go", "../drive/provider.go"}
	engine, err := filepath.Glob("../engine/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range engine {
		if !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
	}
	for _, p := range files {
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		add := func(e ast.Expr) {
			if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				if v != "" {
					codes[v] = true
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ValueSpec:
				for i, name := range n.Names {
					if (strings.HasPrefix(name.Name, "Skip") || (p == "../drive/provider.go" && strings.HasPrefix(name.Name, "Kind"))) && i < len(n.Values) {
						add(n.Values[i])
					}
				}
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "block":
						if len(n.Args) > 0 {
							add(n.Args[0])
						}
					case "SetItemState", "ReleaseItem", "ReleaseItemNoPenalty", "ReleaseItemThrottled", "FailListing":
						for _, arg := range n.Args {
							add(arg)
						}
					}
				}
			case *ast.FuncDecl:
				if n.Name.Name == "codeFor" {
					ast.Inspect(n.Body, func(a ast.Node) bool {
						if ret, ok := a.(*ast.ReturnStmt); ok {
							for _, v := range ret.Results {
								add(v)
							}
						}
						return true
					})
				}
			}
			return true
		})
	}
	for code := range codes {
		for _, lang := range []string{"vi", "en"} {
			e := Explain(code, lang)
			if !e.Known || e.Title == "" || e.Message == "" || e.Action == "" {
				t.Errorf("missing %s/%s: %+v", lang, code, e)
			}
		}
	}
}
func TestUnknownExplanationRetainsCode(t *testing.T) {
	for _, lang := range []string{"vi", "en"} {
		e := Explain("future_remote_code", lang)
		if e.Known || !strings.Contains(e.Message, "future_remote_code") {
			t.Fatal(e)
		}
	}
}
