package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type finding struct {
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type result struct {
	Scanned int       `json:"scanned"`
	Found   []finding `json:"found"`
}

func selected(path string) bool {
	if strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
		return false
	}
	return strings.HasPrefix(path, "internal/cpu/") ||
		strings.HasPrefix(path, "internal/device/") ||
		strings.HasPrefix(path, "internal/platform/clock/")
}

func main() {
	listed, err := exec.Command("git", "ls-files", "-z", "--", "internal").Output()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
	out := result{Found: []finding{}}
	for _, path := range strings.Split(string(listed), "\x00") {
		if !selected(path) {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(2)
		}
		out.Scanned++
		timeNames := map[string]bool{}
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil || name != "time" {
				continue
			}
			alias := "time"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "." {
				out.Found = append(out.Found, finding{"wall-clock-source", path, set.Position(spec.Pos()).Line})
			} else {
				timeNames[alias] = true
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.GoStmt:
				out.Found = append(out.Found, finding{"goroutine-in-core", path, set.Position(current.Pos()).Line})
			case *ast.CallExpr:
				selector, ok := current.Fun.(*ast.SelectorExpr)
				if !ok {
					break
				}
				ident, ok := selector.X.(*ast.Ident)
				if !ok || !timeNames[ident.Name] {
					break
				}
				switch selector.Sel.Name {
				case "Now", "Since", "Until", "After", "AfterFunc", "NewTicker", "NewTimer", "Sleep", "Tick":
					out.Found = append(out.Found, finding{"wall-clock-source", path, set.Position(current.Pos()).Line})
				}
			}
			return true
		})
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
}
