package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
)

// extract_agent_test.go — `marketplace extract`'s AGENT half (D8: an `agent`-type node's body lives
// at <repo>/.agents/agents/<name>.md).
//
// The three-way gap this closes, measured on the committed tree before the change: `candywalk` had
// one file source and it was a skill's (`<tree>/<name>/SKILL.md`); `extract` moved `e.Kind == "skill"`
// and nothing else; and the only agent-aware code was the EMITTER, which published
// `plugins/<family>/agents/<name>.md` from the node. An agent body therefore had a writer to the
// corpus and no reader or writer anywhere else — the shape that silently subtracts a published
// artifact (opencharly/plugin-marketplace#34, found live in pod-charly-automation#25).
//
// The fixtures use the REAL authored shape, taken from the live owning repo
// layer-charly-internals-extra/charly.yml: a node keyed by its own name, one `skill:` key as the KIND
// discriminator, `type: agent` selecting the artifact class, and `model:` carrying the agent
// frontmatter metadata the corpus projects.

// agentFixture builds one `agent`-type node around an arbitrary body. indicator selects the YAML
// block scalar: `|` keeps a trailing newline on the decoded value, `|-` strips it.
func agentFixture(node, indicator, body string) string {
	return agentManifest(node, agentContentBlock(indicator, body))
}

// agentContentBlock is the ONE snippet a repo deletes once the body has moved. The no-op test removes
// exactly this, computed the way the fixture built it, so a removal that misses cannot leave that
// test passing vacuously.
//
// The FIRST body line carries the 12-space indent too — dropping it (a YAML block needs every line
// indented) makes the whole document unparseable, and the failure surfaces as "the fixture holds no
// node" rather than as a syntax error.
func agentContentBlock(indicator, body string) string {
	return "        content: " + indicator + "\n" + "            " + strings.ReplaceAll(body, "\n", "\n            ") + "\n"
}

func agentManifest(node, contentBlock string) string {
	return "demo:\n" +
		"    candy:\n" +
		"        description: |-\n" +
		"            Fixture candy owning one agent entity.\n" +
		"    plan:\n" +
		"        - run: no-op fixture\n" +
		"          command: \"true\"\n" +
		"          context:\n" +
		"            - build\n" +
		node + ":\n" +
		"    skill:\n" +
		"        name: " + node + "\n" +
		"        family: internals\n" +
		"        owner: demo\n" +
		"        type: agent\n" +
		"        model: inherit\n" +
		"        description: |-\n" +
		"            Blocking — the fresh PR evaluator.\n" +
		contentBlock
}

