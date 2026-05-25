// Package apimeta parses TencentCloud-style api.json files into a
// strongly-typed in-memory representation. The package is the single
// source of truth that the API generator, the request validator and the
// mapping anti-corruption checker consume.
//
// The data model intentionally stays close to the api.json format used
// by https://github.com/TencentCloud/tencentcloud-cli, so future API
// updates can flow through this layer without bespoke transforms.
package apimeta

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Spec is the parsed api.json document.
type Spec struct {
	Version string             `json:"-"`
	Actions map[string]Action  `json:"actions"`
	Objects map[string]*Object `json:"objects"`
}

// Action describes a single API action.
type Action struct {
	Name     string `json:"-"`
	Document string `json:"document"`
	Input    string `json:"input"`
	Output   string `json:"output"`
	Status   string `json:"status"`
	Title    string `json:"name"`
}

// Object is a request, response or nested object.
type Object struct {
	Name     string   `json:"-"`
	Type     string   `json:"type"`
	Document string   `json:"document"`
	Usage    string   `json:"usage,omitempty"`
	Members  []Member `json:"members"`
}

// Member is a property of an Object.
type Member struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Member   string `json:"member"`
	Document string `json:"document"`
	Example  string `json:"example,omitempty"`
	Required bool   `json:"required"`
	Disabled bool   `json:"disabled,omitempty"`
}

// LoadSpec reads an api.json file from disk.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read api spec: %w", err)
	}
	return ParseSpec(data)
}

// ParseSpec parses raw api.json bytes.
func ParseSpec(data []byte) (*Spec, error) {
	spec := &Spec{}
	if err := json.Unmarshal(data, spec); err != nil {
		return nil, fmt.Errorf("parse api spec: %w", err)
	}
	for name, action := range spec.Actions {
		action.Name = name
		spec.Actions[name] = action
	}
	for name, obj := range spec.Objects {
		obj.Name = name
	}
	return spec, nil
}

// SortedActionNames returns action names in deterministic order.
func (s *Spec) SortedActionNames() []string {
	names := make([]string, 0, len(s.Actions))
	for n := range s.Actions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SortedObjectNames returns object names in deterministic order.
func (s *Spec) SortedObjectNames() []string {
	names := make([]string, 0, len(s.Objects))
	for n := range s.Objects {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Object resolves a referenced object by name.
func (s *Spec) Object(name string) *Object {
	if obj, ok := s.Objects[name]; ok {
		return obj
	}
	return nil
}

// IsScalar reports whether the given member type is a scalar.
func IsScalar(t string) bool {
	switch t {
	case "string", "int", "int64", "uint", "uint64", "float", "double", "bool", "binary":
		return true
	}
	return false
}

// KebabCase converts PascalCase / camelCase identifiers to kebab-case.
// Example: "StorageMounts" -> "storage-mounts", "ToolId" -> "tool-id",
// "VpcConfig" -> "vpc-config", "APIKey" -> "api-key", "CLSConfig" -> "cls-config".
func KebabCase(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	var buf strings.Builder
	for i, r := range runes {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && i > 0 {
			prev := runes[i-1]
			prevUpper := prev >= 'A' && prev <= 'Z'
			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			nextLower := next >= 'a' && next <= 'z'
			if !prevUpper || nextLower {
				buf.WriteByte('-')
			}
		}
		if isUpper {
			buf.WriteRune(r - 'A' + 'a')
		} else {
			buf.WriteRune(r)
		}
	}
	out := buf.String()
	// collapse consecutive dashes if any (defensive)
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

// ScalarFlagType returns a generator-level flag type identifier for a member.
// For non-scalar / list / object types it returns "json".
func ScalarFlagType(m Member) string {
	switch strings.ToLower(m.Type) {
	case "string":
		return "string"
	case "bool":
		return "bool"
	case "int", "int64":
		return "int64"
	case "uint", "uint64":
		return "uint64"
	case "float", "double":
		return "float64"
	case "list":
		return "json"
	case "object":
		return "json"
	}
	return "string"
}
