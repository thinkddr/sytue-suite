// Repro EXACTO del e2e (calendar-multical.spec.ts:143): crear via modal + kebab
// Renombrar via modal, sin recargas.
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';

const b = await chromium.launch();
const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

page.on('console', (m) => { if (m.type() === 'error') console.log('[console.error]', m.text().slice(0, 300)); });
page.on('response', async (r) => {
  if (r.url().includes('/api/calendars')) {
    let body = '';
    try { body = (await r.text()).slice(0, 300); } catch {}
    console.log('<<', r.status(), r.request().method(), body);
  }
});

await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});

const nombre = `SidebarRepro ${Date.now()}`;
let id = null;
// «+» -> modal nuevo calendario (identico al spec).
await page.getByRole('button', { name: 'Crear calendario' }).click({ timeout: 60000 });
const modal = page.getByRole('dialog', { name: 'Nuevo calendario' });
await modal.waitFor({ state: 'visible' });
await modal.getByLabel(/^Nombre/).fill(nombre);
await modal.getByRole('radio', { name: 'Índigo' }).click();
await modal.getByRole('button', { name: 'Guardar' }).click();
await modal.waitFor({ state: 'hidden' });
console.log('crear: modal cerrado');

const casilla = page.getByRole('checkbox', { name: `Mostrar ${nombre}` });
await casilla.waitFor({ state: 'visible', timeout: 60000 });
console.log('checkbox creado visible OK');
const listado = await (await page.request.get(`${BASE}/api/calendars`, { timeout: 60000 })).json();
id = listado.calendars.find((c) => c.displayName === nombre)?.id ?? null;
console.log('id =', id);

// Kebab -> Renombrar (identico al spec).
const fila = page.locator('li', { has: casilla });
await fila.hover();
await fila.getByRole('button', { name: `Opciones de ${nombre}` }).click();
await page.getByRole('menuitem', { name: 'Renombrar' }).click();
const editar = page.getByRole('dialog', { name: 'Editar calendario' });
await editar.waitFor({ state: 'visible' });
const renombrado = `${nombre} bis`;
await editar.getByLabel(/^Nombre/).fill(renombrado);
await editar.getByRole('button', { name: 'Guardar' }).click();
await editar.waitFor({ state: 'hidden', timeout: 20000 }).then(() => console.log('renombrar: modal cerrado'), () => console.log('RENOMBRAR: MODAL NO CERRO'));

await page.waitForTimeout(8000);
const labels = await page.evaluate(() =>
  [...document.querySelectorAll('input[type=checkbox][aria-label]')].map((i) => i.getAttribute('aria-label')),
);
console.log('sidebar checkboxes:', JSON.stringify(labels));
const visible = await page.getByRole('checkbox', { name: `Mostrar ${renombrado}` }).isVisible().catch(() => false);
console.log('¿checkbox renombrado visible?', visible ? 'SI' : 'NO');
await page.screenshot({ path: '/tmp/repro-rename-ui2.png' });

if (id) await page.request.post(`${BASE}/api/calendars`, { data: { id, delete: true }, timeout: 60000 });
await b.close();
