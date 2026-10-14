// Run with Node's test runner and Playwright on the module path; see README.md.
// Copying from a browser tab with no clipboard API (悠悠哥 on Discord: on
// `fogway web` on a NAS, opened from another computer over http, the
// gateway's API key copy did nothing). fogway can't copy for such a tab
// (/api/copy refuses) and the page, not a secure context, has no
// navigator.clipboard; the copy command still copies there. Tried on the
// Gateway page's Base URL. English and Chinese, Chromium and WebKit; no
// backend, the API is faked here.
const assert = require("node:assert/strict");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { fixture } = require("./fixtures/caller-keys.cjs");

// the Gateway page as the fixture has it, with fogway's own copy refused
function server(lang) {
  const rest = fixture(lang, "light", []);
  return async (route) => {
    if (new URL(route.request().url()).pathname === "/api/copy") return route.fulfill({ status: 501, body: "" });
    return rest(route);
  };
}

const W = {
  en: { copied: "Base URL copied" },
  zh: { copied: "已复制Base URL" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    test(`${engine} ${lang}: a tab with no clipboard API copies with the copy command`, async (t) => {
      const w = W[lang];
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 900, height: 800 }, reducedMotion: "reduce" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      // a page over plain http from another computer: no clipboard API, and
      // the copy command is noted with what was selected when it ran
      await page.addInitScript(() => {
        Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
        window.__copied = [];
        document.execCommand = (cmd) => {
          if (cmd !== "copy") return false;
          const a = document.activeElement;
          window.__copied.push(a && a.tagName === "TEXTAREA" ? a.value.slice(a.selectionStart, a.selectionEnd) : String(getSelection()));
          return true;
        };
      });
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang));
      await page.goto("http://fogway.test/?view=gateway");
      const val = page.locator("#connect .val").first();
      await val.locator("code").waitFor();
      const base = (await val.locator("code").textContent()).trim();
      assert.match(base, /^http:\/\/127\.0\.0\.1:\d+\/v1$/);
      const cp = val.locator("button.copy");

      const scrolls = () => page.evaluate(() => [window.scrollY, ...[...document.querySelectorAll("#view-gateway, #view-gateway *")].filter((e) => e.scrollTop).map((e) => e.scrollTop)]);
      const before = await scrolls();
      await cp.click();
      await page.waitForFunction((s) => document.querySelector("#status").textContent === s, w.copied);
      assert.deepEqual(await page.evaluate(() => window.__copied), [base]);
      assert.equal(await page.locator("textarea[readonly]").count(), 0, "the textarea is gone");
      assert.notEqual(await page.evaluate(() => document.activeElement?.tagName), "TEXTAREA", "the focus went back");
      assert.deepEqual(await scrolls(), before, "nothing scrolled");
      assert.deepEqual(errors, []);
    });
  }
}