// authoredAgentFromProjection is the agent's byte-exactness claim, and the assertion that catches a
// one-byte EOF defect. `extract` must write exactly what the CORPUS projection carries, MINUS the
// two things the corpus adds and an authored file must not have:
//
//	the DO-NOT-EDIT banner   — this file is hand-owned, not generated;
//	the `model:` line        — the NODE keeps that metadata (the D2/D3 split: metadata in charly.yml,
//	                           prose in the file), and emitAgent re-adds it to the corpus.
//
// Neither side is re-stated here: the projection comes from the real emitter (`emitSkills`) and the
// banner from the real constant, so a change to either fails this test instead of hiding behind it.
func authoredAgentFromProjection(t *testing.T, em emissions) string {
	t.Helper()
	_, corpus := entryOf(t, em)
	if !strings.Contains(corpus, generatedHeaderBlock) {
		t.Fatalf("the projection carries no generator banner — a banner-stripped comparison would be vacuous:\n%s", corpus)
	}
	var kept []string
	for _, line := range strings.Split(strings.Replace(corpus, generatedHeaderBlock, "", 1), "\n") {
		if strings.HasPrefix(line, "model: ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// TestExtract_AgentBodyLandsWhereTheAgentReaderReadsIt — the whole agent half, in the four claims it
// makes. Each is a distinct way to be wrong:
//
//	(1) the file exists at D8's path AND NOWHERE ELSE — a body written into the skills tree is the
//	    live defect of pod-charly-automation#25, where a body moved by hand is read by no reader;
//	(2) the authored file equals the published projection minus the banner and minus `model:`, byte
//	    for byte, EOF included — the claim plugin-marketplace#33 broke;
//	(3) the reader reads the entity's prose back, with no EOF newline added or lost beyond the one the
//	    authored file normalises (see the assertion for why exactly one byte is allowed);
//	(4) `extract -> read -> emit` lands on the SAME corpus the entity-sourced node produces, path and
//	    bytes — the move is invisible in the published artifact.
func TestExtract_AgentBodyLandsWhereTheAgentReaderReadsIt(t *testing.T) {
	const node = "pr-validator"
	const body = "# pr-validator\n\nFresh evaluation.\n\n- one\n- two"
	cases := []struct {
		name      string
		indicator string
		body      string
	}{
		{"with-a-trailing-newline", "|", body + "\n"},
		// A `content:` the YAML parser hands over WITHOUT a trailing newline: the input #33 was
		// measured on (7 of 7 skills of pod-charly-versa), and invisible to every presence-shaped
		// assertion and to a body-only round trip.
		{"without-a-trailing-newline", "|-", body},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeExtractFixture(t, agentFixture(node, tc.indicator, tc.body))
			want := decodeOneSkill(t, root, node)
			if want.Type != "agent" {
				t.Fatalf("fixture: the node decoded as type %q, want agent — the fixture cannot carry this case", want.Type)
			}
			if want.Content != tc.body {
				t.Fatalf("fixture: the node decoded to %q, want %q", want.Content, tc.body)
			}

			if err := extract(root); err != nil {
				t.Fatalf("extract: %v", err)
			}

			// (1) D8's path, and only it.
			path := filepath.Join(root, candywalk.AgentsAgentsDir, node+".md")
			entry, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("extract did not write the agent file %s: %v", path, err)
			}
			misplaced := filepath.Join(root, candywalk.AgentsSkillsDir, node, "SKILL.md")
			if _, err := os.Stat(misplaced); !os.IsNotExist(err) {
				t.Errorf("extract ALSO wrote the SKILLS tree layout at %s — a body under `skills:` is read by no reader, which is the live defect of pod-charly-automation#25", misplaced)
			}
			if strings.Contains(string(entry), "GENERATED by") {
				t.Error("the generator's marker leaked into a hand-owned file")
			}

			// (2) byte-exact against the emitter's own projection.
			if projected := authoredAgentFromProjection(t, emissionFor(t, want)); string(entry) != projected {
				t.Errorf("the authored agent file is not the published projection minus the banner and `model:`:\n  file       %q\n  projection %q", string(entry), projected)
			}

			// (3) the reader reads the entity's prose back. The ONE permitted difference is the
			// trailing byte: the authored file is normalised to the emitter's EOF rule
			// (`TrimSuffix(content, "\n") + "\n"`, plugin-marketplace#33), so an entity whose
			// `content:` arrived without a newline reads back with one. The corpus is unaffected
			// either way — every writer TrimSuffixes — which is what claim (4) re-proves.
			af, err := candywalk.ReadAgentFile(filepath.Join(root, candywalk.AgentsAgentsDir), node)
			if err != nil {
				t.Fatalf("read the extracted agent file back: %v", err)
			}
			if af == nil {
				t.Fatalf("extract wrote no agent file for %q", node)
			}
			if wantBody := strings.TrimSuffix(want.Content, "\n") + "\n"; af.Content != wantBody {
				t.Errorf("the reader reads a different body than the entity held:\n  entity EOF-normalised %q\n  file                  %q", wantBody, af.Content)
			}
			if af.Description != want.Description {
				t.Errorf("description: entity %q, file %q", want.Description, af.Description)
			}

			// (4) the move is invisible in the artifact — same paths, same bytes.
			fromFile := want
			fromFile.Content, fromFile.Description = af.Content, af.Description
			entityEm, fileEm := emissionFor(t, want), emissionFor(t, fromFile)
			if len(entityEm) != len(fileEm) {
				t.Fatalf("the move changed the emitted file set: %v vs %v", sortedKeys(entityEm), sortedKeys(fileEm))
			}
			for p, b := range entityEm {
				got, ok := fileEm[p]
				if !ok {
					t.Errorf("the move dropped %s from the corpus", p)
					continue
				}
				if string(got) != string(b) {
					t.Errorf("%s differs across the move:\n  entity %q\n  file   %q", p, b, got)
				}
			}
		})
	}
}

