// Sonda de popovers del ribbon: ¿quedan recortados por el overflow del tabpanel?
// Uso: cd test/e2e && bun verify-header-ribbon-popovers.mjs <docId>
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const docId = process.argv[2];
const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 2,
});
const page = await context.newPage();

let logged = false;
for (let i = 1; i <= 5 && !logged; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded', timeout: 90_000 }).catch(() => {});
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', 'admin@sytue.localhost');
  await page.fill('input[name=password]', 'sytue-dev-admin');
  await page.click('button[type=submit]', { noWaitAfter: true }).catch(() => {});
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' })
    .then(() => true).catch(() => false);
}
if (!logged) { console.error('KO: sin sesión'); process.exit(1); }

await page.goto(`${DOCS}/edit?id=${docId}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForTimeout(400);

const results = {};

/** Mide el popover [sel] respecto al tabpanel y si su punto inferior es clicable. */
async function probe(name, popoverSel) {
  await page.waitForTimeout(200);
  results[name] = await page.evaluate((sel) => {
    const pop = document.querySelector(sel);
    if (!pop) return { found: false };
    const p = pop.getBoundingClientRect();
    const panel = document.querySelector('#ribbon-tabpanel').getBoundingClientRect();
    // ¿El punto inferior del popover recibe el clic o lo recibe otra cosa (recortado/tapado)?
    const x = p.left + p.width / 2;
    const y = Math.min(p.bottom - 6, innerHeight - 1);
    const at = document.elementFromPoint(x, y);
    return {
      found: true,
      popBottom: Math.round(p.bottom),
      popH: Math.round(p.height),
      panelBottom: Math.round(panel.bottom),
      sobresalePx: Math.round(p.bottom - panel.bottom),
      puntoInferiorClicable: pop.contains(at),
      atElement: at ? `${at.tagName}.${(at.className || '').toString().slice(0, 40)}` : null,
    };
  }, popoverSel);
}

// 1) Color de texto (Inicio)
await page.click('button[aria-label="Color de texto"]');
await probe('colorTexto', '[role="group"][aria-label="Paleta: Color de texto"]');
await page.screenshot({ path: 'shots/hr-zoom-popover-color.png', clip: { x: 380, y: 130, width: 360, height: 220 } });
await page.keyboard.press('Escape');

// 2) Tabla (Insertar)
await page.click('#ribbon-tab-insertar');
await page.waitForTimeout(150);
await page.click('button[aria-label="Insertar tabla"]');
await probe('tabla', '#ribbon-tabpanel [role="dialog"], #ribbon-tabpanel [role="grid"], #ribbon-tabpanel .absolute');
await page.screenshot({ path: 'shots/hr-zoom-popover-tabla.png', clip: { x: 0, y: 130, width: 360, height: 320 } });
await page.keyboard.press('Escape');

// 3) Comentario (Insertar)
await page.click('button[aria-label="Insertar comentario"]').catch(() => {});
await probe('comentarioInsertar', '#ribbon-tabpanel [aria-label="Texto del comentario"]');
await page.screenshot({ path: 'shots/hr-zoom-popover-comentario.png', clip: { x: 150, y: 130, width: 500, height: 320 } });
await page.keyboard.press('Escape');

// 4) Símbolo (Insertar)
await page.click('button[aria-label="Insertar símbolo"]').catch(() => {});
await probe('simbolo', '#ribbon-tabpanel .absolute');
await page.keyboard.press('Escape');

// 5) Historial (Colaboración)
await page.click('#ribbon-tab-colaboracion');
await page.waitForTimeout(150);
const hist = page.locator('#ribbon-tabpanel button', { hasText: 'Historial' });
await hist.click().catch(() => {});
await probe('historial', '[role="menu"]');
await page.screenshot({ path: 'shots/hr-zoom-historial.png', clip: { x: 100, y: 130, width: 460, height: 340 } });
await page.keyboard.press('Escape');

// 6) Compartir (Colaboración)
await page.click('#ribbon-tabpanel button[aria-label="Compartir"]').catch(() => {});
await probe('compartirColab', '[aria-label="Compartir documento"]');
await page.screenshot({ path: 'shots/hr-zoom-compartir-colab.png', clip: { x: 150, y: 130, width: 500, height: 320 } });
await page.keyboard.press('Escape');

// 7) Color de página (Disposición)
await page.click('#ribbon-tab-disposicion');
await page.waitForTimeout(150);
await page.click('button[aria-label="Color de página"]').catch(() => {});
await probe('colorPagina', '#ribbon-tabpanel [role="group"][aria-label*="Color de página"], #ribbon-tabpanel .absolute');
await page.keyboard.press('Escape');

// Detalles a 2x: badge BETA (Colaboración) y checkboxes (Vista)
await page.click('#ribbon-tab-colaboracion');
await page.waitForTimeout(150);
await page.screenshot({ path: 'shots/hr-zoom-colaboracion.png', clip: { x: 0, y: 130, width: 420, height: 80 } });
await page.click('#ribbon-tab-vista');
await page.waitForTimeout(150);
await page.screenshot({ path: 'shots/hr-zoom-vista.png', clip: { x: 0, y: 130, width: 500, height: 80 } });
// Detalle de la cabecera (marca + título + chip) y pestañas con hover
await page.hover('#ribbon-tab-referencias');
await page.waitForTimeout(120);
await page.screenshot({ path: 'shots/hr-zoom-tabs-hover.png', clip: { x: 0, y: 104, width: 620, height: 34 } });
await page.screenshot({ path: 'shots/hr-zoom-header.png', clip: { x: 0, y: 56, width: 720, height: 48 } });

console.log(JSON.stringify(results, null, 2));
await browser.close();
