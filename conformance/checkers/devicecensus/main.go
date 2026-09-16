package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type device struct {
	Package  string   `json:"package"`
	Type     string   `json:"type"`
	Methods  []string `json:"methods"`
	Contract string   `json:"contract"`
}

func receiver(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func callsContract(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "CheckSnapshot" {
			found = true
		}
		return true
	})
	return found
}

func main() {
	listed := exec.Command("git", "ls-files", "-z", "--", "internal")
	output, err := listed.Output()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
	methods := map[string]map[string]bool{}
	tests := map[string]string{}
	for _, raw := range strings.Split(string(output), "\x00") {
		if raw == "" || !strings.HasSuffix(raw, ".go") {
			continue
		}
		if _, err := os.Stat(raw); err != nil {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), raw, nil, 0)
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(2)
		}
		pkg := filepath.Dir(raw)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if strings.HasSuffix(raw, "_test.go") {
				if strings.HasPrefix(fn.Name.Name, "Test") && strings.HasSuffix(fn.Name.Name, "HoldsTheDeviceContract") && fn.Body != nil && callsContract(fn.Body) {
					key := pkg + "/" + strings.TrimSuffix(strings.TrimPrefix(fn.Name.Name, "Test"), "HoldsTheDeviceContract")
					tests[strings.ToLower(key)] = fn.Name.Name
				}
				continue
			}
			if fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			name := receiver(fn.Recv.List[0].Type)
			if name == "" {
				continue
			}
			key := pkg + "/" + name
			if methods[key] == nil {
				methods[key] = map[string]bool{}
			}
			methods[key][fn.Name.Name] = true
		}
	}
	result := []device{}
	for key, set := range methods {
		if !set["Name"] || !set["Read"] || !set["Write"] || !set["Reset"] {
			continue
		}
		pkg, name := filepath.Dir(key), filepath.Base(key)
		result = append(result, device{Package: pkg, Type: name,
			Methods:  []string{boolName(set["Snapshot"], "Snapshot"), boolName(set["Restore"], "Restore")},
			Contract: tests[strings.ToLower(key)]})
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
}

func boolName(ok bool, name string) string {
	if ok {
		return name
	}
	return ""
}
