// Sonda del refinador cabecera+ribbon: popovers sin recorte, chip «Cita» real,
// JetBrains Mono en el select de fuente. Timeouts holgados (stack dev congestionado).
// Uso: cd test/e2e && bun verify-refine-header-ribbon.mjs [docId]
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const T = 120_000;
let docId = process.argv[2];
const report = { popovers: {}, cita: {}, fuentes: null, tabpanel: null, consoleErrors: [] };

const browser = await chromium.launch();
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const page = await context.newPage();
page.setDefaultTimeout(T);
page.on('console', (m) => { if (m.type() === 'error') report.consoleErrors.push(m.text().slice(0, 200)); });

// --- Login (patrón de verify-header-ribbon.mjs) ---
let logged = false;
for (let i = 1; i <= 6 && !logged; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded', timeout: T }).catch(() => {});
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 30_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', 'admin@sytue.localhost').catch(() => {});
  await page.fill('input[name=password]', 'sytue-dev-admin').catch(() => {});
  await page.click('button[type=submit]', { noWaitAfter: true }).catch(() => {});
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: T, waitUntil: 'domcontentloaded' })
    .then(() => true).catch(() => false);
}
if (!logged) { console.error('KO: sin sesión'); process.exit(1); }

if (!docId) {
  const created = await page.request.post(`${DOCS}/api/create`, {
    data: { kind: 'document', title: `verif-refine-${Date.now()}` },
    timeout: T,
  });
  if (created.status() !== 201) { console.error('KO create', created.status()); process.exit(1); }
  ({ id: docId } = await created.json());
}
console.log('doc:', docId);

for (let i = 1; i <= 3; i++) {
  const ok = await page
    .goto(`${DOCS}/edit?id=${docId}`, { waitUntil: 'domcontentloaded', timeout: T })
    .then(() => true).catch(() => false);
  if (ok) break;
  if (i === 3) { console.error('KO: /edit no carga'); process.exit(1); }
}
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: T });
await page.waitForTimeout(500);

// --- Overflow del tabpanel ---
report.tabpanel = await page.evaluate(() => {
  const p = document.querySelector('#ribbon-tabpanel');
  const cs = getComputedStyle(p);
  return { overflowX: cs.overflowX, overflowY: cs.overflowY, scrollExtra: p.scrollWidth - p.clientWidth };
});

// --- Fuentes del select ---
report.fuentes = await page.evaluate(() =>
  [...document.querySelectorAll('#ribbon-tabpanel select[aria-label="Fuente"] option')].map((o) => o.value),
);

/** Popover [sel]: ¿sobresale del tabpanel y su punto inferior recibe el clic? */
async function probe(name, popoverSel) {
  await page.waitForTimeout(250);
  report.popovers[name] = await page.evaluate((sel) => {
    const pop = document.querySelector(sel);
    if (!pop) return { found: false };
    const p = pop.getBoundingClientRect();
    const panel = document.querySelector('#ribbon-tabpanel').getBoundingClientRect();
    const at = document.elementFromPoint(p.left + p.width / 2, Math.min(p.bottom - 6, innerHeight - 1));
    return {
      found: true,
      sobresalePx: Math.round(p.bottom - panel.bottom),
      puntoInferiorClicable: pop.contains(at),
    };
  }, popoverSel);
}

// 1) Color de texto (Inicio)
await page.click('button[aria-label="Color de texto"]');
await probe('colorTexto', '[role="group"][aria-label="Paleta: Color de texto"]');
await page.screenshot({ path: 'shots/hr-fix-popover-color.png', clip: { x: 380, y: 130, width: 360, height: 300 } });
await page.keyboard.press('Escape');

// 2) Tabla + 3) Comentario + 4) Símbolo (Insertar)
await page.click('#ribbon-tab-insertar');
await page.waitForTimeout(200);
await page.click('button[aria-label="Insertar tabla"]');
await probe('tabla', '#ribbon-tabpanel .absolute');
await page.screenshot({ path: 'shots/hr-fix-popover-tabla.png', clip: { x: 0, y: 130, width: 360, height: 380 } });
await page.keyboard.press('Escape');
await page.click('button[aria-label="Insertar comentario"]').catch(() => {});
await probe('comentarioInsertar', '#ribbon-tabpanel [aria-label="Texto del comentario"]');
await page.keyboard.press('Escape');
await page.click('button[aria-label="Insertar símbolo"]').catch(() => {});
await probe('simbolo', '#ribbon-tabpanel .absolute');
await page.keyboard.press('Escape');

// 5) Historial (Colaboración)
await page.click('#ribbon-tab-revisar');
await page.waitForTimeout(200);
await page.locator('#ribbon-tabpanel button', { hasText: 'Historial' }).click().catch(() => {});
await probe('historial', '[role="menu"]');
await page.screenshot({ path: 'shots/hr-fix-historial.png', clip: { x: 100, y: 130, width: 460, height: 340 } });
await page.keyboard.press('Escape');

// --- Chip «Cita»: aplica blockquote de verdad y se marca activa ---
await page.click('#ribbon-tab-inicio');
await page.waitForTimeout(200);
await page.click('.sytue-page');
await page.keyboard.type('Una cita célebre para verificar.');
await page.click('button[aria-label="Estilo Cita"]');
await page.waitForTimeout(400); // throttle del selectionchange
report.cita.trasAplicar = await page.evaluate(() => {
  const sel = getSelection();
  const el = sel?.anchorNode instanceof Element ? sel.anchorNode : sel?.anchorNode?.parentElement;
  return {
    blockTag: el?.closest('blockquote,p,h1,h2,h3')?.tagName ?? null,
    citaPressed: document.querySelector('button[aria-label="Estilo Cita"]')?.getAttribute('aria-pressed'),
    borderLeft: (() => {
      const bq = document.querySelector('.sytue-page blockquote');
      return bq ? getComputedStyle(bq).borderLeftWidth : null;
    })(),
  };
});
await page.screenshot({ path: 'shots/hr-fix-cita.png', clip: { x: 0, y: 56, width: 1440, height: 500 } });
// Volver a Normal
await page.click('button[aria-label="Estilo Normal"]');
await page.waitForTimeout(400);
report.cita.trasNormal = await page.evaluate(() => ({
  quedanBlockquotes: document.querySelectorAll('.sytue-page blockquote').length,
  citaPressed: document.querySelector('button[aria-label="Estilo Cita"]')?.getAttribute('aria-pressed'),
}));
// Título 2 no se degrada al re-consultar (regresión del defecto original)
await page.click('button[aria-label="Estilo Título 2"]');
await page.waitForTimeout(400);
report.cita.h2Pressed = await page.getAttribute('button[aria-label="Estilo Título 2"]', 'aria-pressed');

console.log(JSON.stringify(report, null, 2));
await browser.close();
