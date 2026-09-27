// plugin-marketplace's OWN self-contained CUE schema — the SINGLE SOURCE for this
// plugin's declaration surface, served over the Describe channel (there is no
// schema-less plugin). It is used two ways exactly like every other plugin's schema:
//
//  1. GENERATE the Go params — `cue exp gengotypes` → ../params/cue_types_gen.go.
//  2. SERVE over Describe — the host splices `base ++ plugin` at the load gate
//     (registerPluginUnitSchema), so the plugin's declarations travel WITH it and a
//     self-contained schema that will not splice is a LOUD load failure.
//
// `command:marketplace`'s authored input is its pass-through CLI grammar
// (`marketplace generate|drift` plus flags), not a structured plugin_input, so this
// schema DOCUMENTS the command contract and the configuration surface the generator
// reads. SELF-CONTAINED: it references no base def, so it compiles STANDALONE (the
// property the SDK's serve-side compile and `cue exp gengotypes` both need).
#MarketplacePlugin: {
	// The command word the plugin serves.
	command: "marketplace"

	// What the command does, in one line (the public-docs surface).
	contract: string & !=""

	// The configuration surface: the pinned-corpus source the generator clones, declared
	// here so the charly.yml `var:` list and this schema cannot silently drift.
	config?: {
		MARKETPLACE_REPO?: string & =~"^https?://"
		MARKETPLACE_REF?:  string & !=""
	}
}
