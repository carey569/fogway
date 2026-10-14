// Run with Node's test runner and Playwright on the module path; see README.md.
// A plan with many keys (Linx on X: twenty-odd accounts on one provider
// made the Usage page a tower) shows the first five on its card, and any in
// full, the rest behind "Show N more accounts" at the card's foot. The
// button opens them in place, the page not moving, and folds them again; it
// is remembered across a reload. Eight keys aren't folded. At 440px nothing in the card is cut or spills.
// English, Chinese, Japanese and German; no backend, the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const now = new Date().toISOString();
const pad = (i) => String(i).padStart(2, "0");
const acct = (provider, name, user, used, extra) => ({ provider, name, kind: "plan", icon: "generic", plan: "Pro", user, readAt: now, windows: [{ name: "Daily", used }], ...extra });
const three = (used) => ["Daily", "Weekly", "Monthly"].map((name) => ({ name, used }));
const quotas = () => [
  // 22 keys: 5 in sight, 17 folded
  ...Array.from({ length: 22 }, (_, i) => acct("zhipu", "Zhipu GLM", `wb-${pad(i + 1)}`, 3 * i)),
  // 9 keys, the 8th in full (it answered last); with three windows each, so
  // the column beside the first card stays taller than it folded
  ...Array.from({ length: 9 }, (_, i) => acct("kimi", "Kimi Code", `t-${i + 1}`, 5 * i, { windows: three(5 * i), ...(i === 7 ? { lastServedAt: now } : {}) })),
  // 8 keys: not folded
  ...Array.from({ length: 8 }, (_, i) => acct("minimax", "MiniMax", `c${i + 1}`, 10 * i, { windows: three(10 * i) })),
];

