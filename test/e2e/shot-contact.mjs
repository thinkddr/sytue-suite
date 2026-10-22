// Captura de la cabecera de la ficha de contacto tras agrupar las acciones.
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
await page.goto('https://contacts.sytue.localhost/', { waitUntil: 'domcontentloaded' });
await page.getByText('Ingrid Sørensen').first().waitFor({ timeout: 25_000 });
await page.waitForTimeout(1500);
await page.getByText('Ingrid Sørensen').first().click();
await page.locator('header h1').waitFor({ timeout: 15_000 });
await page.waitForTimeout(800);
await page.screenshot({ path: '/tmp/contact-header.png' });

await page.getByRole('button', { name: 'More actions' }).click();
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/contact-menu.png' });
console.log('OK contact');
await browser.close();
