package marketplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
)

// extract.go — the INVERSE of emit_skills.go: a local `--root` operation that moves each `skill:`
// node's in-line body OUT of the node and into the file the repository owns, in the authored
// (marker-less) shape candywalk reads back. BOTH artifact classes move, each to its own tree: a
// skill to <tree>/<name>/SKILL.md plus references/<stem>.md, an `agent`-type node to
// .agents/agents/<name>.md.
//
// It writes into root — the repo that OWNS the entities — and never into the corpus, because only
// the generator authors the corpus. The node keeps its routing metadata; only the prose moves. That
// split is what makes a repository's move atomic: a repo whose prose is in files reads from files, a
// repo whose prose is still inline reads inline, and `generate` stays byte-identical across the move.
//
// WHY THIS READS FILES AND NOT `readKinds`. `readKinds` is the GENERATOR's discovery: it walks the
// charly checkout's require:/candy: closure and unions in every reachable repo's entities, which is
// exactly right for the org-wide corpus. It is the wrong tool here twice over. It reaches the
// network, and — measured — its walk yields NONE of a repository's own root-manifest entities:
// pointed at a copy of the real layer-charly-internals (a root charly.yml carrying ~20
// `agents-skill: skill:` nodes) it returned zero entities of every kind, git repo and non-git alike.
// Reading the manifests directly is simpler, offline, and makes ownership vanish as a question: a
// manifest this repo holds IS this repo's entity.
func extract(root string) error {
	roots, err := candywalk.RepoRoots(root)
	if err != nil {
		return fmt.Errorf("discover repo roots: %w", err)
	}
	moved := 0
	for _, r := range roots {
		manifests, err := repoManifests(r.Dir)
		if err != nil {
			return err
		}
		for _, rel := range manifests {
			ents, err := candywalk.ReadEntityFile(filepath.Join(r.Dir, rel), r, filepath.Dir(rel))
			if err != nil {
				return fmt.Errorf("read %s: %w", rel, err)
			}
			n, err := extractSkills(root, ents)
			if err != nil {
				return err
			}
			moved += n
		}
	}
	fmt.Fprintf(os.Stderr, "marketplace extract: %d skill(s) written under %s\n", moved, root)
	return nil
}

// repoManifests lists the manifests a repo root owns, in the same places the walker reads them: the
// root charly.yml (where a concept candy such as layer-charly-internals keeps its family's entities)
// and every candy/<name>/charly.yml (where a plugin candy keeps its own).
func repoManifests(dir string) ([]string, error) {
	manifests := []string{"charly.yml"}
	candies, err := filepath.Glob(filepath.Join(dir, "candy", "*", "charly.yml"))
	if err != nil {
		return nil, err
	}
	for _, c := range candies {
		rel, err := filepath.Rel(dir, c)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, rel)
	}
	return manifests, nil
}

