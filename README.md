# plugin-marketplace

The marketplace-corpus generator for
[opencharly/charly](https://github.com/opencharly/charly) — it serves
`charly marketplace …`, the command that regenerates the ENTIRE `charly-plugins`
marketplace corpus from candy config.

It reads every candy's `skill:` / `hook:` / `marketplace:` kind entities (the
candywalk discovery), aggregates them by marketplace family, and emits the
standalone `opencharly/marketplace` repo's corpus: `<family>/skills/*/SKILL.md`
+ `references/*.md`, `<family>/agents/*.md`, the per-family
`.claude-plugin/plugin.json` + `.codex-plugin/plugin.json` + `.mcp.json`, the
Claude Code / Cursor `.claude-plugin/marketplace.json`, the Codex / AGENTS
`.agents/plugins/marketplace.json`, `kimi.plugin.json` (the whole marketplace is
one plugin), `package.json` (pi — the whole marketplace is one pi package),
`profiles.json`.

## What it provides

| Capability | Surface |
|---|---|
| `command:marketplace` | `charly marketplace generate` and `charly marketplace drift` |

## The commands

- **`charly marketplace generate --root <corpus> --out <corpus>`** — regenerates
  the corpus under `--out`, pruning generated trees by the DO-NOT-EDIT header
  first, so a deleted source entity disappears instead of lingering. `--root` is
  the repo root that holds the ONE generation input (the refs list,
  `candy/charly-marketplace/charly.yml`); for the corpus that is the corpus
  checkout itself, so both flags name it.
- **`charly marketplace drift --root <corpus> --out <corpus>`** — the fail-closed
  no-op gate: the same pipeline in memory, compared byte-for-byte with the
  artifacts ON DISK. Git is never consulted, so drift proves "regeneration is a
  no-op", never "the tree is committed". A stale mirror is a hard failure (exit 1).

## Placement

The plugin is **NOT** listed in `charly/charly.yml` `compiled_plugins:` — it is a
DEV-TIME generator, run on a contributor's machine to regenerate the harness
surface. The generator is self-contained (it reads files and writes files, never
reaching the host reverse channel), which is what makes the out-of-process
placement free.

## The R10 bed

The root `charly.yml` declares the `marketplace-app` box and the
`check-marketplace` disposable pod bed. The bed composes the LOCAL
`plugin-marketplace` candy, whose `plan:` clones the PUBLISHED
`opencharly/marketplace` repo at the pinned immutable `MARKETPLACE_REF` commit
and gates its shape (catalogs, no-version manifests, baked skill headers, the
pinned-commit check).

## How to use it

The command is served **out-of-process** — charly's loader fetches this candy at
its pinned ref and connects the provider by word, so no project-side candy
composition is needed:

```bash
CORPUS=/path/to/marketplace
charly marketplace generate --root "$CORPUS" --out "$CORPUS"
charly marketplace drift --root "$CORPUS" --out "$CORPUS"
```

## The drift gate

`charly task skills:drift` is this repository's own gate: it runs the drift
comparison against a corpus checkout **and** the half `drift` cannot do — that the
corpus tree is committed (a generated-but-untracked artifact passes the byte
comparison and is lost on the next clone).

```bash
charly task skills:drift --param CORPUS=/path/to/marketplace
```

The `CORPUS` parameter is UPPER-CASE on purpose: the task runner's expansion
grammar matches upper-case names only, so a lower-case parameter never reaches the
command.

`.github/workflows/drift-gate.yml` runs this recipe on every merge to `main`,
against the corpus commit this repository **declares** — `var: MARKETPLACE_REF`
in `candy/plugin-marketplace/charly.yml` — which the job reads from that file
rather than repeating as a literal. The comparison proves that the generator in
this tree reproduces the corpus commit this plugin declares.

The job runs on `main` and not on pull requests, and that is a design
constraint, not a shortcut. A generator change that alters the emitted output
makes the declared corpus commit stale, and the corpus can only be regenerated
with a *released* generator — so a pull-request gate would be red by
construction for exactly the change it should inform, and it would deadlock the
chain that repairs it (the corpus is regenerated with the released tag, then
`MARKETPLACE_REF` is bumped here). A pull-request gate would also be a de-facto
required check: the org validator's `arm` job arms native auto-merge only when
every check on the head is green. A red run on `main` is a signal with a
mechanical remedy: let the corpus repo regenerate with the released tag, then
bump `MARKETPLACE_REF` (and the literal that duplicates it in the candy's
plan).

## Layout

- `candy/plugin-marketplace/` — the plugin module: `plugin.go`, `command.go`,
  `generate.go`, `emit*.go`, `model.go`, `prune.go`, `readkinds.go`,
  `resolve.go`, `schema/marketplace.cue`, `cmd/serve/main.go`.
- `candy/plugin-marketplace/charly.yml` — the `plugin-marketplace:` candy entity
  (with the `var:` `MARKETPLACE_REPO` / `MARKETPLACE_REF` pins).
- `charly.yml` — the root project manifest (`discover: candy`, `defaults:`) plus
  the `marketplace-app` box and `check-marketplace` bed.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-internals:marketplace` — how the skill/marketplace
  corpus is generated, refreshed, and consumed. This candy carries no `skill:`
  entity of its own; the gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-build:docs` — `charly docs generate`, the docs-site generator.
- `/charly-internals:plugin` — the plugin/provider model.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
