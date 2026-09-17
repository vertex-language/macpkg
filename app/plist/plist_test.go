package plist_test

import (
	"testing"

	"github.com/vertex-language/macpkg/app/plist"
)

func TestMarshalUnmarshalXML(t *testing.T) {
	orig := map[string]any{
		"CFBundleIdentifier":      "com.example.app",
		"CFBundleName":            "ExampleApp",
		"CFBundleVersion":         "1.0.0",
		"CFBundleExecutable":      "example",
		"LSMinimumSystemVersion":  "11.0",
		"NSHighResolutionCapable": true,
		"NumberCount":             int64(42),
		"Permissions":             []any{"admin", "user"},
		"Nested": map[string]any{
			"Key": "Value",
		},
	}

	data, err := plist.MarshalXML(orig)
	if err != nil {
		t.Fatalf("MarshalXML: %v", err)
	}

	parsed, err := plist.UnmarshalXML(data)
	if err != nil {
		t.Fatalf("UnmarshalXML: %v", err)
	}

	if parsed["CFBundleIdentifier"] != "com.example.app" {
		t.Errorf("expected com.example.app, got %v", parsed["CFBundleIdentifier"])
	}
	if parsed["NSHighResolutionCapable"] != true {
		t.Errorf("expected true, got %v", parsed["NSHighResolutionCapable"])
	}
	if parsed["NumberCount"] != int64(42) {
		t.Errorf("expected 42, got %v", parsed["NumberCount"])
	}
	nested, ok := parsed["Nested"].(map[string]any)
	if !ok || nested["Key"] != "Value" {
		t.Errorf("expected nested Key=Value, got %v", parsed["Nested"])
	}
}
