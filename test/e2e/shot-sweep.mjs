// Barrido visual: una captura por app para revisar el cromo con ojo crítico.
// ponytail: script de un solo uso para verificación; no es parte de la suite.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';

const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const APPS = process.argv.slice(2);

const browser = await chromium.launch();
const ctx = await browser.newContext({
  storageState: STORAGE,
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
const page = await ctx.newPage();
for (const app of APPS) {
  try {
    await page.goto(`https://${app}.sytue.localhost/`, {
      waitUntil: 'domcontentloaded',
      timeout: 60_000,
    });
    await page.waitForTimeout(7000);
    await page.screenshot({ path: `/tmp/sweep-${app}.png` });
    console.log('OK', app);
  } catch (e) {
    console.log('FALLO', app, String(e).split('\n')[0]);
  }
}
await browser.close();
