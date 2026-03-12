package stepoutput

import (
	"testing"
)

func TestParseRef(t *testing.T) {
	tests := map[string]OutputRef{
		"build.version":               {Step: "build", Key: "version"},
		"build/image.digest":          {Step: "build", Key: "image.digest"},
		"steps.build.outputs.tags.-1": {Step: "build", Key: "tags.-1"},
		"step=build,key=image.digest": {Step: "build", Key: "image.digest"},
	}
	for input, want := range tests {
		got, err := ParseRef(input)
		if err != nil {
			t.Fatalf("ParseRef(%q) error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseRef(%q) got %+v want %+v", input, got, want)
		}
	}
}

func TestFlattenUnder(t *testing.T) {
	patch, err := FlattenUnder("image", map[string]any{
		"name": "repo/app",
		"tags": []any{"latest", "1.2.3"},
		"old":  nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := *patch["image.name"]; got != "repo/app" {
		t.Fatalf("want image.name=repo/app, got %s", got)
	}
	if got := *patch["image.tags.0"]; got != "latest" {
		t.Fatalf("want image.tags.0=latest, got %s", got)
	}
	if patch["image.old"] != nil {
		t.Fatal("expected null leaf to unset key")
	}
}

func TestResolveValue(t *testing.T) {
	values := map[string]string{
		"tags.0": "latest",
		"tags.1": "1.2.3",
		"tags.2": "stable",
	}
	got, ok, err := ResolveValue(values, "tags.-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "stable" {
		t.Fatalf("want stable, got %q ok=%v", got, ok)
	}
}
