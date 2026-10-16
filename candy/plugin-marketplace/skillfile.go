package marketplace

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
)

// skillfile.go — the authored FILE sources, GENERATOR side (the relocation's leg L4).
//
// A skill's body may live in the repository that owns it, at <tree>/<name>/SKILL.md plus
// references/<stem>.md, with the `skill:` node left carrying only the ROUTING metadata the
// consumers read. An `agent`-type node is the same deal at a different path —
// .agents/agents/<name>.md, ONE file, no references/ half — because a sub-agent definition is not a
// skill. The schema half landed in spec#247 (optional `name`/`description`/`content`, plus
// `#Candy.skills`); the readers live in sdk (`candywalk.ReadSkillFile`/`ReadSkillTree`/
// `ReadAgentFile`); this file is the consumer that makes the corpus read from those files.
//
// WHERE THE READ GOES, and why it is not a detail (F13): readkinds.go runs `dedupeEntities`
// BEFORE it decodes anything, so by the time a skill reaches the model, the winning entity — and
// its SourceRoot, the tree that will be read — are already decided. The read therefore belongs
// inside that decode loop, per entity, and nothing in this file may re-decide it. Read it anywhere
// earlier, or from the generator's own root, and 293 of the 355 identities can publish a body
// `preferEntity` did not choose.
//
// The tree constants are READ FROM THE KIT, never mirrored here: one path fact, one definition
// (`candywalk.AgentsSkillsDir` / `candywalk.AgentsAgentsDir`).

// skillTrees maps each candy's node name to the trees its skills are read from: the candy's
// `skills:` list, or the Agent-Skills standard when it declares none.
//
// A candy's `skills:` list is a list of SKILL trees and governs no `agent`-type node — an agent's
// path is fixed (<candywalk.AgentsAgentsDir>), which is what makes a mixed repo legal.
//
// The declaration is visible to the generator because `candywalk.ReadEntityFile` emits one Entity
// per kind discriminator — `candy` included — so a candy arrives with its `skills:` list attached;
// readkinds.go's switch simply ignores those entities today. That is what makes spec#247's
// `#Candy.skills` a real contract rather than a field nothing consumes.
func skillTrees(ents []candywalk.Entity) (map[string][]string, error) {
	trees := make(map[string][]string, len(ents))
	for _, e := range ents {
		if e.Kind != "candy" {
			continue
		}
		var c struct {
			Skills []string `yaml:"skills"`
		}
		if err := e.Value.Decode(&c); err != nil {
			return nil, fmt.Errorf("candy %q: decode: %w", e.Name, err)
		}
		if len(c.Skills) == 0 {
			trees[e.Name] = []string{candywalk.AgentsSkillsDir}
			continue
		}
		// Copy: the decoded slice is the caller's only witness of the declaration, and the
		// returned map outlives this loop.
		trees[e.Name] = append([]string(nil), c.Skills...)
	}
	return trees, nil
}

// fillSkillFromFile completes one WINNING `skill:`-kind entity from the file its repository owns.
// The KIND carries two artifact classes, selected by the node's own `type:`, and this function is
// the single funnel both of them enter the model through:
//
//   - `skill` (the default) — <tree>/<name>/SKILL.md plus references/<stem>.md, where <tree> is the
//     owning candy's `skills:` list;
//   - `agent` — .agents/agents/<name>.md, ONE file, and `skills:` does not govern it (D8): a
//     sub-agent definition is not a skill.
//
// node is the entity's node name; sourceRoot the tree of the entity dedupeEntities chose (F13:
// nothing here may re-decide which tree is read); root is the generator's root, used only when the
// entity is LOCAL and therefore carries no SourceRoot; trees are the SKILL trees its owning candy
// declares.
//
// Three rules are LOUD on purpose, because silence here loses published pages — and they are the
// SAME three for both classes, since a sub-agent that vanishes is the same failure as a skill that
// vanishes:
//   - the file wins when it exists — that is what makes a per-repo move atomic, since a repo with
//     files has moved and a repo with inline bodies has not;
//   - an inline body that DIFFERS from the file is an error naming both: two sources of truth for
//     one published page is the ambiguity this relocation exists to remove;
//   - neither an inline body nor a file is an error — the one failure that would quietly drop a
//     published skill or sub-agent from the corpus.
func fillSkillFromFile(s *spec.Skill, node, sourceRoot, root string, trees []string) error {
	name := s.Name
	if name == "" {
		// The node name IS the id when the node declares none (spec#247's default).
		name = node
	}
	base := sourceRoot
	if base == "" {
		base = root
	}
	if s.Type == "agent" {
		return fillAgentFromFile(s, name, base)
	}
	var sf *candywalk.SkillFile
	for _, t := range trees {
		found, err := candywalk.ReadSkillFile(filepath.Join(base, t), name)
		if err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
		if found != nil {
			// The first declared tree that holds it wins.
			sf = found
			break
		}
	}
	if sf == nil {
		if s.Content == "" {
			return fmt.Errorf("skill %q: no body — neither an inline `content:` nor a SKILL.md under %v", name, trees)
		}
		// The transitional in-line form, still valid until the relocation's own final leg.
		return nil
	}
	if s.Content != "" && s.Content != sf.Content {
		return fmt.Errorf("skill %q: the node's inline `content:` and its file disagree — delete the inline body and keep the file", name)
	}
	if s.Description != "" && s.Description != sf.Description {
		return fmt.Errorf("skill %q: the node's inline `description:` and its file's frontmatter disagree", name)
	}
	s.Description = sf.Description
	s.Content = sf.Content
	if len(s.References) > 0 && len(sf.References) > 0 {
		return fmt.Errorf("skill %q: references are declared BOTH inline and under references/ in the file — keep one", name)
	}
	if len(sf.References) > 0 {
		refs := make([]spec.SkillReference, 0, len(sf.References))
		for _, r := range sf.References {
			refs = append(refs, spec.SkillReference{Name: r.Stem, Content: r.Content})
		}
		s.References = refs
	}
	return nil
}

