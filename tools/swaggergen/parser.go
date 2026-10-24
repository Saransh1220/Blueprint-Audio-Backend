package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

// ParsedRoute contains route registration info found in routes.go
type ParsedRoute struct {
	Method      string
	Path        string
	RawPattern  string
	HandlerFunc string
	Middleware  string // e.g. "RequireAuth", "FlexibleAuth", "RequireUserRole", "RequirePermission"
}

var httpMethodRegex = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD)\s+(/.+)$`)

// ParseRoutesFile parses routes.go AST to extract method, path, and handler identifiers.
func ParseRoutesFile(filename string) ([]ParsedRoute, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var routes []ParsedRoute

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		funcName := sel.Sel.Name
		if funcName != "HandleFunc" && funcName != "Handle" {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		// First argument is usually the pattern string
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		pattern := strings.Trim(lit.Value, `"`)
		method := ""
		path := ""

		if matches := httpMethodRegex.FindStringSubmatch(pattern); len(matches) == 3 {
			method = matches[1]
			path = matches[2]
		} else if strings.HasPrefix(pattern, "/") {
			method = "GET" // default if not specified
			path = pattern
		} else {
			return true
		}

		// Second argument is the handler expression
		handlerExpr := call.Args[1]
		handlerName, mw := extractHandlerAndMiddleware(handlerExpr)

		routes = append(routes, ParsedRoute{
			Method:      method,
			Path:        path,
			RawPattern:  pattern,
			HandlerFunc: handlerName,
			Middleware:  mw,
		})

		return true
	})

	return routes, nil
}

func extractHandlerAndMiddleware(expr ast.Expr) (string, string) {
	mw := "None"
	handlerStr := ""

	// Walk nested calls
	var inspectCall func(e ast.Expr)
	inspectCall = func(e ast.Expr) {
		switch node := e.(type) {
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				name := sel.Sel.Name
				switch name {
				case "RequireAuth":
					mw = "RequireAuth"
				case "FlexibleAuth":
					mw = "FlexibleAuth"
				case "RequireUserRole":
					mw = "RequireUserRole"
				case "RequirePermission":
					mw = "RequirePermission"
				case "RateLimitMiddleware":
					mw = "RateLimited"
				}
			}
			for _, arg := range node.Args {
				inspectCall(arg)
			}
		case *ast.SelectorExpr:
			handlerStr = node.Sel.Name
		}
	}

	inspectCall(expr)
	return handlerStr, mw
}
