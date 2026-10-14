// Run with Node's test runner and Playwright on the module path; see README.md.
// A provider's editor has Duplicate (#268): the Add form opens on a copy
// named "{name} copy", and Add posts it as new, with copyOf, its models,
// headers and balance URL, and no key (the key is the copied provider's,
// taken by fogway). In English and Chinese.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "https://api.relay.example/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "m1", name: "m1", on: true }, { id: "m2", name: "m2", on: false }], agents: [], fallback: [], headers: { "X-Org": "acme" }, keyList: [],
  key: { set: true, masked: "sk-…1234" }, ready: true, balanceURL: "https://api.relay.example/q?key={key}", balancePath: "data.balance",
};
function serve(lang, list, posts) {
  const providers = { providers: list, presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/") && route.request().method() === "POST") {
      posts.push({ action: url.pathname.slice("/api/".length), body: route.request().postDataJSON() });
      return json(providers);
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { dup: "Duplicate", add: "Add", copy: "Relay copy", head: "Copy of Relay" },
  zh: { dup: "复制", add: "添加", copy: "Relay 副本", head: "Relay 的副本" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = words[lang];
    const open = async (t, list, name, wait) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const posts = [];
      await page.route("**/*", serve(lang, list, posts));
      await page.goto("http://fogway.test/?view=providers");
      await page.locator(".row.provider", { hasText: name }).first().click();
      await page.locator(wait).first().waitFor();
      return { page, errors, posts };
    };
    const posted = async (page, posts, n) => {
      for (let i = 0; i < 50 && posts.length === n; i++) await page.waitForTimeout(50);
      return posts.at(-1);
    };

    test(`${engine} ${lang}: Duplicate adds a copy of the provider`, async (t) => {
      const { page, errors, posts } = await open(t, [relay], "Relay", ".editor .bar");
      await page.locator(".editor .bar").getByRole("button", { name: w.dup, exact: true }).click();
      await page.locator(".editor", { hasText: w.head }).waitFor();
      const n = posts.length;
      await page.locator(".editor .bar").getByRole("button", { name: w.add, exact: true }).click();
      const saved = await posted(page, posts, n);
      assert.equal(saved.action, "provider/save");
      assert.equal(saved.body.new, true);
      assert.equal(saved.body.copyOf, "relay");
      assert.equal(saved.body.name, w.copy);
      assert.equal(saved.body.key, "", "the key is left to fogway to copy");
      assert.equal(saved.body.chat, relay.chat);
      assert.deepEqual(saved.body.models, ["m1"]);
      assert.equal(saved.body.balanceURL, relay.balanceURL);
      assert.equal(saved.body.balancePath, relay.balancePath);
      assert.equal(JSON.stringify(saved.body.headers).includes("acme"), true, JSON.stringify(saved.body.headers));
      const missing = await page.evaluate(() => [
        "Duplicate", "{name} copy", "Copy of {name}", "{masked} · {name}'s key, or paste another",
        "A new provider with {name}'s URLs, key, headers, models and balance settings, to change before adding",
      ].filter((k) => !I18N.zh[k]));
      assert.deepEqual(missing, [], "every string has its Chinese");
      assert.deepEqual(errors, []);
    });
  }
}
