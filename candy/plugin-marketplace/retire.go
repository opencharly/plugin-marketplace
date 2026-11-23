package marketplace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// retire.go — the corpus's RETIREMENT list, the one datum that keeps a retired repo out of the
// generated corpus.
//
// WHY IT IS DATA. opencharly/plugin-marketplace#63: after the refs list dropped every
// pod-immich-ml pin, the corpus still published `immich/skills/immich-ml`, because stale subpath
// pins in other sources still reach old distro trees whose box manifests compose the retired repo
// (the measured chain is in corpusScope's comment). Two structural closure rules were implemented
// and measured against the full corpus; both removed 6–11 PUBLISHED skills along with it, because
// a retired repo and a live repo reachable only through an old tree have the same shape. So the
// corpus records retirement explicitly instead of inferring it.
//
// THE FILE is `retired.cue` at the corpus root (the generator's `--root`, i.e. the marketplace
// repo). It is authored data, so it is CUE, and its shape is `#Retired` in this plugin's served
// schema (`schema/marketplace.cue`) — the SAME schema the plugin publishes over Describe, so the
// file's shape and the plugin's are one declaration, not two. It is validated here at read time:
// a malformed list, an unknown field or an unqualified repo path fails the run loudly rather than
// silently retiring nothing.
const retiredFileName = "retired.cue"

// loadRetired reads and validates the corpus's retirement list. A corpus with no list has no
// retirements (and no retired.cue) — that is the shape every corpus had before #63, so it stays a
// no-op rather than an error. Every OTHER problem is fatal.
func loadRetired(root string) (map[string]bool, error) {
	path := filepath.Join(root, retiredFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", retiredFileName, err)
	}
	paths, err := parseRetired(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", retiredFileName, err)
	}
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		out[p] = true
	}
	return out, nil
}

// parseRetired validates one retirement list against `#Retired` and returns its repo paths in the
// form refs are parsed into (`spec.ParseRemoteRef(...).RepoPath`, e.g.
// "github.com/opencharly/pod-immich-ml").
func parseRetired(raw []byte) ([]string, error) {
	schemaSrc, err := schemaFS.ReadFile("schema/marketplace.cue")
	if err != nil {
		return nil, fmt.Errorf("read the plugin schema: %w", err)
	}
	ctx := cuecontext.New()
	schema := ctx.CompileBytes(schemaSrc)
	if err := schema.Err(); err != nil {
		return nil, fmt.Errorf("compile the plugin schema: %w", err)
	}
	def := schema.LookupPath(cue.ParsePath("#Retired"))
	if !def.Exists() {
		return nil, errors.New("the plugin schema declares no #Retired — the retirement list cannot be validated")
	}
	// Compile and unify in the SAME context; #Retired is a definition, so it is closed and an
	// unknown field in the authored file is a hard error.
	data := ctx.CompileBytes(raw)
	if err := data.Err(); err != nil {
		return nil, err
	}
	unified := data.Unify(def)
	if err := unified.Validate(); err != nil {
		return nil, fmt.Errorf("does not match #Retired: %w", err)
	}
	var doc struct {
		Retired []string
	}
	if err := unified.Decode(&doc); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(doc.Retired))
	for _, p := range doc.Retired {
		repoPath := strings.TrimPrefix(strings.TrimSpace(p), "@")
		if !strings.HasPrefix(repoPath, "github.com/") {
			return nil, fmt.Errorf("retired entry %q is not a fully qualified repo path (want e.g. \"github.com/opencharly/pod-immich-ml\")", p)
		}
		out = append(out, repoPath)
	}
	return out, nil
}
