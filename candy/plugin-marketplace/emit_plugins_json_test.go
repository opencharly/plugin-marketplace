package marketplace

// emit_plugins_json_test.go — pins the generated MCP config to PUBLIC servers only
// (opencharly/plugin-marketplace#61). A server on a local host is served by a charly pod while it
// runs; wired into the static config, every harness reports it "failed" wherever that pod is down.

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

func mcpFamily(name string, servers ...spec.MarketplaceMCPServer) family {
	return family{Name: name, Meta: spec.MarketplaceFamily{Description: "d", McpServers: servers}}
}

func TestIsPublicURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://mcp.deepwiki.com/mcp":      true,
		"http://example.org:8080/mcp":       true,
		"http://localhost:18765/mcp":        false,
		"http://LOCALHOST:8888/mcp":         false,
		"http://pod.localhost:9224/mcp":     false,
		"http://127.0.0.1:22718/mcp/server": false,
		"http://127.5.0.1/mcp":              false,
		"http://[::1]:8888/mcp":             false,
		"http://0.0.0.0:8888/mcp":           false,
		"":                                  false,
		"://bad":                            false,
	} {
		if got := isPublicURL(u); got != want {
			t.Errorf("isPublicURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestEmitPluginsJSONPublicMCPOnly(t *testing.T) {
	public := spec.MarketplaceMCPServer{Name: "deepwiki", Type: "http", Url: "https://mcp.deepwiki.com/mcp"}
	local := spec.MarketplaceMCPServer{Name: "jupyter", Type: "http", Url: "http://localhost:8888/mcp"}
	em := emissions{}
	emitPluginsJSON(em, []family{
		mcpFamily("localonly", local),
		mcpFamily("mixed", local, public),
		mcpFamily("none"),
	})

	// A family whose only server is local gets NO .mcp.json and NO manifest reference.
	if _, ok := em["plugins/localonly/.mcp.json"]; ok {
		t.Error("localonly: a .mcp.json was emitted for a family with no public server")
	}
	for _, m := range []string{".claude-plugin/plugin.json", ".codex-plugin/plugin.json"} {
		if strings.Contains(string(em["plugins/localonly/"+m]), "mcpServers") {
			t.Errorf("localonly/%s references an MCP config: %s", m, em["plugins/localonly/"+m])
		}
		if !strings.Contains(string(em["plugins/mixed/"+m]), `"mcpServers": "./.mcp.json"`) {
			t.Errorf("mixed/%s does not reference its public MCP config", m)
		}
	}

	// A mixed family keeps the public server and drops the local one.
	mixed := string(em["plugins/mixed/.mcp.json"])
	if !strings.Contains(mixed, "https://mcp.deepwiki.com/mcp") {
		t.Errorf("mixed: the public server is missing: %s", mixed)
	}
	if strings.Contains(mixed, "localhost") || strings.Contains(mixed, "jupyter") {
		t.Errorf("mixed: the local server leaked into the static config: %s", mixed)
	}

	if _, ok := em["plugins/none/.mcp.json"]; ok {
		t.Error("none: a .mcp.json was emitted for a family that declares no server")
	}
}
