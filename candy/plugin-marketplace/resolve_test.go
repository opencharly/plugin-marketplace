package marketplace

import (
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

const cacheBase = "/home/u/.cache/charly/repos/github.com/opencharly/"

func ent(name, sourceRoot string) candywalk.Entity {
	return candywalk.Entity{Name: name, Kind: "skill", SourceRoot: sourceRoot}
}

// authority builds the authority a real walk hands the dedupe: the generator's own roots, plus
// the refs those roots declare (candywalk keys a fetched entity's Namespace "<repo>:<version>").
func authority(localDir string, direct ...string) pinAuthority {
	a := pinAuthority{localRoots: map[string]bool{}, direct: map[string]bool{}}
	if localDir != "" {
		a.localRoots[filepath.Clean(localDir)] = true
	}
	for _, ref := range direct {
		pr := spec.ParseRemoteRef(ref)
		a.direct[pr.RepoPath+":"+pr.Version] = true
	}
	return a
}

// TestDedupeEntities_IsOrderIndependent is the assertion the whole file exists for.
//
// readKinds assigns `ks.Skills[e.Name] = s` over a collection with no defined order, so
// before dedupeEntities the winner among duplicates was whichever happened to arrive last.
// On the live closure 82 skill names arrive more than once and 3 of them have DIFFERING
// bodies, which means the published corpus varied with iteration order. Shuffling the input
// must not change the output.
func TestDedupeEntities_IsOrderIndependent(t *testing.T) {
	in := []candywalk.Entity{
		ent("hermes-skill", cacheBase+"pod-hermes@v2026.237.936"),
		ent("hermes-skill", cacheBase+"pod-hermes@v2026.239.1605"),
		ent("selkies-core-skill", cacheBase+"pod-selkies-core@v2026.240.2344"),
		ent("selkies-core-skill", cacheBase+"pod-selkies-core@v2026.237.808"),
		ent("xfce4-terminal-skill", cacheBase+"layer-xfce4-terminal@v2026.239.1558"),
		ent("xfce4-terminal-skill", cacheBase+"layer-xfce4-terminal@v2026.237.1057"),
		ent("lonely-skill", cacheBase+"layer-lonely@v2026.200.1"),
	}
	want := map[string]string{
		"hermes-skill":         cacheBase + "pod-hermes@v2026.239.1605",
		"selkies-core-skill":   cacheBase + "pod-selkies-core@v2026.240.2344",
		"xfce4-terminal-skill": cacheBase + "layer-xfce4-terminal@v2026.239.1558",
		"lonely-skill":         cacheBase + "layer-lonely@v2026.200.1",
	}

	rng := rand.New(rand.NewSource(1))
	for round := range 50 {
		shuffled := append([]candywalk.Entity(nil), in...)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		got := dedupeEntities(shuffled, pinAuthority{})
		if len(got) != len(want) {
			t.Fatalf("round %d: got %d entities, want %d", round, len(got), len(want))
		}
		for _, e := range got {
			if e.SourceRoot != want[e.Name] {
				t.Fatalf("round %d: %s resolved to %q, want %q — the winner still depends on input order",
					round, e.Name, e.SourceRoot, want[e.Name])
			}
		}
	}
}

// TestPreferEntity_HighestTagWins pins the rule itself: in every conflict observed on the
// live closure the refs list pins the NEWER tag directly and the older arrives transitively,
// so "highest tag" IS "the refs list's explicit pin".
func TestPreferEntity_HighestTagWins(t *testing.T) {
	older := ent("s", cacheBase+"pod-hermes@v2026.237.936")
	newer := ent("s", cacheBase+"pod-hermes@v2026.239.1605")

	if !preferEntity(older, newer, pinAuthority{}) {
		t.Error("the newer tag must displace the older one")
	}
	if preferEntity(newer, older, pinAuthority{}) {
		t.Error("the older tag must NOT displace the newer one")
	}
}

// TestPreferEntity_CalVerComparedNumerically guards the trap in comparing CalVer as text:
// "v2026.239.1605" sorts BELOW "v2026.239.936" lexicographically, because '1' < '9' — so a
// string comparison would pick the older tag on exactly the pair this fix exists to resolve.
func TestPreferEntity_CalVerComparedNumerically(t *testing.T) {
	lowNumericHighLex := ent("s", cacheBase+"r@v2026.239.936")
	highNumericLowLex := ent("s", cacheBase+"r@v2026.239.1605")

	if !preferEntity(lowNumericHighLex, highNumericLowLex, pinAuthority{}) {
		t.Error("1605 > 936 numerically; a lexicographic comparison would get this backwards")
	}
}

// TestPreferEntity_LocalOutranksFetched — the tree being generated from is the authority, even at
// a lower tag.
//
// The local side is identified by ROOT MEMBERSHIP, which is what the walker actually reports: it
// sets SourceRoot to the local root's dir, never to "". The former empty-string sentinel therefore
// matched nothing the walker produces, and this test could only pass by constructing an entity the
// walker cannot emit. The authority now names the root instead.
func TestPreferEntity_LocalOutranksFetched(t *testing.T) {
	const localRoot = "/work/generated-from"
	fetched := ent("s", cacheBase+"r@v2099.001.1")
	local := ent("s", localRoot)
	auth := authority(localRoot)

	if !preferEntity(fetched, local, auth) {
		t.Error("a local entity must outrank a fetched one, even at a lower tag")
	}
	if preferEntity(local, fetched, auth) {
		t.Error("a fetched entity must not displace a local one")
	}
}

// TestPreferEntity_TheProjectsOwnPinOutranksATransitiveTwin is the assertion
// opencharly/plugin-marketplace#39 exists for.
//
// The project pins `layer-openclaw` at a BRANCH (its bodies have moved to the repo root). The
// closure ALSO reaches the pre-rename `layer-charly-openclaw` at an old TAG, whose
// `openclaw-layer-skill` node still carries the body inline. `tagRank` cannot read a branch, and
// "unparseable never displaces a parseable one" then handed the identity to the old tag — so
// `marketplace generate` read the inline body of a tree nobody pinned and exited 0.
//
// The DIRECT ref is the authority. The transitive twin must not win, whatever tag it carries.
func TestPreferEntity_TheProjectsOwnPinOutranksATransitiveTwin(t *testing.T) {
	branch := candywalk.Entity{
		Name: "openclaw-layer-skill", Kind: "skill",
		Namespace:  "github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root",
		SourceRoot: cacheBase + "layer-openclaw@feat/skill-bodies-to-repo-root",
	}
	staleTag := candywalk.Entity{
		Name: "openclaw-layer-skill", Kind: "skill",
		Namespace:  "github.com/opencharly/layer-charly-openclaw:v2026.271.2234",
		SourceRoot: cacheBase + "layer-charly-openclaw@v2026.271.2234",
	}
	auth := authority("",
		"@github.com/opencharly/pod-charly-hooks:v2026.290.1114",
		"@github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root",
	)

	if !preferEntity(staleTag, branch, auth) {
		t.Fatal("the ref the project pins DIRECTLY must displace a transitive twin, even though the twin's tag is the only one that parses")
	}
	if preferEntity(branch, staleTag, auth) {
		t.Error("a transitive twin must never displace the project's own pin")
	}
}

// TestPreferEntity_TransitiveUnparseableStillLoses — the authority rule is about DIRECT pins, not
// about unparseable refs winning by shape. A branch that only arrived through the closure still
// loses to the tag the project pinned.
func TestPreferEntity_TransitiveUnparseableStillLoses(t *testing.T) {
	pinned := candywalk.Entity{
		Name: "s", Kind: "skill",
		Namespace:  "github.com/opencharly/r:v2026.100.1",
		SourceRoot: cacheBase + "r@v2026.100.1",
	}
	transitiveBranch := candywalk.Entity{
		Name: "s", Kind: "skill",
		Namespace:  "github.com/opencharly/r2:main",
		SourceRoot: cacheBase + "r2@main",
	}
	auth := authority("", "@github.com/opencharly/r:v2026.100.1")

	if preferEntity(pinned, transitiveBranch, auth) {
		t.Error("a transitive unparseable source must not displace the project's pinned one")
	}
	if !preferEntity(transitiveBranch, pinned, auth) {
		t.Error("the project's pinned source must displace a transitive unparseable one")
	}
}

// TestDirectPinAuthority_ReadsOnlyTheGeneratorsOwnRefs — the authority is what the GENERATOR's own
// roots declare, never what a transitively-reached manifest declares.
//
// This is the half that could silently undo the fix: every distro repo carries a GENERATED refs
// list of its own, and one of those still pins `layer-charly-openclaw` — the stale sibling above.
// If a fetched repo's declaration counted as authority, that pin would promote itself back to the
// top of the ladder and the wrong body would return.
func TestDirectPinAuthority_ReadsOnlyTheGeneratorsOwnRefs(t *testing.T) {
	const localRoot = "/work/generated-from"
	roots := []candywalk.Root{{Namespace: "", Dir: localRoot}}

	local := candyEntity("charly-marketplace", localRoot,
		[]string{"@github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root"},
		nil)
	fetched := candyEntity("charly-marketplace",
		cacheBase+"distro-cachyos@v2026.281.1001",
		[]string{"@github.com/opencharly/layer-charly-openclaw:v2026.271.2234"},
		nil)

	auth, err := directPinAuthority(roots, []candywalk.Entity{local, fetched})
	if err != nil {
		t.Fatalf("directPinAuthority: %v", err)
	}
	if !auth.direct["github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root"] {
		t.Error("the generator's own refs list must be read as authority")
	}
	if auth.direct["github.com/opencharly/layer-charly-openclaw:v2026.271.2234"] {
		t.Error("a transitively-reached repo's refs list must NOT be read as authority")
	}
	if !auth.isLocal(local) || auth.isLocal(fetched) {
		t.Error("only the generator's own roots are local")
	}
}

// TestDedupeEntities_TheProjectsOwnPinBeatsATransitiveTwin — the same law, end to end through the
// dedupe, with two entities that carry DIFFERENT bodies (so the winner is observable).
func TestDedupeEntities_TheProjectsOwnPinBeatsATransitiveTwin(t *testing.T) {
	branch := skillEntity("openclaw-layer", "openclaw", cacheBase+"layer-openclaw@feat/skill-bodies-to-repo-root")
	branch.Namespace = "github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root"
	staleTag := skillEntity("openclaw-layer", "openclaw", cacheBase+"layer-charly-openclaw@v2026.271.2234")
	staleTag.Namespace = "github.com/opencharly/layer-charly-openclaw:v2026.271.2234"
	staleTag.Dir = "candy/openclaw"

	auth := authority("", "@github.com/opencharly/layer-openclaw:feat/skill-bodies-to-repo-root")

	got := dedupeEntities([]candywalk.Entity{staleTag, branch}, auth)
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1 — one skill from two sources is one entity", len(got))
	}
	if got[0].SourceRoot != branch.SourceRoot {
		t.Fatalf("winner = %q, want the project's own pin %q", got[0].SourceRoot, branch.SourceRoot)
	}
}

