// Run with Node's test runner and Playwright on the module path; see README.md.
// A copy fogway couldn't make (#1881, BabyLy233: on KDE Plasma Wayland the
// app said "copied" and the clipboard stayed empty). fogway now refuses a
// copy it couldn't read back (/api/copy answers 501), and the page tries the
// clipboard API, then the copy command, and says it failed only when none
// copied, with the text to copy by hand. A copy fogway made is not copied
// again. Tried on the Gateway page's Base URL. Every language, Chromium and
// WebKit; no backend, the API is faked.
const assert = require("node:assert/strict");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { fixture } = require("./fixtures/caller-keys.cjs");

// the Gateway page as the fixture has it; fogwayCopies: /api/copy copies
function server(lang, fogwayCopies) {
  const rest = fixture(lang, "light", []);
  return async (route) => {
    if (new URL(route.request().url()).pathname === "/api/copy") return route.fulfill({ status: fogwayCopies ? 204 : 501, body: "" });
    return rest(route);
  };
}

const W = {
  en: { copied: "Base URL copied", failed: "Couldn't copy. Select it here and copy it yourself: " },
  zh: { copied: "已复制Base URL", failed: "复制失败，请在这里选中后自行复制：" },
  "zh-TW": { copied: "已複製Base URL", failed: "複製失敗，請在這裡選取後自行複製：" },
  ja: { copied: "Base URLをコピーしました", failed: "コピーできませんでした。ここで選択してコピーしてください：" },
  de: { copied: "Base URL kopiert", failed: "Kopieren fehlgeschlagen. Hier markieren und selbst kopieren: " },
};

// fogway: /api/copy copies; api: the clipboard API copies; command: the
// copy command copies; none: nothing does. writes and commands: whether
// the clipboard API and the copy command are asked
const CASES = [
  { name: "fogway copies", fogway: true, api: true, command: true, want: "copied", writes: false, commands: false },
  { name: "fogway refuses, the clipboard API copies", fogway: false, api: true, command: true, want: "copied", writes: true, commands: false },
  { name: "fogway and the clipboard API refuse, the copy command copies", fogway: false, api: false, command: true, want: "copied", writes: true, commands: true },
  { name: "nothing copies", fogway: false, api: false, command: false, want: "failed", writes: true, commands: true },
];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(W)) {
    for (const c of CASES) {
      test(`${engine} ${lang}: ${c.name}`, async (t) => {
        const w = W[lang];
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        const page = await (await browser.newContext({ viewport: { width: 900, height: 800 }, reducedMotion: "reduce" })).newPage();
        t.after(() => browser.close());
        page.setDefaultTimeout(5000);
        await page.addInitScript(({ api, command }) => {
          window.__writes = [];
          window.__commands = [];
          Object.defineProperty(navigator, "clipboard", {
            configurable: true,
            value: { writeText: (s) => { window.__writes.push(s); return api ? Promise.resolve() : Promise.reject(new DOMException("refused", "NotAllowedError")); } },
          });
          document.execCommand = (cmd) => {
            if (cmd !== "copy") return false;
            const a = document.activeElement;
            window.__commands.push(a && a.tagName === "TEXTAREA" ? a.value.slice(a.selectionStart, a.selectionEnd) : String(getSelection()));
            return command;
          };
        }, { api: c.api, command: c.command });
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", server(lang, c.fogway));
        await page.goto("http://fogway.test/?view=gateway");
        const val = page.locator("#connect .val").first();
        await val.locator("code").waitFor();
        const base = (await val.locator("code").textContent()).trim();
        assert.match(base, /^http:\/\/127\.0\.0\.1:\d+\/v1$/);
        const cp = val.locator("button.copy");

        await cp.click();
        const want = c.want === "failed" ? w.failed + base : w.copied;
        await page.waitForFunction((s) => document.querySelector("#status").textContent === s, want);
        const st = page.locator("#status");
        if (c.want === "failed") {
          assert.ok(await st.evaluate((e) => e.classList.contains("err")), "the failure reads as an error");
          assert.equal(await st.getAttribute("role"), "alert");
          assert.ok(await st.isVisible(), "the failure is shown");
          assert.equal(await cp.evaluate((b) => b.classList.contains("done")), false, "the button doesn't say it copied");
        } else {
          assert.ok(await st.evaluate((e) => e.classList.contains("ok")));
          assert.ok(await cp.evaluate((b) => b.classList.contains("done")), "the button says it copied");
        }
        assert.deepEqual(await page.evaluate(() => window.__writes), c.writes ? [base] : [], "clipboard API calls");
        assert.deepEqual(await page.evaluate(() => window.__commands), c.commands ? [base] : [], "copy command calls");
        assert.equal(await page.locator("textarea[readonly]").count(), 0, "the textarea is gone");
        assert.deepEqual(errors, []);
      });
    }
  }
}
