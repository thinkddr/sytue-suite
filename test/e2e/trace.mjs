// Traza errores por producto: login SSO una vez, luego visita cada app y captura
// errores de consola, page errors, respuestas >=400 y una captura de pantalla.
// Uso: bun run trace.mjs [producto]   (sin arg = todos). Requiere la pila (make dev-up).
import { chromium } from 'playwright';
import { mkdirSync, writeFileSync } from 'node:fs';

const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';
const shotsDir = new URL('./shots/', import.meta.url).pathname;
mkdirSync(shotsDir, { recursive: true });

const PRODUCTS = [
  ['portal', 'https://one.sytue.localhost/'],
  ['mail', 'https://mail.sytue.localhost/'],
  ['drive', 'https://drive.sytue.localhost/'],
  ['docs', 'https://docus.sytue.localhost/'],
  ['calendar', 'https://calendar.sytue.localhost/'],
  ['contacts', 'https://contacts.sytue.localhost/'],
  ['chat', 'https://chat.sytue.localhost/'],
  ['meet', 'https://meet.sytue.localhost/'],
  ['cloud', 'https://cloud.sytue.localhost/'],
  ['console', 'https://console.sytue.localhost/'],
];
const only = process.argv[2];
const targets = only ? PRODUCTS.filter(([k]) => k === only) : PRODUCTS;

const browser = await chromium.launch();
const ctx = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });

// --- Login SSO (una vez) ---
{
  const page = await ctx.newPage();
  await page.goto('https://one.sytue.localhost/', { waitUntil: 'domcontentloaded' });
  if (/\/login\//.test(page.url())) {
    await page.fill('input[name=email]', EMAIL);
    await page.fill('input[name=password]', PASS);
    await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
    await page.waitForLoadState('networkidle').catch(() => {});
  }
  console.log('login: url final =', page.url());
  await page.close();
}

const report = [];
for (const [key, url] of targets) {
  const page = await ctx.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  const netErrors = [];
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 300)); });
  page.on('pageerror', (e) => pageErrors.push(String(e).slice(0, 300)));
  page.on('response', (r) => { if (r.status() >= 400) netErrors.push(`${r.status()} ${r.url().replace(/^https:\/\//, '')}`.slice(0, 200)); });
  page.on('requestfailed', (r) => netErrors.push(`FAIL ${(r.failure()?.errorText || '').replace('net::', '')} ${r.url()}`.slice(0, 200)));

  let status = 0;
  try {
    const resp = await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 });
    status = resp ? resp.status() : 0;
    await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});
  } catch (e) {
    pageErrors.push('goto: ' + String(e).slice(0, 200));
  }
  const title = await page.title().catch(() => '');
  const bodyText = (await page.evaluate(() => document.body?.innerText || '').catch(() => '')).slice(0, 120).replace(/\s+/g, ' ');
  await page.screenshot({ path: shotsDir + key + '.png', fullPage: false }).catch(() => {});
  report.push({ key, status, finalUrl: page.url().replace(/^https:\/\//, ''), title, bodyText, consoleErrors, pageErrors, netErrors: [...new Set(netErrors)] });
  await page.close();
}
await browser.close();

writeFileSync(shotsDir + 'report.json', JSON.stringify(report, null, 2));
console.log('\n===== TRAZAS POR PRODUCTO =====');
for (const r of report) {
  const nErr = r.consoleErrors.length + r.pageErrors.length;
  console.log(`\n## ${r.key}  [http ${r.status}]  ${nErr === 0 && r.netErrors.length === 0 ? 'OK' : 'PROBLEMAS'}`);
  console.log(`   title: ${r.title} | url: ${r.finalUrl}`);
  console.log(`   body: ${r.bodyText}`);
  for (const e of r.pageErrors) console.log('   [pageerror] ' + e);
  for (const e of r.consoleErrors.slice(0, 6)) console.log('   [console] ' + e);
  for (const e of r.netErrors.slice(0, 8)) console.log('   [net] ' + e);
}
