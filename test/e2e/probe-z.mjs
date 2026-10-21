import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
const S = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const b = await chromium.launch();
const c = await b.newContext({ storageState: S, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const p = await c.newPage();
await p.goto('https://rooms.sytue.localhost/', { waitUntil: 'domcontentloaded', timeout: 60000 });
await p.waitForURL((u) => u.hostname === 'rooms.sytue.localhost', { timeout: 60000 }).catch(() => {}); await p.locator('header').first().waitFor({ timeout: 60000 }).catch(() => {}); await p.waitForTimeout(4000); console.log('url:', p.url());
console.log(await p.evaluate(() => {
  const out = [];
  const aside = document.querySelector('aside');
  const header = document.querySelector('header');
  for (const [n, el] of [['aside', aside], ['header', header]]) {
    if (!el) { out.push(n + ': no existe'); continue; }
    const s = getComputedStyle(el);
    out.push(`${n}: pos=${s.position} z=${s.zIndex} class="${String(el.className).slice(0, 120)}"`);
  }
  return out.join('\n');
}));
await b.close();
