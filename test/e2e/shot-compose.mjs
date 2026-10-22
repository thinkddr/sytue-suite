// Captura del pie del compositor de Mail tras agrupar las acciones.
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
await page.getByRole('button', { name: /Redactar|Nuevo mensaje/ }).first().click();
const dialog = page.getByRole('dialog').first();
await dialog.waitFor({ timeout: 20_000 });
await page.waitForTimeout(1500);
await page.screenshot({ path: '/tmp/compose-pie.png' });

// Segundo disparo con el menú de adjuntar abierto.
await dialog.getByRole('button', { name: 'Adjuntar' }).click();
await page.waitForTimeout(600);
await page.screenshot({ path: '/tmp/compose-menu.png' });
console.log('OK compose');
await browser.close();
