package marketplace

import (
	"fmt"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
)

// readkinds.go — the discovery + decode of the `skill:` / `hook:` / `marketplace:` kind entities
// across the repo, via the shared sdk/candywalk kit (R3 — the SAME discovery plugin-docs uses).
// Each entity's authored value is decoded into its GENERATED spec type (spec.Skill / spec.Hook /
// spec.Marketplace — SDD, no hand-transcribed wire types); the load path already validated every
// body against the kind's served schema, so a decode here is expected to succeed.

// kindSet is everything the generator reads: the skill/hook entities (keyed by node name) and
// the single marketplace entity.
type kindSet struct {
	Skills      map[string]spec.Skill // node name → skill (node names are document-unique; skills are deduped by body name in the model)
	Hooks       map[string]spec.Hook  // node name → hook
	Marketplace *spec.Marketplace
}

// readKinds walks the repo and decodes every harness-kind entity.
//
// REMOTE-REF-AWARE (the candy de-submodule cutover Phase 3): the walk is CollectEntitiesRemote,
// so every @github.com/opencharly/... ref in the walked files' require:/candy: lists is resolved
// through spec/refs.DownloadRepo and the fetched repos' skill:/hook:/marketplace: entities are
// unioned in, transitively. After Phase 4 deletes the in-repo candy/ dirs this is what keeps
// every moved candy's skill/hook entity emitting.
func readKinds(root string) (*kindSet, error) {
	roots, err := candywalk.RepoRoots(root)
	if err != nil {
		return nil, fmt.Errorf("discover repo roots: %w", err)
	}
	ents, err := candywalk.CollectEntitiesRemote(roots, refs.DownloadRepo)
	if err != nil {
		return nil, fmt.Errorf("collect entities: %w", err)
	}
	// One NAME can arrive from several sources in a single closure — most often the same
	// repo resolved at two tags, because the refs list pins it directly while something in
	// the transitive closure pins an older tag. Assigning into the map below is
	// last-write-wins over an unordered collection, so resolve the winner explicitly first.
	// See resolve.go for the rule and the measurements behind it.
	ents = dedupeEntities(ents)
	ks := &kindSet{Skills: map[string]spec.Skill{}, Hooks: map[string]spec.Hook{}}
	for _, e := range ents {
		switch e.Kind {
		case "skill":
			var s spec.Skill
			if err := e.Value.Decode(&s); err != nil {
				return nil, fmt.Errorf("skill %q: decode: %w", e.Name, err)
			}
			ks.Skills[e.Name] = s
		case "hook":
			var h spec.Hook
			if err := e.Value.Decode(&h); err != nil {
				return nil, fmt.Errorf("hook %q: decode: %w", e.Name, err)
			}
			ks.Hooks[e.Name] = h
		case "marketplace":
			var m spec.Marketplace
			if err := e.Value.Decode(&m); err != nil {
				return nil, fmt.Errorf("marketplace %q: decode: %w", e.Name, err)
			}
			if ks.Marketplace != nil {
				return nil, fmt.Errorf("more than one marketplace entity (node %q + %q) — the repo declares exactly one", ks.Marketplace.Name, m.Name)
			}
			ks.Marketplace = &m
		}
	}
	if ks.Marketplace == nil {
		return nil, fmt.Errorf("no marketplace entity found — declare a `marketplace:` node in a candy-dir charly.yml (e.g. candy/charly-marketplace/charly.yml)")
	}
	return ks, nil
}
