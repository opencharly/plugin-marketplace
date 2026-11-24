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

// retiredCorpus is what the corpus has retired: whole REPOS, and individual SKILL cards.
//
// The skill half exists because a stale skill does not always come with a retired repo: the
// thing a card describes can be consolidated or removed while its owner repo lives on, and the
// node then survives only in the owner's OLD tags. Measured (opencharly/plugin-marketplace#64):
// `herdr` (the owner now publishes `herdr-box`), `fedora-coder` and `fedora-test` (gone from
// distro-fedora), and the four `openclaw-*` variants (consolidated into one `openclaw`) were
// published only through stale subpath pins. The subpath-honouring walk stops the bleeding;
// this list makes the retirement EXPLICIT, reviewable, and enforced against any future edge.
type retiredCorpus struct {
	repos  map[string]bool // repo paths, ParseRemoteRef's RepoPath form
	skills map[string]bool // "<family>/<name>"
}

// skillRetired reports whether a skill card's (family, name) is retired.
func (r retiredCorpus) skillRetired(family, name string) bool {
	return r.skills[family+"/"+name]
}

// loadRetired reads and validates the corpus's retirement list. A corpus with no list has no
// retirements (and no retired.cue) — that is the shape every corpus had before #63, so it stays a
// no-op rather than an error. Every OTHER problem is fatal.
func loadRetired(root string) (retiredCorpus, error) {
	path := filepath.Join(root, retiredFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return retiredCorpus{repos: map[string]bool{}, skills: map[string]bool{}}, nil
	}
	if err != nil {
		return retiredCorpus{}, fmt.Errorf("read %s: %w", retiredFileName, err)
	}
	repos, skills, err := parseRetired(raw)
	if err != nil {
		return retiredCorpus{}, fmt.Errorf("%s: %w", retiredFileName, err)
	}
	out := retiredCorpus{repos: make(map[string]bool, len(repos)), skills: make(map[string]bool, len(skills))}
	for _, p := range repos {
		out.repos[p] = true
	}
	for _, p := range skills {
		out.skills[p] = true
	}
	return out, nil
}

// parseRetired validates one retirement list against `#Retired` and returns its repo paths in the
// form refs are parsed into (`spec.ParseRemoteRef(...).RepoPath`, e.g.
// "github.com/opencharly/pod-immich-ml").
func parseRetired(raw []byte) (repos, skills []string, err error) {
	schemaSrc, err := schemaFS.ReadFile("schema/marketplace.cue")
	if err != nil {
		return nil, nil, fmt.Errorf("read the plugin schema: %w", err)
	}
	ctx := cuecontext.New()
	schema := ctx.CompileBytes(schemaSrc)
	if err := schema.Err(); err != nil {
		return nil, nil, fmt.Errorf("compile the plugin schema: %w", err)
	}
	def := schema.LookupPath(cue.ParsePath("#Retired"))
	if !def.Exists() {
		return nil, nil, errors.New("the plugin schema declares no #Retired — the retirement list cannot be validated")
	}
	// Compile and unify in the SAME context; #Retired is a definition, so it is closed and an
	// unknown field in the authored file is a hard error.
	data := ctx.CompileBytes(raw)
	if err := data.Err(); err != nil {
		return nil, nil, err
	}
	unified := data.Unify(def)
	if err := unified.Validate(); err != nil {
		return nil, nil, fmt.Errorf("does not match #Retired: %w", err)
	}
	var doc struct {
		Retired       []string `json:"retired"`
		RetiredSkills []string `json:"retired_skills"`
	}
	if err := unified.Decode(&doc); err != nil {
		return nil, nil, err
	}
	for _, p := range doc.Retired {
		repoPath := strings.TrimPrefix(strings.TrimSpace(p), "@")
		if !strings.HasPrefix(repoPath, "github.com/") {
			return nil, nil, fmt.Errorf("retired entry %q is not a fully qualified repo path (want e.g. \"github.com/opencharly/pod-immich-ml\")", p)
		}
		repos = append(repos, repoPath)
	}
	for _, p := range doc.RetiredSkills {
		key := strings.TrimSpace(p)
		family, name, ok := strings.Cut(key, "/")
		if !ok || family == "" || name == "" || strings.Contains(name, "/") {
			return nil, nil, fmt.Errorf("retired skill entry %q is not <family>/<name>", p)
		}
		skills = append(skills, key)
	}
	return repos, skills, nil
}
