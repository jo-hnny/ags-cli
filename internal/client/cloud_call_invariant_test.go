package client

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// These wrapper files must pass SDK method values directly to CallCloud. This
// scans calls and method references, including anonymous functions, rather than
// maintaining a list of Actions or relying on exported function names.
func cloudCallViolations(source string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "wrappers.go", source, 0)
	if err != nil {
		return nil, err
	}
	clientAlias := ""
	sdkAlias := ""
	for _, imp := range file.Imports {
		if imp.Path.Value == `"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"` {
			sdkAlias = "v20250920"
			if imp.Name != nil {
				sdkAlias = imp.Name.Name
			}
		}
		if imp.Path.Value == `"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"` {
			clientAlias = "client"
			if imp.Name != nil {
				clientAlias = imp.Name.Name
			}
		}
	}
	// Also reject context-free methods on declared SDK parameters.
	sdkNames := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok {
			return true
		}
		pointer, ok := field.Type.(*ast.StarExpr)
		if !ok {
			return true
		}
		typ, ok := pointer.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := typ.X.(*ast.Ident)
		if ok && pkg.Name == sdkAlias && typ.Sel.Name == "Client" {
			for _, name := range field.Names {
				sdkNames[name.Name] = true
			}
		}
		return true
	})
	var violations []string
	allowed := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := fun.X.(*ast.Ident)
		if !ok || receiver.Name != clientAlias || fun.Sel.Name != "CallCloud" {
			return true
		}
		if len(call.Args) != 4 {
			violations = append(violations, "CallCloud must have four arguments")
			return true
		}
		method, ok := call.Args[3].(*ast.SelectorExpr)
		if !ok || !strings.HasSuffix(method.Sel.Name, "WithContext") {
			violations = append(violations, "CallCloud requires a direct SDK method value")
			return true
		}
		action, ok := call.Args[1].(*ast.BasicLit)
		if !ok || action.Kind != token.STRING {
			violations = append(violations, "CallCloud requires a literal Action")
			return true
		}
		name, err := strconv.Unquote(action.Value)
		if err != nil || name != strings.TrimSuffix(method.Sel.Name, "WithContext") {
			violations = append(violations, "Action does not match SDK method")
			return true
		}
		allowed[method] = true
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		method, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, _ := method.X.(*ast.Ident)
		sdkMethod := receiver != nil && sdkNames[receiver.Name]
		if (sdkMethod || strings.HasSuffix(method.Sel.Name, "WithContext")) && !allowed[method] {
			violations = append(violations, fmt.Sprintf("%s: %s bypasses CallCloud", fset.Position(method.Pos()), method.Sel.Name))
		}
		return true
	})
	return violations, nil
}

func TestCloudWrappersUseCallCloud(t *testing.T) {
	for _, path := range []string{"../cli/cloud_calls.go", "../controlplane/sdk.go"} {
		t.Run(path, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			violations, err := cloudCallViolations(string(source))
			if err != nil || len(violations) > 0 {
				t.Fatalf("%v: %v", err, violations)
			}
			// Remove one real wrapper's helper: the guard must reject the mutation.
			file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
			if err != nil {
				t.Fatal(err)
			}
			var target *ast.CallExpr
			ast.Inspect(file, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "CallCloud" && target == nil {
						target = c
					}
				}
				return true
			})
			if target == nil {
				t.Fatal("no CallCloud wrappers found")
			}
			start, end := int(target.Pos())-1, int(target.End())-1
			method := target.Args[3]
			replacement := string(source[int(method.Pos())-1:int(method.End())-1]) + "(ctx, req)"
			mutated := string(source[:start]) + replacement + string(source[end:])
			violations, err = cloudCallViolations(mutated)
			if err != nil || len(violations) == 0 {
				t.Fatalf("removed helper escaped: %v %v", err, violations)
			}
		})
	}
}

func TestCloudCallGuardRejectsNewBypasses(t *testing.T) {
	const prefix = `package fixture
 import "github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
 import ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
 `
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"function", `func wrapper(){ client.CallCloud(ctx,"NewAction",req,sdk.NewActionWithContext) }`, true},
		{"anonymous", `var wrapper=func(){ client.CallCloud(ctx,"NewAction",req,sdk.NewActionWithContext) }`, true},
		{"context-free call", `func wrapper(api *ags.Client){ api.NewAction(req) }`, false},
		{"new direct call", `func wrapper(){ sdk.NewActionWithContext(ctx,req) }`, false},
		{"anonymous bypass", `var wrapper=func(){ sdk.NewActionWithContext(ctx,req) }`, false},
		{"method alias", `func wrapper(){ call:=sdk.NewActionWithContext; call(ctx,req) }`, false},
		{"wrong action", `func wrapper(){ client.CallCloud(ctx,"WrongAction",req,sdk.NewActionWithContext) }`, false},
		{"hidden invocation", `func wrapper(){ client.CallCloud(ctx,"NewAction",req,func(){ sdk.NewActionWithContext(ctx,req) }) }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violations, err := cloudCallViolations(prefix + tc.body)
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) == 0) != tc.valid {
				t.Fatalf("violations=%v valid=%v", violations, tc.valid)
			}
		})
	}
}
