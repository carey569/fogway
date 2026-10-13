<div align="center">

# Fogway · 雾桥

### The edge-cloud model gateway for your coding agents

A small model on your computer, a large one in the cloud, one gateway in front of both.<br>
Each turn goes where it belongs: what the local model can do stays local, the rest goes to the cloud, and what mustn't leave your computer doesn't.

**English** · [简体中文](README.zh-CN.md)

</div>

## What it does

- **One gateway for every agent.** Claude Code, Codex, Gemini CLI, OpenCode, Pi, omp, dsh, Goose, Cursor, Zed, VS Code, Copilot CLI, Crush, Kimi Code, Qwen Code, Droid and Cline all talk to `http://127.0.0.1:3725`. Fogway writes the one setting that matters into each agent's own config and speaks every API they use: Chat Completions, Responses, Anthropic Messages and Gemini.
- **A local model, set up for you.** Fogway downloads [llama.cpp](https://github.com/ggml-org/llama.cpp)'s `llama-server` for your machine (Metal on Apple silicon, CUDA with an NVIDIA card, else Vulkan or the CPU), pulls a GGUF model from Hugging Face (a mirror such as `hf-mirror.com` works), picks one that fits your memory, and serves it as `local/<model>`.
- **Edge-cloud routing by default.** `group/auto` puts the local model and your cloud models together. As each user turn begins, the local model judges two things: *can I do this well?* and *does this carry what mustn't leave this computer?* The turn goes to the local model or to the cloud, and stays there until the turn ends. A local turn that fails, loops or keeps failing its tools moves to the cloud for the rest of the turn. A cloud that doesn't answer leaves the turn to the local model.
- **Masking, when you turn it on.** With masking on, a sensitive turn the local model can do never leaves your computer, whatever happens. A sensitive turn for the cloud goes masked: the strings the local model finds (names, hosts, codenames, customer data), plus every secret the rules find (API keys, tokens, passwords), go out as placeholders and come back restored in the reply. Masking is off until you turn it on.
- **Cloud providers by API key.** DeepSeek, Kimi, GLM, Qwen, MiniMax, OpenAI, Anthropic, Gemini, OpenRouter, SiliconFlow and many more: add one with its key. Local servers such as Ollama and LM Studio work too.
- **Usage, cost and sessions.** Every request is logged with its tokens and cost, by agent, model and key. Each agent's past sessions can be read, searched and resumed.

## Get started

Fogway is a desktop app (macOS, Windows, Linux) with a CLI and a terminal UI.

```sh
# build from source (Go 1.26)
make build           # ./fogway, the desktop app and the CLI in one
make cli             # a terminal-only build, cross-compiles anywhere

./fogway             # open the app: a window and a menu bar icon
```

Then:

1. **Add a cloud provider**: in the app's Providers page, or `fogway provider add deepseek sk-…`.
2. **Set up the local model**: in the app's Local page, or `fogway local setup`. It downloads llama.cpp and the model recommended for your memory (`fogway local models` lists the catalog; any Hugging Face repository and `.gguf` file works: `fogway local setup Qwen/Qwen3-4B-GGUF Qwen3-4B-Q4_K_M.gguf`). Behind the Great Firewall, add `--hf https://hf-mirror.com`.
3. **Point your agents at `group/auto`**: in the app's Agents page, or `fogway claude group/auto`, `fogway codex group/auto`.
4. **Turn masking on** if you need it: on the Local page, or `fogway local mask on`.

`fogway local` shows what runs, at which address, and how `group/auto` is set up.

## How a turn is routed

```
user's turn ──► local model judges: {local, sensitive}
                 │
                 ├─ local                 ──► local model   (fails, loops → cloud, for the rest of the turn)
                 ├─ local + sensitive*    ──► local model only, never the cloud
                 ├─ cloud                 ──► cloud models in order, the local model last
                 └─ cloud + sensitive*    ──► masked ──► cloud ──► reply restored
                                                      * masking on
```

The decision is made once, when the user's message arrives, and holds for the agent's tool calls that follow, so a turn isn't split between two models mid-task. The routing view shows each turn's decision, why the local model made it, and how long judging took.

## Where things live

| | |
|---|---|
| Settings, providers, groups | `~/.config/fogway/` (`local.json` is the local model's setup) |
| llama.cpp, models, logs | `~/.cache/fogway/local/` (`llama-server.log`) |
| The gateway | `http://127.0.0.1:3725` (`/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1beta/models/*`) |

## Documentation

- [Reference](docs/reference.md): every command and setting.
- [Integrating](docs/integrating.md): calling the gateway from your own tools.
- [Subsystems](docs/subsystems): how the gateway, routing and providers work inside.

## License

MIT. Fogway is built on [magpie](https://github.com/yetone/magpie) by yetone (MIT); its copyright notice is kept in [LICENSE](LICENSE).