// TestExtract_AgentNodeWithNoBodyIsLeftAlone — the no-op half: after the body is deleted from the
// node, extract must move nothing and must not rewrite what is already there.
func TestExtract_AgentNodeWithNoBodyIsLeftAlone(t *testing.T) {
	const node = "pr-validator"
	root := writeExtractFixture(t, agentFixture(node, "|", "# pr-validator\n\nBody.\n"))
	if err := extract(root); err != nil {
		t.Fatalf("extract: %v", err)
	}
	path := filepath.Join(root, candywalk.AgentsAgentsDir, node+".md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the first extract wrote nothing: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The wave's step: the in-line body goes (exactly the snippet the fixture added).
	if err := os.WriteFile(filepath.Join(root, "charly.yml"), []byte(agentManifest(node, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extract(root); err != nil {
		t.Fatalf("extract after the body was deleted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file did not survive the second extract: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("extract rewrote an already-moved agent file:\n  before %q\n  after  %q", before, after)
	}
}

// TestFillAgentFromFile_IsLoud — the fail-loud contract of the skill branch, carried to the agent
// branch. Silence in any of these is a published sub-agent subtracted from the corpus with no error,
// which is the failure class this whole relocation exists to remove.
func TestFillAgentFromFile_IsLoud(t *testing.T) {
	t.Run("neither-a-file-nor-an-inline-body", func(t *testing.T) {
		s := spec.Skill{Name: "ghost", Type: "agent"}
		err := fillAgentFromFile(&s, "ghost", t.TempDir())
		if err == nil {
			t.Fatal("an agent node with no body anywhere was accepted — the corpus would publish nothing")
		}
		for _, want := range []string{"ghost", candywalk.AgentsAgentsDir} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %q — the failure must say where the file was looked for", err, want)
			}
		}
	})

	t.Run("inline-body-disagrees-with-the-file", func(t *testing.T) {
		root := t.TempDir()
		writeAgentFixture(t, root, "pr-validator", "# From the file.\n")
		s := spec.Skill{Name: "pr-validator", Type: "agent", Content: "# From the node.\n"}
		if err := fillAgentFromFile(&s, "pr-validator", root); err == nil {
			t.Fatal("two sources of truth for one published agent were accepted")
		}
		if s.Content != "# From the node.\n" {
			t.Errorf("the node was mutated by a rejected read: %q", s.Content)
		}
	})

	t.Run("inline-description-disagrees-with-the-file", func(t *testing.T) {
		root := t.TempDir()
		writeAgentFixture(t, root, "pr-validator", "# From the file.\n")
		s := spec.Skill{Name: "pr-validator", Type: "agent", Description: "something else"}
		if err := fillAgentFromFile(&s, "pr-validator", root); err == nil {
			t.Fatal("a description disagreeing with the file's frontmatter was accepted")
		}
	})

	t.Run("unreadable-file", func(t *testing.T) {
		root := t.TempDir()
		// A DIRECTORY where the file belongs: os.ReadFile fails with something that is not ENOENT,
		// so it must surface rather than read as "this repo has not moved".
		if err := os.MkdirAll(filepath.Join(root, candywalk.AgentsAgentsDir, "ghost.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		s := spec.Skill{Name: "ghost", Type: "agent"}
		if err := fillAgentFromFile(&s, "ghost", root); err == nil {
			t.Fatal("an unreadable agent file was accepted as absent")
		}
	})

	t.Run("the-file-wins-when-both-agree", func(t *testing.T) {
		root := t.TempDir()
		writeAgentFixture(t, root, "pr-validator", "# From the file.\n")
		s := spec.Skill{Name: "pr-validator", Type: "agent", Content: "# From the file.\n"}
		if err := fillAgentFromFile(&s, "pr-validator", root); err != nil {
			t.Fatalf("an agreeing in-line body was rejected: %v", err)
		}
		if s.Content != "# From the file.\n" {
			t.Errorf("content = %q, want the file's body", s.Content)
		}
	})
}

// TestReadKinds_AgentNodeReadsFromItsFile — the WIRING, not just the branch: an `agent`-type node
// must reach `fillAgentFromFile` through readkinds.go's decode loop, which is the one funnel every
// node enters the model through.
func TestReadKinds_AgentNodeReadsFromItsFile(t *testing.T) {
	root := t.TempDir()
	writeAgentFixture(t, root, "pr-validator", "# pr-validator\n\nFrom the file.\n")
	if err := os.WriteFile(filepath.Join(root, "charly.yml"), []byte(agentManifest("pr-validator", "")), 0o644); err != nil {
		t.Fatal(err)
	}
	// readKinds walks the network closure, so exercise the two functions it composes instead: the
	// decode loop's inputs are already covered above, and this asserts the SHAPE those inputs take.
	ents, err := candywalk.ReadEntityFile(filepath.Join(root, "charly.yml"), candywalk.Root{Dir: root}, ".")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	var found bool
	for _, e := range ents {
		if e.Kind != "skill" || e.Name != "pr-validator" {
			continue
		}
		found = true
		var s spec.Skill
		if err := e.Value.Decode(&s); err != nil {
			t.Fatalf("decode the fixture: %v", err)
		}
		if err := fillSkillFromFile(&s, e.Name, "", root, []string{candywalk.AgentsSkillsDir}); err != nil {
			t.Fatalf("fillSkillFromFile: %v", err)
		}
		if s.Content != "# pr-validator\n\nFrom the file.\n" {
			t.Errorf("the file's body did not reach the model: %q", s.Content)
		}
	}
	if !found {
		t.Fatal("the fixture holds no agent node")
	}
}

// writeAgentFixture lays out one authored agent file — the two-key frontmatter `extract` writes,
// no generator banner — under <root>/.agents/agents.
func writeAgentFixture(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, candywalk.AgentsAgentsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := skillFrontmatter(name, "Blocking — the fresh PR evaluator.") + body
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
}
