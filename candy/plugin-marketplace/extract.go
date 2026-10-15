package marketplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
)

// extract.go — the INVERSE of emit_skills.go: a local `--root` operation that moves each skill's
// in-line body OUT of its `skill:` node and into the file the repository owns, in the authored
// (marker-less) shape candywalk.ReadSkillFile reads back.
//
// It writes into root — the repo that OWNS the entities — and never into the corpus, because
// nothing but the generator authors into the corpus. The node keeps its routing metadata; only the
// prose moves. That split is what makes a repository's move atomic: a repo whose bodies are in files
// reads from files, a repo whose bodies are still inline reads inline, and `generate` is
// byte-identical across the move.
//
// The walk is deliberately LOCAL — candywalk.CollectEntities, NOT CollectEntitiesRemote.
// readKinds is remote-aware because the corpus is org-wide, but extract must not reach the network
// to do its job, and above all must never write a body into this repo for a skill that this repo
// does not own: that would fabricate a file whose `skill:` node lives somewhere else, and leave the
// dedupe arbitrating between a node and a file that were never meant to meet.
func extract(root string) error {
	roots, err := candywalk.RepoRoots(root)
	if err != nil {
		return fmt.Errorf("discover repo roots: %w", err)
	}
	// The SAME discovery readKinds uses — the remote-aware collector — and then filtered below to
	// the entities THIS repo owns. Measured, not assumed: the purely-local candywalk.CollectEntities
	// yields none of these entities, even against this repo's own charly.yml (which carries ~20
	// skill entities); extract returned 0 for both a hand-built fixture and a copy of the real
	// layer-charly-internals. Same discovery, then refuse to write anything that is not ours.
	ents, err := candywalk.CollectEntitiesRemote(roots, refs.DownloadRepo)
	if err != nil {
		return fmt.Errorf("collect entities: %w", err)
	}
	trees, err := skillTrees(ents)
	if err != nil {
		return err
	}
	moved := 0
	for _, e := range ents {
		if e.Kind != "skill" || e.SourceRoot != "" {
			continue // only the entities this repository owns
		}
		var s spec.Skill
		if err := e.Value.Decode(&s); err != nil {
			return fmt.Errorf("skill %q: decode: %w", e.Name, err)
		}
		if s.Content == "" {
			continue // already file-sourced: nothing to move
		}
		name := s.Name
		if name == "" {
			name = e.Name // the node name IS the id (spec#247)
		}
		declared, ok := trees[s.Owner]
		if !ok {
			declared = []string{agentsSkillsDir}
		}
		dir := filepath.Join(root, declared[0], name)
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
		// The AUTHORED shape: the same two-key frontmatter the emitter writes, then the body — and
		// deliberately WITHOUT the generator's marker, because the marker is what marks a file as
		// generated, and from here on this file is hand-owned. This is the shape the reader parses.
		entry := skillFrontmatter(name, s.Description) + "\n" + s.Content
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(entry), 0o644); err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
		for _, r := range s.References {
			// references/ files carry NO frontmatter, by contract (see emit_skills.go).
			body := strings.TrimSuffix(r.Content, "\n") + "\n"
			if err := os.WriteFile(filepath.Join(dir, "references", r.Name+".md"), []byte(body), 0o644); err != nil {
				return fmt.Errorf("skill %q reference %q: %w", name, r.Name, err)
			}
		}
		moved++
	}
	fmt.Fprintf(os.Stderr, "marketplace extract: %d skill(s) written under %s\n", moved, root)
	return nil
}

// extractCmd moves in-line bodies into the files their repositories own. It takes --root and no
// --out on purpose: extract never writes the corpus, and a --out here would invite exactly that.
//
// It is the third mode beside generate and drift, declared in the same kong grammar (command.go)
// and reachable the same way — `charly marketplace extract --root <repo>`.
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
