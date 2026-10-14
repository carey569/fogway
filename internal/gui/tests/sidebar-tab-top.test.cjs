// Run with Node's test runner and Playwright on the module path; see README.md.
// #1844: a tab picked in the sidebar opens at its top (app.js navSubs). A page's
// tabs share its one scroll: scrolled to the foot of Settings › General and
// Models picked, the new tab was shown from where the old one had been left,
// at its foot (Library's tabs, cut since, did the same). The tab already
// open, picked again, leaves the page where it is, and another page and back
// finds it where it was left. The page's own strip is unchanged. Chromium
// and WebKit, every language; no backend, the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

function server(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { url: "http://127.0.0.1:3725", running: true, window: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const sub = (page, name) => page.locator("#side .nav-sub:not(.leaving) .nav-s", { hasText: name });
const top = (page, v) => page.locator(v).evaluate((e) => e.scrollTop);
// down to the view's foot by the wheel, as the reader goes there
async function toFoot(page, v) {
  const box = await page.locator(v).boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < 40; i++) {
    await page.mouse.wheel(0, 300);
    await page.waitForTimeout(20);
    if (await page.locator(v).evaluate((e) => e.scrollTop + e.clientHeight >= e.scrollHeight - 1)) break;
  }
  await page.waitForTimeout(300);
  const at = await top(page, v);
  assert.ok(at > 100, `${v} scrolled down: ${at}`);
  return at;
}
// the click's hold has had its frames and the page what it loads
const settle = (page) => page.waitForTimeout(500);
const room = (page, v) => page.locator(v).evaluate((e) => e.scrollHeight - e.clientHeight);

// Settings' tabs taller than a 480-high window: General, Models, Observability
const words = {
  en: { models: "Models", otel: "Observability" },
  zh: { models: "模型", otel: "可观测性" },
  "zh-TW": { models: "模型", otel: "可觀測性" },
  ja: { models: "モデル", otel: "オブザーバビリティ" },
  de: { models: "Modelle", otel: "Observability" },
};
const set = "#view-settings";
const picked = (page, id) => page.waitForFunction((id) => document.querySelector("#setTab-" + id).classList.contains("on"), id);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: a tab picked in the sidebar opens at its top`, async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());

    for (const [lang, w] of Object.entries(words)) await t.test(lang, async () => {
      const context = await browser.newContext({ viewport: { width: 1100, height: 480 }, reducedMotion: "reduce" });
      await context.addInitScript(() => { try { localStorage.setItem("fogway.settingsTab", "general"); } catch {} });
      const page = await context.newPage();
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang));
      await page.goto("http://fogway.test/");
      await page.waitForSelector("#nav button.on");
      assert.ok(await page.evaluate(() => document.body.classList.contains("side-nav")), "the sidebar is shown");

      // Settings › General at its foot, then Models
      await page.locator("#prefs").click();
      await sub(page, w.models).waitFor();
      await toFoot(page, set);
      await sub(page, w.models).click();
      await picked(page, "models");
      await settle(page);
      assert.ok(await room(page, set) > 100, "Models is taller than the window");
      assert.equal(await top(page, set), 0, "Models opens at its top");

      // Models at its foot, picked again: the page stays there
      const at = await toFoot(page, set);
      await sub(page, w.models).click();
      await settle(page);
      assert.equal(await top(page, set), at, "the tab already open leaves the page where it is");

      // from the foot of Models, Observability; and back from its foot
      await sub(page, w.otel).click();
      await picked(page, "otel");
      await settle(page);
      assert.ok(await room(page, set) > 100, "Observability is taller than the window");
      assert.equal(await top(page, set), 0, "Observability opens at its top");
      await toFoot(page, set);
      await sub(page, w.models).click();
      await picked(page, "models");
      await settle(page);
      assert.equal(await top(page, set), 0, "Models opens at its top");

      // another page and back: Settings is where it was left, as before
      const setAt = await toFoot(page, set);
      await page.locator('#nav button[data-view="usage"]').click();
      await settle(page);
      await page.locator("#prefs").click();
      await settle(page);
      assert.equal(await top(page, set), setAt, "Settings is where it was left");

      assert.deepEqual(errors, []);
      await context.close();
    });
  });
}

// The page's own strip (the header's tabs: the sidebar turned off, or a window
// narrower than SIDE_AT) is unchanged: a tab clicked there, like every click,
// leaves the page where it is (73af4c28, "where the reader is").
for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: a tab picked in the page's own strip leaves the page where it is`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const [name, width, layout] of [["sidebar off", 1100, "top"], ["narrow", 900, ""]]) for (const lang of Object.keys(words)) await t.test(`${name}, ${lang}`, async () => {
      const context = await browser.newContext({ viewport: { width, height: 480 }, reducedMotion: "reduce" });
      await context.addInitScript((layout) => { try { localStorage.setItem("fogway.settingsTab", "general"); if (layout) localStorage.setItem("fogway.navLayout", layout); } catch {} }, layout);
      const page = await context.newPage();
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang));
      await page.goto("http://fogway.test/");
      await page.waitForSelector("#nav button.on");
      assert.ok(!(await page.evaluate(() => document.body.classList.contains("side-nav"))), "no sidebar");

      await page.locator("#prefs").click();
      await page.locator("#setTab-models").waitFor();
      const at = await toFoot(page, set);
      // Settings' strip scrolls away with its page: clicked where it is, not
      // brought into sight first as Playwright's click would
      await page.locator("#setTab-models").evaluate((b) => b.click());
      await picked(page, "models");
      await settle(page);
      const after = await top(page, set);
      assert.ok(after > 100, `the strip's click left Settings where it was, not at its top: ${at} then ${after}`);
      assert.deepEqual(errors, []);
      await context.close();
    });
  });
}
