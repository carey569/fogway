package gateway

// forkNoneTask is the first request Codex 0.162 (rust-v0.162.0-alpha.17.2)
// sends for a subagent its lead spawned with fork_turns="none", rebuilt
// from Codex's source, not the reporter's bytes (#1855 has none):
// spawn.rs's fork_mode is None for "none", so no turn of the lead's is
// copied in, and the input is the subagent's own context and its task.
// The task is spawn_agent's message, which the ChatGPT backend returned
// sealed (encrypted_function_args), so agent_message_from_tool makes it
// AgentMessage::Encrypted and to_model_input_item writes it under the
// NEW_TASK header as encrypted_content.
const forkNoneTask = `{"model":"ag/gemini-3.8-flash","stream":true,"store":false,"instructions":"You are Codex.","tools":[],"tool_choice":"auto","parallel_tool_calls":true,"include":["reasoning.encrypted_content"],"input":[` +
	`{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>Filesystem sandboxing: workspace-write.</permissions instructions>"}]},` +
	`{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/Users/u/proj</cwd>\n  <shell>zsh</shell>\n</environment_context>"}]},` +
	`{"type":"agent_message","author":"/root","recipient":"/root/gemini_worker","content":[{"type":"input_text","text":"Message Type: NEW_TASK\nTask name: /root/gemini_worker\nSender: /root\nPayload:\n"},{"type":"encrypted_content","encrypted_content":"gAAAAABo_sealedTaskFromTheLead-Q3x9_aZ=="}]}]}`
