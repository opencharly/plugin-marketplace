package marketplace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
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
// The LAW is: the tree being generated from, and the refs that tree names directly, are the
// authority — the refs list is the ONE generation input.
//
// The rule that IMPLEMENTS it has two steps, and the order matters:
//
//  1. an entity from one of the generator's OWN roots beats any fetched one;
//  2. among fetched trees, the ref the project's own manifests name DIRECTLY beats a twin that
//     merely arrived through the closure;
//  3. only then does the highest source tag win. That is not an arbitrary tiebreak: in every
//     conflict observed on the live closure both candidates are tags, the refs list pins the
//     NEWER one directly, and the older arrives transitively — so "highest tag" and "the refs
//     list's explicit pin" agreed, and it matches how Go's MVS resolves a module required at two
//     versions. Ties fall back to a stable comparison so the result is never order-dependent.
//
// STEP 2 EXISTS BECAUSE STEP 3 IS A PROXY, and a proxy has a blind spot it cannot see
// (opencharly/plugin-marketplace#39). `tagRank` reads the CalVer out of a cache dir; a repo
// pinned to a BRANCH or a COMMIT has no CalVer to read, and "unparseable never displaces a
// parseable one" then handed the identity to whichever transitive TAG still carried it. Measured
// on a two-ref corpus pinning `layer-openclaw:feat/skill-bodies-to-repo-root` (bodies at the repo
// root) alongside `pod-charly-hooks`: the closure also reaches the pre-rename
// `layer-charly-openclaw@v2026.271.2234`, whose `openclaw-layer-skill` node still carries the body
// INLINE. The tag won, `fillSkillFromFile` found no file under that tree and fell back to the
// inline body, so `marketplace generate` exited 0 and published an 8-month-old document — a body
// neither the branch nor the newest tag contains. Reading the DIRECT pin as the authority is what
// makes the published body the pinned tree's body.

// sourceTagRe pulls the CalVer tag out of a fetched repo's cache dir, which candywalk reports
// as Entity.SourceRoot — e.g. ".../opencharly/pod-hermes@v2026.239.1605".
var sourceTagRe = regexp.MustCompile(`@v(\d+)\.(\d+)\.(\d+)$`)

// pinAuthority is WHERE THE AUTHORITY TO PUBLISH ONE IDENTITY LIVES, carried explicitly instead
// of inferred from a ref's shape. It is the two facts readKinds already has when it walks:
// the roots the generator was pointed at, and the refs those roots' own candies declare.
//
// `direct` is keyed exactly as candywalk keys a fetched root's Entity.Namespace — "<repo path>
// :<version>" — so membership is a map lookup on the entity itself, not a re-parse of the path.
type pinAuthority struct {
	localRoots map[string]bool // cleaned dirs of the roots the generator was pointed at
	direct     map[string]bool // "<repo>:<version>" refs THOSE roots declare
}

// directPinAuthority builds the authority from the walk's own output. A LOCAL entity is one whose
// SourceRoot IS one of the roots passed in — that is what candywalk sets for a local root (it
// never leaves SourceRoot empty, so an empty-string sentinel would identify nothing), and it is
// why this is a set membership test rather than a path-shape guess.
//
// Only the generator's OWN roots declare direct refs. A distro repo's generated refs list is
// itself a transitively-reached manifest: treating its pins as authority would let a stale
// sibling pin (the very kind that shadows the corpus here) promote itself.
func directPinAuthority(roots []candywalk.Root, ents []candywalk.Entity) (pinAuthority, error) {
	a := pinAuthority{
		localRoots: make(map[string]bool, len(roots)),
		direct:     make(map[string]bool),
	}
	for _, r := range roots {
		a.localRoots[filepath.Clean(r.Dir)] = true
	}
	for _, e := range ents {
		if e.Kind != "candy" || !a.isLocal(e) {
			continue
		}
		var c struct {
			Candy   []string `yaml:"candy"`
			Require []string `yaml:"require"`
		}
		if err := e.Value.Decode(&c); err != nil {
			return pinAuthority{}, fmt.Errorf("candy %q: decode: %w", e.Name, err)
		}
		// BOTH lists: a project declares its corpus sources on `candy:` today, but a `require:`
		// is the same claim ("this repo is part of my closure") and candywalk collects refs from
		// either. Reading one list would leave half the declared closure unranked.
		for _, ref := range append(c.Candy, c.Require...) {
			pr := spec.ParseRemoteRef(ref)
			if pr == nil || pr.RepoPath == "" || pr.Version == "" {
				continue
			}
			a.direct[pr.RepoPath+":"+pr.Version] = true
		}
	}
	return a, nil
}

// isLocal reports whether the entity was read from one of the generator's own roots.
func (a pinAuthority) isLocal(e candywalk.Entity) bool {
	return a.localRoots[filepath.Clean(e.SourceRoot)]
}

// tagRank returns the CalVer of an entity's source as a comparable triple, and whether it parsed.
//
// It ranks TAGS only. A branch or a commit pin has no CalVer to read and returns ok=false — that
// is the blind spot `pinAuthority` closes one step earlier in preferEntity, so this function
// never has to pretend to order a ref it cannot read.
func tagRank(sourceRoot string) (rank [3]int, ok bool) {
	m := sourceTagRe.FindStringSubmatch(strings.TrimRight(sourceRoot, "/"))
	if m == nil {
		return [3]int{}, false
	}
	for i := range 3 {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		rank[i] = n
	}
	return rank, true
}

// preferEntity reports whether candidate should replace incumbent for the same entity name.
//
// Order: a local entity beats a fetched one; among fetched ones the ref the project declares
// DIRECTLY beats a twin that only arrived through the closure; otherwise the higher CalVer tag
// wins, an unparseable tag never displaces a parseable one, and a genuine tie is broken on
// SourceRoot so the outcome cannot depend on collection order.
func preferEntity(incumbent, candidate candywalk.Entity, auth pinAuthority) bool {
	// 1. The tree being generated from is the authority over anything downloaded.
	switch iLocal, cLocal := auth.isLocal(incumbent), auth.isLocal(candidate); {
	case iLocal != cLocal:
		return cLocal
	}
	// 2. The ref the project names directly is the authority over a transitive twin. Checked
	// BEFORE the rank, because the rank cannot read a branch or a commit — and a stale tag
	// winning that comparison is exactly how a moved body's page was published from its
	// pre-move copy (opencharly/plugin-marketplace#39).
	switch iDirect, cDirect := auth.direct[incumbent.Namespace], auth.direct[candidate.Namespace]; {
	case iDirect != cDirect:
		return cDirect
	}
	// 3. Two sources the project is equally entitled to: the higher tag wins.
	iRank, iOK := tagRank(incumbent.SourceRoot)
	cRank, cOK := tagRank(candidate.SourceRoot)
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
// preferEntity. `auth` carries the authority the rank cannot read: the generator's own roots and
// the refs they declare.
func dedupeEntities(ents []candywalk.Entity, auth pinAuthority) []candywalk.Entity {
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
		if preferEntity(cur, e, auth) {
			best[k] = e
		}
	}
	out := make([]candywalk.Entity, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}
