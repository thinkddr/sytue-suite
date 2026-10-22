// Probe autenticado: login SSO una vez, luego vuelca el cuerpo real de las
// respuestas de los endpoints BFF que fallan (para ver el error de String(err)).
import { chromium } from 'playwright';

const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';
const TARGETS = [
  ['mail', 'https://mail.sytue.localhost/', '/api/mailboxes'],
  ['calendar', 'https://calendar.sytue.localhost/', '/api/calendars'],
  ['contacts', 'https://contacts.sytue.localhost/', '/api/addressbooks'],
  ['chat', 'https://chat.sytue.localhost/', '/api/channels'],
  ['meet', 'https://meet.sytue.localhost/', '/api/rooms'],
];

const browser = await chromium.launch();
const ctx = await browser.newContext({ ignoreHTTPSErrors: true });
const page = await ctx.newPage();
await page.goto('https://one.sytue.localhost/', { waitUntil: 'domcontentloaded' });
if (/\/login\//.test(page.url())) {
  await page.fill('input[name=email]', EMAIL);
  await page.fill('input[name=password]', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
  await page.waitForLoadState('networkidle').catch(() => {});
}
for (const [key, base, api] of TARGETS) {
  const p = await ctx.newPage();
  await p.goto(base, { waitUntil: 'domcontentloaded' }).catch(() => {});
  const out = await p.evaluate(async (api) => {
    try {
      const r = await fetch(api, { headers: { accept: 'application/json' } });
      return { status: r.status, body: (await r.text()).slice(0, 500) };
    } catch (e) { return { status: 0, body: 'fetch-throw: ' + String(e) }; }
  }, api);
  console.log(`\n## ${key} ${api} -> ${out.status}\n${out.body}`);
  await p.close();
}
await browser.close();
