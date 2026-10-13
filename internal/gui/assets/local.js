// Local: the model on this computer and the edge-cloud route. The status
// card says where llama.cpp and the model are (downloading, loading,
// serving, or why not); the setup picks the model — the one recommended
// for this computer's memory, another of the catalog, or any GGUF on
// Hugging Face — and where it is pulled from; the route card is group/auto,
// which sends each turn to the local model or the cloud as the local model
// judges it, and its masking switch. Everything comes from /api/local and
// goes back through POST /api/local/{action}.
(() => {
  const page = $("#view-local");
  if (!page) return;

  Object.assign((I18N.zh ||= {}), {
    "Local model": "本地模型",
    "Edge": "端侧",
    "The model on this computer": "本机模型",
    "Runs a small model on this computer with llama.cpp, pulled from Hugging Face. Fogway serves it as local/…, and group/auto sends each turn to it or to the cloud.": "用 llama.cpp 在本机运行一个小模型，模型从 Hugging Face 拉取。Fogway 以 local/… 提供它，group/auto 按轮把任务分给它或云端。",
    "Off": "已关闭",
    "Getting llama.cpp ready": "正在准备 llama.cpp",
    "Downloading the model": "正在下载模型",
    "Loading the model": "正在加载模型",
    "Serving": "运行中",
    "Stopped": "已停止",
    "Turn on": "开启",
    "Turn off": "关闭",
    "Model": "模型",
    "Recommended for this computer": "本机推荐",
    "Another model on Hugging Face": "Hugging Face 上的其他模型",
    "Repository": "仓库",
    "File": "文件",
    "Download from": "下载源",
    "Hugging Face": "Hugging Face 官方",
    "hf-mirror.com (China)": "hf-mirror.com（国内镜像）",
    "Context window": "上下文窗口",
    "tokens": "tokens",
    "Save and restart": "保存并重启",
    "Computer": "本机",
    "memory": "内存",
    "Apple silicon, memory shared with the GPU": "Apple 芯片，内存与 GPU 共享",
    "llama.cpp build": "llama.cpp 构建",
    "Its log": "运行日志",
    "Edge-cloud route": "端云路由",
    "Each user turn is judged by the local model as it begins: what it can do stays here, the rest goes to the cloud. A turn stays where it went; a local turn that fails goes on in the cloud.": "每轮对话开始时由本地模型判断：它能做的留在本机，其余交给云端。一轮之内不换模型；本地失败时由云端接着完成。",
    "Cloud models": "云端模型",
    "Masking": "脱敏",
    "On: a sensitive turn the local model can do stays here even if it fails; one it can't goes to the cloud with what is sensitive masked by the local model, and comes back unmasked.": "开启后：本地能做的敏感任务只在本机完成（失败也不上云）；本地做不了的，由本地模型脱敏后发往云端，回复再还原。",
    "Point agents at group/auto on the Agents page to use it.": "在 Agents 页把 Agent 的模型设为 group/auto 即可使用。",
    "No edge-cloud group yet: it is made once the local model serves and a cloud provider is added.": "还没有端云路由组：本地模型运行起来、并添加了云端 provider 后会自动创建。",
    "Add": "添加",
    "Remove": "移除",
    "Saved": "已保存",
  });

  let data = null; // /api/local
  let failed = "";
  let poll = 0;

  const PHASES = {
    off: "Off", preparing: "Getting llama.cpp ready", downloading: "Downloading the model",
    starting: "Loading the model", ready: "Serving", failed: "Stopped",
  };
  const gb = (n) => (n >= 1 << 30 ? (n / (1 << 30)).toFixed(n >= 10 * (1 << 30) ? 0 : 1) + " GB" : Math.round(n / (1 << 20)) + " MB");

  async function load() {
    try {
      data = await api("local");
      failed = "";
    } catch (e) {
      failed = e.message;
    }
    draw();
    // while it gets ready, the card follows it
    clearTimeout(poll);
    const ph = data?.status?.phase;
    if (!page.hidden && (ph === "preparing" || ph === "downloading" || ph === "starting")) poll = setTimeout(load, 1000);
  }
  window.loadLocal = load;
  // the window opened on this page (a link, or where it was left) before
  // this file was read: app.js asked for loadLocal before there was one
  if (!page.hidden) queueMicrotask(load);

  async function act(action, body) {
    try {
      data = await api("local/" + action, body ?? {});
      failed = "";
    } catch (e) {
      failed = e.message;
    }
    draw();
    clearTimeout(poll);
    poll = setTimeout(load, 800);
  }

  function row(label, value) {
    const r = el("div", "lm-row");
    r.append(el("span", "lm-k", t(label)));
    if (value instanceof Node) r.append(value);
    else r.append(el("span", "lm-v", value));
    return r;
  }

  function statusCard() {
    const s = data.status || { phase: "off" };
    const c = data.config || {};
    const card = el("section", "card lm-card");
    const head = el("div", "lm-head");
    const dot = el("span", "lm-dot lm-" + s.phase);
    head.append(dot, el("h3", "", t(PHASES[s.phase] || s.phase)));
    const toggle = el("button", c.on ? "" : "primary", t(c.on ? "Turn off" : "Turn on"));
    toggle.onclick = () => act(c.on ? "off" : "on");
    head.append(toggle);
    card.append(head);
    const m = s.model?.repo ? s.model : data.model;
    card.append(row("Model", `${data.ref} · ${m.repo} / ${m.file}`));
    if (s.progress && s.progress.total > 0) {
      const p = s.progress;
      const bar = el("div", "lm-bar");
      const fill = el("div", "lm-fill");
      fill.style.width = Math.min(100, (100 * p.done) / p.total).toFixed(1) + "%";
      bar.append(fill);
      card.append(row(p.what, `${gb(p.done)} / ${gb(p.total)}`), bar);
    }
    if (s.phase === "ready" && s.url) card.append(row("Serving", `${s.url} · ${s.context || c.context || 32768} ${t("tokens")}`));
    if (s.error) card.append(el("p", "lm-err", s.error));
    const h = data.hardware || {};
    let comp = `${h.os}/${h.arch} · ${gb(h.ram || 0)} ${t("memory")}`;
    if (h.gpu) comp += ` · ${h.gpu} ${gb(h.vram || 0)}`;
    else if (h.unified) comp += ` · ${t("Apple silicon, memory shared with the GPU")}`;
    card.append(row("Computer", comp));
    if (s.backend) card.append(row("llama.cpp build", `${s.build || ""} ${s.backend}`.trim()));
    if (data.log) {
      const a = el("a", "", data.log);
      a.href = "#";
      a.onclick = (e) => { e.preventDefault(); api("open", { url: "file://" + data.log }); };
      card.append(row("Its log", a));
    }
    return card;
  }

  function setupCard() {
    const c = data.config || {};
    const card = el("section", "card lm-card");
    card.append(el("h3", "", t("Model")));
    const custom = c.model?.repo && !data.catalog.some((m) => m.repo === c.model.repo && m.file === c.model.file);
    const pick = el("select");
    for (const m of data.catalog) {
      const o = el("option", "", `${m.name} · ${gb(m.size)} · ${m.note}${m.name === data.recommended ? " ★ " + t("Recommended for this computer") : ""}`);
      o.value = m.repo + "|" + m.file;
      pick.append(o);
    }
    const other = el("option", "", t("Another model on Hugging Face"));
    other.value = "custom";
    pick.append(other);
    const cur = data.model;
    pick.value = custom ? "custom" : cur.repo + "|" + cur.file;
    const repo = el("input");
    repo.placeholder = "owner/name-GGUF";
    repo.value = custom ? c.model.repo : "";
    const file = el("input");
    file.placeholder = "model-Q4_K_M.gguf";
    file.value = custom ? c.model.file : "";
    const customBox = el("div", "lm-custom");
    customBox.append(row("Repository", repo), row("File", file));
    customBox.hidden = pick.value !== "custom";
    pick.onchange = () => (customBox.hidden = pick.value !== "custom");
    card.append(row("Model", pick), customBox);

    const from = el("select");
    for (const [v, label] of [["", "Hugging Face"], ["https://hf-mirror.com", "hf-mirror.com (China)"]]) {
      const o = el("option", "", t(label));
      o.value = v;
      from.append(o);
    }
    from.value = c.hfEndpoint || "";
    card.append(row("Download from", from));

    const ctx = el("select");
    for (const n of [8192, 16384, 32768, 65536, 131072]) {
      const o = el("option", "", `${n / 1024}K ${t("tokens")}`);
      o.value = n;
      ctx.append(o);
    }
    ctx.value = c.context || 32768;
    card.append(row("Context window", ctx));

    const save = el("button", "primary", t("Save and restart"));
    save.onclick = () => {
      let model = null;
      if (pick.value === "custom") model = { repo: repo.value.trim(), file: file.value.trim() };
      else { const [r, f] = pick.value.split("|"); model = { repo: r, file: f }; }
      act("save", { model, hfEndpoint: from.value, context: +ctx.value });
    };
    card.append(save);
    return card;
  }

  function routeCard() {
    const g = data.hybrid;
    const card = el("section", "card lm-card");
    card.append(el("h3", "", t("Edge-cloud route")));
    card.append(el("p", "lm-note", t("Each user turn is judged by the local model as it begins: what it can do stays here, the rest goes to the cloud. A turn stays where it went; a local turn that fails goes on in the cloud.")));
    if (!g) {
      card.append(el("p", "lm-note", t("No edge-cloud group yet: it is made once the local model serves and a cloud provider is added.")));
      return card;
    }
    const list = el("div", "lm-cloud");
    for (const m of g.cloud) {
      const item = el("div", "lm-member");
      item.append(el("span", "", m));
      const rm = el("button", "icon", "×");
      rm.title = t("Remove");
      rm.onclick = () => act("cloud", { members: g.cloud.filter((x) => x !== m) });
      item.append(rm);
      list.append(item);
    }
    const add = el("select");
    add.append(el("option", "", t("Add")));
    for (const id of g.choices.filter((x) => !g.cloud.includes(x))) {
      const o = el("option", "", id);
      o.value = id;
      add.append(o);
    }
    add.onchange = () => add.value && act("cloud", { members: [...g.cloud, add.value] });
    list.append(add);
    card.append(row("Cloud models", list));

    const sw = el("label", "switch");
    const box = el("input");
    box.type = "checkbox";
    box.checked = !!g.mask;
    box.onchange = () => act("mask", { on: box.checked });
    sw.append(box, el("span"));
    card.append(row("Masking", sw));
    card.append(el("p", "lm-note", t("On: a sensitive turn the local model can do stays here even if it fails; one it can't goes to the cloud with what is sensitive masked by the local model, and comes back unmasked.")));
    card.append(el("p", "lm-note", t("Point agents at group/auto on the Agents page to use it.")));
    return card;
  }

  function draw() {
    const nodes = [];
    const head = el("header", "lm-top");
    head.append(el("h2", "", t("The model on this computer")));
    head.append(el("p", "lm-note", t("Runs a small model on this computer with llama.cpp, pulled from Hugging Face. Fogway serves it as local/…, and group/auto sends each turn to it or to the cloud.")));
    nodes.push(head);
    if (failed) nodes.push(el("p", "lm-err", failed));
    if (data) nodes.push(statusCard(), setupCard(), routeCard());
    page.replaceChildren(...nodes);
  }
})();
