package marketplace

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
)

// readkinds.go — the discovery + decode of the `skill:` / `hook:` / `marketplace:` kind entities
// across the repo, via the shared sdk/candywalk kit (R3 — the SAME discovery plugin-docs uses).
// Each entity's authored value is decoded into its GENERATED spec type (spec.Skill / spec.Hook /
// spec.Marketplace — SDD, no hand-transcribed wire types); the load path already validated every
// body against the kind's served schema, so a decode here is expected to succeed.

// kindSet is everything the generator reads: the skill/hook entities and the single marketplace
// entity.
type kindSet struct {
	// Skills is a SLICE, not a map keyed by node name. Once the family joined the dedupe key
	// (resolve.go), two families may legitimately publish a skill carrying the same node name —
	// the corpus is per-family, and a skill is addressed `/charly-<family>:<skill>` — so a map
	// keyed by node name would silently drop one of them HERE, re-creating exactly the pruning
	// the family-aware key exists to stop. The model sorts each family's skills by name, so no
	// key is needed downstream.
	Skills      []spec.Skill
	Hooks       map[string]spec.Hook // node name → hook (hooks declare no family; node names are document-unique)
	Marketplace *spec.Marketplace
}

// corpusScope is the corpus generator's closure rule: the RETIREMENT exclusion. It is the
// consumer half of candywalk.CollectEntitiesRemoteScoped (opencharly/sdk#626); the walker keeps
// its full-closure contract for every other caller, because this rule is the CORPUS's.
//
// THE RULE. The refs list names the corpus's SOURCES; `retired.cue` at the corpus root names the
// repos the corpus has RETIRED, and a retired repo is never resolved — not from the refs list,
// not from a fetched tree's composition list, not transitively. Everything else keeps the full
// closure, exactly as before this change.
//
// WHY A RETIREMENT LIST, AND NOT A STRUCTURAL RULE — MEASURED. After the refs list dropped every
// pod-immich-ml pin, `charly marketplace generate` STILL published `immich/skills/immich-ml`.
// An instrumented walk recorded the mechanism: `pod-immich-ml:v2026.239.1604` was declared by
// `distro-fedora@v2026.256.1134/{box/immich-ml,candy/charly-marketplace}/charly.yml` and
// `:v2026.274.0811` by `distro-cachyos@v2026.319.0820` and `@v2026.319.1138`, both files each —
// old trees reached through STALE SUBPATH PINS in other sources
// (`pod-dsh@v2026.292.2042/charly.yml:18-20` importing `distro-fedora/box/fedora/charly.yml:
// v2026.256.1134`; `layer-agentteams@v2026.319.1239/box/cachyos-base/charly.yml:39` composing
// `distro-cachyos/candy/cachyos-base-check:v2026.319.0820`), while the refs list pins those two
// repos at v2026.325.1900 / v2026.327.0930. Honouring a ref's SUBPATH is the separate residue,
// opencharly/plugin-marketplace#64.
//
// Two structural rules were implemented and MEASURED against the full corpus, and both were
// rejected because they cut PUBLISHED content: "only corpus-owning files contribute refs" dropped
// 11 live skills (the NEWEST distro boxes are the legitimate carriers of pod repos' skills), and
// "an older tree stops extending the closure" dropped 6 (openclaw-desktop, openclaw-full,
// openclaw-full-layer, openclaw-full-ml-layer, fedora-coder, fedora-test — their only remaining
// carriers are old trees). A retired repo and a live repo reachable only through an old tree have
// the SAME SHAPE, so no rule that walks the graph can separate them. Retirement is DATA, and the
// corpus records it in one place.
//
// A retired ref declared by the generator's OWN roots is an authoring contradiction — the refs
// list must not pin what the corpus has retired — so the ref is dropped AND the conflict is
// reported, and readKinds fails the run instead of hiding it.
type corpusScope struct {
	retired    map[string]bool // repo paths (ParseRemoteRef's RepoPath form)
	own        []string        // the generator's own roots, each with a trailing separator
	ownRetired []string        // retired refs the generator's own roots declare
}

func newCorpusScope(roots []candywalk.Root, retired map[string]bool) *corpusScope {
	own := make([]string, 0, len(roots))
	for _, r := range roots {
		own = append(own, filepath.Clean(r.Dir)+string(filepath.Separator))
	}
	return &corpusScope{retired: retired, own: own}
}

