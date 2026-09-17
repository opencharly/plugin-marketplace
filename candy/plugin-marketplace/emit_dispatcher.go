package marketplace

import (
	"sort"
)

// emit_dispatcher.go — the R0 skill-dispatcher projection. The dispatcher table is the routing
// half of the marketplace corpus: each skill entity's `triggers:` list becomes one row pointing
// at that skill's canonical `/charly-<family>:<name>` reference. The corpus is the single source;
// a consuming repo (charly, the umbrella) copies the emitted fragment into its own rulebook
// between the two stable markers, so the table is generated rather than hand-maintained prose.
//
// Only `type: skill` entities with a non-empty `triggers:` produce rows. Agents are dispatch
// targets, not dispatch sources, and a skill with no triggers simply contributes no row (the
// table is partial but never wrong) — this is what lets `triggers:` be authored incrementally
// across owning repos without breaking generation.

const (
	dispatcherBegin = "<!-- BEGIN GENERATED SKILL DISPATCHER -->"
	dispatcherEnd   = "<!-- END GENERATED SKILL DISPATCHER -->"
)

// dispatcherEntry is one rendered row: the trigger phrase and the skill it selects.
type dispatcherEntry struct {
	Family string
	Name   string
	Phrase string
}

func emitDispatcher(em emissions, families []family) {
	var entries []dispatcherEntry
	for _, f := range families {
		for _, s := range f.Skills {
			if s.Type == "agent" || len(s.Triggers) == 0 {
				continue
			}
			entries = append(entries, dispatcherEntry{
				Family: f.Name,
				Name:   s.Name,
				Phrase: joinTriggers(s.Triggers),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Family != entries[j].Family {
			return entries[i].Family < entries[j].Family
		}
		return entries[i].Name < entries[j].Name
	})

	var b []byte
	b = append(b, dispatcherBegin...)
	b = append(b, "\n| Trigger (what the user said or you're about to do) | Skill to load |\n|---|---|\n"...)
	for _, e := range entries {
		b = append(b, "| "...)
		b = append(b, e.Phrase...)
		b = append(b, " | `/charly-"...)
		b = append(b, e.Family...)
		b = append(b, ':')
		b = append(b, e.Name...)
		b = append(b, "` |\n"...)
	}
	b = append(b, dispatcherEnd...)
	b = append(b, '\n')

	em["plugins/DISPATCHER.md"] = b
}

// joinTriggers renders a triggers list as one table cell. A trigger is a single phrase; several
// are joined with " / " so a row reads as the alternation the dispatcher matches. Newlines and
// pipes would break the markdown table, so they are folded to spaces.
func joinTriggers(triggers []string) string {
	out := ""
	for i, t := range triggers {
		if i > 0 {
			out += " / "
		}
		out += sanitizeCell(t)
	}
	return out
}

func sanitizeCell(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == '\n' || b[i] == '\r' || b[i] == '|' {
			b[i] = ' '
		}
	}
	return string(b)
}

// emitOpencodeIndex emits the opencode `skills.urls` index. opencode fetches `${url}/index.json`
// and each skill's listed `files` from `${url}/${name}/${file}` — a FLAT per-skill directory
// layout, which the marketplace corpus is not (it is `<family>/skills/<name>/`). The index is
// therefore the manifest of a flat tree the consuming site assembles (the deploy workflow copies
// each skill's files to `<base>/<name>/`); `version` is the marketplace version, which opencode
// uses to invalidate its cache when the corpus moves.
func emitOpencodeIndex(em emissions, ks *kindSet, families []family) {
	idx := opencodeIndex{}
	for _, f := range families {
		for _, s := range f.Skills {
			if s.Type == "agent" {
				continue
			}
			files := []string{"SKILL.md"}
			for _, r := range s.References {
				files = append(files, "references/"+r.Name+".md")
			}
			idx.Skills = append(idx.Skills, opencodeIndexSkill{
				Name:    s.Name,
				Files:   files,
				Version: ks.Marketplace.Version,
			})
		}
	}
	sort.Slice(idx.Skills, func(i, j int) bool { return idx.Skills[i].Name < idx.Skills[j].Name })
	em["plugins/.well-known/skills/index.json"] = mustJSON(idx)
}

// opencodeIndex is the `skills.urls` index shape opencode fetches (opencode config schema
// `skills.urls`; the `Index`/`IndexSkill` structs in opencode's skill-discovery service).
type opencodeIndex struct {
	Skills []opencodeIndexSkill `json:"skills"`
}

type opencodeIndexSkill struct {
	Name    string   `json:"name"`
	Files   []string `json:"files"`
	Version string   `json:"version,omitempty"`
}
