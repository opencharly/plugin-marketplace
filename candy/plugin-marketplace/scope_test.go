package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/candywalk"
)

// TestParseRetired pins the retirement list's shape and its strictness: validation is what makes
// `retired.cue` a gate rather than a comment — a typo must fail the run, never retire nothing
// silently.
func TestParseRetired(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    []string
		wantErr string
	}{
		{
			name: "valid, with the @ form accepted",
			src: `retired: [
	"github.com/opencharly/pod-immich-ml",
	"@github.com/opencharly/another",
]`,
			want: []string{"github.com/opencharly/pod-immich-ml", "github.com/opencharly/another"},
		},
		{
			name: "empty list retires nothing",
			src:  `retired: []`,
			want: nil,
		},
		{
			name:    "unknown field is a hard error (closed definition)",
			src:     "retired: []\nretierd: []\n",
			wantErr: "field not allowed",
		},
		{
			name:    "unqualified path",
			src:     `retired: ["pod-immich-ml"]`,
			wantErr: "not a fully qualified repo path",
		},
		{
			name:    "not a list of strings",
			src:     `retired: [{repo: "x"}]`,
			wantErr: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRetired([]byte(tc.src))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if tc.name == "not a list of strings" {
				if err == nil {
					t.Fatalf("a non-string entry must fail; got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRetired: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("retired = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCorpusScope_RetiredIsNeverFollowed pins the closure rule: a retired ref is dropped wherever
// it is declared, and a retired ref in the generator's OWN roots is ALSO recorded as an authoring
// contradiction (the refs list must not pin what the corpus has retired).
//
// FAILS WITHOUT THE CHANGE: the walker followed every ref every walked file declared, so
// pod-immich-ml's entities came back into the corpus (measured: immich/skills/immich-ml
// republished after its pin was deleted).
func TestCorpusScope_RetiredIsNeverFollowed(t *testing.T) {
	root := t.TempDir()
	retired := map[string]bool{"github.com/opencharly/pod-immich-ml": true}
	scope := newCorpusScope([]candywalk.Root{{Dir: root}}, retired)
	refs := []string{"@github.com/opencharly/pod-immich-ml:v2026.274.0811", "@github.com/opencharly/pod-immich:v2026.327.0043"}

	// A fetched tree: the retired ref vanishes, the live one survives, nothing is a conflict.
	fetched := "/cache/.files/github.com/opencharly/distro-cachyos@v2026.319.0820/box/immich-ml/charly.yml"
	got := scope.refs(fetched, "immich-ml", nil, refs)
	if len(got) != 1 || !strings.Contains(got[0], "pod-immich:") {
		t.Fatalf("fetched refs = %v, want only the live pod-immich ref", got)
	}
	if len(scope.ownRetired) != 0 {
		t.Fatalf("a fetched tree must not be reported as an own-root conflict: %v", scope.ownRetired)
	}

	// The generator's own roots: the ref is still dropped, and now it IS a conflict.
	own := filepath.Join(root, "candy", "charly-marketplace", "charly.yml")
	got = scope.refs(own, "charly-marketplace", nil, refs)
	if len(got) != 1 {
		t.Fatalf("own-root refs = %v, want only the live ref", got)
	}
	if len(scope.ownRetired) != 1 || !strings.Contains(scope.ownRetired[0], "pod-immich-ml") {
		t.Fatalf("ownRetired = %v, want the retired own-root ref recorded", scope.ownRetired)
	}

	// Nothing retired: every ref survives, byte for byte.
	live := []string{"@github.com/opencharly/pod-ai-toolkit:v2026.327.0042"}
	if got := scope.refs(fetched, "x", nil, live); len(got) != 1 || got[0] != live[0] {
		t.Fatalf("live refs = %v, want %v", got, live)
	}
}

// TestReadKinds_RefsListPinsRetiredRepo: a retirement that the refs list contradicts fails the
// run instead of being silently applied. No network: the retired ref is dropped before it can be
// fetched, so the walk sees only the local marketplace entity.
func TestReadKinds_RefsListPinsRetiredRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "charly-marketplace"), 0o755); err != nil {
		t.Fatal(err)
	}
	refsList := `charly-marketplace:
    candy:
        description: fixture
        candy:
            - '@github.com/opencharly/pod-immich-ml:v2026.274.0811'
    marketplace:
        name: charly
        owner: opencharly
`
	if err := os.WriteFile(filepath.Join(root, "candy", "charly-marketplace", "charly.yml"), []byte(refsList), 0o644); err != nil {
		t.Fatal(err)
	}
	retired := "retired: [\"github.com/opencharly/pod-immich-ml\"]\n"
	if err := os.WriteFile(filepath.Join(root, "retired.cue"), []byte(retired), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readKinds(root)
	if err == nil || !strings.Contains(err.Error(), "pins retired repo") {
		t.Fatalf("readKinds err = %v, want a retirement contradiction", err)
	}
}

// TestLoadRetired_AbsentIsEmpty: a corpus that has retired nothing carries no list, and that is a
// no-op — the shape every corpus had before the list existed.
func TestLoadRetired_AbsentIsEmpty(t *testing.T) {
	got, err := loadRetired(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("loadRetired = %v, %v; want empty, nil", got, err)
	}
}

// TestLoadRetired_MalformedIsFatal: a present-but-broken list must fail, never degrade to "nothing
// retired" — that silent degradation is the failure mode a gate exists to prevent.
func TestLoadRetired_MalformedIsFatal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, retiredFileName), []byte("retired: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRetired(root); err == nil {
		t.Fatal("loadRetired accepted a malformed list")
	}
}
