package provider

// PLUGIN-SERVED (see AGENTS.md): Cursor ("cursor") is a deprecated built-in
// subscription served by its plugin, @magpie-community/opencode-cursor-auth,
// once moved onto it (provider.Moved; the default for a new sign-in). A
// moved one's sign-ins, models, requests and usage are all the plugin's,
// never this code's (only the move, in migrate*.go, still reads its
// accounts). A fix here alone doesn't reach those users; fix the plugin
// (github.com/magpie-community/plugins, packages/cursor) and raise the
// mover's min in internal/provider/migrate_side.go.

// `cursor-agent models` lists what GetUsableModels has, and Cursor's model
// picker (AiService/AvailableModels) offers models it leaves out: one added
// since, or one the account hasn't turned on (ARNO on Discord: "cursor
// provider 里少了 glm-5.3 和 glm-5.3-flash"). They are listed too, by the
// slug or name the agent API takes.

// cursorPickerSkip are left out of the CLI's picker as it leaves them out.
var cursorPickerSkip = map[string]bool{"claude-4.5-haiku": true, "claude-4.5-haiku-thinking": true, "gemini-2.5-pro": true, "gemini-2.5-flash": true}

type cursorPickerModel struct {
	Name              string   `json:"name"`
	ClientDisplayName string   `json:"clientDisplayName"`
	ServerModelName   string   `json:"serverModelName"`
	LegacySlugs       []string `json:"legacySlugs"`
	IDAliases         []string `json:"idAliases"`
	IsHidden          bool     `json:"isHidden"`
	IsChatOnly        bool     `json:"isChatOnly"`
	OnlySupportsCmdK  bool     `json:"onlySupportsCmdK"`
	SupportsAgent     *bool    `json:"supportsAgent"`
	ContextTokenLimit int      `json:"contextTokenLimit"`
	Variants          []struct {
		DisplayName                 string `json:"displayName"`
		DisplayNameOutsidePicker    string `json:"displayNameOutsidePicker"`
		LegacySlug                  string `json:"legacySlug"`
		VariantStringRepresentation string `json:"variantStringRepresentation"`
	} `json:"variants"`
}
