// Repro del round-trip renombrar calendario: crear -> PATCH -> GET lista.
import { chromium } from 'playwright';
const STORAGE = '/Users/ddr/Documents/Sytue/sytueOne/test/e2e/.auth/admin.json';
const BASE = 'https://calendar.sytue.localhost';

const b = await chromium.launch();
const ctx = await b.newContext({ storageState: STORAGE, ignoreHTTPSErrors: true });
const page = await ctx.newPage();
await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});
const req = page.request;

const nombre = `ReproRename ${Date.now()}`;
const crear = await req.post(`${BASE}/api/calendars`, { data: { displayName: nombre, color: '#146a47' }, timeout: 60000 });
console.log('CREATE status', crear.status());
const { calendar: creado } = await crear.json();
console.log('CREATE ->', JSON.stringify(creado));

const renombrado = `${nombre} bis`;
const patch = await req.post(`${BASE}/api/calendars`, { data: { id: creado.id, displayName: renombrado, color: '#4338ca' }, timeout: 60000 });
console.log('PATCH status', patch.status());
console.log('PATCH ->', await patch.text());

const lista = await req.get(`${BASE}/api/calendars`, { timeout: 60000 });
console.log('GET status', lista.status());
const { calendars } = await lista.json();
const mio = calendars.find((c) => c.id === creado.id);
console.log('GET (mi calendario) ->', JSON.stringify(mio));
console.log('¿GET refleja el rename?', mio?.displayName === renombrado ? 'SI' : `NO (dice: ${mio?.displayName})`);

// limpieza
await req.post(`${BASE}/api/calendars`, { data: { id: creado.id, delete: true }, timeout: 60000 });
await b.close();
