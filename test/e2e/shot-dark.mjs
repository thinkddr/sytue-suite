// Captura en modo oscuro (docs home + drive) tras el rediseño de tokens.
// ponytail: script de un solo uso para verificación; no es parte de la suite.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';

const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));

const browser = await chromium.launch();
const ctx = await browser.newContext({
  storageState: STORAGE,
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
await ctx.addInitScript(() => localStorage.setItem('sytue-theme', 'dark'));
const page = await ctx.newPage();
for (const [name, url] of [
  ['docs-dark', 'https://docus.sytue.localhost/'],
  ['drive-dark', 'https://drive.sytue.localhost/'],
]) {
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(4000);
  await page.screenshot({ path: `/tmp/design-pass/${name}.png` });
  console.log('OK', name);
}
await browser.close();