// refs is the candywalk.RefScope this scope contributes: drop every retired ref, from any file.
func (s *corpusScope) refs(path, _ string, _ []candywalk.Entity, all []string) []string {
	own := s.isOwn(filepath.Clean(path))
	kept := make([]string, 0, len(all))
	for _, ref := range all {
		pr := spec.ParseRemoteRef(ref)
		if pr == nil || pr.RepoPath == "" || !s.retired[pr.RepoPath] {
			kept = append(kept, ref)
			continue
		}
		if own {
			s.ownRetired = append(s.ownRetired, ref)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// isOwn reports whether a walked file lives under one of the generator's own roots. The trailing
// separator keeps a sibling directory that merely starts with the root's name out of it.
func (s *corpusScope) isOwn(p string) bool {
	for _, dir := range s.own {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}
	return false
}

// readKinds walks the repo and decodes every harness-kind entity.
//
// REMOTE-REF-AWARE (the candy de-submodule cutover Phase 3): the walk is
// CollectEntitiesRemoteScoped, so every @github.com/opencharly/... ref a walked file declares is
// resolved and the fetched repos' skill:/hook:/marketplace: entities are unioned in,
// transitively — subject to the corpus's RETIREMENT list (corpusScope), which drops a ref to a
// retired repo wherever it is declared. After Phase 4 deletes the in-repo candy/ dirs this is
// what keeps every moved candy's skill/hook entity emitting.
//
// The resolver is refs.DownloadRepoFiles, NOT refs.DownloadRepo: a corpus source is read for its
// OWN TREE only — charly.yml and the .agents/ trees — so the fetch certifies exactly that.
// refs.DownloadRepo populates every submodule the fetched repo declares and fails the whole fetch
// when one is unreachable: right for a `--repo` cache, fatal here, because it made
// opencharly/opencharly (412 declared gitlinks) unusable as a corpus source at all (spec#279)
// after already costing a 431-gitlink population the walk reads none of (spec#267). The two
// fetches share one signature, so this stays a plain seam — no SDK change, and the `--repo` cache
// keeps populating.
func readKinds(root string) (*kindSet, error) {
	roots, err := candywalk.RepoRoots(root)
	if err != nil {
		return nil, fmt.Errorf("discover repo roots: %w", err)
	}
	retired, err := loadRetired(root)
	if err != nil {
		return nil, err
	}
	scope := newCorpusScope(roots, retired)
	ents, err := candywalk.CollectEntitiesRemoteScoped(roots, refs.DownloadRepoFiles, scope.refs)
	if err != nil {
		return nil, fmt.Errorf("collect entities: %w", err)
	}
	if len(scope.ownRetired) > 0 {
		return nil, fmt.Errorf("the refs list pins retired repo(s): %s — remove the pin, or drop the retirement from %s",
			strings.Join(scope.ownRetired, ", "), retiredFileName)
	}
	// One NAME can arrive from several sources in a single closure — most often the same
	// repo resolved at two tags, because the refs list pins it directly while something in
	// the transitive closure pins an older tag. Assigning into the map below is
	// last-write-wins over an unordered collection, so resolve the winner explicitly first.
	// See resolve.go for the rule and the measurements behind it. The AUTHORITY the winner is
	// chosen by — the generator's own roots, and the refs those roots declare — is derived from
	// the RAW collection: it is the dedupe's input, so it cannot be read off its output.
	auth, err := directPinAuthority(roots, ents)
	if err != nil {
		return nil, err
	}
	ents = dedupeEntities(ents, auth)
	// The SKILL trees each skill's files are read from, per owning candy (spec#247's `skills:`,
	// default `.agents/skills`). Built from the SAME deduped slice, so a file read can never disagree
	// with the entity the dedupe chose — see skillfile.go. An `agent`-type node is NOT in this map's
	// scope: a sub-agent definition's path is fixed, and `skills:` — a list of SKILL trees — does not
	// govern it (D8).
	trees, err := skillTrees(ents)
	if err != nil {
		return nil, err
	}
	ks := &kindSet{Hooks: map[string]spec.Hook{}}
	for _, e := range ents {
		switch e.Kind {
		case "skill":
			var s spec.Skill
			if err := e.Value.Decode(&s); err != nil {
				return nil, fmt.Errorf("skill %q: decode: %w", e.Name, err)
			}
			declared, ok := trees[s.Owner]
			if !ok {
				declared = []string{candywalk.AgentsSkillsDir}
			}
			if err := fillSkillFromFile(&s, e.Name, e.SourceRoot, root, declared); err != nil {
				return nil, err
			}
			// ONE canonical reference order, applied AFTER both source forms have been settled —
			// this is the single funnel every skill enters the model through, so no source form can
			// escape it (see canonicalizeReferences in skillfile.go). An `agent`-type node carries no
			// references (the emitter publishes none), so this is a no-op for it, not a second rule.
			canonicalizeReferences(s.References)
			ks.Skills = append(ks.Skills, s)
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
