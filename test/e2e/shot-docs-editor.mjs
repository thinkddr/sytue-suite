// Captura del editor de Sytue Docs tras el rediseño de paleta/líneas.
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
await page.goto('https://docus.sytue.localhost/', { waitUntil: 'domcontentloaded' });
// Abre el primer DOCUMENTO de texto reciente (tile de la cuadrícula).
await page.locator('a[href*="edit"]', { hasText: 'e2e-collab' }).first().click();
await page.waitForTimeout(6000);
await page.screenshot({ path: '/tmp/design-pass/docs-editor.png' });
console.log('OK docs-editor');
await browser.close();
