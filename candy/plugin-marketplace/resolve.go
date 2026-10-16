package marketplace

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/opencharly/sdk/candywalk"
)

// resolve.go — deterministic resolution when one entity NAME arrives from more than one
// source in a single closure.
//
// It happens routinely: the refs list pins `pod-hermes:v2026.239.1605` directly, while some
// other repo in the transitive closure still pins `pod-hermes:v2026.237.936`. Both are
// fetched, both yield a `hermes-skill` entity, and readKinds used to do
//
//	ks.Skills[e.Name] = s
//
// which is last-write-wins over an unordered collection: whichever arrived last won, with no
// warning and no gate. Measured on the live closure, 82 skill names arrive more than once.
// For 79 the two bodies are byte-identical, so the choice never mattered; for 3
// (hermes-skill, selkies-core-skill, xfce4-terminal-skill) the bodies DIFFER, and the
// published corpus therefore depended on iteration order.
//
// The rule here is the highest source tag wins. That is not an arbitrary tiebreak: in every
// observed conflict the refs list pins the NEWER tag directly and the older one arrives
// transitively, so "highest tag" is the same thing as "the refs list's explicit pin", and it
// matches how Go's MVS resolves a module required at two versions. Ties and unparseable tags
// fall back to a stable comparison so the result is never order-dependent.

// sourceTagRe pulls the CalVer tag out of a fetched repo's cache dir, which candywalk reports
// as Entity.SourceRoot — e.g. ".../opencharly/pod-hermes@v2026.239.1605". A local (non-fetched)
// root has an empty SourceRoot.
var sourceTagRe = regexp.MustCompile(`@v(\d+)\.(\d+)\.(\d+)$`)

// tagRank returns the CalVer of an entity's source as a comparable triple, and whether it
// parsed. A LOCAL entity (empty SourceRoot) outranks every fetched one: the tree being
// generated from is the authority over anything downloaded.
func tagRank(sourceRoot string) (rank [3]int, local bool, ok bool) {
	if sourceRoot == "" {
		return [3]int{}, true, true
	}
	m := sourceTagRe.FindStringSubmatch(strings.TrimRight(sourceRoot, "/"))
	if m == nil {
		return [3]int{}, false, false
	}
	for i := range 3 {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false, false
		}
		rank[i] = n
	}
	return rank, false, true
}

// preferEntity reports whether candidate should replace incumbent for the same entity name.
//
// Order: a local entity beats a fetched one; otherwise the higher CalVer tag wins; an
// unparseable tag never displaces a parseable one; and a genuine tie is broken on SourceRoot
// so the outcome cannot depend on collection order.
func preferEntity(incumbent, candidate candywalk.Entity) bool {
	iRank, iLocal, iOK := tagRank(incumbent.SourceRoot)
	cRank, cLocal, cOK := tagRank(candidate.SourceRoot)

	if iLocal != cLocal {
		return cLocal // local wins
	}
	if iOK != cOK {
		return cOK // parseable wins
	}
	if iOK && cOK && iRank != cRank {
		for i := range 3 {
			if cRank[i] != iRank[i] {
				return cRank[i] > iRank[i]
			}
		}
	}
	// Same rank (or neither parseable): stable, order-independent tiebreak.
	return candidate.SourceRoot > incumbent.SourceRoot
}

// entityFamily reports the FAMILY an entity's body declares, for the kinds that have one.
//
// The family is part of a skill's identity, not metadata about it: the corpus is per-family
// (`<family>/skills/<name>/SKILL.md`) and a skill is addressed `/charly-<family>:<skill>`, so two
// families declaring one node name are two different skills and both must be published. Keying
// the dedupe on (Kind, Name) alone collapsed them and let tag order decide the survivor —
// measured on the live closure, `layer-crabbox` (tools) and `plugin-crabbox` (check) both declare
// a `crabbox` skill, and a refresh that advanced both tags swapped the published skill and
// dropped the losing entity's dispatcher row with it (opencharly/plugin-marketplace#20, observed
// on opencharly/marketplace#490).
//
// Only `skill` declares one (spec#247/spec.Skill): a `hook` has no family field, and the walker's
// entity is deliberately generic — the family is read here rather than added to candywalk.Entity,
// which is a shared kit and must not learn a kind word (the boundary law, plugin AGENTS.md).
//
// A decode failure returns "" (no family) rather than an error: the same body is decoded in FULL
// by readKinds immediately after, which hard-errors on a malformed skill, so a bad value cannot
// pass silently — it only cannot pick the dedupe winner, and the run fails loudly a moment later.
func entityFamily(e candywalk.Entity) string {
	if e.Kind != "skill" || e.Value.Kind == 0 {
		return ""
	}
	var v struct {
		Family string `yaml:"family"`
	}
	if err := e.Value.Decode(&v); err != nil {
		return ""
	}
	return v.Family
}

// dedupeEntities collapses entities that share a (Kind, FAMILY, Name) to one winner each, applying
// preferEntity. The returned slice is in a stable order so downstream emission cannot vary
// between runs.
//
// The family is in the key because it is what makes the name unambiguous: with (Kind, Name) alone,
// two families declaring one name collided and the higher tag won, which silently removed a
// published skill (and its dispatcher row) from the corpus. Two entities that share all three are
// the SAME skill arriving from two sources — the case this file exists for — and still resolve by
// preferEntity.
func dedupeEntities(ents []candywalk.Entity) []candywalk.Entity {
	type key struct{ kind, family, name string }
	best := map[key]candywalk.Entity{}
	order := []key{}
	for _, e := range ents {
		k := key{e.Kind, entityFamily(e), e.Name}
		cur, seen := best[k]
		if !seen {
			best[k] = e
			order = append(order, k)
			continue
		}
		if preferEntity(cur, e) {
			best[k] = e
		}
	}
	out := make([]candywalk.Entity, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}
