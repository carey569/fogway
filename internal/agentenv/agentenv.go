// Package agentenv names the environment variables through which an agent
// really installed on this machine reaches fogway: the one list a test
// sandbox clears, so that a test meets only the agents it put there itself.
//
// A sandbox fakes fogway's own folders through HOME, USERPROFILE and the XDG
// variables (appdir decides those), and an agent's folder through the
// variable that agent's own CLI reads before it falls back to its folder in
// the home: CLAUDE_CONFIG_DIR, CODEX_HOME, QODER_CONFIG_DIR and the rest.
// Any one of them left over from the developer's shell points fogway at a
// real agent, whose sessions and settings a test then reads — and, where
// fogway writes what it wires, writes into (#522). Qoder's CLI sets both of
// its variables for every child it starts, and fogway manages Qoder, so
// writing fogway from inside Qoder was one way to meet this.
//
// Each package keeps its own sandbox, because a test in internal/sessions
// cannot import internal/agent, which imports it, to ask which variables
// there are; this is the list they all clear. A name here is one fogway
// reads, and TestVarsAreRead says so, so a variable fogway stopped reading
// does not linger — that is how GEMINI_CLI_HOME and OPENCODE_CONFIG came to
// be cleared by a sandbox while OPENCODE_CONFIG_DIR, the one fogway reads,
// was not. What is deliberately not here: APPDATA and LOCALAPPDATA, Windows'
// folders rather than an agent's, which a sandbox sets to a folder of its
// own instead of clearing; and CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC, a
// behaviour flag whose one test sets it on purpose to read what fogway then
// does.
package agentenv

// Vars are the variables, by the agent they belong to. A sandbox clears all
// of them, and sets back the ones its own fixtures need.
var Vars = []string{
	// Claude Code and Codex
	"CLAUDE_CONFIG_DIR", "CODEX_HOME",
	// Zed, or a fork of it that keeps its settings (ZedG)
	"FOGWAY_ZED_BIN", "FOGWAY_ZED_CONFIG_DIR", "FOGWAY_ZED_PROCESS_NAMES",
	// Codex's state database, when it is kept apart from CODEX_HOME
	"CODEX_SQLITE_HOME",
	// Gemini CLI's session/config home
	"GEMINI_CLI_HOME",
	// Cline: its folder, its data and its sessions
	"CLINE_DIR", "CLINE_DATA_DIR", "CLINE_SESSION_DATA_DIR",
	// Pi, whose profile and config are apart
	"PI_CODING_AGENT_DIR", "PI_CODING_AGENT_SESSION_DIR", "PI_CONFIG_DIR", "PI_PROFILE",
	"PRIME_AGENT_CODING_AGENT_DIR",
	// omp
	"OMP_PROFILE",
	// Kimi Code and its shared folder
	"KIMI_CODE_HOME", "KIMI_SHARE_DIR",
	// Qwen Code's home, ~/.qwen without it
	"QWEN_HOME",
	// OpenHanako, dsh
	"HANA_HOME", "DSH_HOME",
	// PI-Desktop's data folder, where Sessions reads its chat titles
	"PI_DESKTOP_DATA_DIR",
	// T3 Code's base folder (its settings in userdata/)
	"T3CODE_HOME",
	// Cursor's CLI: its config folder (its chats) and its data folder
	"CURSOR_CONFIG_DIR", "CURSOR_DATA_DIR",
	// OpenCode
	"OPENCODE_CONFIG_DIR", "OPENCODE_DB",
	// Droid's home: what makes an installation visible that the test
	// didn't make
	"FACTORY_HOME_OVERRIDE",
	// nvm's and fnm's folders, which hold the Node versions an agent's CLI
	// is installed under, and which of them a terminal uses
	"NVM_DIR", "FNM_DIR",
	// mise's and asdf's folders, whose shims stand for an agent's CLI and
	// whose installs hold the Node versions it is under
	"MISE_DATA_DIR", "ASDF_DIR",
	// Google Cloud's credentials file and gcloud's folder, which Vertex AI's
	// tokens are minted from (and Gemini CLI's own, on Vertex AI)
	"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_CONFIG",
}

// NotPaths are the Vars that name no folder or file under the working
// folder: a profile's name, a program's name, or a name its
// agent puts under a folder of its own (PI_CONFIG_DIR under the home,
// OPENCODE_DB in OpenCode's data folder), which a relative value is meant
// for.
var NotPaths = map[string]bool{
	"PI_PROFILE": true, "OMP_PROFILE": true,
	"PI_CONFIG_DIR": true, "OPENCODE_DB": true,
	"FOGWAY_ZED_BIN": true, "FOGWAY_ZED_PROCESS_NAMES": true,
}
