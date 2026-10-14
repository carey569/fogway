// Run with Node's test runner and Playwright on the module path; see README.md.
// The sidebar's list of a page's tabs (app.js navSubs) unrolls as the page
// opens. A list drawn again while it unrolls (a page that draws its strip
// once more as its data comes in, as Plugins' Installed tab once did with
// its count) put the selection where the rows were squeezed into the
// unrolling list, and it stayed there: on the second tab it sat a third of
// a row up, over the first tab's text, and shorter than the row (#1787).
// The rows keep their size while the list unrolls, so the selection is on
// its row however early the page draws its strip again. Here Usage is left
// on Requests, its second tab, which gets a count mid-unroll, as Installed
// did: a strip drawn again the same is left as it is (navSubs' sig).
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

function serve(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { url: "http://127.0.0.1:3725", running: true, window: true } });
    if (url.pathname === "/api/groups") return json({ groups: [], pools: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname.startsWith("/api/")) return json({});
    if (!/^\/[\w./-]*$/.test(url.pathname) || url.host !== "fogway.test") return route.fulfill({ status: 404, body: "" });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const requestsWord = { en: "Requests", zh: "请求", "zh-TW": "請求", ja: "リクエスト", de: "Anfragen" };

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: the sidebar's selection sits on its row when the list is drawn again as it unrolls`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of Object.keys(requestsWord)) for (const width of [1024, 1440]) await t.test(`${lang} ${width}`, async () => {
      const context = await browser.newContext({ viewport: { width, height: 760 } });
      // the reader left Usage on Requests
      await context.addInitScript(() => { try { localStorage.setItem("fogway.usageTab", "requests"); } catch {} });
      const page = await context.newPage();
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.goto("http://fogway.test/");
      await page.waitForSelector("#nav button.on");
      assert.ok(await page.evaluate(() => document.body.classList.contains("side-nav")), "a sidebar at " + width);

      await page.locator('#nav button[data-view="usage"]').click();
      const box = page.locator("#side .nav-sub:not(.leaving)");
      await box.waitFor();
      // the page's strip changes while the list is still unrolling
      await page.waitForFunction(() => document.querySelector(".nav-sub:not(.leaving)")?.getAnimations().some((a) => a.playState === "running"));
      await page.evaluate(() => { document.querySelectorAll("#usageTab .opt")[1].textContent += " · 7"; });
      await page.waitForFunction((w) => document.querySelector(".nav-s.cur")?.textContent === w + "7", requestsWord[lang]);
      // every row, the list and its selection, done moving
      await page.waitForFunction(() => {
        const b = document.querySelector(".nav-sub:not(.leaving)");
        return b && [b, ...b.querySelectorAll("*")].every((e) => e.getAnimations().every((a) => a.playState !== "running"));
      });
      await page.waitForTimeout(350); // the selection's own transition (.3s)

      const [rows, hl] = await page.evaluate(() => {
        const r = (e) => { const b = e.getBoundingClientRect(); return { top: b.top, bottom: b.bottom, text: e.textContent }; };
        return [[...document.querySelectorAll(".nav-sub:not(.leaving) .nav-s")].map(r), r(document.querySelector(".nav-sub:not(.leaving) .nav-hl"))];
      });
      assert.equal(rows.length, 4);
      assert.ok(rows[1].top >= rows[0].bottom, `the rows don't overlap: ${JSON.stringify(rows)}`);
      assert.ok(Math.abs(hl.top - rows[1].top) < 1 && Math.abs(hl.bottom - rows[1].bottom) < 1,
        `the selection is on Requests (${rows[1].top}–${rows[1].bottom}), not ${hl.top}–${hl.bottom}`);
      assert.ok(hl.top >= rows[0].bottom, "and nothing of it over Overview");
      assert.deepEqual(errors, []);
      await context.close();
    });
  });
}
