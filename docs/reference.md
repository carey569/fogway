# fogway reference

The full manual: every agent fogway wires, every provider option, the gateway, routing, the CLI, Docker and the files fogway keeps. For the overview, see the [README](../README.md).

Renamed a project folder? See [relocating Claude Code sessions](session-relocation.md).

One place to pick every agent's model: Codex on DeepSeek, Claude Code
on Kimi, Gemini CLI on GLM, from the menu bar.

`fogway` is a single screen that lists each AI agent on your machine and
the model it is set to. Click a value, pick a model. That is the whole app.

It lives in the menu bar: click the icon and a panel drops down; the same
screen also opens as a normal window (`fogway`, or *Open fogway* in the tray menu),
which opens on the page you last left, as a Mac app comes back as it was,
and there is a terminal version (`fogway tui`) and a plain CLI. The
terminal version serves the gateway while it is open when no other fogway
(the app, `fogway web`, `fogway serve`) does, and says so on its status
line; agents connected to fogway lose it when it quits.

```
  ◉ fogway

  ▸ Claude Code   claude-fable-5-1[1m]                        ~/.claude/settings.json
    Codex         gpt-6-astra   effort medium
    Gemini CLI    gemini-3.1-pro
    OpenCode      anthropic/claude-sonnet-5   small anthropic/claude-haiku-4-5
    Pi            openrouter/z-ai/glm-5.2:batch
    Goose         anthropic/claude-sonnet-5
    Cursor        auto
    Copilot CLI   claude-fable-5

  ↑↓ agent  ·  ←→ field  ·  ↵ change  ·  s save profile  ·  p profiles  ·  q quit
```

