# plugin-marketplace

The marketplace-corpus generator for
[opencharly/charly](https://github.com/opencharly/charly) — it serves
`charly marketplace …`, the command that regenerates the ENTIRE `charly-plugins`
marketplace + harness surface from candy config.

It reads every candy's `skill:` / `hook:` / `marketplace:` kind entities (the
candywalk discovery), aggregates them by marketplace family, and emits the
standalone `opencharly/marketplace` repo's corpus: `<family>/skills/*/SKILL.md`
+ `references/*.md`, `<family>/agents/*.md`, the per-family
`.claude-plugin/plugin.json` + `.codex-plugin/plugin.json` + `.mcp.json`, the
Claude Code / Cursor `.claude-plugin/marketplace.json`, the Codex / AGENTS
`.agents/plugins/marketplace.json`, `kimi.plugin.json` (the whole marketplace is
one plugin), `package.json` (pi — the whole marketplace is one pi package),
`profiles.json`, and the `setup` launcher.

## What it provides

| Capability | Surface |
|---|---|
| `command:marketplace` | `charly marketplace generate` and `charly marketplace drift` |

## The commands

- **`charly marketplace generate --out <dir>`** — regenerates the corpus under
  `--out`, pruning generated trees by the DO-NOT-EDIT header first, so a deleted
  source entity disappears instead of lingering.
- **`charly marketplace drift --out <dir>`** — the fail-closed no-op gate: the
  same pipeline in memory, compared byte-for-byte with the artifacts ON DISK. Git
  is never consulted, so drift proves "regeneration is a no-op", never "the tree
  is committed". A stale mirror is a hard failure (exit 1).

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
executable `setup` launcher, the pinned-commit check).

## How to use it

The command is invoked directly — no candy composition is needed:

```bash
charly marketplace generate --out /path/to/marketplace
charly marketplace drift --out /path/to/marketplace
```

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
