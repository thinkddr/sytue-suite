// Depuración 2: ¿por qué el formulario «Comentar» no dispara el POST?
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const DOC_ID = process.argv[2];

const browser = await chromium.launch();
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const page = await context.newPage();
page.setDefaultNavigationTimeout(120_000);
for (let i = 1; i <= 5; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' });
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') break;
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  const hayForm = await page.waitForSelector('input[name=email]', { timeout: 10_000 }).then(() => true).catch(() => false);
  if (!hayForm) continue;
  await page.fill('input[name=email]', 'admin@sytue.localhost');
  await page.fill('input[name=password]', 'sytue-dev-admin');
  await page.click('button[type=submit]', { timeout: 20_000 }).catch(() => {});
  const ok = await page.waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' }).then(() => true).catch(() => false);
  if (ok) break;
}
console.log('login ok');
page.on('response', async (r) => {
  if (r.url().includes('/api/comments')) console.log(`HTTP ${r.request().method()} → ${r.status()}`);
});
page.on('console', (m) => { if (m.type() === 'error') console.log('CONSOLE:', m.text().slice(0, 200)); });

await page.goto(`${DOCS}/edit?id=${DOC_ID}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForTimeout(800);
await page.click('button[aria-label="Comentarios"]');
await page.waitForTimeout(800);

const ta = page.locator('textarea[aria-label="Nuevo comentario"]');
console.log('textareas coincidentes:', await ta.count());
await ta.fill('Prueba de formulario.');
await page.waitForTimeout(300);
const state = await page.evaluate(() => {
  const t = document.querySelector('textarea[aria-label="Nuevo comentario"]');
  const form = t?.closest('form');
  const btn = form?.querySelector('button[type="submit"]');
  const allBtns = Array.from(form?.querySelectorAll('button') ?? []).map((b) => ({
    text: b.textContent?.trim(), type: b.getAttribute('type'), disabled: b.disabled,
  }));
  return { value: t?.value, hasForm: !!form, submitBtn: !!btn, allBtns };
});
console.log('estado del formulario:', JSON.stringify(state, null, 1));

// Clic real sobre el botón del formulario (dentro del form).
await page.locator('textarea[aria-label="Nuevo comentario"]').locator('xpath=ancestor::form').getByRole('button').click();
await page.waitForTimeout(2000);
console.log('tarjetas tras clic:', await page.locator('aside[aria-label="Comentarios"] article').count());

// También Ctrl-menos probable: submit programático del form.
await ta.fill('Prueba submit programático.');
await page.waitForTimeout(300);
await page.evaluate(() => {
  const t = document.querySelector('textarea[aria-label="Nuevo comentario"]');
  t?.closest('form')?.requestSubmit();
});
await page.waitForTimeout(2000);
console.log('tarjetas tras requestSubmit:', await page.locator('aside[aria-label="Comentarios"] article').count());
await browser.close();