- **One binary.** About 30 MB with the desktop app (it uses the system
  webview through [Wails](https://wails.io), nothing bundled; a 15 MB
  download on a Mac), and under 30 MB for the terminal-only build. macOS,
  Linux and Windows.
- **Edits config files surgically.** Only the one key you change is touched;
  comments, ordering and indentation in your `settings.json`, `config.toml`,
  `opencode.jsonc` or `config.yaml` survive intact. Writes are atomic.
- **One endpoint for every agent.** fogway runs a local gateway that speaks
  OpenAI chat completions, OpenAI Responses and the Anthropic Messages API,
  and forwards to whichever vendor serves the model. Codex, Claude Code,
  OpenCode and the rest all point at `http://127.0.0.1:3725/v1` and pick
  from one catalog; the translation between APIs happens in fogway, streaming
  and tool calls included.
- **Providers with one field.** Pick a preset (Anthropic, OpenAI, Gemini,
  DeepSeek, Kimi, GLM, MiniMax, StepFun, Qwen, iFlytek Spark, Baidu Qianfan, Tencent Cloud Token Plan,
  Huawei Cloud MaaS, Volcengine Ark, Mistral, Groq, xAI, OpenRouter, Together,
  Fireworks, SiliconFlow, NVIDIA NIM, ModelScope, AiHubMix, PipeLLM, 302.AI, Ollama, LM Studio…),
  paste a key, done. Custom vendors need a name and a base URL. fogway never
  reads keys from your shell environment.
- **Real model lists, nothing compiled in.** With a key in hand fogway asks
  the vendor which models it serves and offers exactly those; the
  [models.dev](https://models.dev) catalog fills in names, reasoning efforts
  and the list for vendors that have none (a model the vendor's own entry
  doesn't list is named as the other providers serving it name it, so
  glm-5-turbo reads GLM-5-Turbo under every provider that serves it; an Azure
  deployment keeps its own name), and refreshes itself in the
  background once it goes stale. Choose which models each provider exposes,
  or expose them all — a model released this morning is in the picker on
  the next refresh.
- **Each agent's own model list.** Under an agent's name on the Agents page,
  "Showing 5 / 32 models" opens its list: click a model to take it out of
  that agent's picker (Codex's `/model` included, its ChatGPT models too) or
  put it back; other agents still use it, and a new model is shown. Its
  "Only models I pick" switch turns the list around: the models ticked
  when it is switched on stay, and a model added later, of any provider,
  stays out of that agent's picker and its config until it is ticked
  (`fogway visible <agent> --only-picked`, back with `--show-new`). A
  model asked for by name still works either way.
- **Profiles.** Snapshot every agent's settings under a name and switch all of
  them back in one move. The profile in use, the one applying would change
  nothing, is ticked, the tray panel's Profiles button shows its name, and
  `fogway profiles` marks it ✓.
- **Real logos, no framework.** Plain HTML over the system webview; brand
  icons from [lobehub/icons](https://github.com/lobehub/lobe-icons).

## Agents

| Agent        | File                              | Fields          |
| ------------ | --------------------------------- | --------------- |
| Claude Code  | `~/.claude/settings.json`         | provider, model, opus/sonnet/haiku/fable, sign-in (through fogway) |
| Codex        | `~/.codex/config.toml`            | provider, model, effort |
| Gemini CLI   | `~/.gemini/settings.json`, `~/.gemini/.env` | auth, model |
| OpenCode     | `~/.config/opencode/opencode.json(c)` (`$OPENCODE_CONFIG_DIR`) | model, small |
| Pi           | `~/.pi/agent/settings.json`       | model           |
| Goose        | `~/.config/goose/config.yaml`     | model           |
| Cursor CLI   | `~/.cursor/cli-config.json`       | model           |
| Zed          | `~/.config/zed/settings.json` (`$XDG_CONFIG_HOME` on Linux, `%APPDATA%\Zed` on Windows) | model (a `fogway` OpenAI-compatible provider; its catalog in Zed's picker) |
| VS Code (Chat) | `~/Library/Application Support/Code/User/settings.json` + `chatLanguageModels.json` (`~/.config/Code/User` on Linux, `%APPDATA%\Code\User` on Windows) | model (`chat.defaultModel`; a `fogway` Custom Endpoint group, its catalog in Chat's model picker; VS Code 1.122+, no Copilot sign-in or key needed). Each profile's own pair under `User/profiles/<id>/` (listed in `globalStorage/storage.json`) gets the same, unless the profile uses the default's |
| VS Code Insiders (Chat) | the same files under `Code - Insiders/User` in place of `Code/User` | as VS Code, a row of its own; its models send the token `fogway-vscode-insiders`, since its chat's User-Agent is VS Code's |
| Copilot CLI  | `~/.copilot/settings.json`        | model           |
| Crush        | `~/.config/crush/crush.json`      | large, small    |
| DeepSeek Harness (dsh) | `~/.dsh/profiles/*/cordis.patch.yml` (`$DSH_HOME`; a custom provider, Fogway, its key `FOGWAY_GATEWAY_KEY` in `~/.dsh/.env` and dsh's own key store `~/.dsh/.credentials.yaml`, which the desktop app reads; on one of fogway's models its `web-search-deepseek` row also goes to the gateway, which searches with the model or Settings › Web search, unless dsh has a `DEEPSEEK_API_KEY` or a row of your own), or `~/.dsh/config.yaml` before dsh 0.1.5 | model, effort |
| omp (oh-my-pi) | `~/.omp/agent/config.yml` (+ `models.yml`) | model |
| Kimi Code    | `~/.kimi/config.toml` (`$KIMI_SHARE_DIR`) | model (a `fogway` provider; fogway's models in Kimi's /model) |
| Qwen Code    | `~/.qwen/settings.json` (`$QWEN_HOME`) | model (fogway's models as `modelProviders.openai` entries on a `FOGWAY_QWEN_API_KEY` env var; settings.model; `security.auth.selectedType` openai while wired) |
| Droid (Factory) | `~/.factory/settings.json` (`$FACTORY_HOME_OVERRIDE`) | model (fogway's models as BYOK `customModels`, in Droid's /model) |
| Cline (CLI)  | `~/.cline/data/settings/providers.json` (`$CLINE_DIR`) | model, effort (fogway takes its openai-compatible provider) |

Provider-scoped agents (OpenCode, Pi, Goose, Crush, omp) take `provider/model`.
Only agents that are installed or configured are shown.

### Notes on some agents

#### Zed-compatible Agent paths

Fogway can use a Zed-compatible fork or installation whose executable or
configuration directory is different from the upstream defaults. Set these
variables before starting Fogway:

```sh
export FOGWAY_ZED_BIN=/Applications/ZedG.app/Contents/MacOS/zedg
export FOGWAY_ZED_CONFIG_DIR="$HOME/.config/zed"
export FOGWAY_ZED_PROCESS_NAMES=zedg,ZedG
fogway serve
```

`FOGWAY_ZED_BIN` controls installation detection, `FOGWAY_ZED_CONFIG_DIR`
selects the directory containing `settings.json`, and
`FOGWAY_ZED_PROCESS_NAMES` supplies comma-separated process names used for
restart notices. The existing Zed configuration adapter is reused, so this is
intended for forks that keep Zed's `settings.json` and Agent model schema, such
as [ZedG](https://github.com/x6nux/zed-globalization). These variables affect
the Fogway process in which they are set; put them in the service environment
when running Fogway under systemd or Docker. They configure a Zed-compatible
Agent on the same machine as Fogway; they do not discover or modify an Agent
running on another host. On macOS, if the configured binary does not resolve to
a `.app` bundle, Fogway falls back to the standard Zed application locations
when authorizing the gateway credential.

## Providers and the gateway

Every model an agent can pick is spelled `provider/model` and served by
fogway's gateway, so agents never hold vendor keys or vendor URLs. Add a
provider, and its models appear in every agent's picker:

```sh
fogway presets                          # the vendors fogway knows, grouped: vendors, relays, local
fogway provider add deepseek sk-…       # a preset needs only the key
fogway provider add ollama              # local servers need none
fogway provider add "My Relay" url=https://relay.example.com/v1 key=sk-… models=gpt-5.5,claude-sonnet-5
fogway providers                        # host, key, exposed models, who uses what
fogway provider deepseek                # one provider in detail
fogway provider models deepseek         # re-fetch the vendor's list (add ids to choose which to expose)
fogway provider models deepseek +deepseek-v4 -deepseek-chat   # expose one more, take one out; the rest stay
fogway provider models deepseek a b c   # the whole list of models to expose, replacing it (all: the default)
fogway provider refresh deepseek        # re-fetch it, and drop picks it no longer has (the TUI: m)
fogway provider test deepseek           # one tiny request per API, with latency
fogway provider key deepseek sk-…       # replace the key
fogway provider rm deepseek
fogway models                           # the catalog agents see
fogway claude deepseek/deepseek-chat    # use it
```

Custom providers take `url=` (an OpenAI-compatible base), `anthropic=` (an
Anthropic-compatible base), or both, plus `responses=` when the vendor has a
separate Responses endpoint, `catalog=` to borrow a models.dev list, and
`models=` to name the models to expose. Anything a preset does not know can
be overridden the same way.

A model whose vendor answers that it is deprecated or retired (OpenCode
Zen's 410 "Model exo-free has been deprecated.") leaves that provider's
models in `fogway models`, `/v1/models` and every agent's list, even while
the vendor's own list still names it. It comes back once a request for it
as `provider/model` is answered, or after a week.

`fogway provider set <id> header.<Name>=<value>` sends a header of your own
on every request to a key+URL provider (an empty value removes it; signed-in
accounts ignore them). It replaces a header of the same name fogway would
send, except `anthropic-beta`, which is a list: your betas are added after
the ones the agent asked for that request (Claude Code's, its 1M context's
`context-1m-2025-08-07` for a `[1m]` model, fast mode's), each once. A beta
the provider turns away (`Unexpected value(s) … for the anthropic-beta
header`) is dropped from the retry and from then on, yours as well as the
agent's; any other of yours is always sent. The beta of the agent's own
sign-in (`oauth-2025-04-20`, which Claude Code signed in to claude.ai asks)
never goes, as the sign-in never does; one you set yourself does.

Requests go to a vendor with fogway's own User-Agent (`fogway/<version>`).
A coding plan that serves only the agents it names knows them by theirs:
`fogway provider set <id> user-agent=agent` (the editor's "Send the
agent's own User-Agent") sends a key+URL provider the User-Agent the agent
sent fogway, as it sent it, on every API (chat, Anthropic, Responses,
Gemini, count_tokens, embeddings, images, videos). A request the agent
sent without one, and fogway's own (its classifier's, the image tool's),
still go as fogway. A `header.User-Agent` you set wins over both.
`user-agent=fogway` turns it off again; off is the default. A vendor whose
own client fogway goes as (OpenCode, Cline, Kilo) keeps the User-Agent that
client sends, and doesn't take it.

`fogway usage` also lists **upstream provider keys** to help check upstream bills.
Each request records the fingerprint and saved name of the key that actually
served it, including image calls and account/key failover. The CSV adds
`provider_key_id` and `provider_key_name`. JSON uses `providerKeyId` and `providerKeyName`, distinct from gateway caller
keys. No raw credential is stored
in usage records.
Rotating the provider's first key does not move old usage to its replacement;
deleted keys keep their historical identity. Older records appear as
**key not recorded**, never inferred from today's configured key.
These are upstream credentials, not keys clients use to call Fogway.

The CSV's `cache_write_tokens` is still every cache write; the two
columns, `cache_write_5m_tokens` and `cache_write_1h_tokens`, split it by how
long Anthropic keeps it (a record from before the split is all 5-minute).
The app shows the split on a request's Cache and the usage tiles' tooltips.

**Requests and failures** count requests, each once by how it ended: a
request failed when the agent got an error. One that a later try answered
(the next member of a group, another account) is not a failure; the Usage
page, the Requests card, its rankings and group rows, and `fogway usage`
count it as answered on a retry (`retried` in the JSON). On the Requests
card each count is a button for its requests in the list below (the
Failed and Retried filters, `failed=1` and `retried=1`). A try a vendor
refused or turned away keeps its own row, for what it billed and why, marked
handed on, and isn't counted as a request. The CSV's last columns are
`tries` (on the final row, when the request took more than one),
`handed_on`, then `tier_asked`, `tier_sent` and `tier_served`: the
service tier the client asked for (Claude's fast speed reads `fast`), the
one that try went out at (a fast pick sends `priority`; Ultrafast becomes
`priority` on an account whose plan doesn't offer it), and the one the
vendor's reply said it ran at, empty when the reply doesn't say. The JSON
log keeps them as `tier_asked`, `tier_sent` and `tier_served`, and each
try in `/v1/fogway/route` has `tier` and `tier_served`. The last column,
`upstream_stop`, is why the upstream said its reply ended, in its own
words (`stop`, `tool_calls`, `end_turn`, a Responses reply's
`completed`), empty when it said none. A request's row on the Requests
page shows it as Upstream stop reason.

**Output speed** (tok/s on the Usage and Routing pages, `fogway usage`) is
output tokens over the time after the first token, summed per request: the
tokens of every timed request over the sum of their windows, so concurrent
subagents never add up. A reply that reasoned counts only its answer, from its
first text (`usage.DecodeOf`): hidden reasoning is written before the stream
shows anything, so counting it in a window that starts after it read
gpt-6.1-sol at hundreds of tok/s. A reply that reasoned and then wrote only
tool calls, one not streamed, and a burst (under 100 ms, or over 10,000 tok/s)
tell no speed. So does a reply its vendor held back and let go in a burst at
its end, however long the burst took to come: one whose content came in under
a quarter of its window (`flow_ms`, from a tenth of its bytes to nine tenths,
scaled to the whole), or that reasoned and whose answer would have come over
20 times faster than its reasoning came before its first text (its text came
with the burst). The time a burst takes to come is not the time it took to
write, and that time isn't seen. History is read the same way, as it keeps the
reasoning, the first text and the flow; a record from before the flow was kept
is told by its reasoning's pace alone.

A call read from an agent's session file rather than seen by the gateway has
no first token: Claude Code's file has only when the line that asked for a
reply and the reply's last line were written. Such a row on the Requests tab
shows the least its reply wrote at instead (`≥98 tok/s`): its output tokens
over that whole time, the wait for the first token in it, so the speed after
the first token is never under it. Only a reply of 1,000 tokens or more
written over a second or more shows one; a shorter reply's wait is too much
of its time to tell anything; a count of tokens bounds that share, a
duration doesn't (a 5 s reply can be 4 s of wait). Claude Code's, Claude
Desktop's and Codex's rows show one: Claude's line that asked for a reply is
written before it is sent, and a Codex call in a turn of several is timed
from what the model had last been given before the call's first output
item to its last, never from the output of a tool the call itself ran,
which an older rollout writes before the call's token count. OpenCode's rows
show none.
A call whose session file timed it (a Codex turn of one call) shows its own
speed when the time after its first token is a second or more and it wrote
100 tokens or more in it (`usage.CodexWindowMs`, `usage.CodexBodyTokens`).
That time can be one network push rather than the writing: a reply whose
first token came at 6.8 s and the rest 0.2 s later read 1,900 tok/s. When
that call reasoned, the file has its first token but not its first text, so
no speed is measured; it shows the least its answer wrote at instead: its
output less the reasoning, all written after the first token, over the time
from the first token to its end, held to the same second and 100 tokens. Its
whole output over that time would count reasoning written before the first
token and read high. A Codex turn of one call that tells neither shows the
least it wrote at over the whole turn, the wait for the first token in it
(`≥120 tok/s` for 1,200 tokens in 10 s), by the same rule as any floor over
a whole time: 1,000 tokens or more over a second or more. Under that the
wait is too much of the time to tell anything, so an 11-token answer in
2.75 s shows none, as an 815-token Claude Code reply does. Gateway rows and other agents' keep their own
speed. A floor never mixes into a measured speed: the Speed trend and
`fogway usage` count timed calls only.

A group's speed (a row of the Usage page's ranking, by model, provider or
agent, its Output speed card, and the overview's lists) says how many
replies it is of, by one rule: with replies whose speed was measured, their
speed and how many they are (`62 tok/s · 2 timed replies`, not beside "260
requests" alone); with rows that tell a floor, the least those wrote at and
how many (`≥100 tok/s · 30 replies`); with both, both, the one of more
replies first (`≥50 tok/s · 259 replies; 125 tok/s · 1 timed reply`), as a
few measured replies may not stand for the rest; with neither, nothing.
Counts are words, never a bare number in parentheses. The Output speed card
shows the first as its figure, its count beside it, the other under it, and
the mean wait for the first token on a line of its own with how many timed
replies it is of. The tooltip says what each is of. A ranking by speed
orders and bars a group by the figure it shows first (`usage.Totals`
`decode_n`, `floor_out`, `floor_ms`, `floor_n`; `usage.Row.Floor`).

It lists **accounts** too: each of a provider's keys, its tokens and cost,
by the key that actually answered — the one that took over after a
failover, the one `X-Fogway-Account` pinned. A key is named as the Routing
view names it (its name or fingerprint, never the key itself); the CSV adds
`provider_account` (JSON `providerAccount`), `fogway usage --csv --account
<name>` keeps one key's calls, and the app's Usage page has an Accounts list
and an Account filter on Requests. A record that names none appears as
**account not recorded**. OTLP export never carries the account.

The gateway issues **gateway keys** for clients, separate from a provider's
upstream API keys. Turn on **Settings → Share on local network**, then open
**Gateway → Gateway keys → Add gateway key**. This block appears only while
sharing is on. Create a named key for each client and copy it from its row.
To move from another gateway without touching its clients, enter the key they
already send in **Your own key (optional)** (8–256 characters, no spaces, not
starting with `sk-fogway-`, unlike any other key); left empty, fogway makes
one. The list shows only its last four characters, and it is counted,
limited and restricted as any other key, from this computer too.
Rename, disable, rotate or remove keys independently; rotation and removal
ask for confirmation. Rotation keeps the name, enabled state and usage
history; other keys are unchanged. While sharing is on, **Gateway → Connect**
offers loopback and shared addresses, plus enabled gateway keys, for all
connection examples. With sharing off, Connect keeps the original **API key**
field and local `fogway` token, without a gateway-key picker. Its arbitrary
local option is **This computer**, distinct from the named **Fogway** key.

For a headless gateway, use the CLI before exposing the port:

```sh
fogway gateway-key add "Remote laptop" # prints the new credential once
fogway gateway-key add Family --key -  # keeps a key clients already send, read from stdin
fogway gateway-key list                # ids, names, enabled state and masked keys
fogway gateway-key rotate <id>         # prints the replacement; identity stays the same
fogway gateway-key remove <id>         # revokes remote access
fogway gateway-key limit <id> week --tokens 2m --cost 5   # its own limit
fogway gateway-key limit <id>          # limit, used, left and reset
fogway gateway-key limit <id> 10d --tokens 5m   # every 10 days, from now
fogway gateway-key limit <id> reset    # count its usage from 0 again
fogway gateway-key limit <id> off      # no limit
fogway gateway-key models <id> openai/gpt-5 anthropic/*   # only these models
fogway gateway-key models <id> all     # every model
fogway gateway-key accounts <id> codex/me@example.com   # only these accounts
fogway gateway-key accounts <id> all    # every account
```

Each gateway key can have its own **limit**: a token total, an estimated
cost in US$, or both, per day, week or month (calendar windows in local
time: from midnight, from Monday, from the 1st), or every N days (#1509;
N from 1 to 3650). An N-day cycle starts when the limit is set and runs
N×24 hours, then the next one starts where it ended, so a restart, a
time-zone change or a daylight-saving shift doesn't move it. Changing only
the caps or **Count cache reads too** keeps the cycle; changing N or the
period, or turning the limit on, starts a new one then. Set it with **Limit** on
the key's row (saved with Save) or `fogway gateway-key limit`; the row shows
what the key has used, what is left and when it resets. Tokens counted are a
call's uncached input, output and cache writes, plus cache reads when **Count
cache reads too** is on. Cost is an estimate at the Usage page's prices; a
call with no known price adds none. A call counts in the window it started
in. Once a key is spent, its requests are refused before any provider is
asked, with a 429 in the API's own error shape that names the key, the
limit and the reset time, plus `Retry-After`; other keys are unaffected. The
counts are read from the usage log, so they survive a restart. A request in
flight holds a reservation (its body's size in tokens plus the key's mean
output per call), so requests sent at once overshoot by about one call; a
streamed reply is settled when it ends with the usage its vendor reported.
**Reset usage** in the key's limit editor (or `fogway gateway-key limit
<id> reset`) counts the key's tokens and cost from 0 together, from now. An
N-day key starts a new N-day cycle then. A day, week or month key keeps
its window's end (midnight, Monday, the 1st): only the calls before the
reset stop counting, so it doesn't drift off the calendar. The reset is
stored with the key (`since` in caller-keys.json, fogway's own: a value a
client sends is ignored), so it survives a restart; the usage log itself is
untouched. Keys saved before keep their JSON as it was.
A key can read its own status with `GET /v1/fogway/limit`; it is told its
own and no other key's, whatever the request names. Requests from
this computer that send no gateway key are not limited; a gateway key used
from this computer is.

A gateway key can also be held to some **models** (#882): pick them with the
**All models** badge on the key's row, or run `fogway gateway-key models <id>
openai/gpt-5 anthropic/*` (`all` takes the restriction off). A pattern is
`<provider>/<model>` or `<provider>/*`, matched against the provider that
serves the call, so a bare model name is resolved first, or a routing group,
`group/<id>` (`group/*` for every group). A key that names a group may use
the group with every member in it, though not those members asked for by
name. Such a key sees only its models in `/v1/models`, `/v1/codex/models`
and the Anthropic and Gemini lists, a routing group it doesn't name only when it may use every
member, and is refused any other model with a 403 in the API's error shape
before a provider is asked; a fallback it may not use is skipped. A key with
no models listed may use every model.

A gateway key can also be held to some **accounts** (#905): the accounts
and keys its requests may use, picked in the same menu right after their
provider's models — an account by who is signed in, a key by its
fingerprint — or with
`fogway gateway-key accounts <id> codex/me@example.com openai/<key id>`
(`all` takes the restriction off). The list holds a key to some accounts
**of the providers it names**: a provider it names no account of, the key
uses as it always did. The list keeps an account by its stable id, kept
through renames, shown by who is signed in; one gone later — a key
rotated, an account signed out, an account of an agent that keeps no
logins renamed — is kept as it is, matching nothing, so the key is held
closer, never wider. A routing group the key names is its members'
through the accounts it may use only: naming a group is not naming its
accounts, and fogway's own calls for the key — a web search, a picture
described, a Codex title — are of the models the user picked, but their
spend lands on an account, so the accounts hold them too. When every
account or key behind a model is one the key may not use, the request is
refused with a 403 in the API's own error shape naming the model and the
accounts it may use, before a provider is asked — after a usage cap's 429
and before an account barred for the model's — and `/v1/models` lists the
key only the models an account or key it may use serves, the Routing
view's left-out list marking what the key held out. A key with no
accounts listed may use every account, as keys always did.

While LAN sharing is enabled, or `FOGWAY_ADDR` puts the gateway on the
network (a host other than loopback or `localhost`: `0.0.0.0` in the Docker
image, a server's address), remote requests require an enabled gateway key
sent as Bearer, `x-api-key`, `x-goog-api-key` or `?key=`; one without gets
401. A request is remote when its peer isn't loopback, which includes
Docker's port publishing (the peer is the bridge, not the host's loopback)
and a WSL distro under NAT. Loopback remains permissive: any token works,
including a stale or disabled gateway key. Only a valid, enabled key is
attributed to its named identity. Neither shared nor on the network, a
remote request is refused (403). A `FOGWAY_ADDR` pinned to one interface's
address asks a key even of this computer's agents calling that address;
`0.0.0.0` with agents on `127.0.0.1` doesn't.
Sharing listens on every interface, unless `FOGWAY_ADDR` names a host of its
own: `FOGWAY_ADDR=127.0.0.1:3725` behind Tailscale Serve, or one interface's
address, stays where it is while shared, and what reaches it from elsewhere
still needs an enabled gateway key (#1112).

**Codex on another computer** (#1281) can name the shared fogway its model
provider, with a gateway key, and read fogway's models in Codex's own
catalog shape from `GET /v1/codex/models`. `/v1/models` is OpenAI's list,
which Codex can't read, and `/backend-api/codex/models` is the ChatGPT
backend's, for a Codex signed in to ChatGPT on the gateway's own computer;
a gateway key gets a 401 there.

```toml
model_provider = "fogway"
model = "provider/model"

[model_providers.fogway]
name = "fogway"
base_url = "http://192.168.1.20:3725/v1"
model_catalog_url = "http://192.168.1.20:3725/v1/codex/models"
env_key = "FOGWAY_API_KEY"   # the gateway key
wire_api = "responses"
supports_websockets = false
```

The list is the models the shared fogway shows Codex (its Agents page
picks), each as the `model_catalog_json` fogway writes for a Codex on its
own computer describes it, and only the models the key may use. Codex 0.161
reads it by default; 0.160 needs `[features] api_key_model_discovery = true`.
Codex reads at most 1 MiB of a catalog, and every entry carries Codex's
prompt, so past about 125 models the list keeps the first that fit and the
answer's `X-Fogway-Left-Out` header says how many it left out: pick fewer
for Codex, or hold the key to fewer.

A request that reaches loopback through a proxy or tunnel on this computer
(Cloudflare Tunnel's `cloudflared`, ngrok, Tailscale serve or funnel, frp's
HTTP proxy, Caddy, nginx) is someone else's, and is answered as one from
another machine (#1022): it carries a forwarding header (`Forwarded`,
`X-Forwarded-For`, `X-Real-IP`, `Cf-Connecting-IP`, `True-Client-IP`,
`Tailscale-User-Login`), which no agent sends. Without sharing it is refused;
with sharing, or a `FOGWAY_ADDR` on the network, it needs an enabled gateway
key, as from the network. This
covers every route: the model APIs, quotas, MCP sign-ins. To serve fogway
through a Cloudflare Tunnel, turn on **Settings → Share on local network**,
add a gateway key for each client, point the tunnel's service at
`http://127.0.0.1:3725`, and give clients the tunnel's address with that key.
A tunnel that forwards bare TCP (frp's `tcp`, `ssh -R`, socat) adds no header
and can't be told from an agent here: point it at this computer's LAN
address with sharing on, not at loopback. A proxy here that signs its
clients in itself can be trusted with `FOGWAY_TRUST_PROXY=1`, which keeps
what it forwards local, as before.

To let other computers in only from some addresses, list them as IPs and
CIDR ranges in **Settings → Network and sharing → Allowed addresses**, or in
`FOGWAY_ALLOW_IPS` (`203.0.113.7,10.0.0.0/8,2001:db8::/32`), which comes
first. A request from any other address is refused 403, even with an
enabled gateway key, and the refusal names the address it came from. This
computer is always let in. The address is the connection's own, never a
forwarding header, so behind a proxy or tunnel every client has the proxy's
address: limit them at the proxy. A mistyped entry in `FOGWAY_ALLOW_IPS`
lets nothing in rather than everything. When settings.json can't be read, or
doesn't parse and has no good backup, the list read last stays in force; if
none was read since fogway started, only this computer is let in until the
file is repaired. `fogway web`'s page honours the same list.

**Usage → Overview → Gateway keys** groups calls by the client's key, never
the provider's credential. **Usage → Requests** offers the same filter;
CSV includes `caller_key_id` and `caller_key_name`. Deleted keys keep their
history. The usual local `fogway` token and older records stay unattributed.
Caller attribution includes chat and embeddings.
Provider attribution stays independent; CSV puts `provider_key_*` before
`caller_key_*`.
An existing LAN key becomes **Fogway** without changing the credential.
`lanKey` remains in settings for older Fogway versions. Disabling or removing
the default key replaces that mirror with a random non-empty revoked value;
rotation does not re-enable it. Turning sharing off and on again neither
re-enables a disabled default key nor makes a removed one again; only
asking for a new sharing key makes one. The key store records migration completion,
even if `lanKeyId` cannot be saved, so reads do not keep retrying that write.
A migration write failure is logged without preventing gateway startup,
CLI key management, or the Settings and key-list pages from opening.
If settings are read-only, changing the default key fails without changing
it: make `settings.json` writable and retry so older versions cannot keep
accepting its old credential. Independent named keys remain manageable.
Gateway credentials stay in `~/.config/fogway/caller-keys.json` (XDG-aware,
mode `0600`), never in usage records or list responses.

Baidu Qianfan's [Token Plans](https://cloud.baidu.com/doc/qianfan/s/Dmrabu8b6)
are available as `baidu-qianfan`: a personal (个人版) and an enterprise (企业版)
plan and pay as you go, each with its own Chat Completions, Responses and
Anthropic Messages endpoints, and a key that works only on its own plan. Add
it with `fogway provider add baidu-qianfan <api-key>` — the id it carried its
first day, `qianfan-token-plan`, is taken too. The plans serve no model list,
so the preset carries their documented models; pay as you go serves its own
at `/v2/models`.

Alibaba's Token Plan is sold in two places, each with its own `sk-sp-` key
that only its own host takes: on the Qwen AI platform as `qwen-token-plan`
(`token-plan.maas.qianwenaiapi.com`), and on Alibaba Cloud Bailian as
`bailian-token-plan` (`token-plan.cn-beijing.maas.aliyuncs.com`). Both serve
Chat Completions at `/compatible-mode/v1` and Anthropic Messages at
`/apps/anthropic`, and carry the plans' documented text models for when the
host lists none. Neither plan has an API for its credits: the console's subscription page
shows them, so fogway shows no plan usage for these keys.

iFlytek Spark (讯飞星火) is `iflytek`, with four plans, each on a key that
only its own endpoints take:

| plan | `plan=` | endpoints | models |
|---|---|---|---|
| [Astron Token Plan](https://www.xfyun.cn/doc/spark/TokenPlan.html) (the default) | `token` | Chat Completions at `maas-token-api.cn-huabei-1.xf-yun.com/v2`, Anthropic Messages at `/anthropic` | Spark X2.5 (`spark-x2.5`), Spark X2 Flash and other makers' models by iFlytek's ids (`xopglm52` is GLM-5.2, `xopdeepseekv4pro` DeepSeek V4 Pro) |
| [Astron Coding Plan](https://www.xfyun.cn/doc/spark/CodingPlan.html) | `coding` | `maas-coding-api.cn-huabei-1.xf-yun.com`: `/v2`, Responses at `/v1`, `/anthropic` | `astron-code-latest`, whichever model its console has picked |
| [Spark X2](https://www.xfyun.cn/doc/spark/X1http.html) | `x2` | Chat Completions at `spark-api-open.xf-yun.com/x2` | `spark-x`; the key is the APIPassword on the console's X2 page |
| [Astron MaaS, pay as you go](https://www.xfyun.cn/doc/spark/%E6%8E%A8%E7%90%86%E6%9C%8D%E5%8A%A1-http.html) | `api` | Chat Completions at `maas-api.cn-huabei-1.xf-yun.com/v2`, Anthropic Messages at `/anthropic` | `spark-x2.6-preview`; any other inference service by the modelId on its card |

```sh
fogway provider add iflytek <api-key>             # the Token Plan
fogway provider set iflytek plan=api              # or coding, x2
```

None of them lists its models where fogway can ask (the Token Plan's
`/v2/models` is 404, pay as you go's is empty), so the preset carries each
plan's documented models; add another with `fogway provider models`.
models.dev has no entry for iFlytek, so their context windows aren't known:
set them on the provider (*Context window*, or `fogway model context`),
256K for Spark X2.5 as its doc gives it.

Pay as you go was tried with a real key (2026-10-14): `spark-x2.6-preview`
answered through fogway on Chat Completions (streaming and tool calls),
Anthropic Messages and Responses (which fogway sends as Chat Completions:
`/v2` has no Responses). A service published before 2026-01-10 is at
`/v1` instead; set `chat=https://maas-api.cn-huabei-1.xf-yun.com/v1`.
The Token Plan, Coding Plan and Spark X2 aren't tried yet: that key has
no order on the Token Plan and isn't taken by the others.

### Google Vertex AI

Google Vertex AI (`google-vertex`), which Google's documentation now calls
Gemini Enterprise Agent Platform, serves Google's Gemini models from your own
Google Cloud project, which is billed for them. It takes no API key: each
request is signed with a token minted from your Google credentials. First turn
on the Agent Platform API (`aiplatform.googleapis.com`) in the project (billing
has to be on there), then sign in with `gcloud auth application-default login`
or have a service account's key file
([Google's guide](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start/gcp-auth)).
[Where fogway looks for the credentials, and the kinds it reads](subsystems/providers-accounts.md#responsibilities-and-sources-of-truth).

```sh
fogway provider add google-vertex project=my-project
fogway provider set google-vertex location=us-central1        # global (the default), us, eu or a region
fogway provider set google-vertex credentials=~/keys/vertex.json
fogway provider set google-vertex impersonate=vertex@my-project.iam.gserviceaccount.com
fogway provider set google-vertex impersonate=                # empty clears it
fogway provider test google-vertex
```

`project=` is the project's id. `location=` is `global` (the default, and what
an empty value goes back to), `us`, `eu` or a region such as `us-central1`.
`credentials=` names a credentials file to sign with in place of gcloud's
sign-in, such as a service account's key; a relative path is made full from
the folder the command runs in. `impersonate=` has requests made as that
service account; the account signed in needs the Service Account Token Creator
role on it. The app's editor asks the same in **Project ID**,
**Location**, **Credentials file** and **Service account**; the TUI's `a` asks
for the project alone, and the CLI sets the rest. A key is refused, and so is
an address (`url=` and the like): the project and location make the only one
its token is sent to. Adding, listing and showing it ask Google nothing; the
credentials are read when a request is signed.

Each location serves models of its own, and Vertex AI has no list to ask which,
so fogway gives each location the models it answered with on 2026-10-06:

| Location | Models |
| --- | --- |
| `global` | `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.6-flash`, `gemini-3.5-flash`, `gemini-3.5-flash-lite`, `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`, `gemini-3.1-flash-lite`, `gemini-3-flash-preview`, `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.5-flash-lite` |
| `us`, `eu` | `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.6-flash`, `gemini-3.5-flash`, `gemini-3.5-flash-lite`, `gemini-3.1-flash-lite` |
| A region | `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.5-flash-lite` |

A region that serves one more, as asia-northeast1 serves `gemini-3.5-flash`,
has it typed in with the rest you want:

```sh
fogway provider models google-vertex gemini-2.5-pro gemini-2.5-flash gemini-2.5-flash-lite gemini-3.5-flash
fogway provider models google-vertex all     # back to the location's list
```

`fogway provider set google-vertex location=…` keeps the models picked, so pick
again after a move.

Google's [Priority PayGo](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/priority-paygo)
and [Flex PayGo](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/flex-paygo)
are asked for with request headers. The editor's **Headers** offers both,
`X-Vertex-AI-LLM-Request-Type` and `X-Vertex-AI-LLM-Shared-Request-Type`, to
add; their values are the ones Google's pages give. From the CLI:
`fogway provider set google-vertex header.<Name>=<value>`. [What was seen of
Priority PayGo](subsystems/providers-accounts.md#constraints-and-failure-behavior).

When a request fails:

- `no Google credentials at <file>`: there is no credentials file there. When
  it is the provider's own, correct `credentials=`, or clear it with
  `credentials=` alone; otherwise run `gcloud auth application-default login`,
  or set `credentials=`.
- `GOOGLE_APPLICATION_CREDENTIALS names <file>, which isn't there`, or
  `GOOGLE_APPLICATION_CREDENTIALS is "…", not a full path` (`CLOUDSDK_CONFIG`
  too): the variable names no file fogway can read, and gcloud's own isn't
  read in its place. Give the whole path (a `~/` at its start is the home),
  unset it, or set `credentials=`.
- `the Google credentials in <file>: the sign-in was refused`, with the advice
  to sign in again: the sign-in expired or was revoked. Run the command the
  advice names for that file: `gcloud auth application-default login` (with
  `--impersonate-service-account=` when the file impersonates one) for
  gcloud's Application Default Credentials, `gcloud auth login <account>`
  for a file under gcloud's `legacy_credentials`. A file of your own is
  written anew, or another one given with `credentials=`. The next request
  reads the new file, with no restart.
- A 403 `PERMISSION_DENIED` naming `aiplatform.endpoints.predict`: the account
  that signs, or the service account it goes as, may not use Vertex AI in the
  project. Give it Agent Platform User (`roles/aiplatform.user`) on the project
  ([the role](https://docs.cloud.google.com/gemini-enterprise-agent-platform/machine-learning/general/access-control#aiplatform.user)).
- `impersonating …: the sign-in was refused (403)`: the account signed in
  needs Service Account Token Creator (`roles/iam.serviceAccountTokenCreator`)
  on that service account.

### What a model costs

A call is counted at its **effective price**: what you set for that provider
and model if you did, otherwise what the provider's own catalogue lists, and
otherwise what models.dev lists for the model's maker. Out of the box that
last one is the whole story, and it is the wrong number for any provider that
does not charge list price — a relay reselling at a discount or a multiplier
is counted at whatever the model's maker charges. Say what a provider
actually charges, and the usage ledger and the session totals use that:

```sh
fogway model price relay-a/gpt-5.5                        # what it is counted at, and where that came from
fogway model price relay-a/gpt-5.5 0.12,0.60,0.01,0.15   # input,output,cache read,cache write
fogway model price relay-a/gpt-5.5 --reset                # take your price off this model
fogway model prices                                       # every model you priced
```

The four numbers are USD per million tokens. All four are asked for, because
a price missing one would understate the rest of every call; `0` is a model
served at no cost, which is a price, not the absence of one. Decimals take a
point or, in the app's boxes, a comma (`0,25`).

A call to a model no price is known for is counted at the price of the model
the reply said answered, at the same provider, when that one has a price: a
relay that sells `kimi-k3` as `moonshot-kimi-k3` answers as `kimi-k3`, so its
calls are counted at `relay-a/kimi-k3`'s price. A model with a price of its
own, `0` included, keeps it whatever its reply says.

A call with no price at all is left out of a total's cost, never counted as
free: the Usage page's sum says how many calls it couldn't price beside it
("≈$1.20+", then "3 calls unpriced"; Sessions, which prices by model, says how
many models), and `fogway usage` and the terminal app's Usage page say the
same after their totals. fogway doesn't guess a variant's price from a
similar name (`-auto`, `-max`, `-fast` can each cost differently); give it
one with `fogway model price`, or in the provider's editor under the model.

A **Kimi Code** membership's models are counted at the Kimi API model each
one is, not at the $0 models.dev lists them at for the plan: `k3` and
`k3-256k` at `kimi-k3`, `kimi-for-coding-highspeed` at
`kimi-k2.7-code-highspeed`. `kimi-for-coding` is K2.8 Preview, which the API
doesn't sell, so it has no price until you give it one
(`fogway model price kimi-code/kimi-for-coding …`).

A **fifth number** is a 1-hour cache write's price. Anthropic bills a cache
write kept for 5 minutes at 1.25× input and one kept for an hour at 2× input,
and its usage says which were which (`cache_creation.ephemeral_5m_input_tokens`
/ `ephemeral_1h_input_tokens`, from the API and from Claude Code's
transcripts). The fourth number is the 5-minute price; the fifth, when not
given, is 2× input for a Claude model — Anthropic's rule. Any other model has
no 1-hour price unless you give one: a 1-hour write it reports is counted at
the 5-minute price, and neither `fogway model price` nor the app's boxes show
a 1-hour price for it. A call recorded
before fogway kept the split counts all its writes at the 5-minute price, as
it did.

A **long-context price** is what the whole request costs once its prompt is
over a size, as OpenAI bills gpt-6-astra, gpt-6.1-sol and gpt-6-luna over
272K input (2× input and cache, 1.5× output):

```sh
fogway model price openai/gpt-6-astra 10,50,1,12.5 --tier 272k 20,75,2,25
```

The size counts the prompt the way OpenAI does: uncached input + cache reads
+ cache writes, and the tier applies when that is **over** the size (272,000
is not, 272,001 is). Then every part of the call — input, output, cache — is
at the tier's price, not only the tokens past the size. `--tier` may be given
again for another size. The built-in prices carry a tier only where
models.dev lists one (`cost.tiers` of type `context`), as it does for those
three. Session and day totals sum many calls, so they are counted at the base
price; the ledger and each request are counted at the tier they reached. In
the app, a model's price row has a quiet **Long-context price** row under it,
shown with the list's tier greyed in, or behind a link where there is none.

The order a price is looked for in is: **the price for this model → the price
for `<provider id>/*`, which covers every model of that provider → what the
provider's own catalogue lists → what models.dev lists for the maker.**
`--reset` removes the first, and says so when a `<provider id>/*` price is
still in force; reset that one by name to take it away too.

A price is **one provider's tariff for one model**, not the model's own: the
same model through two providers is two prices, and each keeps its own.
Nothing an agent can see changes. The model list, the agents' own settings
and the pickers that choose a model for a background task — an image, a web
search, a description — all still work from the catalogue; only the cost
reports read the effective price.

Two things worth knowing. The ledger and the session totals re-price when they
are read, so adding or changing a price restates earlier figures: they are
estimates at the effective price, not settled charges. And a price is per
provider and model. Records now identify upstream API keys, but a provider
charging different tariffs per key still cannot be costed exactly from a
single provider-wide price.

### What a model takes

A provider that serves a model models.dev does not list, or lists at the
wrong size, has a window and a reply limit fogway cannot know. Say what they
are:

```sh
fogway model context relay-a/gpt-5.5 262144    # the window a request may hold, or 1m
fogway model output  relay-a/gpt-5.5 131072    # the most a reply may hold, or 128k
fogway model context "relay-a/*" 200000        # every model of that provider
fogway model output  relay-a/gpt-5.5 --reset   # take your limit off this model
```

The `*` is quoted because zsh treats a name it cannot expand as a command
that failed, rather than passing the name on as bash does.

Both are looked for in this order: **this model → this provider's `*` → the
provider's own list → models.dev.** `--reset` removes only the value this
model has of its own.

A provider you keep unlisted, or switch off, takes them like any other: the
numbers are kept, and are what its models take once it is serving again.

A window is a number agents are shown **and a routing input**: at 95% of the
window a request held on a routing-group member moves to one that takes more,
so overstating a window makes that move happen too late. A reply limit is
advertised in `/models` and is what a group advertises the smallest of. The
gateway doesn't lower a limit an agent sends by it. A request that names none
and goes to Anthropic's Messages API, which needs one (Codex's Responses turns,
a Chat client's), is sent it as `max_tokens`, within the window as agents are
told it. Only a model whose limit nobody knows goes at 16384. A reply that
runs into it ends with `stop_reason: max_tokens`, which a Responses client
reads as `incomplete_details.reason: "max_output_tokens"` (#1792).

Saving a window or a reply limit writes that number into the model lists fogway
keeps in the agents' own files — Pi's `contextWindow` and `maxTokens`, OpenCode's
`limit`, and Crush's, droid's and Cline's — which an agent reads
at start-up. A session already running therefore keeps the window it began with,
while the gateway's own `/models` and every request from then on are right at
once.

### The name a vendor knows a model by

A relay often serves a model under an id of its own — a prefix it namespaces
with, a dated name, a `-latest` that is not what models.dev calls it. Say
which name to ask for:

```sh
fogway model wire relay-b/model-2                           # what the vendor is asked for
fogway model wire relay-b/model-2 vendor-c/model-2-preview  # ask for it by this
fogway model wire 'relay-b/*' 'vendor-c/*'                  # every model, * being the model
fogway model wire relay-b/model-2 --reset                   # ask for it by its own name again
fogway model wires                                          # every name your vendors are asked for models by
```

Quote the arguments with a `*` in them: zsh reads a bare `*` as a glob and
answers `no matches found`. In the name, `*` stands for the model itself, so
`'relay-b/*' 'vendor-c/*'` sends `vendor-c/model-3` for `model-3` and
`vendor-c/model-2` for `model-2` — one name for a relay that namespaces its
models, each still asked for by its own. A name with no `*` in it sends every
model of that key under that one name, which is the right answer only for a
relay that does serve them all alike. The model's own key wins over the
provider's, and `--reset` takes away only the one it is given, so resetting
`relay-b/model-2` while `'relay-b/*'` is set leaves the provider's name in
force — the CLI says which of the two is in force after every change, naming
the models a name for the whole provider leaves to their own, and
`--reset` over a name that was never given says so rather than ticking a
removal that took nothing away.

Only the request that goes out carries that name, in the model field of a
chat, Responses or Anthropic Messages request and of an Anthropic token
count; image requests are left with the name fogway knows the model by.

Everything else keeps the name fogway knows the model by: the catalog agents
pick from, the routing groups' membership, `GET /models`, and what a call is
recorded and priced as. What the vendor's own reply said answered is kept
beside that, in the ledger's `served_model`, which is what it is for, and it
is compared with the name the vendor was asked for — so a model answered
under the relay's own id is not read as a swap. That is against the names
in force when the ledger is read, since a record keeps what the vendor
answered and not the name the request went out under: naming a model after
the call re-judges that call, which is then left reading as a swap. What a
provider *supports* — whether it takes a temperature, which reasoning levels
fit — is still asked about the model fogway knows, so a rename upstream does
not change how fogway behaves towards the model.

An upstream name is **one provider's**, not the model's: another provider
serving the same id is asked for it under its own name, or this one. The
model it is given for has to be one that provider serves — `<provider>/*`
is the way to say one for every model — because a name for a model fogway
would never ask the provider for is not a name of its own: the model is
asked for by the name fogway knows it by, and nothing anywhere would say the
name given for it is not the one in force. A relay that serves a model under
an id of its own *and* lists it does get the name; what is refused is a
model the provider's list does not have at all. The model test uses the name
too, so a relay that only knows its own ids does not report a working model
as broken. An image model is the exception that follows the rule above: it is
tested by the name fogway knows it by, on the images API and on the chat it
falls back to, because that is how a drawing is asked for.

### Routing groups

A routing group is several models, from one provider or many, that an agent
picks as one: `group/<id>`. The gateway routes each request over every
member's keys and accounts together. A model two of your providers serve
under the same name becomes a group on its own; the Routing view in the app
and `fogway group` make any other:

```sh
fogway groups                           # yours, then those fogway found
fogway group add "Opus anywhere" models=claude/claude-opus-5-5,copilot/claude-opus-5.5 routing=order stays=session
fogway group opus-anywhere              # one group, its models in order
fogway group set opus-anywhere models+=openrouter/anthropic/claude-opus-5.5 routing=usage
fogway group set opus-anywhere models-=copilot/claude-opus-5.5
fogway group set opus-anywhere routing=weight weight=claude/claude-opus-5-5=3,copilot/claude-opus-5.5=1
fogway group copy opus-anywhere "Opus test"  # the same models, routing and rules, to change on its own
fogway group rm opus-anywhere           # one fogway found is hidden; fogway group restore <id> brings it back
fogway claude group/opus-anywhere       # use it
```

`routing=` is `smart` (the default: of the keys on plans with quota to
spare, the one whose allowance renews soonest first), `order` (the first
model until it can't answer, then the next), `rotate` (each turn to the next
member), `usage` (least used first) or `pace` (weekly pace: the account with
the most of its week left per hour until it renews first, so less of a week
is lost at its reset) or `weight` (each model takes its share of the
requests by its weight, smoothly interleaved: `weight=claude/claude-opus-5-5=3`
gives it three requests for every one a model left at weight 1 takes; a
weight is 1 to 1000, `weight=` alone clears them all, and a group in the
group counts as one member, `weight=group/fast=2`). A key has no allowance to weigh, so it goes by its
order or by what fogway sent it lately — except a sub2api key its owner gave
a 5-hour, day or 7-day limit: with the provider's Balance URL set to the
relay's `/v1/usage`, its card shows those windows and routing weighs it by
them as it does a plan, resting it till the window renews once the
relay says `api key 7天限额已用完` — or till its windows are next read short
of full, its limit raised or its usage reset. A key bought as a plan — Kimi Code, GLM Coding Plan
or Z.ai, MiniMax Coding Plan, OpenCode Go, Command Code, Ollama Cloud — is weighed by its
plan's windows the same way. With one key or account, the Routing page still
shows what it has left. `stays=` is how long a conversation
stays with the key or account that answered it: `auto` (the default, while
the vendor's cache of it is worth keeping), `session`, `turn`, `off` or
`strict` (never moved to another key or account, even past its usage cap:
its errors go to the agent).
`models=` replaces the whole list, in order; a bare model id works when only
one provider serves it.

`listed=no` (the group editor's **Model lists: The group alone**) keeps the
models a group names, and those its patterns match, out of `fogway models`,
`/v1/models` and every agent's list, so a client offers the group alone.
They are still served: the group routes over them, and a request naming one
as `provider/model` is answered. A group in it keeps its own setting, and a
group switched off keeps nothing out. `listed=yes` lists them again. To keep
a whole provider for groups, use its **Only through routing groups** switch
instead.

The Routing page's Requests list defaults to the time-ordered By request view.
Choose By session to group calls by the agent's session ID; the page remembers
your choice across reloads.
Codex title helpers with an explicit parent or fork source join their originating
chat, retaining their title badge and contributing to its cost. Titles without
ancestry and ordinary forked chats stay separate.
Codex chat names come from its local name index and follow renames. Other
agents' sessions (Claude Code and the rest the Sessions page reads) take the
name the Sessions page shows for them, read from their own files on this
computer. Unknown or remote-only names fall back to the ID; the full ID
remains in the heading tooltip.
Expand a session to see each request. Each request and session shows its estimated cost at the effective model
prices, including cache reads and writes. Session totals cover the listed
requests only (the live trace or the selected day's retained history), and a
`+` marks a partial estimate. Calls without a session ID are listed separately;
old history without token tiers, or a model without a known price, shows `—`.

Opening the Providers page automatically checks Alma, CC Switch, Claude Code
and Codex for local provider configurations that can be imported. With no
providers yet, a hint shows how many were found and which apps they came from;
otherwise a small **Import local configurations** link sits beside **Add
provider**. Both open the existing picker with only the offered configurations
selected; nothing is added until you confirm the selection. **Ignore** remembers
the offered configurations on this device across window restarts; new or changed
configurations can be offered again. **Add provider → Import…** still includes ignored configurations.
Discovery runs in the background, at most once a minute when opening or
refreshing the page, and refreshes after an import. Configurations already in
fogway, entries that cannot be imported, providers turned off in their source
app, and ID collisions that cannot join as another key are excluded from the
hint. The latter two remain available through manual import. Counts refer to
configurations in source apps, which may include the same provider
in more than one app. Only app names and opaque configuration fingerprints
reach the automatic hint; ignored fingerprints are kept in local browser storage.

The app's Import from other apps dialog can copy providers from Claude Code's
`settings.json` (`CLAUDE_CONFIG_DIR` when set) and Codex's `config.toml`
(`CODEX_HOME` when set) into fogway. Codex imports custom
`[model_providers.*]` entries with an inline `experimental_bearer_token`,
including fixed headers for custom providers in
`[model_providers.*.http_headers]` and models from
`[profiles.*]` or `model_catalog_json`. Review the entries before importing;
subsequent changes to agent settings are not automatically copied to fogway.
Entries that point back to fogway or only name an `env_key` are skipped.

### How fogway picks an account

When a provider has several keys on, or a routing group has several
members, the gateway orders them for each request in these steps. The Routing page shows the
result for every request: who went first, and why.

1. **Who can answer.** The keys that are ticked, unless paused. One set
   not to serve the model is never tried, and one whose plan doesn't list
   the model is tried only when no other lists it.
2. **The routing.** What is left is put in order by the routing the
   provider or group uses (below).
3. **Rate limited ones last**, only with *Sink* on: an account a 429 rate
   limited while it still had quota goes behind every one not rate limited
   since.
4. **The provider's fallback models** come after its own accounts.
5. **Resting ones last.** An account resting after a failure goes to the
   back, behind every fallback. It is never dropped: when it is the only
   one, it is tried anyway.
6. **The conversation's account first.** Once an account has answered a
   conversation, it is moved to the front for the conversation's next
   requests, as *Stays* says. In auto mode that is within a turn, and
   across turns while at least 1,024 tokens of the last request were read
   from the vendor's cache and it answered in the last 5 minutes. This
   never brings back an account that is resting, nor one that has used 98%
   of a window (100% In order). *Stays: Strict* does, and asks no other:
   the conversation stays on its account, resting, spent or past its cap,
   and its errors go to the agent.

So routing decides who goes first only for a conversation nobody has
answered yet. An account that has used 90% goes behind the others for new
conversations, but a conversation already on it stays there until 98%.

**The routings.** Smart, Least used first and Weekly pace go by the share
used of the fullest window that counts the model, the 5-hour window
included. Smart and Weekly pace put an account behind the others once a
window is at 90% or more, and behind those once one is at 98% or more,
each of these groups ordered by the share used.

- **Smart** (the default): of the accounts under 90%, the one whose
  allowance renews soonest goes first, because what it has left is lost at
  the reset. The biggest window decides: the week, then the 5 hours only
  when the weeks renew in the same hour. Reset times are compared to the
  hour, by the clock: 13:55 and 14:05 are different hours, 14:05 and
  14:55 the same. One with no week goes by its 5 hours. Credits that run
  out before their window renews, or that have no reset at all, count as
  renewing when they run out. One not known yet goes after those known.
  One with a month beside its week goes by its month: when the month
  holds less than its weeks add up to, what a week leaves isn't lost at
  the week's reset, as the month still has it to spend later. Its row
  on the Routing page says the fullest window's share with when that
  window renews: "32% used · renews in 3 days" is the week's.
- **In order**: the accounts in their order. The first takes every request
  until it is used up (100%) or fails.
- **In turn**: each turn of a conversation goes to the next account. A new
  conversation goes to the account the fewest other conversations of the
  last 30 minutes are on.
- **Least used first**: the lowest share used first; then the account
  fogway sent the fewest tokens lately (half of them stop counting after an
  hour). An account not known counts as unused.
- **Weekly pace**: the account with the highest pace first. Pace is the
  share of the week left divided by the hours until the week renews (at
  least 1): 60% left with 20 hours to go is 3 per hour. With two weekly
  windows that count the model, the lower pace counts. An account with no week goes by its 5 hours. Accounts whose
  pace is within a tenth of the highest in their band count as alike, and
  among them the one fogway sent the fewest tokens lately goes first. One
  not known counts as a fresh week.

**Make first** sets the accounts' order. How much that order counts
depends on the routing:

| Routing | What the order decides |
| --- | --- |
| In order | Everything: the first takes every request it can. |
| In turn | Where the rotation goes next. |
| Smart | Only between accounts whose windows all renew in the same hours, or whose shares used are equal. |
| Least used first | Only between accounts with the same share used and the same tokens sent lately. |
| Weekly pace | Only between accounts in the same pace band that fogway sent the same tokens lately, in practice none since fogway started. |

**When an account runs out.** A 429 that says the allowance is used up
rests the account until it is back, and the same request goes to the next
candidate at once, as long as none of the reply has been sent. How long it
rests comes from the first of these that is known: the reset time in the
refusal, the reset of a window fogway last read as used up (98%, 100%
In order), the vendor's `Retry-After` or rate-limit reset header (an hour
at most), else 15 minutes. It is never longer than 8 days. The account's
windows are read again right away, and once a reading finds the window it
filled started again (its reset gone by), it is back at once, not at the time the refusal
named. A five hours started again while its week is still used up doesn't
bring it back. A 429 that is a short rate limit rests
the account for as long as the vendor asks (an hour at most), or a
minute, doubled each time it comes back right after its rest, up to 30
minutes. Out of credit rests half an hour. Any other failure rests a minute, longer each time it fails
again, up to 10 minutes; a plan's key that fails with a window used up
rests until that window renews.

**When the safety filter refuses.** A reply the vendor's safety filter
refuses before any of it is said rests nobody: it is about what was asked,
not the account. By default the same request is asked of the model's other
accounts, then the routing group's other members, until one answers.
Settings › Models › Replies › *Ask again after a refusal* sets how many more
are asked: 1, 2 or 3, or *Never ask again*. When nobody else is asked, an
agent on Anthropic's Messages API (Claude Code) gets the refusal as the
vendor said it, `stop_reason` `refusal` with its category, which Claude Code
answers by switching to its own fallback model, as it does on Anthropic's
API. Codex and Chat clients get a 400 saying the model refused the request,
which they don't ask again for. Any other 400 is never asked again.

**How often an allowance is read.** Routing uses what was read last and
never waits for a reading, except the first one after fogway starts (3
seconds at most). A reading over a minute old is read again in the
background as a request is routed, and an account that fails for its
quota is read again at once; if a reading was already under way as it
failed, the account is read again as soon as that reading is back. A
reading the Usage page made counts as one, its age from when it was made.
A plan's windows are read from the vendor this way.

### Connecting anything else

The gateway listens on `127.0.0.1:3725` (`FOGWAY_ADDR` changes it) and starts
with the app; `fogway serve` runs it alone. For reverse-proxied or container
deployments, set `FOGWAY_PUBLIC_URL=https://fogway.example.com` to the base
URL shown in the console and CLI, including connection examples. While
other machines are let in (Share on local network, or a `FOGWAY_ADDR` on
the network), the Gateway page's Connect starts at it until you pick
another address there, Settings › About lists it beside the local URL, and
`fogway web` shows it as the gateway's address. Local agent configs still
use the local gateway address.

Building an app or agent that should use fogway, or get a row on the
Agents page: see [Integrating your app or agent](integrating.md).

External clients behind a reverse proxy need an enabled gateway key
(Gateway → Gateway keys): with Settings → Share on local network on, or a
`FOGWAY_ADDR` on the network, fogway asks for one; otherwise it refuses
them, unless the proxy authenticates its clients itself
(`FOGWAY_TRUST_PROXY=1`, below). A public URL with no port of
its own — a reverse proxy's `https://fogway.example.com` — is the address
`fogway web` prints for its own page too, so the proxy must forward `/v1`
and `/v1beta` to the gateway's port and the rest to the page's. When the
proxy and fogway run on the same machine, a request it forwards over
loopback carries a forwarding header and needs a gateway key as one from
the network does (see [Providers and the gateway](#providers-and-the-gateway)); a proxy that
authenticates its clients itself can be trusted with `FOGWAY_TRUST_PROXY=1`.

It exposes:

| Path                     | API                        |
| ------------------------ | -------------------------- |
| `/v1/chat/completions`   | OpenAI chat completions    |
| `/v1/responses`          | OpenAI Responses (HTTP, streamed as SSE; a WebSocket upgrade is answered 426) |
| `/v1/messages`           | Anthropic Messages         |
| `/v1/messages/count_tokens` | Anthropic token counting |
| `/v1beta/models/{model}:generateContent` | Google Gemini (also `:streamGenerateContent`, `:countTokens`) |
| `/v1/models`, `/v1beta/models` | the catalog            |
| `/v1/codex/models`       | the catalog as Codex's `model_catalog_url` reads it ([Codex on another computer](#providers-and-the-gateway)) |

Each `/v1/models` entry includes `reasoning` and `supported_reasoning_levels`
(`[{"effort":"low"}, ...]`). A routing group is marked `reasoning` when any
member thinks, even where it offers no levels to pick from (a member known
to take none still leaves the group none); `supported_reasoning_levels`
lists only the levels every member supports, as before. A member fogway
knows does not think is sent no reasoning ask when the group routes to it,
so the effort the group is asked for reaches the members that take it and
doesn't turn a vendor away as a 400; a member nothing speaks for is sent
what the agent asked, as before. `native_endpoints`
(`["/v1/messages"]`) names the APIs a request for the model is passed
straight through on; it is left out of a routing group, and of a model every
request to which is translated anyway.

Requests pass straight through when the vendor speaks the agent's API and
are translated otherwise, streaming, tool calls and reasoning included. The
key is `fogway` (any value works; the gateway only listens on loopback), and
models are named `provider/model`.

- A routing group is asked by its id (`group/<id>`) or its name.
- A bare model id (`glm-5.3`) works too. When several providers serve it,
  it goes to the routing group fogway found for it, which asks them in the
  order of the Providers page and falls over to the next. With found
  groups off, it goes to the first of them in that order alone.
- Every reply names who answered in `X-Fogway-Provider` (the provider)
  and `X-Fogway-Model` (`provider/model`), and so does the usage log.

Anything with a base-URL setting can use it:

| Tool speaks | Base URL                   | Environment                                   |
| ----------- | -------------------------- | --------------------------------------------- |
| OpenAI      | `http://127.0.0.1:3725/v1` | `OPENAI_BASE_URL`, `OPENAI_API_KEY=fogway`      |
| Anthropic   | `http://127.0.0.1:3725`    | `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=fogway` |
| Gemini      | `http://127.0.0.1:3725`    | `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY=fogway` |

An optional `X-Fogway-Account: <key>` header (a key's name as the Routing
page shows it, or its id there) pins a request to one key of a provider
with several: only it is tried, and an unknown key, one whose plan lacks
the model, or one resting is an error rather than another key's reply. The header is not sent on to the vendor.

A status bar can show where a turn went before its first token arrives:
send `X-Fogway-Session: <id>` with the requests (an agent's own session
header, such as Pi's or Claude Code's, works too) and read
`GET /v1/fogway/route?session=<id>`. It answers the session's latest
request as routing has it so far — `asked` (the model the agent named),
`group`, `rule` (the group's rule that matched, and `rule.pick`, the
effort its classifier picked), `model` and `effort` (the member being
tried now, as `provider/model`, and the reasoning it was sent at), and
`tries`, one per member tried, each failed one a fallback with its `fail`
— then `done`, `status` and `served` once the reply is over; `route` is
`null` before the session has one. The route appears once routing has
decided, before the vendor is asked. `after=<seq>&wait=<seconds>` (up to 60)
holds the answer until the route changes past the `seq` of the last one,
so a UI can follow a turn with one request at a time. Only the session
named is told; like `/v1/fogway/quotas`, it answers this machine, and
another only with the key of a gateway shared on the local network. A key
held to a budget, to some models or to some accounts is told the routes of
its own requests alone: another key's session answers `route: null`.

To show the one in use without a session, `GET /v1/fogway/quotas` (and
`fogway quota --json`, the same list) has `lastServedAt` on each
plan and key that answered a request through the
gateway in the last 30 days, and `last: true` on the latest. It is kept
in `served.json` beside `providers.json`, so a restart keeps it.

A gateway key held to a budget, to some models or to some accounts is told
none of this fogway's accounts (#1981): `/v1/fogway/quotas`,
`/v1/fogway/quotas/history`, `/v1/fogway/quotas/cards`,
`POST /v1/fogway/quotas/refresh` and `/v1/fogway/concurrency` answer it
403 with `"code": "gateway_key_held"`, from this machine or another. It
reads its own limit from `GET /v1/fogway/limit`. Another fogway that uses
such a key shows only that key's own card.

An app of your own can read the Usage page's numbers too.
`GET /v1/fogway/usage?period=<today|week|month|all>` is the page's summary,
the same JSON as the window's `/api/usage` (`path` is left out for another
machine), with every session in `sessions` where the window's lists the
top 100 by tokens; `sessionCount` is how many there are. `GET /v1/fogway/usage/requests` is its requests a page at a time
(`offset`, `limit` up to 500), with the same filters as the Requests log:
`period`, `day`, `agent`, `provider`, `model`, `account`, `callerKey`,
`purpose`, `failed`, `retried`, `q`, `via`, `computer`. What was said in a call is not
in either. Like `/v1/fogway/quotas`, they answer this machine, and another
only with a gateway key. A key held to a budget, to some models or to some
accounts is told its own calls alone: `/v1/fogway/usage` refuses it with a
403, and `/v1/fogway/usage/requests` keeps only its rows and filter lists.

An app that calls its vendor itself, not through fogway, can report those
calls so the Usage page counts them: `POST /v1/fogway/usage/requests` with
`Content-Type: application/json`. The body is one record or an array of up
to 5000 (32 MiB at most), shaped like a `usage.jsonl` line:

```sh
curl -s http://127.0.0.1:3725/v1/fogway/usage/requests \
  -H 'Content-Type: application/json' \
  -d '[{"t":"2026-10-13T08:00:00Z","agent":"my-app","provider":"anthropic",
        "model":"claude-sonnet-4-6","in":1200,"out":340,"cache_read":9000,
        "cache_write":0,"ms":2400,"ttft_ms":610,"status":200,"rid":"req_01"}]'
```

- **Fields:** `t` (RFC 3339), `agent`, `provider`, `model` and `rid` are
  required. `in`, `out`, `cache_read`, `cache_write`, `cache_write_1h`,
  `reasoning`, `ms`, `ttft_ms`, `status`, `cost` (USD), `effort`,
  `session` and `err` may be left out.
- **Pricing:** a record without `cost` is priced at fogway's prices for
  its provider and model.
- **Past times:** `t` is taken as given, however far back, so a history
  can be backfilled; it may not be before 2000-01-01T00:00:00Z or more
  than an hour ahead.
- **Repeats:** a `rid` already reported is skipped, after a restart too,
  so a batch can be sent again.
- **Refusals:** a bad record refuses the whole post with a 400 naming it,
  as `error.index` and `error.param` (`record 3: in: must not be
  negative`), and nothing is written.
- **The reply:** `{"accepted":1,"skipped":0,"records":[{"index":0,"rid":"req_01","status":"accepted"}]}`.
- **Showing:** reported rows carry `source: "reported"`, count in every
  total, and can be listed alone with `via=reported` (`reported` in the
  GET's reply has their totals).
- **Nothing live:** they never count against a gateway key's limit or a
  budget, a plan's window, an alert, a rate limit or routing.
- **Who may post:** the same as the GETs: this machine, or another with a
  gateway key. A web page needs a key even on this machine.

A row's `session` is the conversation it belongs to. A Claude Code
subagent's call keeps its parent conversation there and adds `subagent`,
its agent id (the `agentId` of its lines and its
`<session>/subagents/agent-<id>.jsonl` file), and `parent_agent`: the
subagent that started it (Claude Code's `x-claude-code-parent-agent-id`, or
the `parentAgentId` of the subagent's `.meta.json`); none when the main
conversation did.
Records written before these fields came have neither. Codex's sub-agents are told
apart by `kind` only.

`fogway quota wait <provider|key>` blocks until that provider (any of its
keys fogway has on) or that one key has allowance again — no
window that stops it used up — then exits 0, so a long task stopped by its
limit can go on unattended:

```sh
until codex exec "…"; do fogway quota wait codex || break; done
```

It reads the vendors itself, whether or not the gateway runs, and again
shortly after the soonest reset it knows (every 1 to 10 minutes; an
allowance it can't read is asked again less often each time), saying on
stderr what it waits for and until when. Name a key as its card names it,
or as `<provider>/<name>` when two providers share the name.
`--timeout 6h` exits 1 if it passes first, `--quiet` says nothing; an
unknown name exits 2 and Ctrl+C 130.

The Usage page's cards and the menu bar panel's *Allowances* tab are in one
order: drag a card's logo on the Usage page (or Alt+arrow keys on it), or
press *Arrange* at the foot of the panel's tab and move the rows there. The
same *Arrange* hides a card from the panel's tab (*Hide*, *Show*
puts it back), and the tab's foot says how many are hidden. Hiding is the
panel's only: routing, the Usage page and the menu bar's own cells
still have it, and a menu bar cell for it still opens its card. Both are
kept in fogway's settings (`usageOrder`, `panelUsageHidden`). Which
allowances the menu bar itself shows beside its icon is picked in Settings.
The same list has *Tokens and cost today* and *Tokens and cost, last 7
days*: every agent's requests as the Usage page's Overview sums them, the
tokens over the cost in your currency (`usage:today`, `usage:7d` in
`trayUsages`). Clicked, one opens the Usage page at that period.

The *Gateway* tab in the app has this as copy buttons and ready-made
snippets (shell, curl, Python, Node) for each API, the list of model ids,
and the recent calls; `FOGWAY_DEBUG=1` logs every call to the terminal.

**Claude Code** gets `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` and the
model variables in the `env` block of `settings.json`; picking a native
model (`opus`, `sonnet`…) removes them and restores whatever was there.
A key in `ANTHROPIC_AUTH_TOKEN` signs Claude Code out of claude.ai while it
runs through fogway: claude.ai's plan limits in `/usage`, its connectors,
voice and `/teleport` are off. Set its sign-in to claude.ai to keep that
login: fogway writes the token empty, Claude Code sends fogway its claude.ai
sign-in, and fogway never passes it on (nor its `oauth-2025-04-20` beta). A
provider that refuses fogway's own key is then reported to Claude Code as
a 502, not as its sign-in refused, so Claude Code doesn't log out. Remote
Control and ultrareview stay off: Claude Code has them only on Anthropic's
own address. The choice is offered where the gateway takes any key, so not
from a WSL distro under NAT while fogway is shared on the network.

**Codex** gets a `[model_providers.fogway]` table, `model_catalog_json`
pointing at `~/.codex/fogway-models.json` (written from the catalog, so the
models show in Codex's own list) and a valid `model`/`effort`; picking a
native model or disconnecting removes `model_provider` and the catalog. The
`[model_providers.fogway]` table stays, so a thread started on fogway can
still be opened. Codex won't load its config at all when `model_provider =
"fogway"` has no table ("Model provider `fogway` not found"). If another
tool leaves that state behind, fogway writes the table back the next time
it syncs. Signed in to ChatGPT (the sign-in field's default), Codex keeps
its own provider and sign-in, and fogway's models join its list through
`openai_base_url`. fogway becomes Codex's provider then only while the
Codex app holds the account: OpenAI no longer allows it and it has no
credits left, except a workspace account still within its overage. The
app sends nothing for a held account, so fogway steps in, and it steps
back once the account has room again. A window at 100% with credits left
doesn't count: Codex stays signed in and keeps sending on those. Your
ChatGPT sign-in is never touched.
Codex reads its model list at start-up, so restart it after a switch.

**OpenCode, Pi, Crush** get a `fogway` provider entry and `fogway/provider/model`.

**Gemini CLI** switches `auth` between API key, Google account and Vertex;
the API key goes to `~/.gemini/.env`. Picking a catalog model points
`GOOGLE_GEMINI_BASE_URL` at the gateway (which speaks the Gemini API), sets
`auth` to API key with the gateway token, and names the model in
`settings.json`; a native model puts the previous auth back.

### Import links

A vendor or relay can hand its users a ready-made provider as a link:

```
fogway://import?preset=deepseek&key=sk-…
fogway://import?name=Acme%20Relay&chat=https://api.acme.example/v1&anthropic=https://api.acme.example&key=sk-…&models=gpt-5.5,claude-sonnet-5
```

Opening one brings up fogway with what the link would add: the name, the
hosts your prompts and key would go to, the models. Nothing is saved until
you press *Add*. `fogway import <link>` does the same in a terminal.

| Parameter   | Meaning                                                            |
| ----------- | ------------------------------------------------------------------ |
| `preset`    | a preset id (`fogway presets`); its endpoints are used. Not `google-vertex`, which is asked at your own Google Cloud project |
| `region`    | with a preset that has regions, which one                          |
| `name`      | the provider's name; required without a preset                     |
| `id`        | its id; derived from the name when absent                          |
| `key`       | the API key; the user pastes one when absent                       |
| `chat`      | OpenAI Chat Completions base URL (`…/v1`)                          |
| `responses` | OpenAI Responses base URL (`…/v1`)                                 |
| `anthropic` | Anthropic Messages base URL (the root, without `/v1`)              |
| `models`    | model ids to expose, comma separated                               |
| `catalog`   | models.dev provider id, for model names and reasoning levels       |
| `website`, `keys` | the vendor's site and its API-key page (https)               |
| `icon`      | an https picture of the vendor's own (PNG, JPEG, GIF, WebP, ICO, SVG, at most 1 MB). fogway downloads it once, after you confirm the import, into its icons folder; without one it falls back to the catalog's logo or a plain mark |

Base URLs must be https (plain http only to this machine or the local
network).

## The local model

Fogway runs a small model on this computer with llama.cpp's `llama-server`,
downloaded for the platform (Metal on Apple silicon, CUDA with an NVIDIA
card, else Vulkan, else the CPU), and serves it as `local/<model>`.
`group/auto` puts it in front of the cloud models: as each user turn begins,
it judges whether it can do the turn and whether the turn carries what
mustn't leave the computer.

```sh
fogway local check                    # which model suits this computer, and how fast each would run
fogway local setup                    # download llama.cpp and the model recommended, and turn it on
fogway local setup spark-x2.5-1.7b    # a model of the catalog by name, or <repo> <file.gguf>
fogway local models                   # the catalog, ★ the model recommended here
fogway local use <model>              # another model; fogway local on|off
fogway local mask on                  # mask a sensitive turn before group/auto sends it to the cloud
fogway local think on                 # let the model reason before it answers: steadier, slower
fogway local context 256k             # the window it runs at; with none, each window and its memory
fogway local                          # what runs, where, and how group/auto is set up
```

### Which model suits this computer

`fogway local check`, and the Local page's assessment, read the computer —
its memory, an NVIDIA card's memory, Apple silicon's GPU cores, the cores
llama.cpp computes on, the disk's free space, and on Linux the C library —
and measure in about a second how fast its cores read memory and multiply.
From those they estimate how iFlytek's [Spark X2.5](https://huggingface.co/XHToken)
1.7B and 4B (Q4_K_M, Apache 2.0) would run:

| | Spark X2.5 1.7B | Spark X2.5 4B |
|---|---|---|
| download | 1.0 GB | 2.4 GB |
| memory at a 32K window | 1.9 GB | 4.0 GB |

- *reading*: tokens a second through a prompt, which is arithmetic;
- *writing*: tokens a second of an answer, which reads every weight once,
  so goes as fast as memory is read;
- *judging a turn*: the seconds group/auto's question takes, 700 tokens in
  and 40 out, which every turn waits for.

A model runs on the GPU with Metal, or on an NVIDIA card it fits on in
full (85% of the card's memory; the computer needs 7 GB of its own
besides); otherwise it is estimated on the CPU (llama.cpp's Vulkan build
may find an AMD or Intel GPU, which the measurement below then shows).
Each is graded:

| grade | when |
|---|---|
| smooth | judges a turn within 4 s and writes 15 tokens a second or more |
| usable | within 6 s, 8 tokens a second or more |
| too slow | slower |
| too much memory | it runs, but takes more than a third of the memory |
| doesn't fit | it would take more than half of the memory (60% on a Mac) |
| no disk room | the disk hasn't its download and 1 GB more free |
| llama.cpp can't run | Linux with a glibc older than llama.cpp's build needs: 2.34 on x64 (Ubuntu 22.04, Debian 12), 2.38 on arm64 (Ubuntu 24.04) |

The 4B is recommended where it is smooth; else the 1.7B, where it is
smooth or usable; else no local model, and the reason says why. The cloud
models serve agents without one; `group/auto` needs one. `fogway local
setup` with no model named installs the one recommended, and stops where
none is; naming one installs it all the same. Turned on with no model of
its own, the local model keeps the one recommended then, so a later
assessment doesn't swap it for another download.

What the estimates make of some computers:

| computer | 1.7B judges a turn | 4B judges a turn | recommended |
|---|---|---|---|
| M4 Pro, 24 GB | 0.8 s | 1.6 s | 4B |
| M1, 16 GB | 2.2 s | 5.0 s | 1.7B |
| M1, 8 GB | 2.2 s | (too much memory) | 1.7B |
| 8-core desktop, CPU only, 32 GB | 3.7 s | 8.7 s | 1.7B |
| 4-core laptop, CPU only, 16 GB | 7.8 s | 18 s | none |
| 4 GB of memory | (too much memory) | (doesn't fit) | none |

The first time a model serves with a llama.cpp build, it is measured: once
`llama-server` is idle, it reads a 700-token prompt and writes 48 tokens,
twice. What it did replaces its estimate, and sets the other model's right
by how far its own estimate was off. `fogway local check` measures the
model running again, as does *Measure the running model* on the Local
page. The assessment is kept in `~/.cache/fogway/local/assessment.json`,
and made again for another computer.

### The context window

The window is how much of a conversation the local model holds: the
agent's instructions and tools, the turns so far and the answer. It is
32K tokens unless set, on the Local page or with `fogway local context`,
up to what the model was trained for: 1M for both Spark X2.5 models, 256K
for Qwen3.5, 128K offered for a model of your own. llama-server takes the
window's cache from memory as the model loads, whether the window fills
or not. One layer in four keeps the whole window (the others 512 tokens),
so a token takes 14 KB with the 1.7B and 36 KB with the 4B:

| window | Spark X2.5 1.7B | Spark X2.5 4B |
|---|---|---|
| 32K | 1.9 GB | 4.0 GB |
| 128K | 3.2 GB | 7.4 GB |
| 256K | 5.0 GB | 11.9 GB |
| 512K | 8.5 GB | 20.9 GB |
| 1M | 15.5 GB | 38.9 GB |

These are the file, the cache and 0.5 GB for llama.cpp's buffers; what
`llama-server` took, measured at 32K to 512K, came within 0.3 GB below
them. The Local page and `fogway local context` list each window with
the memory it takes on this computer. A window that takes more
than a third of the memory is marked tight on memory; one that takes more
than llama.cpp can have (half the memory, 60% on a Mac, 85% of an NVIDIA
card's) can't be set, as `llama-server` loads it and then fails every
request: the 1.7B at 1M ran out of GPU memory on a 24 GB M4 Pro. A window
set for another model runs at the most the model now in use holds.

A large window is slow to fill. With 140K tokens in a 256K window, the 4B
on an M4 Pro read 153 tokens a second on average (815 on a short prompt)
and wrote 11 (77), about 15 minutes before its answer, which found the
one line asked for. `llama-server` keeps what it has read: the next turn
of the same conversation reads only what is new, even with group/auto's
judgement in between.

## Install

Download the app for macOS or Windows, or the terminal build for macOS,
Linux, Windows or Android (Termux), from
[the releases](https://github.com/carey569/fogway/releases); `SHA256SUMS`
beside them holds every file's checksum. On Linux, build the desktop
app from source (below). The builds are not signed yet:
macOS asks you to allow the app in System Settings › Privacy & Security on
its first run, and Windows SmartScreen may ask too.

A release build doesn't update itself: it has no update feed, and
`fogway update` says where to get the new version. A build of your own
gets one with
`-ldflags "-X github.com/carey569/fogway/internal/update.FeedURL=https://…"`.

From source:

```sh
go install github.com/carey569/fogway@latest
```

or build locally:

```sh
make build            # ./fogway with the desktop app (needs cgo + the platform webview)
make app              # macOS: fogway.app, a menu bar app with no Dock icon
make cli              # terminal-only build, no cgo, cross-compiles anywhere
make release          # dist/: native app build + cli builds for every platform
make release-windows  # dist/: the Windows app, amd64 and arm64 (cross-compiles)
make release-linux    # dist/: the Linux app for this machine's arch
```

Linux needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` for the app build
(the Makefile adds the `gtk3` tag; with plain `go build`, pass `-tags gtk3`);
Windows uses the WebView2 runtime that ships with the OS.

Termux uses the Android terminal build: the installer puts it in
`$PREFIX/bin`, and `make build` selects it automatically. With plain
`go build` or `go install`, pass `-tags nogui`; use `fogway web` or
`fogway tui` for the interface. `fogway autostart on` requires Termux:Boot.

### As a background service

`fogway daemon install` runs `fogway serve` as a service of your own, with
no window and no desktop session: a server, or a machine you reach over
SSH. The system starts it at login (and at boot, see below) and starts it
again when it stops with a failure.

```sh
fogway daemon install    # write the service and start it; again, to take a new PATH or proxy
fogway daemon status     # running or not, its pid, file and logs, whether the gateway answers
fogway daemon restart    # after updating fogway
fogway daemon stop|start
fogway daemon uninstall  # stop it and remove the file
```

- **Linux:** a systemd user unit, `~/.config/systemd/user/fogway.service`
  (`$XDG_CONFIG_HOME/systemd/user` when set), run with `systemctl --user`,
  no root. Its log: `journalctl --user -u fogway -f`. systemd starts a
  user's units at their first login and stops them at their last, so for
  the gateway to run from boot with no one logged in, run
  `sudo loginctl enable-linger $USER` once; `fogway daemon status` says so
  when it isn't on. From `su` or `sudo -u`, `systemctl --user` can't reach
  your session: log in as the user, or set
  `XDG_RUNTIME_DIR=/run/user/$(id -u)`. On WSL, turn systemd on first
  (`[boot] systemd=true` in `/etc/wsl.conf`). A system without systemd is
  told so; run `fogway serve` under the manager it has.
- **macOS:** a launch agent, `~/Library/LaunchAgents/com.yetone.fogway.serve.plist`,
  loaded with `launchctl bootstrap gui/$UID`. Its log:
  `~/Library/Logs/fogway/serve.log`.
- **Windows:** not a service; `fogway autostart on` opens fogway, which
  serves the gateway, when you log in. Inside WSL, `fogway daemon` works as
  on Linux.

The service gets the environment `fogway daemon install` ran in: `PATH`
(where it finds the agents' CLIs), the proxy variables, `FOGWAY_*`
settings such as `FOGWAY_ADDR`, the XDG folders and the agents' own folder
variables (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`…). To that `PATH` it adds
what a terminal of yours has and the shell may not: nvm's default Node
(not its newest), the other version managers' and npm's folders, and your
login shell's own `PATH`, so what it starts finds `node` with nvm loaded
lazily or over SSH. When no `node` is found even
then, the install says so. The file is readable by you
alone, since a proxy's address can hold a password. Run the install again
after changing any of them. It runs fogway by the name it has on `PATH`
when that is the same file (Homebrew's link stays right across updates),
and refuses a `go run` build. It is refused while `fogway autostart` is on:
the app opened at login serves the gateway too, and the two would take
turns failing on the port. The app opened by hand while the service runs
uses the service's gateway.

### Docker

`docker build` makes a server image: the terminal-only binary on
distroless (`cc`, with glibc) with bash and
busybox for a terminal in the container (`docker exec -it fogway bash`, or a
NAS panel's terminal; `fogway` is on its PATH), run as nonroot,
with everything it keeps in a volume at `/config`: fogway's own files
(`/config/fogway`), the agents' own files (HOME is `/config/home`, so
`~/.codex`, `~/.claude`… are in it) and the cache (`/config/cache`). A volume
made by an older image keeps working: fogway adds these folders to it on
start.

```sh
docker build -t fogway .
docker run -d --name fogway -p 127.0.0.1:3725:3725 -p 127.0.0.1:3730:3730 -v fogway-config:/config fogway
```

3725 is the gateway for agents. Inside the container it takes any key; a
request from outside it, the host's own included, must carry an enabled
gateway key as its API key, and one without gets 401. Make one in the
browser UI (Gateway → Gateway keys) or with
`docker exec fogway fogway gateway-key add "Docker client"`. Sharing
(Settings → Share on local network) isn't needed for this; turning it on
also creates a key named **Fogway**. The ports above are published on the
host's loopback; Docker's `-p 3725:3725` puts the gateway on every interface
of the host, for other machines to reach. Upgrading from an earlier image:
an agent outside the container that sends `fogway`, or another key that
isn't an enabled gateway key, now gets 401 ("the API key sent is not an
enabled fogway gateway key"); give it a gateway key.
Inside the container
fogway only sees the container's own address (Docker's 172.17.x), so set
`-e FOGWAY_PUBLIC_URL=http://<the host's or NAS's address>:3725` (the port
published on the host) for the address it shows and prints to be the one
other machines use; behind a reverse proxy, set it to that external base URL
and follow the [authentication requirements above](#connecting-anything-else),
especially when the proxy reaches fogway over loopback.

For the browser UI run the image with `fogway web --addr 0.0.0.0:3730 --no-open`
in place of the default `serve`, and open
`http://localhost:3730/?k=<key from docker logs fogway>` (set `FOGWAY_WEB_KEY`
to keep one key across restarts). There you add providers. Keys and
settings live in the volume, so a restart, or a new container on the same
volume, keeps them.

The image has a `HEALTHCHECK`: `fogway healthcheck` exits 0 while the gateway
answers on `FOGWAY_ADDR`, under `serve` and `web` alike, so `docker ps` shows
the container as healthy (Compose: `depends_on: condition: service_healthy`)
with no curl in the image. Bind-mounting a folder at `/config` in place of a
named volume works too; it has to be writable by uid 65532.

#### NAS with agents on other machines

Fogway can run on a headless NAS as a shared gateway even when the agents that
use it (Claude Code, Codex, OpenCode, and so on) run on other computers. The
container does not discover agent installations on the NAS host or on remote
machines: configure those agents to use the NAS address and an enabled gateway
key. See [Connecting anything else](#connecting-anything-else) for the
protocol-specific base URLs. Keep `3725` behind a firewall or VPN and do not
expose it publicly without gateway-key authentication.

Such a page is in **gateway mode**: it shows Providers, Gateway, Routing,
Usage and Settings, with no Agents or Sessions tab, and
Settings leaves out what is written into this machine's agents (provider in
model names, Codex subagents, long conversations, Codex thread titles,
Codex thread descriptions, Codex composer predictions) and
the desktop's alerts and tray. `fogway web` is in it by itself when it finds
no agents on its machine, as in the container, and with
`fogway web --gateway`. *Settings › General › Gateway mode* picks Automatic,
On or Off for this machine (it wins over both); Off brings every page back.
The app's own window is never in gateway mode.

For a bind-mounted configuration directory, the directory must be writable by
the non-root container user (uid 65532):

```sh
sudo chown -R 65532:65532 /volume1/docker/fogway/config
```

An example Docker Compose project is:

```yaml
services:
  fogway:
    image: ghcr.io/carey569/fogway:latest
    restart: unless-stopped
    ports:
      # 127.0.0.1: the host alone; every caller outside the container needs a gateway key.
      - "127.0.0.1:3725:3725"
      - "127.0.0.1:3730:3730"
    environment:
      FOGWAY_ADDR: 0.0.0.0:3725
      FOGWAY_PUBLIC_URL: http://<nas-address>:3725
      FOGWAY_WEB_KEY: <random-key>
    volumes:
      - ./config:/config
    command: [web, --addr, 0.0.0.0:3730, --no-open]
```

Open `http://127.0.0.1:3730/?k=<random-key>` through an SSH tunnel to
configure providers, then create a gateway key for each remote agent host.
To let remote agents reach it, change the gateway mapping to `3725:3725` (or
bind it only to the NAS interface or VPN address) and recreate the
container. For Internet access, prefer a
VPN such as Tailscale or WireGuard; a reverse proxy or tunnel forwarding to
Fogway over loopback needs a gateway key from its clients, as the network does.

### Developing

```sh
make dev
```

builds with `-tags dev` and opens the app with the UI served straight from
`internal/gui/assets`: save `app.css`, `app.js` or `index.html` and the window
reloads itself. With `fswatch` installed (`brew install fswatch`), a change to a
Go file rebuilds and relaunches the app too. The dev build uses its own gateway
port (`DEV_ADDR`, default 127.0.0.1:3726), so a fogway you already run keeps
serving your agents. Point it at a scratch home to keep your real agent
configs out of it:

```sh
HOME=/tmp/fogway-home XDG_CONFIG_HOME=/tmp/fogway-home/.config make dev
```

`FOGWAY_THEME=light|dark` forces the palette and `FOGWAY_DEBUG=1` prints what the
gateway translates.

## Use

```sh
fogway                          # open the app: a window plus the menu bar icon
fogway tray                     # menu bar icon only (use this in your login items)
fogway tui                      # the same thing, in the terminal; serves the gateway while open when no other fogway does
fogway web                      # the app's window in a browser (WSL, a server over SSH); --lan, --addr, --no-open, --gateway
                                # (a new key each run; FOGWAY_WEB_KEY keeps one, for a page run as a service)
                                # (behind a reverse proxy, FOGWAY_WEB_URL=https://<the page there> prints the link through it)
fogway ls                       # list every agent and its current settings
fogway claude opus              # set a model (agent names accept prefixes: cc, oc, gem …)
fogway codex gpt-5.6-sol
fogway codex effort high        # other fields
fogway codex effort             # one field, as it is now
fogway codex xhigh              # bare effort levels are recognised too
fogway codex deepseek/deepseek-chat   # any catalog model, through the gateway
fogway claude moonshot/kimi-k2.5
fogway claude haiku deepseek/deepseek-v4-flash   # one tier on its own model
fogway claude haiku ""          # back to the main model
fogway gemini auth api-key
fogway opencode anthropic/claude-sonnet-5
fogway oc small anthropic/claude-haiku-4-5
fogway mimo anthropic/claude-sonnet-5

fogway save work                # snapshot everything as a profile
fogway use work                 # switch back
fogway profiles
fogway rm work

fogway sync                     # refresh the models.dev catalog and every live model list

fogway quota                    # what is left of every plan and key balance
fogway quota wait kimi-code     # block until the plan has allowance again
```

In the app, click any value to open a filtered list; type to search or to
enter something that is not listed; `esc` closes the panel. Profiles are the
chips at the bottom: click to apply, `×` to delete, *+ save current* to add.
The *Providers* tab of the window lists your providers with the agents on
each; click a row to change the key or the exposed models, *Test* it, or
click an agent icon to point that agent at one of its models. *Add
provider* shows the presets as tiles: pick one, paste the key.

In *Usage → Requests* and the tray's *Usage* tab, click a daily bar to see
that day's totals and details. Other days turn gray; click the selected day
again to return to the whole period. Changing the period clears the selection.

On macOS, *Settings → Preferences → Session terminal* chooses which installed
app opens a session from the terminal button in *Usage → Sessions*. The list
contains apps registered to open `.command` files, with the current system
default listed once. With no saved choice, fogway follows that default, or
Terminal when the default is not a terminal. The *Resume* button still copies
the command.

Keys in the terminal version:

| Key        | Action                                |
| ---------- | ------------------------------------- |
| `↑` `↓`    | choose agent                          |
| `←` `→`    | choose field (model, effort, small …) |
| `↵`        | open the picker                       |
| type       | filter; enter accepts custom values   |
| `s`        | save current setup as a profile       |
| `p`        | apply or delete (`ctrl+d`) a profile  |
| `S`        | sync the model catalog                |
| `q`        | quit                                  |

Agents read their config at startup, so a running session keeps its model
until you start a new one.

## OTLP export

Settings → Observability can export gateway request metadata over OTLP/HTTP
(JSON). Export is off by default. Set the collector's base URL and optional
headers, then enable **OTLP export**. **Export metrics** is separately off by
default; enable it for a collector that accepts duration and token histograms.
No restart is needed for saved settings.

For `fogway serve`, environment variables override the saved preferences:

```sh
FOGWAY_OTEL_ENABLED=true FOGWAY_OTEL_ENDPOINT=http://localhost:4318 fogway serve
```

- `FOGWAY_OTEL_ENABLED`: `true` or `false`; an endpoint alone does not enable export.
- `FOGWAY_OTEL_ENDPOINT`: an HTTP(S) base URL; `/v1/traces` and `/v1/metrics` are appended.
- `FOGWAY_OTEL_HEADERS`: comma-separated `name=value` pairs, for example
  `Authorization=Bearer%20token`. Percent-encode spaces and commas in values.
- `FOGWAY_OTEL_METRICS`: `true` or `false`, off by default.
- `FOGWAY_OTEL_SESSIONS`: `true` or `false`, off by default; trace supported local
  agent interactions from newly recorded session events.
- `FOGWAY_OTEL_BODIES`: `true` or `false`, off by default; sends each call's
  request and reply as the trace's Langfuse input and output.
- `FOGWAY_OTEL_BODIES_WHOLE`: `true` or `false`, off by default; with bodies on,
  keeps them entire rather than cut at 256 KB. A long reply is written to a
  temporary file; a body too large for the collector is still refused.

For Langfuse, use `https://<your-langfuse-host>/api/public/otel` as the base
URL and `Authorization=Basic%20<base64(public-key:secret-key)>` as the header.
Leave metrics off. This uses Langfuse's OTLP ingestion endpoint. For Langfuse
v4, add the header `x-langfuse-ingestion-version=4` for real-time ingestion.

Traces include agent, provider, model, token counts (including cache and
reasoning), HTTP status, timing, and route ID. Attempts with the same route ID
share a trace ID. Chat, Responses, Anthropic and Gemini gateway requests
also export a parent request span and child spans for every routing attempt,
including unbilled failures, retries and fallbacks, so Langfuse can render a
waterfall. A valid incoming W3C version-00 `traceparent` connects these spans
to the caller's trace. Attempt spans carry token usage; the parent does not
duplicate it. Attempt timings include any wait for a concurrency slot, while
routing and retry delays remain visible as gaps inside the parent span.
Tool execution inside the caller is outside the gateway's trace.

Model-call spans also send estimated USD input, output and total costs to
Langfuse via `langfuse.observation.cost_details`, using the same effective
prices as Fogway's usage ledger (custom prices first, then catalog prices).
Input cost includes cache reads and writes; reasoning tokens are already
included in output cost. Explicit zero prices are exported too. Unknown
prices are omitted so Langfuse can use its own model pricing. Parent and tool
spans carry no costs, avoiding duplicate counting. A plan's costs at
catalog prices are API-equivalent estimates, not what the plan charges.

Enable **Trace agent conversations** to instead export one trace per
user interaction, grouping model calls and tool executions under an agent
root. Conversation IDs group those traces into Langfuse sessions. Gateway traces
remain available until a visible local store covers that exact agent/session.
For Codex and Pi, a readable session header establishes readiness without
waiting for a model/tool span. The first request checks a header bounded to
256 KiB when needed (including Codex base instructions); subsequent polls keep
idle sessions ready while their files remain among the reader's 200 newest main
files. Claude children have a separate 200-file quota and share the same 8 MiB
polling read budget. Header checks never upload history or bodies. Discovery is shared across
request IDs for two seconds. The request that refreshes discovery waits for the
scan; other unresolved concurrent requests retain gateway traces without waiting.
Polling reuses headers by path, size and mtime.
Other adapters still use recent observations (five minutes).
Only loopback requests without a gateway key can be deduplicated. The native
client session ID takes precedence over a Fogway routing override; unknown
sessions retain gateway traces, including WSL mirrored and Docker Desktop
clients whose sessions are not visible locally. Requests without a session ID
fall back to recent observations for that agent.
Claude Code/Cowork readers also follow `<session>/subagents/*.jsonl` and a
workflow's (ultracode) `<session>/subagents/workflows/<run>/agent-*.jsonl`
(not its `journal.jsonl`). Child
interactions share their parent session and use separate trace/span IDs, even
when a user UUID is copied. Child final responses are combined across content
blocks and exported when the transcript is unchanged for two seconds; no
`turn_duration` is required.
Their start times remain inferred; a parent tool link is not guessed. Main-thread calls can therefore be deduplicated. Claude
tool-less small requests (`max_tokens` at most 4096), including title/haiku
helpers, retain gateway traces because they may not enter a transcript. This
conservative rule can retain a short tool-less main request as well. Explicitly
classified auxiliary calls also retain gateway traces for other agents. This
includes Codex calls with an explicit kind (subagent, review, memgen, title or
compact): even if a child has a readable rollout, these can appear twice. Only
its unclassified calls use local-session deduplication; a rollout alone does not
prove every auxiliary request was recorded. Keyed and non-loopback clients
always retain gateway traces. Fogway reads new events from local session stores
every two seconds; it does not upload
completed history when enabled. Restarting or changing the destination starts
an observation window. Span IDs remain stable across repeated records.

Pi and Codex have been tested end to end with real clients. The other adapters
are covered by format fixtures but have **not been tested end to end**.

Supported clients and formats:

| Client | Local store | Timing |
| --- | --- | --- |
| Codex | JSONL rollouts (`token_usage_record`, `item_completed`, `response_item`) | Recorded operations and paired native tools; inferred model/native-tool intervals |
| Pi | Version-3 JSONL; optional `timing-final` | Recorded model times; inferred tool intervals |
| Oh My Pi | Pi-compatible JSONL, including `model_usage` | Pi timing; auxiliary calls have inferred zero duration |
| Claude Code / Cowork | `projects/*/*.jsonl`, `<session>/subagents/*.jsonl` and `<session>/subagents/workflows/*/agent-*.jsonl`, repeated assistant blocks and paired tool results | Inferred model/tool starts; recorded transcript boundaries |
| OpenCode | SQLite V1/V2 (`message`/`part` or `session_message`); legacy JSON storage | Recorded model and tool timestamps |
| Gemini CLI | `~/.gemini/tmp/*/chats/session-*.json[l]`, including patches and rewinds | Inferred intervals from message/tool event boundaries |

The adapters follow upstream schemas:
[OpenCode V1](https://github.com/anomalyco/opencode/blob/dev/packages/schema/src/v1/session.ts),
[OpenCode V2](https://github.com/anomalyco/opencode/blob/dev/packages/schema/src/session-message.ts),
[Gemini CLI](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/services/chatRecordingTypes.ts),
[Oh My Pi](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/session/session-manager.ts).
Claude transcript records are checked against local Claude Code sessions;
its [hook documentation](https://code.claude.com/docs/en/hooks) describes
transcript locations. Separate subagent transcripts are not joined: the parent
agent's task/delegation tool is included, but its children's internal work is
not yet linked. No hooks or client changes are required.

Polling uses a soft 8 MiB reading budget and up to 200 recent files per store.
JSONL files are read incrementally; incomplete lines wait for the next poll.
OpenCode SQLite is opened read-only, including WAL changes, and unchanged
stores are skipped. SQLite message/part updates are read incrementally in bounded
batches on separate read-only connections; oversized conversations resume on the
next poll. OpenCode retains up to 256 messages and an 8 MiB body window per
session; individual rows above 8 MiB export metadata only. Legacy JSON and
Gemini snapshot reads cap at 32 MiB; Gemini retains the current prompt plus
255 recent messages.
Completed observations are deduplicated with bounded metadata. Claude's final
response and Gemini's final token records may take one extra poll to settle.

Inferred timings carry `fogway.timing.source=inferred`. Tool arguments/results
and user input/output follow **Include request and response bodies**, masking
secrets and respecting the whole-body preference. Session directory/title
metadata, credentials and account names are not exported.

Conversation tracing replaces standalone gateway traces for these local
clients to avoid duplicate token usage. Unsupported clients and remote
forwarded requests retain gateway tracing; the local usage ledger is unchanged.
This mode shows agent activity rather than gateway retries or provider-routing
details. It reads this computer's local sessions, including calls made without
Fogway as the proxy.

Metrics group duration and input/output token histograms by agent, provider,
model, operation and error status. Prompt/reply text is exported only when
**Include request and response bodies** is enabled (`FOGWAY_OTEL_BODIES=true`);
secrets are masked and each body is limited to 256 KiB unless **Include the
whole bodies** is enabled. Conversation IDs are exported only with
conversation tracing; provider account names/keys are never exported.

Whole bodies increase transient memory and allocation costs during read-back,
secret scrubbing and JSON encoding; a 32 MiB request and reply can roughly
double total allocations compared with truncated export. The export limits
queued bodies to 128 MiB, but this does not bound in-flight processing memory.
Recent calls retain only the first 256 KiB of each body.

Export runs in the background with a bounded queue (128 records) and batches
of up to 32 records, flushed every five seconds. A full queue drops telemetry
without delaying gateway requests. Network errors and HTTP 429/502/503/504
are retried up to two times, with retry delays capped at 60 seconds; other
errors and partial rejection are logged
without the collector's response body. Each HTTP attempt times out after
three seconds. Graceful gateway shutdown allows at most three seconds to
drain; `fogway serve` currently exits on SIGTERM without draining, so its last
batch may be lost. This is best-effort export; local usage records remain
available if it fails.
Requests follow fogway's proxy setting, with loopback collectors going direct.
Queued records are discarded if export is disabled or the destination or
credentials change before sending. Redirects are not followed.

## Mirrors

With `FOGWAY_MIRRORS=on` (or `true`, `1`, `cn`), what fogway downloads by
itself is asked of a mirror when its official address can't be reached:
npm's registry (the agents' CLI versions) at `registry.npmmirror.com`, or
`FOGWAY_NPM_REGISTRY`, and GitHub (fogway's update downloads) through
`gh-proxy.com`, or `FOGWAY_GITHUB_MIRROR`. The official address stays
first. `chinaMirror: true` in settings.json asks npmmirror before npm's
registry instead; the window has no switch for it.

Only fogway's own update assets are checked against a checksum that doesn't
come from the mirror (their SHA-256 comes from the unmirrored update feed).
npm metadata and version checks are not checksummed; a mirror can report a
different version.

The official source and a mirror share one request budget, so a hanging
official source doesn't make the whole wait twice as long.

Requests that carry credentials, such as an `Authorization`, `Cookie`,
API key, or URL userinfo/query token, never use a mirror; they go to the
official source alone.

```sh
FOGWAY_MIRRORS=on \
FOGWAY_NPM_REGISTRY=https://registry.example.com \
FOGWAY_GITHUB_MIRROR=https://gh.example fogway serve
```

## Files

- `~/.config/fogway/profiles.json` — saved profiles
- `~/.config/fogway/providers.json` — your providers, keys included (0600)
- `~/.config/fogway/stash.json` — values fogway replaced, restored on switch-back
- `~/.cache/fogway/models.json` — models.dev catalog (OpenCode's cache at
  `~/.cache/opencode/models.json` is used when present)
- `~/.cache/fogway/models/<provider>.json` — model lists fetched from vendors
- `~/.config/fogway/local.json` — the [local model](#the-local-model)'s setup
- `~/.cache/fogway/local/` — llama.cpp, the models (`models/<owner>--<repo>/`),
  `llama-server.log`, and `assessment.json`, this computer's assessment and
  the speeds measured on it

`XDG_CONFIG_HOME` and `XDG_CACHE_HOME` are respected.

## Community

Questions, ideas and bugs: issues and pull requests are welcome here.

## License

MIT. See [LICENSE](../LICENSE).
