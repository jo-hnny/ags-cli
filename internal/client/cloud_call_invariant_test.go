package client

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// Derive both context-aware and context-free Action names from the installed SDK.
// Matching method references also catches aliases and receivers obtained in other
// files, without requiring every caller to import or name the SDK client type.
func cloudActionMethods() map[string]bool {
	methods := map[string]bool{}
	sdk := reflect.TypeFor[*ags.Client]()
	for i := range sdk.NumMethod() {
		method := sdk.Method(i)
		if strings.HasSuffix(method.Name, "WithContext") && method.Type.NumIn() == 3 {
			request := method.Type.In(2)
			if request.Kind() == reflect.Pointer && request.Elem().PkgPath() == reflect.TypeFor[ags.Client]().PkgPath() {
				methods[method.Name] = true
				methods[strings.TrimSuffix(method.Name, "WithContext")] = true
			}
		}
	}
	return methods
}

func cloudCallViolations(filename, source string, methods map[string]bool) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, 0)
	if err != nil {
		return nil, err
	}
	clientAlias := ""
	for _, imp := range file.Imports {
		if imp.Path.Value == `"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"` {
			clientAlias = "client"
			if imp.Name != nil {
				clientAlias = imp.Name.Name
			}
		}
	}
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
			violations = append(violations, fmt.Sprintf("%s: CallCloud must have four arguments", fset.Position(call.Pos())))
			return true
		}
		method, ok := call.Args[3].(*ast.SelectorExpr)
		if !ok || !methods[method.Sel.Name] || !strings.HasSuffix(method.Sel.Name, "WithContext") {
			violations = append(violations, fmt.Sprintf("%s: CallCloud requires a direct SDK method value", fset.Position(call.Pos())))
			return true
		}
		action, ok := call.Args[1].(*ast.BasicLit)
		if !ok || action.Kind != token.STRING {
			violations = append(violations, fmt.Sprintf("%s: CallCloud requires a literal Action", fset.Position(call.Pos())))
			return true
		}
		name, err := strconv.Unquote(action.Value)
		if err != nil || name != strings.TrimSuffix(method.Sel.Name, "WithContext") {
			violations = append(violations, fmt.Sprintf("%s: Action does not match SDK method", fset.Position(call.Pos())))
			return true
		}
		allowed[method] = true
		return true
	})
	// Locally declared function fields are dependency-injection hooks, not SDK
	// methods. Resolve the receiver object so this exemption cannot cover another
	// variable with the same spelling (including a real SDK parameter).
	hooks := map[string]map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structure, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		fields := map[string]bool{}
		for _, field := range structure.Fields.List {
			if _, ok := field.Type.(*ast.FuncType); ok {
				for _, name := range field.Names {
					fields[name.Name] = true
				}
			}
		}
		hooks[spec.Name.Name] = fields
		return true
	})
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			continue
		}
		receiver := fn.Recv.List[0]
		typ := receiver.Type
		if ptr, ok := typ.(*ast.StarExpr); ok {
			typ = ptr.X
		}
		name, ok := typ.(*ast.Ident)
		if !ok || len(receiver.Names) != 1 {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			object, ok := selector.X.(*ast.Ident)
			if ok && object.Obj != nil && object.Obj == receiver.Names[0].Obj && hooks[name.Name][selector.Sel.Name] {
				allowed[selector] = true
			}
			return true
		})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		method, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if methods[method.Sel.Name] && !allowed[method] {
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
			violations, err := cloudCallViolations(path, string(source), cloudActionMethods())
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
			violations, err = cloudCallViolations(path, mutated, cloudActionMethods())
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
		{"function hook", `type S struct{NewAction func()}; func(s *S) wrapper(){ s.NewAction() }`, true},
		{"SDK with hook receiver name", `type S struct{NewAction func()}; func(s *S) wrapper(){ func(s *ags.Client){s.NewAction(req)}(sdk) }`, false},
		{"unrelated context method", `func wrapper(){ http.NewRequestWithContext(ctx,"GET",url,nil) }`, true},
		{"method alias", `func wrapper(){ call:=sdk.NewActionWithContext; call(ctx,req) }`, false},
		{"wrong action", `func wrapper(){ client.CallCloud(ctx,"WrongAction",req,sdk.NewActionWithContext) }`, false},
		{"hidden invocation", `func wrapper(){ client.CallCloud(ctx,"NewAction",req,func(){ sdk.NewActionWithContext(ctx,req) }) }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violations, err := cloudCallViolations("fixture.go", prefix+tc.body, map[string]bool{"NewAction": true, "NewActionWithContext": true})
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) == 0) != tc.valid {
				t.Fatalf("violations=%v valid=%v", violations, tc.valid)
			}
		})
	}
}

// Scan source independently of build tags so a new platform/channel file is
// checked even when it is not compiled by this test's build configuration.
// tests/ and testdata contain test harnesses/fixtures, not shipped CLI code.
func cloudSourceViolations(root fs.FS) ([]string, error) {
	var violations []string
	methods := cloudActionMethods()
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata" || path == "tests") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		found, err := cloudCallViolations(path, string(source), methods)
		if err != nil {
			return err
		}
		violations = append(violations, found...)
		return nil
	})
	return violations, err
}

func TestCloudCallsAcrossRepository(t *testing.T) {
	violations, err := cloudSourceViolations(os.DirFS("../.."))
	if err != nil || len(violations) > 0 {
		t.Fatalf("%v: %v", err, violations)
	}
}

func TestCloudGuardDiscoversThirdFile(t *testing.T) {
	const safe = `package fixture
 import "github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
 func call(){client.CallCloud(ctx,"CreateAPIKey",req,sdk.CreateAPIKeyWithContext)}`
	files := fstest.MapFS{
		"internal/cli/cloud_calls.go":  {Data: []byte(safe)},
		"internal/controlplane/sdk.go": {Data: []byte(safe)},
	}
	if found, err := cloudSourceViolations(files); err != nil || len(found) != 0 {
		t.Fatalf("baseline: %v %v", found, err)
	}
	for _, call := range []string{"sdk.CreateAPIKeyWithContext(ctx,req)", "sdk.CreateAPIKey(req)", "invoke := sdk.CreateAPIKeyWithContext; invoke(ctx,req)"} {
		// The new file intentionally has no SDK import and is excluded from this OS's build.
		files["cmd/newcommand/call_windows.go"] = &fstest.MapFile{Data: []byte("//go:build windows\n\npackage fixture\nfunc call(){" + call + "}")}
		found, err := cloudSourceViolations(files)
		if err != nil || len(found) != 1 || !strings.Contains(found[0], "cmd/newcommand/call_windows.go") {
			t.Fatalf("new file escaped: %v %v", found, err)
		}
	}
}

func TestCloudGuardDiagnosticLocation(t *testing.T) {
	const path = "internal/zzprobe/probe_windows.go"
	for _, tc := range []struct{ source, want string }{
		{"package probe\nfunc probe(){ sdk.CreateAPIKeyWithContext(ctx, req) }", path + ":2:15: CreateAPIKeyWithContext bypasses CallCloud"},
		{"package probe\nimport \"github.com/TencentCloudAgentRuntime/ags-cli/internal/client\"\nfunc probe(){ client.CallCloud(ctx, \"Wrong\", req, sdk.CreateAPIKeyWithContext) }", path + ":3:15: Action does not match SDK method"},
	} {
		found, err := cloudSourceViolations(fstest.MapFS{path: {Data: []byte(tc.source)}})
		if err != nil || len(found) == 0 || found[0] != tc.want {
			t.Fatalf("got %v, %v; want %s", found, err, tc.want)
		}
	}
}
