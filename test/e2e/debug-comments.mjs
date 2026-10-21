// Depuración de las mutaciones de comentarios del panel (403 observado).
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const DOC_ID = process.argv[2];
if (!DOC_ID) { console.error('uso: bun debug-comments.mjs <docId>'); process.exit(1); }

const browser = await chromium.launch();
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const page = await context.newPage();
page.setDefaultNavigationTimeout(120_000);

// login
for (let i = 1; i <= 5; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' });
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') break;
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', 'admin@sytue.localhost');
  await page.fill('input[name=password]', 'sytue-dev-admin');
  await page.click('button[type=submit]', { timeout: 20_000 }).catch(() => {});
  const ok = await page.waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' }).then(() => true).catch(() => false);
  if (ok) break;
}
console.log('login ok');

page.on('response', async (r) => {
  if (!r.url().includes('/api/comments')) return;
  let body = '';
  try { body = (await r.text()).slice(0, 300); } catch {}
  console.log(`HTTP ${r.request().method()} ${r.url().replace(DOCS, '')} → ${r.status()} ${body}`);
});

await page.goto(`${DOCS}/edit?id=${DOC_ID}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForTimeout(1000);
await page.click('button[aria-label="Comentarios"]');
await page.waitForTimeout(1000);
console.log('tarjetas:', await page.locator('aside[aria-label="Comentarios"] article').count());

console.log('\n--- añadir por UI ---');
await page.locator('textarea[aria-label="Nuevo comentario"]').fill('Comentario de depuración.');
await page.getByRole('button', { name: 'Comentar', exact: true }).click();
await page.waitForTimeout(1500);
console.log('tarjetas tras añadir:', await page.locator('aside[aria-label="Comentarios"] article').count());
const toasts1 = await page.evaluate(() => Array.from(document.querySelectorAll('[role="alert"],[role="status"]')).map((t) => t.textContent?.trim()).filter(Boolean));
console.log('toasts:', JSON.stringify(toasts1));

console.log('\n--- resolver el primero ---');
await page.locator('aside[aria-label="Comentarios"] article').first().getByRole('button', { name: 'Resolver' }).click();
await page.waitForTimeout(1500);
console.log('opacity:', await page.evaluate(() => getComputedStyle(document.querySelector('aside[aria-label="Comentarios"] article')).opacity));
const toasts2 = await page.evaluate(() => Array.from(document.querySelectorAll('[role="alert"],[role="status"]')).map((t) => t.textContent?.trim()).filter(Boolean));
console.log('toasts:', JSON.stringify(toasts2));

console.log('\n--- eliminar el último ---');
await page.locator('button[aria-label="Eliminar comentario"]').last().click();
await page.waitForTimeout(1500);
console.log('tarjetas tras eliminar:', await page.locator('aside[aria-label="Comentarios"] article').count());

console.log('\n--- mismas mutaciones via page.request (fuera del navegador) ---');
const list = await page.request.get(`${DOCS}/api/comments?id=${DOC_ID}`);
const data = await list.json();
const first = (data.comments ?? [])[0];
console.log('GET →', list.status(), 'total:', (data.comments ?? []).length);
if (first) {
  const put = await page.request.put(`${DOCS}/api/comments?id=${DOC_ID}&commentId=${first.id}`, { data: { resolved: true } });
  console.log('PUT resolve →', put.status(), (await put.text()).slice(0, 200));
  const del = await page.request.delete(`${DOCS}/api/comments?id=${DOC_ID}&commentId=${first.id}`);
  console.log('DELETE →', del.status(), (await del.text()).slice(0, 200));
}
const post = await page.request.post(`${DOCS}/api/comments?id=${DOC_ID}`, { data: { body: 'directo', anchor: null } });
console.log('POST →', post.status(), (await post.text()).slice(0, 200));

await browser.close();
