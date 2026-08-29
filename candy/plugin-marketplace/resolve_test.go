package marketplace

import (
	"math/rand"
	"testing"

	"github.com/opencharly/sdk/candywalk"
)

const cacheBase = "/home/u/.cache/charly/repos/github.com/opencharly/"

func ent(name, sourceRoot string) candywalk.Entity {
	return candywalk.Entity{Name: name, Kind: "skill", SourceRoot: sourceRoot}
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
		got := dedupeEntities(shuffled)
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

	if !preferEntity(older, newer) {
		t.Error("the newer tag must displace the older one")
	}
	if preferEntity(newer, older) {
		t.Error("the older tag must NOT displace the newer one")
	}
}

// TestPreferEntity_CalVerComparedNumerically guards the trap in comparing CalVer as text:
// "v2026.239.1605" sorts BELOW "v2026.239.936" lexicographically, because '1' < '9' — so a
// string comparison would pick the older tag on exactly the pair this fix exists to resolve.
func TestPreferEntity_CalVerComparedNumerically(t *testing.T) {
	lowNumericHighLex := ent("s", cacheBase+"r@v2026.239.936")
	highNumericLowLex := ent("s", cacheBase+"r@v2026.239.1605")

	if !preferEntity(lowNumericHighLex, highNumericLowLex) {
		t.Error("1605 > 936 numerically; a lexicographic comparison would get this backwards")
	}
}

// TestPreferEntity_LocalOutranksFetched — the tree being generated from is the authority.
func TestPreferEntity_LocalOutranksFetched(t *testing.T) {
	fetched := ent("s", cacheBase+"r@v2099.001.1")
	local := ent("s", "")

	if !preferEntity(fetched, local) {
		t.Error("a local entity must outrank a fetched one, even at a lower tag")
	}
	if preferEntity(local, fetched) {
		t.Error("a fetched entity must not displace a local one")
	}
}

// TestPreferEntity_UnparseableNeverDisplaces — a source dir with no @vTag (a branch ref, a
// hand-placed dir) must not silently win over a properly pinned one.
func TestPreferEntity_UnparseableNeverDisplaces(t *testing.T) {
	pinned := ent("s", cacheBase+"r@v2026.100.1")
	branch := ent("s", cacheBase+"r@main")

	if preferEntity(pinned, branch) {
		t.Error("an unparseable source must not displace a pinned one")
	}
	if !preferEntity(branch, pinned) {
		t.Error("a pinned source must displace an unparseable one")
	}
}

// TestDedupeEntities_KindsAreSeparate — the same NAME under two kinds is two entities.
func TestDedupeEntities_KindsAreSeparate(t *testing.T) {
	a := candywalk.Entity{Name: "x", Kind: "skill", SourceRoot: cacheBase + "r@v2026.1.1"}
	b := candywalk.Entity{Name: "x", Kind: "hook", SourceRoot: cacheBase + "r@v2026.1.1"}

	if got := dedupeEntities([]candywalk.Entity{a, b}); len(got) != 2 {
		t.Fatalf("got %d entities, want 2 — a skill and a hook sharing a name are distinct", len(got))
	}
}
