// plugin-marketplace's OWN self-contained CUE schema — the SINGLE SOURCE for this plugin's
// served declaration surface (there is no schema-less plugin: every plugin ships a
// non-empty schema over Describe).
//
// SELF-CONTAINED and PACKAGE-LESS: it references no base def and carries no package
// clause, so it compiles STANDALONE — the property the SDK's serve-side compile needs
// and the property that lets the host splice `base ++ plugin` at the load gate
// (registerPluginUnitSchema); a self-contained schema that will not splice is a LOUD
// load failure.
//
// NO GO CONSUMER: the plugin declares no typed `plugin_input` (its authored input is
// its pass-through CLI grammar), so this schema generates NO `params` package and has
// NO `cue exp gengotypes` artifact — it is the SERVED documentation/config surface,
// not a code-generation source.
//
// It DOCUMENTS the `command: marketplace` contract and the pinned-corpus configuration surface (`MARKETPLACE_REPO` / `MARKETPLACE_REF`).
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
