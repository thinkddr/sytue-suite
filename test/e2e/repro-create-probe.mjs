// Probe del modal de CREAR: ¿que pasa cuando se queda abierto tras Guardar?
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';
const N = Number(process.env.N ?? 8);

const b = await chromium.launch();
for (let i = 1; i <= N; i++) {
  const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const posts = [];
  const errs = [];
  page.on('request', (r) => { if (r.url().includes('/api/calendars') && r.method() === 'POST') posts.push(r.postData()); });
  page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text().slice(0, 150)); });

  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});

  const nombre = `Probe${i} ${Date.now()}`;
  let id = null;
  try {
    await page.getByRole('button', { name: 'Crear calendario' }).click({ timeout: 30000 });
    const modal = page.getByRole('dialog', { name: 'Nuevo calendario' });
    await modal.waitFor({ state: 'visible' });
    await modal.getByLabel(/^Nombre/).fill(nombre);
    const trasFill = await page.evaluate(() => ({
      v: document.getElementById('cal-name')?.value,
      a: document.activeElement?.tagName + '#' + (document.activeElement?.id || document.activeElement?.getAttribute?.('aria-label') || ''),
    }));
    await modal.getByRole('radio', { name: 'Índigo' }).click();
    const trasRadio = await page.evaluate(() => document.getElementById('cal-name')?.value);
    await modal.getByRole('button', { name: 'Guardar' }).click();
    const cerro = await modal.waitFor({ state: 'hidden', timeout: 8000 }).then(() => true, () => false);
    let detalle = '';
    if (!cerro) {
      detalle = await page.evaluate(() => {
        const v = document.getElementById('cal-name')?.value;
        const err = document.getElementById('cal-name-error')?.textContent ?? '';
        return `value=${JSON.stringify(v)} error=${JSON.stringify(err)}`;
      });
    }
    console.log(`#${i} cerro=${cerro} trasFill=${JSON.stringify(trasFill)} trasRadio=${JSON.stringify(trasRadio)} ${detalle} posts=${posts.length} errs=${errs.join('|')}`);
    if (cerro) {
      id = (await (await page.request.get(`${BASE}/api/calendars`)).json()).calendars.find((c) => c.displayName === nombre)?.id ?? null;
    }
  } catch (e) {
    console.log(`#${i} EXCEPCION ${String(e).slice(0, 150)}`);
  } finally {
    if (id) await page.request.post(`${BASE}/api/calendars`, { data: { id, delete: true } }).catch(() => {});
    await ctx.close();
  }
}
await b.close();
