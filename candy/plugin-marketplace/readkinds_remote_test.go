package marketplace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/refs"
)

// TestReadKinds_ResolvesMovedCandySkill is the R10 gate for the Phase-3 call-site change:
// readKinds must resolve a MOVED candy's skill: entity from its standalone repo via
// candywalk.CollectEntitiesRemote + refs.DownloadRepo (the same mechanism the runtime uses).
// FAILS without the change: the pre-Phase-3 code used candywalk.CollectEntities (pure local FS
// walk), which cannot resolve any remote ref — the moved candy's skill would be absent.
func TestReadKinds_ResolvesMovedCandySkill(t *testing.T) {
	// Fetch a REAL moved candy with a skill: entity (layer-ripgrep — the Phase-0 pilot,
	// carries ripgrep-skill with family: tools).
	tag := newestTag(t, "github.com/opencharly/layer-ripgrep")
	if _, err := refs.DownloadRepo("github.com/opencharly/layer-ripgrep", tag); err != nil {
		t.Fatalf("fetch layer-ripgrep: %v", err)
	}

	// A synthetic local project referencing the moved candy.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "local-keeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := "@github.com/opencharly/layer-ripgrep:" + tag
	local := "local-keeper:\n    candy:\n        version: 2026.200.1000\n        description: Local fixture.\n        candy:\n            - '" + ref + "'\n"
	if err := os.WriteFile(filepath.Join(root, "candy", "local-keeper", "charly.yml"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	// The generator requires a marketplace: entity (the corpus manifest).
	if err := os.MkdirAll(filepath.Join(root, "candy", "charly-marketplace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "candy", "charly-marketplace", "charly.yml"),
		[]byte("charly-marketplace:\n    marketplace:\n        name: charly\n        owner: opencharly\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ks, err := readKinds(root)
	if err != nil {
		t.Fatalf("readKinds: %v", err)
	}
	// Skills is a slice since the family joined the dedupe key (resolve.go), because two families
	// may now publish a skill with the same node name — keying a map by node name would drop one
	// of them here. Assert the moved candy's skill entity is present by its own name, and that its
	// family is tools (the skill: entity from the fetched layer-ripgrep repo).
	found := false
	for _, s := range ks.Skills {
		if s.Name == "ripgrep" {
			found = true
			if s.Family != "tools" {
				t.Fatalf("ripgrep skill family = %q, want tools", s.Family)
			}
			t.Logf("readKinds resolved moved candy skill (name=%s family=tools) from the remote walk", s.Name)
		}
	}
	if !found {
		t.Fatalf("moved candy skill ripgrep not resolved by readKinds (remote walk missing); got %d skills: %v", len(ks.Skills), ks.Skills)
	}
}

// newestTag resolves a repo's newest v-calver tag via git ls-remote (the refs kit shells out
// to git for clone/fetch, so an ls-remote here is the same toolchain).
func newestTag(t *testing.T, repoPath string) string {
	t.Helper()
	out, err := exec.Command("git", "ls-remote", "--tags", refs.RepoGitURL(repoPath)).Output()
	if err != nil {
		t.Fatalf("git ls-remote %s: %v", repoPath, err)
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/tags/")
		if !strings.HasPrefix(name, "v") || strings.HasSuffix(name, "^{}") {
			continue
		}
		tags = append(tags, name)
	}
	if len(tags) == 0 {
		t.Fatalf("no v-tags for %s", repoPath)
	}
	return tags[len(tags)-1]
}

// TestReadKinds_FetchesCorpusSourcesFilesOnly pins the FETCH MODE the walk uses (spec#279).
//
// A corpus source is read for its OWN TREE — charly.yml and the .agents/ trees — so the fetch
// must certify exactly that and no more. refs.DownloadRepo (the pre-fix resolver) populates every
// submodule the fetched repo declares and fails the whole fetch when one is unreachable, which is
// right for a `--repo` cache and was fatal here: opencharly/opencharly declares 412 gitlinks and
// could not be a corpus source at all (spec#279), after the population had already been measured
// as a pure cost on the same walk (spec#267).
//
// The DISCRIMINATOR is the cache entry: a files-only fetch publishes ONLY its own entry, so after
// readKinds the whole-project entry for the fetched ref must not exist. FAILS without the change:
// the pre-fix resolver writes exactly that entry. (Arm 2 — the `--repo` cache still populating —
// and arm 3 — an unreachable submodule still failing it loudly — are pinned in spec/refs, at the
// code path that owns them.)
func TestReadKinds_FetchesCorpusSourcesFilesOnly(t *testing.T) {
	tag := newestTag(t, "github.com/opencharly/layer-ripgrep")
	// A FRESH cache: the verdict is about what THIS walk fetched, not about what an earlier
	// session left behind (a warm entry is exactly the trap spec#279 was filed for).
	t.Setenv("CHARLY_REPO_CACHE", t.TempDir())

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "local-keeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := "@github.com/opencharly/layer-ripgrep:" + tag
	local := "local-keeper:\n    candy:\n        version: 2026.200.1000\n        description: Local fixture.\n        candy:\n            - '" + ref + "'\n"
	if err := os.WriteFile(filepath.Join(root, "candy", "local-keeper", "charly.yml"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "candy", "charly-marketplace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "candy", "charly-marketplace", "charly.yml"),
		[]byte("charly-marketplace:\n    marketplace:\n        name: charly\n        owner: opencharly\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ks, err := readKinds(root)
	if err != nil {
		t.Fatalf("readKinds: %v", err)
	}
	fetched := false
	for _, s := range ks.Skills {
		if s.Name == "ripgrep" {
			fetched = true
		}
	}
	if !fetched {
		t.Fatalf("the walk must still RESOLVE the remote candy (it fetched nothing?); got %d skills", len(ks.Skills))
	}
	cached, err := refs.IsRepoCached("github.com/opencharly/layer-ripgrep", tag)
	if err != nil {
		t.Fatalf("IsRepoCached: %v", err)
	}
	if cached {
		t.Fatal("readKinds wrote a WHOLE-PROJECT cache entry: the corpus walk must fetch files-only (spec#279)")
	}
}
