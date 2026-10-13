<div align="center">

# Fogway · 雾桥

### 面向编程 Agent 的端云一体化模型网关

本机一个小模型，云端一个大模型，前面一个网关。<br>
每一轮对话各归其位：本地能做的留在本地，其余交给云端，不该离开这台电脑的，就不离开。

[English](README.md) · **简体中文**

</div>

## 它做什么

- **所有 Agent 共用一个网关。** Claude Code、Codex、Gemini CLI、OpenCode、Pi、omp、dsh、Goose、Cursor、Zed、VS Code、Copilot CLI、Crush、Kimi Code、Qwen Code、Droid、Cline 都连到 `http://127.0.0.1:3725`。雾桥只改各 Agent 自己配置里关键的那一项，并兼容它们用到的所有协议：Chat Completions、Responses、Anthropic Messages、Gemini。
- **自动部署本地模型。** 雾桥会为本机下载 [llama.cpp](https://github.com/ggml-org/llama.cpp) 的 `llama-server`（Apple 芯片用 Metal，有 NVIDIA 显卡用 CUDA，否则用 Vulkan 或 CPU），从 Hugging Face 拉取 GGUF 模型（可用 `hf-mirror.com` 等镜像），按内存大小推荐合适的模型，并以 `local/<模型>` 对外提供。
- **默认端云路由。** `group/auto` 把本地模型和云端模型编成一组。每轮用户消息到达时，本地模型先判断两件事：*这件事我能做好吗？* 以及 *里面有没有不该离开这台电脑的内容？* 据此把这一轮交给本地或云端，并在整轮内保持不变。本地模型在这一轮里失败、陷入循环或工具调用屡次出错时，剩下的部分升级到云端；云端不可用时，由本地模型兜底。
- **脱敏，开关由你决定。** 打开脱敏后：本地能做的敏感任务始终留在本机，无论成败都不升级；需要上云的敏感任务先脱敏再发送：本地模型找出的敏感字符串（人名、主机名、项目代号、客户数据），加上规则识别出的所有密钥（API Key、Token、密码），都以占位符发往云端，回复回来时再还原。脱敏默认关闭。
- **云端用厂商 API Key。** DeepSeek、Kimi、智谱 GLM、通义千问、MiniMax、OpenAI、Anthropic、Gemini、OpenRouter、硅基流动等，填入 Key 即可添加；Ollama、LM Studio 等本地服务也能接入。
- **用量、花费与会话。** 每个请求的 token 和花费按 Agent、模型、Key 记录；各 Agent 的历史会话可以查看、搜索和恢复。

## 快速开始

雾桥是一个桌面应用（macOS、Windows、Linux），同时带有命令行和终端界面。

```sh
# 从源码构建（Go 1.26）
make build           # ./fogway：桌面应用和命令行合一
make cli             # 仅命令行版本，可交叉编译到任意平台

./fogway             # 打开应用：主窗口加菜单栏图标
```

然后：

1. **添加云端厂商**：在应用的「供应商」页，或 `fogway provider add deepseek sk-…`。
2. **部署本地模型**：在应用的「端侧」页，或 `fogway local setup`。它会下载 llama.cpp 和适合本机内存的推荐模型（`fogway local models` 列出目录；也可以指定任意 Hugging Face 仓库和 `.gguf` 文件：`fogway local setup Qwen/Qwen3-4B-GGUF Qwen3-4B-Q4_K_M.gguf`）。国内网络可加 `--hf https://hf-mirror.com`。
3. **把 Agent 指向 `group/auto`**：在应用的「Agents」页，或 `fogway claude group/auto`、`fogway codex group/auto`。
4. **需要时打开脱敏**：在「端侧」页，或 `fogway local mask on`。

`fogway local` 会显示正在运行的模型、地址，以及 `group/auto` 的配置。

## 一轮对话怎么路由

```
用户消息 ──► 本地模型判断：{本地可做, 敏感}
              │
              ├─ 本地可做              ──► 本地模型（失败、循环 → 本轮剩余部分升级到云端）
              ├─ 本地可做 + 敏感*      ──► 只用本地模型，绝不上云
              ├─ 需要云端              ──► 依次尝试云端模型，本地模型兜底
              └─ 需要云端 + 敏感*      ──► 脱敏 ──► 云端 ──► 回复还原
                                                    * 脱敏开启时
```

判断只在用户消息到达时做一次，并对之后 Agent 的工具调用保持有效，一个任务不会中途被拆给两个模型。路由视图会显示每一轮的决定、本地模型给出的理由，以及判断耗时。

## 文件位置

| | |
|---|---|
| 设置、供应商、路由组 | `~/.config/fogway/`（`local.json` 是本地模型的配置） |
| llama.cpp、模型、日志 | `~/.cache/fogway/local/`（`llama-server.log`） |
| 网关 | `http://127.0.0.1:3725`（`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1beta/models/*`） |

## 文档

- [参考手册](docs/reference.md)：所有命令和设置。
- [集成指南](docs/integrating.md)：在你自己的工具里调用网关。
- [子系统](docs/subsystems)：网关、路由和供应商的内部原理。

## 许可证

MIT。雾桥基于 yetone 的 [magpie](https://github.com/yetone/magpie)（MIT）构建，其版权声明保留在 [LICENSE](LICENSE) 中。
