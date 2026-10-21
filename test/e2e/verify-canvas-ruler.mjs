// Verificador de la zona «lienzo + regla» del editor Sytue Docs.
// Uso: cd test/e2e && bun verify-canvas-ruler.mjs
// Login → crea documento → contenido real (títulos, negrita, tabla, TDC) →
// interactúa (pestañas, zoom 75/100/150, márgenes, arrastre de sangría) →
// capturas 1440x900 y 1280x800 en shots/verify-canvas/ + métricas JSON.
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = 'admin@sytue.localhost';
const PASS = 'sytue-dev-admin';
const OUT = 'shots/verify-canvas';
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 1,
});
const page = await context.newPage();
const consoleErrors = [];
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text());
});
page.on('pageerror', (e) => consoleErrors.push(String(e)));

const metrics = {};

// --- 1. Login (patrón de smoke-docs-editor.mjs) ---
let logged = false;
for (let intento = 1; intento <= 6 && !logged; intento++) {
  if (intento > 1) await page.waitForTimeout(13_000);
  await page
    .goto(`${DOCS}/`, { waitUntil: 'domcontentloaded', timeout: 45_000 })
    .catch(() => {});
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') {
    logged = true;
    break;
  }
  await page
    .waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 })
    .catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', EMAIL);
  await page.fill('input[name=password]', PASS);
  await page.click('button[type=submit]', { noWaitAfter: true, timeout: 15_000 }).catch(() => {});
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', {
      timeout: 90_000,
      waitUntil: 'domcontentloaded',
    })
    .then(() => true)
    .catch(() => false);
}
if (!logged) {
  console.error('KO: sin sesión');
  process.exit(1);
}
console.log('1. Sesión iniciada.');

// --- 2. Crear documento ---
const created = await page.request.post(`${DOCS}/api/create`, {
  data: { kind: 'document', title: `verif-lienzo-${Date.now()}` },
});
if (created.status() !== 201) {
  console.error(`KO create: ${created.status()} ${await created.text()}`);
  process.exit(1);
}
const { id } = await created.json();
console.log(`2. Documento: ${id}`);

// --- 3. Abrir editor ---
await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForSelector('[role="tablist"][aria-label="Cinta de opciones"]', {
  state: 'visible',
  timeout: 30_000,
});

const tab = (name) => page.click(`[role="tab"]:has-text("${name}")`);
const savedChip = () =>
  page
    .waitForFunction(
      () =>
        Array.from(document.querySelectorAll('[aria-live="polite"]')).some((el) =>
          /^\s*Guardado(\s|$)/.test(el.textContent || ''),
        ),
      { timeout: 20_000 },
    )
    .catch(() => console.log('AVISO: chip «Guardado» no llegó a tiempo'));

// --- 4. Contenido real ---
await page.click('.sytue-page');
await page.keyboard.type('Informe de verificación del lienzo');
await page.click('button[aria-label="Estilo Título 1"]');
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Normal"]');
await page.keyboard.type(
  'Este documento comprueba la experiencia del folio A4: tipografía Inter del contenido, ' +
    'márgenes reales de página, sombra del papel sobre el lienzo gris y la regla horizontal ' +
    'con sus zonas de margen sombreadas. El párrafo es deliberadamente largo para observar ' +
    'el interlineado, la anchura de columna de texto y el ajuste de línea del cuerpo. ',
);
await page.keyboard.type('Este fragmento va en ');
await page.click('button[aria-label="Negrita"]');
await page.keyboard.type('negrita de verdad');
await page.click('button[aria-label="Negrita"]');
await page.keyboard.type('.');
await page.keyboard.press('Enter');
await page.keyboard.type('Metodología');
await page.click('button[aria-label="Estilo Título 2"]');
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Normal"]');
await page.keyboard.type('Sección con tabla insertada desde la pestaña Insertar.');
await page.keyboard.press('Enter');

// Tabla 3×4 desde Insertar
await tab('Insertar');
await page.click('button[aria-label="Insertar tabla"]');
await page.click('button[aria-label="Tabla de 3 × 4"]');
await page.waitForSelector('.sytue-page table', { timeout: 10_000 });
const firstCell = page.locator('.sytue-page table td').first();
await firstCell.click();
await page.keyboard.type('Celda A1');
await page.locator('.sytue-page table td').nth(1).click();
await page.keyboard.type('Métrica');

// Título 2 final tras la tabla
await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  const sel = getSelection();
  const r = document.createRange();
  r.selectNodeContents(root.lastElementChild || root);
  r.collapse(false);
  sel.removeAllRanges();
  sel.addRange(r);
  root.focus();
});
await page.keyboard.type('Resultados');
await tab('Inicio');
await page.click('button[aria-label="Estilo Título 2"]');
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Normal"]');
await page.keyboard.type('Cierre del documento de prueba con texto normal.');

