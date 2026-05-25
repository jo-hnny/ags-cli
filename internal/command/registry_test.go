package command

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryRejectsDuplicateCommandPath(t *testing.T) {
	registry := NewRegistry()
	first := testModule("one", []string{"tool", "list"})
	second := testModule("two", []string{"tool", "list"})
	if err := registry.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	err := registry.Register(second)
	if err == nil || !strings.Contains(err.Error(), "duplicate command path") {
		t.Fatalf("expected duplicate command path error, got %v", err)
	}
}

func TestRegistryRejectsMissingBuild(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Module{Descriptor: Descriptor{Spec: Spec{ID: "x", Path: []string{"x"}}}})
	if err == nil || !strings.Contains(err.Error(), "missing Build") {
		t.Fatalf("expected missing Build error, got %v", err)
	}
}

func TestRegistryDescriptorsDoesNotBuild(t *testing.T) {
	registry := NewRegistry()
	module := testModule("x", []string{"x"})
	module.Build = func(Deps) (Runtime, error) {
		t.Fatal("Descriptors must not call Build")
		return Runtime{}, nil
	}
	if err := registry.Register(module); err != nil {
		t.Fatalf("register module: %v", err)
	}
	descriptors := registry.Descriptors()
	if len(descriptors) != 1 {
		t.Fatalf("len(Descriptors) = %d, want 1", len(descriptors))
	}
	if descriptors[0].Spec.ID != "x" {
		t.Fatalf("descriptor id = %q, want x", descriptors[0].Spec.ID)
	}
}

func TestRegistryDescriptorsReturnsStableOrderAndCopiesMetadata(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testModule("b", []string{"b"})); err != nil {
		t.Fatalf("register b: %v", err)
	}
	if err := registry.Register(testModule("a", []string{"a"})); err != nil {
		t.Fatalf("register a: %v", err)
	}
	first := registry.Descriptors()
	second := registry.Descriptors()
	if got := []string{first[0].Spec.ID, first[1].Spec.ID}; strings.Join(got, ",") != "b,a" {
		t.Fatalf("descriptor order = %v, want [b a]", got)
	}
	if got := []string{second[0].Spec.ID, second[1].Spec.ID}; strings.Join(got, ",") != "b,a" {
		t.Fatalf("second descriptor order = %v, want [b a]", got)
	}
	first[0].Spec.Path[0] = "mutated"
	if got := registry.Descriptors()[0].Spec.Path[0]; got != "b" {
		t.Fatalf("descriptor path was mutated through returned slice: %q", got)
	}
}

func testModule(id string, path []string) Module {
	return Module{
		Descriptor: Descriptor{Spec: Spec{ID: id, Path: path}},
		Build: func(Deps) (Runtime, error) {
			return Runtime{Handler: HandlerFunc(func(context.Context, Request) (*Result, error) { return &Result{}, nil })}, nil
		},
	}
}
