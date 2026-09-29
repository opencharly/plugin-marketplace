# AGENTS.md — plugin-marketplace

Standalone plugin repo for the marketplace-corpus generator
(`command:marketplace`). The plugin is a Go module at
`candy/plugin-marketplace/` (module path
`github.com/opencharly/plugin-marketplace/candy/plugin-marketplace`); the root
`charly.yml` declares `discover: candy`, `defaults:`, the `marketplace-app` box,
and the `check-marketplace` disposable bed.

Canonical files:

- `candy/plugin-marketplace/charly.yml` — the `plugin-marketplace:` candy entity
  (`plugin:` block, `var:` pins, `plan:` checks).
- `candy/plugin-marketplace/` — the Go source: `plugin.go`, `command.go`,
  `generate.go`, `emit*.go`, `model.go`, `prune.go`, `readkinds.go`,
  `resolve.go`, `schema/marketplace.cue`, `cmd/serve/main.go`.
- `charly.yml` — the root manifest (`discover: candy`, `defaults:`) + the
  `marketplace-app` box and `check-marketplace` bed.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:marketplace` — how the skill/marketplace corpus is
  generated, refreshed, and consumed (the ownership split, the regeneration
  command, the deploy drift gate). Load before changing an emitter.
- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, the per-plugin CUE-schema contract.
- `/charly-build:docs` — `charly docs generate`, the sibling generator.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-marketplace/` — compile the plugin module.
- `go test ./...` in `candy/plugin-marketplace/` — the generator's Go tests.
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The R10 witness is the `check-marketplace` bed in the root `charly.yml`.

## Modify this repo

- Edit the `plugin-marketplace:` candy entity, the Go source, and
  `schema/marketplace.cue` **together**.
- The `var:` `MARKETPLACE_REF` MUST be an IMMUTABLE COMMIT equal to the
  `opencharly/marketplace` repo's current `main` head — `task marketplace:pin`
  checks it, and the pinned-commit check asserts it from inside the built image.
  The sha is duplicated as a literal in the check step by design.
- `generate` and `drift` run the SAME emission pipeline; keep them in step.

## Landing

- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Load
  `/charly-internals:git-workflow` before any git/PR action; history lives in
  `CHANGELOG/`. Do not restate its rules here.
