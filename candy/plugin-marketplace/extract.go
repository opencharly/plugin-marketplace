package marketplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
)

// extract.go — the INVERSE of emit_skills.go: a local `--root` operation that moves each skill's
// in-line body OUT of its `skill:` node and into the file the repository owns, in the authored
// (marker-less) shape candywalk.ReadSkillFile reads back.
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
		// The tree comes from the OWNING CANDY's declaration, keyed exactly as readkinds.go keys it
		// (`trees[s.Owner]`) — `skillTrees` builds the map from the `candy` entities and keys it by
		// CANDY name. Keying it by the skill's own node name can never match, so a repo that declares
		// a non-default tree would have its body written to `.agents/skills/<name>/SKILL.md` while
		// `fillSkillFromFile` looks for `<declared-tree>/<name>/SKILL.md` — the file extract wrote
		// would not be the file the reader reads.
		declared, ok := trees[s.Owner]
		if !ok {
			declared = []string{agentsSkillsDir}
		}
		dir := filepath.Join(root, declared[0], name)
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			return moved, fmt.Errorf("skill %q: %w", name, err)
		}
		// The AUTHORED shape: the emitter's two-key frontmatter, then the body — and deliberately
		// WITHOUT the generator's marker, because the marker is what marks a file as generated and
		// from here on this file is hand-owned. This is the shape candywalk.ReadSkillFile parses.
		entry := skillFrontmatter(name, s.Description) + "\n" + s.Content
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
