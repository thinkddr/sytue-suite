// Captura de la fila de la lista de Mail con el ratón encima: las acciones de
// triaje no deben taparle el asunto.
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
const page = await ctx.newPage();
await page.goto('https://mail.sytue.localhost/', { waitUntil: 'domcontentloaded' });
const fila = page.locator('li').filter({ hasText: 'Acta del comité' }).first();
await fila.waitFor({ timeout: 20_000 });
await page.waitForTimeout(2000);
await fila.hover();
await page.waitForTimeout(700);
await page.screenshot({ path: '/tmp/row-hover.png', clip: { x: 260, y: 60, width: 640, height: 420 } });
console.log('OK row-hover');
await browser.close();