// TestDedupeEntities_KindsAreSeparate — the same NAME under two kinds is two entities.
func TestDedupeEntities_KindsAreSeparate(t *testing.T) {
	a := candywalk.Entity{Name: "x", Kind: "skill", SourceRoot: cacheBase + "r@v2026.1.1"}
	b := candywalk.Entity{Name: "x", Kind: "hook", SourceRoot: cacheBase + "r@v2026.1.1"}

	if got := dedupeEntities([]candywalk.Entity{a, b}, pinAuthority{}); len(got) != 2 {
		t.Fatalf("got %d entities, want 2 — a skill and a hook sharing a name are distinct", len(got))
	}
}

// skillEntity builds a `skill:` entity whose body declares a family, the shape candywalk yields
// (Value is the kind value body, so a mapping node carrying `name:`/`family:`).
func skillEntity(name, family, sourceRoot string) candywalk.Entity {
	var doc yaml.Node
	body := "name: " + name + "\nfamily: " + family + "\ndescription: a skill\n"
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		panic(err)
	}
	return candywalk.Entity{Name: name, Kind: "skill", Value: *doc.Content[0], SourceRoot: sourceRoot}
}

// TestDedupeEntities_FamiliesAreSeparate — the same NAME in two FAMILIES is two skills, and the
// tag rule still resolves the SAME one arriving twice.
//
// The family is what makes a skill name unambiguous: the corpus is per-family
// (`<family>/skills/<name>/SKILL.md`) and a skill is addressed `/charly-<family>:<skill>`. Keying
// the dedupe on (Kind, Name) alone let tag order decide which of two different skills survived,
// and the loser's file and dispatcher row were pruned from the corpus. Measured live:
// `layer-crabbox` (family tools, v2026.288.2114) and `plugin-crabbox` (family check,
// v2026.288.1538) both declare a `crabbox` skill, and the refresh that advanced both tags
// swapped the published one (opencharly/plugin-marketplace#20, observed on
// opencharly/marketplace#490). Both must survive, because both are real skills in their own
// family.
func TestDedupeEntities_FamiliesAreSeparate(t *testing.T) {
	tools := skillEntity("crabbox", "tools", cacheBase+"layer-crabbox@v2026.288.2114")
	check := skillEntity("crabbox", "check", cacheBase+"plugin-crabbox@v2026.288.1538")

	got := dedupeEntities([]candywalk.Entity{tools, check}, pinAuthority{})
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2 — the same name in two families is two skills, and dropping one publishes the wrong one by tag order", len(got))
	}
	families := map[string]bool{}
	for _, e := range got {
		families[entityFamily(e)] = true
	}
	if !families["tools"] || !families["check"] {
		t.Fatalf("kept families %v, want both tools and check", families)
	}
	// The tag rule is untouched by the new key: the SAME skill (kind, family, name) arriving from
	// two sources still collapses to the higher tag, which is the case this file exists for.
	older := skillEntity("crabbox", "tools", cacheBase+"layer-crabbox@v2026.274.0609")
	if got := dedupeEntities([]candywalk.Entity{older, tools}, pinAuthority{}); len(got) != 1 {
		t.Fatalf("got %d entities, want 1 — one skill from two sources is still one entity", len(got))
	} else if got[0].SourceRoot != tools.SourceRoot {
		t.Fatalf("winner = %q, want the higher tag %q", got[0].SourceRoot, tools.SourceRoot)
	}
}

// candyEntity builds a `candy:` entity carrying the refs list a project declares, the shape
// candywalk yields for candy/charly-marketplace/charly.yml (Value is the kind's value node).
func candyEntity(name, sourceRoot string, candyRefs, requireRefs []string) candywalk.Entity {
	var doc yaml.Node
	body := "description: a candy that names the corpus sources\n"
	if len(candyRefs) > 0 {
		body += "candy:\n" + strings.Join(quoteEach(candyRefs), "\n") + "\n"
	}
	if len(requireRefs) > 0 {
		body += "require:\n" + strings.Join(quoteEach(requireRefs), "\n") + "\n"
	}
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		panic(err)
	}
	return candywalk.Entity{
		Name:       name,
		Kind:       "candy",
		Value:      *doc.Content[0],
		SourceRoot: sourceRoot,
	}
}

func quoteEach(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, "    - '"+r+"'")
	}
	return out
}