function serve(lang) {
  const settings = { theme: "light", lang, tray: "panel", quotaLeft: false, currency: "usd" };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/usage/quotas") return json(quotas());
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const words = {
  en: { more17: "Show 17 more accounts", more3: "Show 3 more accounts", fewer: "Show fewer accounts" },
  zh: { more17: "展开其余 17 个账号", more3: "展开其余 3 个账号", fewer: "收起其余账号" },
  ja: { more17: "さらに 17 個のアカウントを表示", more3: "さらに 3 個のアカウントを表示", fewer: "アカウントの表示を減らす" },
  de: { more17: "17 weitere Konten anzeigen", more3: "3 weitere Konten anzeigen", fewer: "Weniger Konten anzeigen" },
};

// each card: the accounts in sight and its foot button
const cards = (page) => page.evaluate(() => Object.fromEntries([...document.querySelectorAll("#subscriptionUsage > .subscription-card")].map((c) => [c.dataset.key, {
  users: [...c.querySelectorAll(".subscription-account .user")].filter((u) => u.offsetParent).map((u) => u.title),
  more: c.querySelector(".quota-accts-more")?.textContent ?? null,
  height: Math.round(c.getBoundingClientRect().height),
}])));

// nothing in the card cut short or past its edge
const fits = (page, key) => page.evaluate((key) => {
  const c = document.querySelector(`#subscriptionUsage > [data-key="${key}"]`);
  const box = c.getBoundingClientRect();
  const bad = [];
  if (document.documentElement.scrollWidth > innerWidth) bad.push("page scrolls sideways");
  if (c.scrollWidth > c.clientWidth) bad.push("card spills");
  for (const e of c.querySelectorAll(".subscription-head > *, .quota-accts-more, .subscription-account .user")) {
    if (!e.offsetParent) continue;
    const r = e.getBoundingClientRect();
    if (r.left < box.left - 0.5 || r.right > box.right + 0.5) bad.push(`${e.className || e.tagName} past the card`);
    if (e.matches(".quota-accts-more") && e.scrollWidth > e.clientWidth + 0.5) bad.push(`button cut: ${e.textContent}`);
  }
  return bad;
}, key);

const wb = (i) => `wb-${pad(i)}`;
const range = (a, b) => Array.from({ length: b - a + 1 }, (_, i) => wb(a + i));

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "ja", "de"]) {
    const w = words[lang];
    test(`${engine} ${lang}: many accounts fold behind a button and open in place`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const context = await browser.newContext({ viewport: { width: 900, height: 500 }, reducedMotion: "reduce" });
      const page = await context.newPage();
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.addInitScript(() => { try { if (!sessionStorage.getItem("set")) { localStorage.clear(); localStorage.setItem("fogway.quotaRange", "off"); sessionStorage.setItem("set", "1"); } } catch {} });
      await page.goto("http://fogway.test/?view=usage");
      const sel = '#subscriptionUsage > [data-key="zhipu"]';
      await page.waitForSelector(`${sel} .subscription-account .user`, { state: "attached" });

      let got = await cards(page);
      assert.deepEqual(got.zhipu.users, range(1, 5));
      assert.equal(got.zhipu.more, w.more17);
      // the one in full stays in sight
      assert.deepEqual(got.kimi.users, ["t-1", "t-2", "t-3", "t-4", "t-5", "t-8"]);
      assert.equal(got.kimi.more, w.more3);
      // eight: all in sight, no button
      assert.equal(got.minimax.users.length, 8);
      assert.equal(got.minimax.more, null);
      const foldedHeight = got.zhipu.height;

      // opening them doesn't move the page
      const view = page.locator("#view-usage");
      const more = page.locator(`${sel} .quota-accts-more`);
      const head = page.locator(`${sel} .subscription-head`);
      await page.mouse.move(450, 300);
      for (let i = 0; i < 20 && (await more.boundingBox()).y > 400; i++) {
        await page.mouse.wheel(0, 80);
        await page.waitForTimeout(150);
      }
      await page.waitForTimeout(300);
      const before = await view.evaluate((v) => v.scrollTop);
      const headAt = await head.evaluate((e) => e.getBoundingClientRect().top);
      const box = await more.boundingBox();
      assert.ok(box.y > 0 && box.y + box.height < 500, "the button is in sight");
      await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
      await page.waitForTimeout(500);
      assert.equal(await view.evaluate((v) => v.scrollTop), before, "the view didn't scroll");
      assert.equal(await head.evaluate((e) => e.getBoundingClientRect().top), headAt, "the card stays where it was");
      got = await cards(page);
      assert.deepEqual(got.zhipu.users, range(1, 22));
      assert.equal(got.zhipu.more, w.fewer);
      assert.equal(await more.getAttribute("aria-expanded"), "true");
      assert.ok(got.zhipu.height > foldedHeight, "the card grew");
      assert.equal(got.kimi.more, w.more3, "another card stays as it was");

      // remembered across a reload
      await page.reload();
      await page.waitForSelector(`${sel} .subscription-account .user`, { state: "attached" });
      got = await cards(page);
      assert.equal(got.zhipu.users.length, 22);

      // and folded again from its foot
      const fewer = page.locator(`${sel} .quota-accts-more`);
      await page.mouse.move(450, 300);
      for (let i = 0; i < 40 && (await fewer.boundingBox()).y > 400; i++) {
        await page.mouse.wheel(0, 120);
        await page.waitForTimeout(150);
      }
      await page.waitForTimeout(300);
      const at = await head.evaluate((e) => e.getBoundingClientRect().top);
      const fb = await fewer.boundingBox();
      assert.ok(fb.y > 0 && fb.y + fb.height < 500, "the button is in sight");
      await page.mouse.click(fb.x + fb.width / 2, fb.y + fb.height / 2);
      await page.waitForTimeout(500);
      got = await cards(page);
      assert.deepEqual(got.zhipu.users, range(1, 5));
      assert.equal(got.zhipu.more, w.more17);
      // the card's head stays where it was, as a provider's keys' does: the
      // cards packed into columns (#860), the next goes on under this one, so
      // the page no longer has to end there and come down to the button
      assert.equal(await head.evaluate((e) => e.getBoundingClientRect().top), at, "the card's head stays where it was");

      // 440px: nothing cut or spilling, folded and open
      await page.setViewportSize({ width: 440, height: 700 });
      await page.waitForTimeout(300);
      for (const k of ["zhipu", "kimi", "minimax"]) assert.deepEqual(await fits(page, k), [], `${k} fits at 440px, folded`);
      await page.locator(`${sel} .quota-accts-more`).evaluate((b) => b.click());
      await page.waitForFunction(() => document.querySelectorAll('#subscriptionUsage > [data-key="zhipu"] .subscription-account').length === 22);
      assert.deepEqual(await fits(page, "zhipu"), [], "fits at 440px, open");
      assert.deepEqual(errors, []);
    });
  }
}
