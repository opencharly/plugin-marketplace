package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extract_declared_tree_test.go — the case the first cut of `extract` got wrong, and which nothing
// else in the diff covered.
//
// `skillTrees` builds its map from the `candy` entities and keys it by CANDY name; `readkinds.go`
// looks it up with the skill's OWNER (`trees[s.Owner]`). `extract` first looked it up with the skill's
// own node name, which can never match a candy key — so `ok` was always false, every repo fell through
// to `.agents/skills`, and a repo declaring a non-default tree would have had its body written to a
// path the reader does not read. The reader-side test (`HonoursADeclaredTree`) cannot see that bug:
// it exercises `fillSkillFromFile`, not `extract`. This one exercises the writer.
const declaredTreeManifest = `demo:
    candy:
        description: |-
            Fixture candy that declares a NON-DEFAULT tree for its skills.
        skills:
            - custom-tree
        plan:
            - run: no-op fixture
              command: "true"
              context:
                - build
git-workflow:
    skill:
        name: git-workflow
        family: internals
        owner: demo
        description: |-
            branch-per-change landing.
        content: |
            # git-workflow

            Body.
`

func TestExtract_HonoursTheOwningCandysDeclaredTree(t *testing.T) {
	root := writeExtractFixture(t, declaredTreeManifest)
	if err := extract(root); err != nil {
		t.Fatalf("extract: %v", err)
	}

	want := filepath.Join(root, "custom-tree", "git-workflow", "SKILL.md")
	entry, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("the body was not written to the declared tree %s: %v", want, err)
	}
	if !strings.Contains(string(entry), "# git-workflow\n\nBody.") {
		t.Errorf("the wrong content landed at %s:\n%s", want, string(entry))
	}
	// And it must NOT be at the standard path: a file there is one the reader would never look for
	// once the candy declares its own tree, which is precisely how the two halves would drift apart.
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "git-workflow", "SKILL.md")); err == nil {
		t.Error("the body was ALSO written under the standard tree — extract and the reader now disagree")
	}
}