// TDC al principio (caret al inicio del primer bloque → Referencias → TDC)
await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  const sel = getSelection();
  const r = document.createRange();
  r.setStart(root.firstElementChild || root, 0);
  r.collapse(true);
  sel.removeAllRanges();
  sel.addRange(r);
  root.focus();
});
await tab('Referencias');
await page.click('button[aria-label="Insertar tabla de contenidos"]');
await page.waitForSelector('.sytue-page nav[data-toc]', { timeout: 10_000 });
await savedChip();
console.log('4. Contenido creado (H1/H2, negrita, tabla 3×4, TDC).');

// --- 5. Métricas @100 % ---
const measure = () =>
  page.evaluate(() => {
    const scroller = document.querySelector('.sytue-page').closest('.overflow-auto');
    const pageEl = document.querySelector('.sytue-page');
    const ruler = document.querySelector('[role="group"][aria-label="Regla horizontal"]');
    const cs = getComputedStyle(pageEl);
    const sr = scroller.getBoundingClientRect();
    const pr = pageEl.getBoundingClientRect();
    const rr = ruler ? ruler.getBoundingClientRect() : null;
    const h1 = pageEl.querySelector('h1');
    const h2 = pageEl.querySelector('h2');
    const td = pageEl.querySelector('td');
    const toc = pageEl.querySelector('nav[data-toc]');
    const status = document.querySelector('.sytue-editor > .shrink-0:last-child');
    return {
      canvasBg: getComputedStyle(scroller).backgroundColor,
      page: {
        rectW: +pr.width.toFixed(1),
        rectH: +pr.height.toFixed(1),
        layoutW: cs.width,
        minH: cs.minHeight,
        padding: cs.padding,
        font: cs.fontFamily.split(',')[0],
        fontSize: cs.fontSize,
        lineHeight: cs.lineHeight,
        shadow: cs.boxShadow !== 'none',
        bg: cs.backgroundColor,
        border: cs.borderTopWidth,
        centerOffset: +(pr.left + pr.width / 2 - (sr.left + sr.width / 2)).toFixed(1),
      },
      ruler: rr && {
        rectW: +rr.width.toFixed(1),
        rectH: +rr.height.toFixed(1),
        leftDelta: +(rr.left - pr.left).toFixed(1),
        widthDelta: +(rr.width - pr.width).toFixed(1),
        gapToPage: +(pr.top - rr.bottom).toFixed(1),
        marginRects: Array.from(ruler.querySelectorAll('svg rect')).map((r) =>
          Number(r.getAttribute('width')),
        ),
        labels: Array.from(ruler.querySelectorAll('svg text'))
          .slice(0, 24)
          .map((t) => t.textContent)
          .join(' '),
        markers: Array.from(ruler.querySelectorAll('[role="slider"]')).map((m) => ({
          label: m.getAttribute('aria-label'),
          now: m.getAttribute('aria-valuenow'),
        })),
      },
      typography: {
        h1: h1 && getComputedStyle(h1).fontSize + ' / ' + getComputedStyle(h1).fontWeight,
        h2: h2 && getComputedStyle(h2).fontSize + ' / ' + getComputedStyle(h2).fontWeight,
        tdBorder: td && getComputedStyle(td).border,
        tableW: td && +td.closest('table').getBoundingClientRect().width.toFixed(1),
        tocBorder: toc && getComputedStyle(toc).border,
        tocLinks: toc && toc.querySelectorAll('a').length,
      },
      scroll: {
        clientW: scroller.clientWidth,
        scrollW: scroller.scrollWidth,
        hScroll: scroller.scrollWidth > scroller.clientWidth + 1,
      },
      statusText: status ? status.textContent.trim().replace(/\s+/g, ' ') : null,
    };
  });

const shootCanvas = async (name) => {
  const box = await page
    .locator('.sytue-page')
    .evaluate((el) => {
      const r = el.closest('.overflow-auto').getBoundingClientRect();
      return { x: r.x, y: r.y, width: r.width, height: r.height };
    });
  await page.screenshot({ path: `${OUT}/${name}.png`, clip: box });
};

metrics.zoom100 = await measure();
await page.screenshot({ path: `${OUT}/1440-full-z100.png` });
await shootCanvas('1440-canvas-z100');
// primer plano de la regla + arranque del folio
const rulerBox = await page
  .locator('[aria-label="Regla horizontal"]')
  .evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.max(0, r.x - 30), y: Math.max(0, r.y - 8), width: r.width + 60, height: 140 };
  });
await page.screenshot({ path: `${OUT}/1440-ruler-z100.png`, clip: rulerBox });

// --- 6. Arrastre del marcador de sangría de primera línea ---
await page.locator('.sytue-page p').first().click();
const marker = page.locator('[role="slider"][aria-label="Sangría de primera línea"]');
const mb = await marker.boundingBox();
await page.mouse.move(mb.x + mb.width / 2, mb.y + mb.height / 2);
await page.mouse.down();
await page.mouse.move(mb.x + mb.width / 2 + 47, mb.y + mb.height / 2, { steps: 8 });
await page.mouse.up();
metrics.dragIndent = await page.evaluate(() => {
  const p = document.querySelector('.sytue-page p');
  const m = document.querySelector('[role="slider"][aria-label="Sangría de primera línea"]');
  return { textIndent: p.style.textIndent, ariaNow: m.getAttribute('aria-valuenow') };
});
await page.screenshot({ path: `${OUT}/1440-ruler-drag.png`, clip: rulerBox });
// deshacer la sangría para no ensuciar el resto de capturas
await page.keyboard.press('ControlOrMeta+z');
console.log('6. Arrastre de sangría:', JSON.stringify(metrics.dragIndent));

