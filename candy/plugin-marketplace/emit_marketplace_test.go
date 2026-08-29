package marketplace

// emit_marketplace_test.go — pins .claude-plugin/marketplace.json to the Claude Code
// plugin-marketplace spec (https://code.claude.com/docs/en/plugin-marketplaces).
//
// The spec defines `description` and `version` as TOP-LEVEL marketplace fields and documents
// `metadata` as the carrier for `pluginRoot`. This emitter previously nested description and
// version under `metadata`, so neither was read as the marketplace's own description/version —
// the file validated, because `metadata` is free-form, which is exactly why nothing caught it.
// These assertions fail if that nesting comes back.

import (
	"encoding/json"
	"testing"

	"github.com/opencharly/spec/spec"
)

func testMarketplaceKindSet() *kindSet {
	return &kindSet{
		Marketplace: &spec.Marketplace{
			Name:        "charly-plugins",
			Description: "the marketplace description",
			Version:     "3.1.0",
		},
	}
}

// TestMarketplaceJSON_DescriptionAndVersionAreTopLevel is the spec-conformance assertion.
func TestMarketplaceJSON_DescriptionAndVersionAreTopLevel(t *testing.T) {
	raw := mustJSON(buildMarketplace(testMarketplaceKindSet(), nil))

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("marketplace.json does not parse: %v", err)
	}

	if got["description"] != "the marketplace description" {
		t.Errorf("description must be a TOP-LEVEL field; got %#v", got["description"])
	}
	if got["version"] != "3.1.0" {
		t.Errorf("version must be a TOP-LEVEL field; got %#v", got["version"])
	}

	// The spec reserves `metadata` for pluginRoot. Carrying description/version there means the
	// marketplace has no description or version as far as Claude Code is concerned.
	if md, ok := got["metadata"].(map[string]any); ok {
		if _, bad := md["description"]; bad {
			t.Error("metadata.description is back — the spec puts description at top level")
		}
		if _, bad := md["version"]; bad {
			t.Error("metadata.version is back — the spec puts version at top level")
		}
	}
}

// TestMarketplaceJSON_RequiredFieldsPresent guards the two fields the spec marks required.
func TestMarketplaceJSON_RequiredFieldsPresent(t *testing.T) {
	raw := mustJSON(buildMarketplace(testMarketplaceKindSet(), nil))

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("marketplace.json does not parse: %v", err)
	}
	if got["name"] != "charly-plugins" {
		t.Errorf(`required field "name" wrong: %#v`, got["name"])
	}
	owner, ok := got["owner"].(map[string]any)
	if !ok || owner["name"] == "" {
		t.Errorf(`required field "owner.name" missing: %#v`, got["owner"])
	}
}

// TestMarketplaceJSON_RenamesOmittedWhileEmpty keeps the generated file free of an empty
// `"renames": {}`; the field exists so a future rename has a supported path.
func TestMarketplaceJSON_RenamesOmittedWhileEmpty(t *testing.T) {
	raw := mustJSON(buildMarketplace(testMarketplaceKindSet(), nil))

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("marketplace.json does not parse: %v", err)
	}
	if _, present := got["renames"]; present {
		t.Error(`"renames" should be omitted while empty`)
	}
}
