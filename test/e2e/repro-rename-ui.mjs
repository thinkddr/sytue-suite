// Repro UI del bug: crear calendario por API, renombrar por UI (kebab->modal),
// observar red y sidebar.
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';

const b = await chromium.launch();
const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

page.on('console', (m) => { if (m.type() === 'error') console.log('[console.error]', m.text().slice(0, 200)); });
page.on('request', (r) => { if (r.url().includes('/api/calendars')) console.log('>>', r.method(), r.url()); });
page.on('response', async (r) => {
  if (r.url().includes('/api/calendars')) {
    let body = '';
    try { body = (await r.text()).slice(0, 400); } catch {}
    console.log('<<', r.status(), r.request().method(), body);
  }
});

await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});

const nombre = `ReproUI ${Date.now()}`;
const crear = await page.request.post(`${BASE}/api/calendars`, { data: { displayName: nombre, color: '#146a47' }, timeout: 60000 });
const { calendar: creado } = await crear.json();
console.log('CREADO', creado.id, creado.displayName);

// Recargar para que la isla vea el nuevo calendario.
await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
const casilla = page.getByRole('checkbox', { name: `Mostrar ${nombre}` });
await casilla.waitFor({ state: 'visible', timeout: 60000 });
console.log('checkbox original visible OK');

// Kebab -> Renombrar.
const fila = page.locator('li', { has: casilla });
await fila.hover();
await fila.getByRole('button', { name: `Opciones de ${nombre}` }).click();
await page.getByRole('menuitem', { name: 'Renombrar' }).click();
const editar = page.getByRole('dialog', { name: 'Editar calendario' });
await editar.waitFor({ state: 'visible', timeout: 10000 });
const renombrado = `${nombre} bis`;
await editar.getByLabel(/^Nombre/).fill(renombrado);
await editar.getByRole('button', { name: 'Guardar' }).click();
await editar.waitFor({ state: 'hidden', timeout: 20000 }).then(() => console.log('modal cerrado OK'), () => console.log('MODAL NO CERRO'));

// Dar 8s a la recarga y volcar el estado de la sidebar.
await page.waitForTimeout(8000);
const labels = await page.evaluate(() =>
  [...document.querySelectorAll('input[type=checkbox][aria-label]')].map((i) => i.getAttribute('aria-label')),
);
console.log('sidebar checkboxes:', JSON.stringify(labels, null, 1));
const nuevo = await page.getByRole('checkbox', { name: `Mostrar ${renombrado}` }).isVisible().catch(() => false);
console.log('¿checkbox renombrado visible?', nuevo ? 'SI' : 'NO');
await page.screenshot({ path: '/tmp/repro-rename-ui.png' });

await page.request.post(`${BASE}/api/calendars`, { data: { id: creado.id, delete: true }, timeout: 60000 });
await b.close();