// fillAgentFromFile completes one WINNING `agent`-type entity from the file its repository owns:
// .agents/agents/<name>.md, ONE file directly under the agents tree.
//
// It is a separate branch rather than a path parameter to the skill read, because the two shapes
// differ in more than their path: an agent has no entry DIRECTORY (so there is no `trees` lookup to
// walk) and no references/ half (the emitter publishes none, so there is nothing to reconcile
// there). `skills:` deliberately plays no part — a list of SKILL trees does not govern a sub-agent
// definition, which is what makes a mixed repo legal.
//
// The three loud rules are the skill branch's, unchanged, and for the same reason: an agent body
// that goes missing is a published sub-agent silently subtracted from the corpus.
func fillAgentFromFile(s *spec.Skill, name, base string) error {
	af, err := candywalk.ReadAgentFile(filepath.Join(base, candywalk.AgentsAgentsDir), name)
	if err != nil {
		return fmt.Errorf("agent %q: %w", name, err)
	}
	if af == nil {
		if s.Content == "" {
			return fmt.Errorf("agent %q: no body — neither an inline `content:` nor %s/%s.md", name, candywalk.AgentsAgentsDir, name)
		}
		// The transitional in-line form, still valid until the relocation's own final leg.
		return nil
	}
	if s.Content != "" && s.Content != af.Content {
		return fmt.Errorf("agent %q: the node's inline `content:` and its file disagree — delete the inline body and keep the file", name)
	}
	if s.Description != "" && s.Description != af.Description {
		return fmt.Errorf("agent %q: the node's inline `description:` and its file's frontmatter disagree", name)
	}
	s.Description = af.Description
	s.Content = af.Content
	return nil
}

// canonicalizeReferences puts a skill's references in the ONE order both source forms can agree
// on, and it is called on every skill as it enters the model (readkinds.go), AFTER the file-vs-entity
// reconciliation — so no source form can escape it.
//
// WHY IT IS NEEDED. An ORDER-SENSITIVE artifact is built from this slice: emitOpencodeIndex writes
// each skill's `files` list in `s.References` order. The two source forms disagree about that order
// through no fault of their own:
//
//   - the FILE form has no document order to preserve — a directory listing is unordered, so
//     candywalk.ReadSkillFile sorts by stem;
//   - the ENTITY form arrives in the manifest's document order.
//
// So a skill that merely moved house reordered its own entry in `.well-known/skills/index.json`,
// which means the corpus could not be byte-identical across a move — the invariant this whole
// relocation is proven by (opencharly/plugin-marketplace#28). The file form's sort cannot be the
// answer: document order is not recoverable from a directory listing. Sorting here is, and it makes
// the entity form agree with the file form instead of the other way round.
//
// NOTHING DEPENDS ON THE OTHER ORDER. Each reference is emitted as its own `references/<name>.md`
// file, and the per-skill `files` array in index.json is a manifest of those files, fetched one by
// one by the consumer. The order is presentation, which is exactly why it must be canonical rather
// than incidental.
func canonicalizeReferences(refs []spec.SkillReference) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
}
