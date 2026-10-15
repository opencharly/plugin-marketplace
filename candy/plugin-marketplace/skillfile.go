package marketplace

import (
	"fmt"
	"path/filepath"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
)

// skillfile.go — the `.agents/skills` FILE source, GENERATOR side (the relocation's leg L4).
//
// A skill's body may live in the repository that owns it, at <tree>/<name>/SKILL.md plus
// references/<stem>.md, with the `skill:` node left carrying only the ROUTING metadata the
// consumers read. The schema half landed in spec#247 (optional `name`/`description`/`content`,
// plus `#Candy.skills`); the reader landed in sdk (`candywalk.ReadSkillFile`/`ReadSkillTree`);
// this file is the consumer that makes the corpus read from those files.
//
// WHERE THE READ GOES, and why it is not a detail (F13): readkinds.go runs `dedupeEntities`
// BEFORE it decodes anything, so by the time a skill reaches the model, the winning entity — and
// its SourceRoot, the tree that will be read — are already decided. The read therefore belongs
// inside that decode loop, per entity, and nothing in this file may re-decide it. Read it anywhere
// earlier, or from the generator's own root, and 293 of the 355 identities can publish a body
// `preferEntity` did not choose.

// agentsSkillsDir is the Agent-Skills standard location: what a candy's `skills:` list means when
// it declares none.
const agentsSkillsDir = ".agents/skills"

// skillTrees maps each candy's node name to the trees its skills are read from: the candy's
// `skills:` list, or the Agent-Skills standard when it declares none.
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
			trees[e.Name] = []string{agentsSkillsDir}
			continue
		}
		// Copy: the decoded slice is the caller's only witness of the declaration, and the
		// returned map outlives this loop.
		trees[e.Name] = append([]string(nil), c.Skills...)
	}
	return trees, nil
}

// fillSkillFromFile completes one WINNING skill entity from the file its repository owns.
//
// node is the entity's node name; sourceRoot the tree of the entity dedupeEntities chose (F13:
// nothing here may re-decide which tree is read); root is the generator's root, used only when the
// entity is LOCAL and therefore carries no SourceRoot; trees are the trees its owning candy
// declares.
//
// Three rules are LOUD on purpose, because silence here loses published pages:
//   - the file wins when it exists — that is what makes a per-repo move atomic, since a repo with
//     files has moved and a repo with inline bodies has not;
//   - an inline body that DIFFERS from the file is an error naming both: two sources of truth for
//     one published page is the ambiguity this relocation exists to remove;
//   - neither an inline body nor a file is an error — the one failure that would quietly drop a
//     published skill from the corpus.
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
