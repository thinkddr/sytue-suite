// Sonda: lee tokens CSS computados en una app en vivo (verificación del rediseño).
// ponytail: script de un solo uso; no es parte de la suite.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';

const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const url = process.argv[2] ?? 'https://one.sytue.localhost/';

const browser = await chromium.launch();
const ctx = await browser.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true });
const page = await ctx.newPage();
await page.goto(url, { waitUntil: 'commit', timeout: 60000 });
await page.waitForSelector('body', { timeout: 60000 });
await page.waitForTimeout(4000);
const vals = await page.evaluate(() => {
  const cs = getComputedStyle(document.documentElement);
  const get = (n) => cs.getPropertyValue(n).trim();
  return {
    border: get('--sytue-border'),
    borderSubtle: get('--sytue-border-subtle'),
    headerOne: get('--sytue-header-one'),
    headerDocs: get('--sytue-header-docs'),
    sidebarDocs: get('--sytue-sidebar-docs'),
    selected: get('--sytue-selected'),
  };
});
console.log(url, JSON.stringify(vals, null, 2));
await browser.close();
