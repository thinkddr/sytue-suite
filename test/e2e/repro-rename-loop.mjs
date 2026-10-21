// Busca la iteracion que falla: crea via modal + renombra via kebab en bucle,
// capturando el body del POST de rename y el value vivo del input antes de Guardar.
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';
const N = Number(process.env.N ?? 12);

const b = await chromium.launch();

for (let i = 1; i <= N; i++) {
  const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const bodies = [];
  page.on('request', (r) => {
    if (r.url().includes('/api/calendars') && r.method() === 'POST') bodies.push(r.postData());
  });

  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});

  const nombre = `Loop${i} ${Date.now()}`;
  let id = null;
  try {
    await page.getByRole('button', { name: 'Crear calendario' }).click({ timeout: 30000 });
    const modal = page.getByRole('dialog', { name: 'Nuevo calendario' });
    await modal.waitFor({ state: 'visible' });
    await modal.getByLabel(/^Nombre/).fill(nombre);
    await modal.getByRole('radio', { name: 'Índigo' }).click();
    await modal.getByRole('button', { name: 'Guardar' }).click();
    await modal.waitFor({ state: 'hidden' });

    const casilla = page.getByRole('checkbox', { name: `Mostrar ${nombre}` });
    await casilla.waitFor({ state: 'visible', timeout: 30000 });
    id = (await (await page.request.get(`${BASE}/api/calendars`)).json()).calendars.find((c) => c.displayName === nombre)?.id ?? null;

    const fila = page.locator('li', { has: casilla });
    await fila.hover();
    await fila.getByRole('button', { name: `Opciones de ${nombre}` }).click();
    await page.getByRole('menuitem', { name: 'Renombrar' }).click();
    const editar = page.getByRole('dialog', { name: 'Editar calendario' });
    await editar.waitFor({ state: 'visible' });
    const renombrado = `${nombre} bis`;
    await editar.getByLabel(/^Nombre/).fill(renombrado);
    // Estado real del input y del foco justo antes de guardar.
    const antes = await page.evaluate(() => ({
      inputValue: document.getElementById('cal-name')?.value,
      active: document.activeElement ? `${document.activeElement.tagName}#${document.activeElement.id || document.activeElement.getAttribute('aria-label') || ''}` : null,
    }));
    await editar.getByRole('button', { name: 'Guardar' }).click();
    await editar.waitFor({ state: 'hidden', timeout: 15000 });

    const ok = await page
      .getByRole('checkbox', { name: `Mostrar ${renombrado}` })
      .waitFor({ state: 'visible', timeout: 5000 })
      .then(() => true, () => false);
    const renameBody = bodies.filter((x) => x?.includes('"id"') && !x?.includes('delete')).pop();
    console.log(`#${i} ${ok ? 'OK ' : 'FALLO'} input=${JSON.stringify(antes.inputValue)} active=${antes.active} body=${renameBody}`);
  } catch (e) {
    console.log(`#${i} EXCEPCION ${String(e).slice(0, 200)}`);
  } finally {
    if (id) await page.request.post(`${BASE}/api/calendars`, { data: { id, delete: true } }).catch(() => {});
    await ctx.close();
  }
}
await b.close();