// --- 7. Márgenes: Estrecho (48) y vuelta a Normal (96) ---
await tab('Disposición');
await page.selectOption('select[aria-label="Márgenes de página"]', '48');
await page.waitForTimeout(300);
metrics.margin48 = await measure().then((m) => ({
  padding: m.page.padding,
  rulerMarginRects: m.ruler?.marginRects,
}));
await page.screenshot({ path: `${OUT}/1440-ruler-margen48.png`, clip: rulerBox });
await page.selectOption('select[aria-label="Márgenes de página"]', '96');
await page.waitForTimeout(300);

// --- 8. Zoom 75 / 150 desde Vista (select de presets) ---
await tab('Vista');
await page.selectOption('select[aria-label="Zoom"]', '75');
await page.waitForTimeout(400);
metrics.zoom75 = await measure();
await page.screenshot({ path: `${OUT}/1440-full-z75.png` });
await shootCanvas('1440-canvas-z75');

await page.selectOption('select[aria-label="Zoom"]', '150');
await page.waitForTimeout(400);
metrics.zoom150 = await measure();
await page.screenshot({ path: `${OUT}/1440-full-z150.png` });
await shootCanvas('1440-canvas-z150');
// con scroll horizontal al extremo izquierdo y derecho
await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  s.scrollLeft = 0;
});
await page.waitForTimeout(200);
await shootCanvas('1440-canvas-z150-scrollL');
await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  s.scrollLeft = s.scrollWidth;
});
await page.waitForTimeout(200);
await shootCanvas('1440-canvas-z150-scrollR');
metrics.zoom150scrolled = await page.evaluate(() => {
  const pr = document.querySelector('.sytue-page').getBoundingClientRect();
  const rr = document.querySelector('[aria-label="Regla horizontal"]').getBoundingClientRect();
  return { leftDelta: +(rr.left - pr.left).toFixed(1), widthDelta: +(rr.width - pr.width).toFixed(1) };
});

// vuelta a 100 y scroll al inicio
await page.selectOption('select[aria-label="Zoom"]', '100');
await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  s.scrollLeft = 0;
  s.scrollTop = 0;
});

// --- 9. Panel de ajustes abierto (recentrado del lienzo) ---
const panelCheck = page.getByRole('checkbox', { name: 'Panel de ajustes' });
await panelCheck.click();
await page.waitForTimeout(400);
metrics.zoom100panel = await measure().then((m) => ({
  centerOffset: m.page.centerOffset,
  clientW: m.scroll.clientW,
}));
await page.screenshot({ path: `${OUT}/1440-full-z100-panel.png` });
await panelCheck.click();
await page.waitForTimeout(300);

// --- 10. Regla desactivable (Vista → Regla) ---
const rulerCheck = page.getByRole('checkbox', { name: 'Regla', exact: true });
await rulerCheck.click();
await page.waitForTimeout(200);
metrics.rulerOff = await page.evaluate(
  () => !document.querySelector('[aria-label="Regla horizontal"]'),
);
await rulerCheck.click();
await page.waitForTimeout(200);

// --- 11. Viewport 1280x800 ---
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(400);
metrics.z100at1280 = await measure();
await page.screenshot({ path: `${OUT}/1280-full-z100.png` });
await shootCanvas('1280-canvas-z100');
await tab('Vista');
await page.selectOption('select[aria-label="Zoom"]', '150');
await page.waitForTimeout(400);
metrics.z150at1280 = await measure().then((m) => ({
  centerOffset: m.page.centerOffset,
  hScroll: m.scroll.hScroll,
  clientW: m.scroll.clientW,
  scrollW: m.scroll.scrollW,
}));
await page.screenshot({ path: `${OUT}/1280-full-z150.png` });
await page.selectOption('select[aria-label="Zoom"]', '100');

// --- 12. Modo oscuro (media query) ---
await page.emulateMedia({ colorScheme: 'dark' });
await page.waitForTimeout(400);
metrics.dark = await page.evaluate(() => {
  const scroller = document.querySelector('.sytue-page').closest('.overflow-auto');
  const pageEl = document.querySelector('.sytue-page');
  return {
    canvasBg: getComputedStyle(scroller).backgroundColor,
    pageBg: getComputedStyle(pageEl).backgroundColor,
  };
});
await page.screenshot({ path: `${OUT}/1280-full-z100-dark.png` });
await page.emulateMedia({ colorScheme: 'light' });

await savedChip();
metrics.consoleErrors = consoleErrors.slice(0, 8);
console.log('METRICS ' + JSON.stringify(metrics, null, 1));
await browser.close();
console.log('LISTO');
