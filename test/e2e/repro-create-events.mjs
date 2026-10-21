// ¿Llega el evento 'input' al DOM cuando fill() pierde el texto?
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';
const N = Number(process.env.N ?? 10);

const b = await chromium.launch();
for (let i = 1; i <= N; i++) {
  const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});

  const nombre = `Ev${i} ${Date.now()}`;
  let id = null;
  try {
    await page.getByRole('button', { name: 'Crear calendario' }).click({ timeout: 30000 });
    const modal = page.getByRole('dialog', { name: 'Nuevo calendario' });
    await modal.waitFor({ state: 'visible' });

    // Espia de eventos ANTES del fill: registra input/keydown en captura + si
    // el nodo #cal-name es reemplazado.
    await page.evaluate(() => {
      window.__spy = { inputs: [], nodeSwaps: 0 };
      const nodo = document.getElementById('cal-name');
      window.__nodo0 = nodo;
      document.addEventListener('input', (e) => {
        window.__spy.inputs.push({
          target: e.target?.id || e.target?.tagName,
          value: e.target?.value?.slice(0, 40),
          mismoNodo: e.target === window.__nodo0,
        });
      }, true);
    });

    await modal.getByLabel(/^Nombre/).fill(nombre);
    const spy = await page.evaluate(() => ({
      ...window.__spy,
      valorAhora: document.getElementById('cal-name')?.value,
      mismoNodoAhora: document.getElementById('cal-name') === window.__nodo0,
    }));
    await modal.getByRole('radio', { name: 'Índigo' }).click();
    await modal.getByRole('button', { name: 'Guardar' }).click();
    const cerro = await modal.waitFor({ state: 'hidden', timeout: 8000 }).then(() => true, () => false);
    console.log(`#${i} cerro=${cerro} spy=${JSON.stringify(spy)}`);
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
