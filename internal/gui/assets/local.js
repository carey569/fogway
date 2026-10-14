// Local: the model on this computer and the edge-cloud route. The
// assessment card measures this computer and recommends Spark X2.5 4B, the
// 1.7B, or no local model, with the speeds each would run at (estimated,
// then measured once one runs); the status card says where llama.cpp and
// the model are (downloading, loading, serving, or why not); the setup
// picks the model — the one recommended, another of the catalog, or any
// GGUF on Hugging Face — and where it is pulled from; the route card is group/auto,
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
    "about {gb}": "约 {gb}",
    "tight on memory": "内存吃紧",
    "The memory shown is taken as the model loads, whether the window fills or not. A long conversation is slow to read the first time: the 4B took about 15 minutes over 140K tokens on an M4 Pro; after that only what is new is read.": "所示内存在模型加载时就会占用，不论窗口是否用满。很长的对话第一次读入会很慢：在 M4 Pro 上，4B 读 14 万 token 用了约 15 分钟；之后每轮只读新增的部分。",
    "too much for this computer": "本机内存不够",
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
    "This computer's assessment": "本机评测",
    "Assess again": "重新评测",
    "Assessing…": "正在评测…",
    "Recommended: {model}": "推荐安装：{model}",
    "A local model isn't recommended for this computer": "不建议在本机运行本地模型",
    "Spark X2.5 1.7B": "星火 X2.5 1.7B",
    "Spark X2.5 4B": "星火 X2.5 4B",
    "Qwen3.5 2B": "通义千问 Qwen3.5 2B",
    "Qwen3.5 4B": "通义千问 Qwen3.5 4B",
    "Qwen3.5 9B": "通义千问 Qwen3.5 9B",
    "{big} judges a turn in about {bj} s here and writes {bg} tokens a second.": "{big} 在本机判断一轮约 {bj} 秒，每秒生成 {bg} 个 token，运行流畅。",
    "{big} would take {bneed}, too much of this computer's {ram}; {small} judges a turn in about {sj} s.": "{big} 需要约 {bneed} 内存，占本机 {ram} 的比例过高；{small} 判断一轮约 {sj} 秒。",
    "The disk has {disk} free, too little for {big} ({bsize}); {small} judges a turn in about {sj} s.": "磁盘只剩 {disk}，放不下{big}（{bsize}）；{small} 判断一轮约 {sj} 秒。",
    "{big} would take about {bj} s to judge each turn; {small} about {sj} s.": "{big} 判断一轮约需 {bj} 秒，偏慢；{small} 约 {sj} 秒。",
    "Even {small} would take {sneed}, too much of this computer's {ram}.": "本机内存 {ram}，连{small}（约 {sneed}）也放不下或过于吃紧。",
    "The disk has {disk} free, too little for {small} ({ssize}). Free some space first.": "磁盘只剩 {disk}，放不下{small}（{ssize}），请先清理磁盘空间。",
    "Even {small} would take about {sj} s to judge each turn and write {sg} tokens a second: every turn would wait on it.": "即使是{small}，判断一轮也要约 {sj} 秒、每秒只生成 {sg} 个 token，每轮对话都会明显变慢。",
    "group/auto needs a local model; without one, agents use the cloud models directly. To try anyway, pick Spark X2.5 1.7B below: it is measured once it runs.": "端云路由（group/auto）需要本地模型；不装的话，Agent 直接使用云端模型即可。如仍想尝试，可在下方选择星火 X2.5 1.7B，运行后会自动实测速度。",
    "group/auto needs a local model; without one, agents use the cloud models directly.": "端云路由（group/auto）需要本地模型；不装的话，Agent 直接使用云端模型即可。",
    "{gb} memory": "占用内存 {gb}",
    "judges a turn in {s} s": "判断一轮约 {s} 秒",
    "{n} tokens/s": "每秒 {n} token",
    "Smooth": "流畅",
    "Usable": "可用",
    "Too slow": "太慢",
    "Takes too much memory": "内存吃紧",
    "Doesn't fit in memory": "内存不足",
    "No room on the disk": "磁盘空间不足",
    "llama.cpp can't run here": "无法运行 llama.cpp",
    "llama.cpp's builds for this computer need glibc {need} or newer, and this Linux has {have}: {systems} run them.": "本机对应的 llama.cpp 构建需要 glibc {need} 或更新版本，而这台 Linux 是 {have}：需要 {systems} 才能运行。",
    "Ubuntu 22.04, Debian 12 or later": "Ubuntu 22.04、Debian 12 或更新的系统",
    "Ubuntu 24.04, Debian 13 or later": "Ubuntu 24.04、Debian 13 或更新的系统",
    "measured": "实测",
    "estimated": "估算",
    "estimated, set by a measurement": "估算（已按实测校正）",
    "{n} cores": "{n} 核",
    "GPU {n} cores, memory shared": "GPU {n} 核（与内存共享）",
    "{gb} free on the disk": "磁盘剩余 {gb}",
    "Measured": "硬件测得",
    "memory read at {bw} GB/s · {c} billion multiply-adds a second": "内存读取 {bw} GB/s · 每秒 {c10} 亿次乘加",
    "Speeds marked measured come from the model running here; the others are estimated and set right by it.": "标为“实测”的是模型在本机实际运行测得的速度，其余为估算，并已按实测结果校正。",
    "Estimates from this computer's figures. A model is measured once it runs here, and that is what counts.": "以上为根据本机硬件测得的估算值；模型装好运行后会自动实测一次，以实测为准。",
    "Use {model}": "安装并使用{model}",
    "Measuring…": "正在实测…",
    "Measure the running model": "实测当前模型",
    "for 8 GB of memory: routes well and does small tasks; the 4B catches more of what is sensitive": "适合 8 GB 内存：路由判断够用，能做小任务；识别敏感内容不如 4B 稳",
    "for 16 GB: steadier judgements, a fair coder for its size": "适合 16 GB 内存：判断更稳，写代码在同尺寸里不错",
    "Qwen's, for 8 GB": "通义千问，适合 8 GB 内存",
    "Qwen's, for 16 GB": "通义千问，适合 16 GB 内存",
    "Qwen's, for 32 GB, or a GPU with 10 GB and more": "通义千问，适合 32 GB 内存，或 10 GB 以上显存的显卡",
  });

  let data = null; // /api/local
  let failed = "";
  let poll = 0;
  let assessing = false, measuring = false, autoAssessed = false;

  const PHASES = {
    off: "Off", preparing: "Getting llama.cpp ready", downloading: "Downloading the model",
    starting: "Loading the model", ready: "Serving", failed: "Stopped",
  };
  const tokens = (n) => (n >= 1 << 20 && n % (1 << 20) === 0 ? n / (1 << 20) + "M" : n >= 1024 && n % 1024 === 0 ? n / 1024 + "K" : String(n));
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
    // never assessed here: measure this computer once, as the page opens
    if (!page.hidden && data?.assessment?.quick && !autoAssessed && !assessing) {
      autoAssessed = true;
      runAssess();
    }
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

  async function runAssess() {
    assessing = true;
    draw();
    await act("assess");
    assessing = false;
    draw();
  }

  async function runMeasure() {
    measuring = true;
    draw();
    await act("measure");
    measuring = false;
    draw();
  }

  const GRADES = {
    smooth: "Smooth", usable: "Usable", slow: "Too slow", tight: "Takes too much memory",
    memory: "Doesn't fit in memory", disk: "No room on the disk", runtime: "llama.cpp can't run here",
  };
  const WHY = {
    fast: "{big} judges a turn in about {bj} s here and writes {bg} tokens a second.",
    "4b-memory": "{big} would take {bneed}, too much of this computer's {ram}; {small} judges a turn in about {sj} s.",
    "4b-disk": "The disk has {disk} free, too little for {big} ({bsize}); {small} judges a turn in about {sj} s.",
    "4b-slow": "{big} would take about {bj} s to judge each turn; {small} about {sj} s.",
    memory: "Even {small} would take {sneed}, too much of this computer's {ram}.",
    disk: "The disk has {disk} free, too little for {small} ({ssize}). Free some space first.",
    slow: "Even {small} would take about {sj} s to judge each turn and write {sg} tokens a second: every turn would wait on it.",
    runtime: "llama.cpp's builds for this computer need glibc {need} or newer, and this Linux has {have}: {systems} run them.",
  };
  const LIBC_SYSTEMS = { "2.34": "Ubuntu 22.04, Debian 12 or later", "2.38": "Ubuntu 24.04, Debian 13 or later" };

  // reasonText is the assessment's why, with its figures, in the reader's
  // language: the candidates are the 1.7B and the 4B, smallest first
  function reasonText(a) {
    const [small = {}, big = {}] = a.models;
    const sec = (v) => (v || 0).toFixed(1);
    return WHY[a.why] ? t(WHY[a.why], {
      big: t(big.title), small: t(small.title), bj: sec(big.judge), sj: sec(small.judge),
      bg: Math.round(big.gen || 0), sg: Math.round(small.gen || 0), bneed: gb(big.need || 0), sneed: gb(small.need || 0),
      ram: gb(a.hardware?.ram || 0), disk: gb(a.disk || 0), bsize: gb(big.size || 0), ssize: gb(small.size || 0),
      need: a.libcNeed || "", have: (a.hardware?.libc || "").replace(/^glibc /, ""), systems: t(LIBC_SYSTEMS[a.libcNeed] || ""),
    }) : "";
  }

  // assessCard is this computer measured, and the model recommended for
  // it: Spark X2.5 4B, the 1.7B, or none
  function assessCard() {
    const a = data.assessment;
    const card = el("section", "card lm-card lm-assess");
    const head = el("div", "lm-head");
    head.append(el("h3", "", t("This computer's assessment")));
    const again = el("button", "text action", t(assessing ? "Assessing…" : "Assess again"));
    again.disabled = assessing || measuring;
    again.onclick = runAssess;
    head.append(again);
    card.append(head);
    if (!a?.models?.length) return card;
    const pick = a.models.find((m) => m.model === a.pick);
    const verdict = el("div", "lm-verdict " + (pick ? "lm-yes" : "lm-no"));
    verdict.append(el("strong", "", pick ? t("Recommended: {model}", { model: t(pick.title) }) : t("A local model isn't recommended for this computer")));
    verdict.append(el("p", "", reasonText(a)));
    if (!pick) verdict.append(el("p", "", t(a.why === "runtime"
      ? "group/auto needs a local model; without one, agents use the cloud models directly."
      : "group/auto needs a local model; without one, agents use the cloud models directly. To try anyway, pick Spark X2.5 1.7B below: it is measured once it runs.")));
    card.append(verdict);
    for (const m of a.models) {
      const r = el("div", "lm-row lm-cand");
      r.append(el("span", "lm-k", t(m.title) + (m.model === a.pick ? " ★" : "")));
      const facts = [t("{gb} memory", { gb: gb(m.need) }), t("judges a turn in {s} s", { s: (m.judge || 0).toFixed(1) }),
        t("{n} tokens/s", { n: Math.round(m.gen || 0) }), m.where === "gpu" ? "GPU" : "CPU"];
      r.append(el("span", "lm-facts", facts.join(" · ")));
      r.append(el("span", "lm-grade lm-g-" + m.grade, t(GRADES[m.grade] || m.grade)));
      r.append(el("span", "lm-how", t(m.measured ? "measured" : m.corrected ? "estimated, set by a measurement" : "estimated")));
      card.append(r);
    }
    const h = a.hardware || {};
    let comp = `${h.cpu || h.os + "/" + h.arch} · ${t("{n} cores", { n: h.cores || "?" })} · ${gb(h.ram || 0)} ${t("memory")}`;
    if (h.gpu) comp += ` · ${h.gpu} ${gb(h.vram || 0)}`;
    else if (h.unified && h.gpuCores) comp += " · " + t("GPU {n} cores, memory shared", { n: h.gpuCores });
    if (a.disk) comp += " · " + t("{gb} free on the disk", { gb: gb(a.disk) });
    card.append(row("Computer", comp));
    if (!a.quick && a.bench) {
      const c = a.bench.compute || 0;
      card.append(row("Measured", t("memory read at {bw} GB/s · {c} billion multiply-adds a second", { bw: Math.round(a.bench.bandwidth || 0), c: Math.round(c), c10: Math.round(c * 10) })));
    }
    card.append(el("p", "lm-note", t(a.models.some((m) => m.measured)
      ? "Speeds marked measured come from the model running here; the others are estimated and set right by it."
      : "Estimates from this computer's figures. A model is measured once it runs here, and that is what counts.")));
    const acts = el("div", "lm-acts");
    const cur = data.config?.model || {};
    const want = pick && data.catalog.find((c) => c.name === pick.model);
    if (want && (!data.config?.on || cur.repo !== want.repo || cur.file !== want.file)) {
      const use = el("button", "text action primary", t("Use {model}", { model: t(pick.title) }));
      use.disabled = assessing || measuring;
      use.onclick = () => act("save", { model: { repo: want.repo, file: want.file } });
      acts.append(use);
    }
    if (data.status?.phase === "ready") {
      const ms = el("button", "text action", t(measuring ? "Measuring…" : "Measure the running model"));
      ms.disabled = assessing || measuring;
      ms.onclick = runMeasure;
      acts.append(ms);
    }
    if (acts.childNodes.length) card.append(acts);
    return card;
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
    const toggle = el("button", c.on ? "text action" : "text action primary", t(c.on ? "Turn off" : "Turn on"));
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
    if (s.phase === "ready" && s.url) card.append(row("Serving", `${s.url} · ${tokens(s.context || c.context || 32768)} ${t("tokens")}`));
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
      const o = el("option", "", `${t(m.title || m.name)} · ${gb(m.size)} · ${t(m.note)}${m.name === data.recommended ? " ★ " + t("Recommended for this computer") : ""}`);
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
    card.append(row("Model", pick), customBox);

    const from = el("select");
    for (const [v, label] of [["", "Hugging Face"], ["https://hf-mirror.com", "hf-mirror.com (China)"]]) {
      const o = el("option", "", t(label));
      o.value = v;
      from.append(o);
    }
    from.value = c.hfEndpoint || "";
    card.append(row("Download from", from));

    // the windows the model picked can run at here, each with the memory it
    // takes; one that doesn't fit can't be picked, and the window kept is
    // the largest that fits within the one asked for
    const ctx = el("select");
    const windows = () => {
      const want = +ctx.value || c.context || 32768;
      const ws = data.windows?.[pick.value] || data.windows?.custom || [];
      ctx.replaceChildren();
      let keep = 0;
      for (const w of ws) {
        let label = `${tokens(w.tokens)} ${t("tokens")}`;
        if (w.need) label += " · " + t("about {gb}", { gb: gb(w.need) });
        if (w.room === "tight") label += " · " + t("tight on memory");
        if (w.room === "memory") label += " · " + t("too much for this computer");
        const o = el("option", "", label);
        o.value = w.tokens;
        o.disabled = w.room === "memory";
        ctx.append(o);
        if (!o.disabled && w.tokens <= want) keep = w.tokens;
      }
      ctx.value = keep || ws[0]?.tokens || 32768;
    };
    windows();
    pick.onchange = () => {
      customBox.hidden = pick.value !== "custom";
      windows();
    };
    card.append(row("Context window", ctx));
    card.append(el("p", "lm-note", t("The memory shown is taken as the model loads, whether the window fills or not. A long conversation is slow to read the first time: the 4B took about 15 minutes over 140K tokens on an M4 Pro; after that only what is new is read.")));

    const save = el("button", "text action primary", t("Save and restart"));
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
    if (data) nodes.push(assessCard(), statusCard(), setupCard(), routeCard());
    page.replaceChildren(...nodes);
  }
})();