// extractSkills moves every in-line skill body found in one manifest out to its file, and reports
// how many it moved. A skill with no in-line content is already file-sourced and is left alone,
// which is what makes a second run a no-op.
func extractSkills(root string, ents []candywalk.Entity) (int, error) {
	trees, err := skillTrees(ents)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, e := range ents {
		if e.Kind != "skill" {
			continue
		}
		var s spec.Skill
		if err := e.Value.Decode(&s); err != nil {
			return moved, fmt.Errorf("skill %q: decode: %w", e.Name, err)
		}
		if s.Content == "" {
			continue
		}
		name := s.Name
		if name == "" {
			name = e.Name // the node name IS the id (spec#247)
		}
		// An `agent`-type node is a SUB-AGENT DEFINITION, not a skill: its body is ONE file at
		// .agents/agents/<name>.md, with no entry directory and no references/ half (D8). It is the
		// only type whose destination does not come from the candy's `skills:` list — so it is
		// branched BEFORE the tree lookup, not through it.
		if s.Type == "agent" {
			if err := writeAgentFile(root, name, s); err != nil {
				return moved, err
			}
			moved++
			continue
		}
		// The tree comes from the OWNING CANDY's declaration, keyed exactly as readkinds.go keys it
		// (`trees[s.Owner]`) — `skillTrees` builds the map from the `candy` entities and keys it by
		// CANDY name. Keying it by the skill's own node name can never match, so a repo that declares
		// a non-default tree would have its body written to `.agents/skills/<name>/SKILL.md` while
		// `fillSkillFromFile` looks for `<declared-tree>/<name>/SKILL.md` — the file extract wrote
		// would not be the file the reader reads.
		declared, ok := trees[s.Owner]
		if !ok {
			declared = []string{candywalk.AgentsSkillsDir}
		}
		dir := filepath.Join(root, declared[0], name)
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			return moved, fmt.Errorf("skill %q: %w", name, err)
		}
		// The AUTHORED shape: the emitter's two-key frontmatter, then the body — and deliberately
		// WITHOUT the generator's marker, because the marker is what marks a file as generated and
		// from here on this file is hand-owned. This is the shape candywalk.ReadSkillFile parses.
		//
		// `skillFrontmatter` ALREADY ends with the `---\n\n` separator, and that single blank line
		// is exactly what `splitFrontmatter` strips, with ONE `TrimPrefix` of the newline that
		// follows the closing fence. Writing a second one hands the reader a body carrying a
		// leading newline the entity did not have: the generator then rejects the repo in the
		// mixed state ("the node's inline `content:` and its file disagree") and, once the inline
		// body is deleted, silently publishes that newline into the corpus. Write the separator
		// ONCE and let the body be exactly the body — a body that itself starts with blank lines
		// round-trips too, because nothing here trims.
		//
		// The TRAILING byte is the same discipline one rule over, and it is normalised here for the
		// same reason the reference writer two lines below normalises it: the emitter writes
		// `TrimSuffix(content, "\n") + "\n"`, so an authored file that merely echoes the entity's
		// content differs from the projection its own repo publishes whenever that content arrives
		// without a trailing newline — one byte, at EOF, on every skill of such a repo
		// (plugin-marketplace#33, measured 7 of 7 on pod-charly-versa). A hand-owned markdown file
		// also ends with a newline, which is what makes the two writers agree by construction.
		entry := skillFrontmatter(name, s.Description) + strings.TrimSuffix(s.Content, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(entry), 0o644); err != nil {
			return moved, fmt.Errorf("skill %q: %w", name, err)
		}
		for _, ref := range s.References {
			// references/ files carry NO frontmatter, by contract (see emit_skills.go).
			body := strings.TrimSuffix(ref.Content, "\n") + "\n"
			if err := os.WriteFile(filepath.Join(dir, "references", ref.Name+".md"), []byte(body), 0o644); err != nil {
				return moved, fmt.Errorf("skill %q reference %q: %w", name, ref.Name, err)
			}
		}
		moved++
	}
	return moved, nil
}

// writeAgentFile writes one `agent`-type node's body to the file its repository owns, at
// <root>/.agents/agents/<name>.md.
//
// The SHAPE is what separates it from the skill writer: ONE markdown file directly under the agents
// tree — no entry directory, no references/ directory, and no `skills:` lookup, because a `skills:`
// list is a list of SKILL trees. Everything else is the skill writer's contract, byte for byte: the
// emitter's two-key frontmatter, no generator marker (this file is hand-owned from here on), and the
// emitter's EOF rule `TrimSuffix(content, "\n") + "\n"` — which is what makes the authored file
// identical to the projection the same repo publishes, with no one-byte difference at EOF
// (plugin-marketplace#33).
func writeAgentFile(root, name string, s spec.Skill) error {
	dir := filepath.Join(root, candywalk.AgentsAgentsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("agent %q: %w", name, err)
	}
	entry := skillFrontmatter(name, s.Description) + strings.TrimSuffix(s.Content, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(entry), 0o644); err != nil {
		return fmt.Errorf("agent %q: %w", name, err)
	}
	return nil
}

// extractCmd moves in-line bodies into the files their repositories own. It takes --root and no
// --out on purpose: extract never writes the corpus, and a --out here would invite exactly that.
type extractCmd struct {
	Root string `name:"root" help:"Repo root that OWNS the skills to move (default: cwd)"`
}

func (c extractCmd) Run() error {
	root, err := resolveRoot(c.Root)
	if err != nil {
		return err
	}
	return extract(root)
}
