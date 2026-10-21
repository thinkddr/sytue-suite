// Verificador visual de la zona cabecera+ribbon del editor Sytue Docs.
// Uso: cd test/e2e && bun verify-header-ribbon.mjs
// Login → crea documento → interactúa (pestañas, texto, popovers, teclado) →
// captura la zona en 1440×900 y 1280×800 → mide alturas y atributos.
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = 'admin@sytue.localhost';
const PASS = 'sytue-dev-admin';
const CLIP = { x: 0, y: 0, width: 1440, height: 240 }; // topbar 56 + header 48 + tabs 32 + grupos 68 + margen
const out = (n) => `shots/hr-${n}.png`;
const report = { measures: {}, checks: [], consoleErrors: [] };

const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
const page = await context.newPage();
page.on('console', (m) => {
  if (m.type() === 'error') report.consoleErrors.push(m.text().slice(0, 200));
});

// --- Login (patrón de smoke-docs-editor.mjs, con reintento por lockout suave) ---
let logged = false;
for (let i = 1; i <= 5 && !logged; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded', timeout: 90_000 }).catch(() => {});
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', EMAIL);
  await page.fill('input[name=password]', PASS);
  await page.click('button[type=submit]', { noWaitAfter: true }).catch(() => {});
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' })
    .then(() => true).catch(() => false);
}
if (!logged) { console.error('KO: sin sesión'); process.exit(1); }

// --- Crear documento (kind document = todas las pestañas) ---
const created = await page.request.post(`${DOCS}/api/create`, {
  data: { kind: 'document', title: `verif-header-ribbon-${Date.now()}` },
});
if (created.status() !== 201) { console.error('KO create', created.status(), await created.text()); process.exit(1); }
const { id } = await created.json();
console.log('doc:', id);

await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForSelector('[role="tablist"][aria-label="Cinta de opciones"]', { state: 'visible', timeout: 30_000 });
await page.waitForTimeout(400);

// --- Medidas estructurales ---
report.measures = await page.evaluate(() => {
  const r = (sel) => {
    const el = document.querySelector(sel);
    if (!el) return null;
    const b = el.getBoundingClientRect();
    return { top: Math.round(b.top), h: Math.round(b.height), w: Math.round(b.width) };
  };
  const header = document.querySelector('#editor-title-input')?.closest('header');
  const tablist = document.querySelector('[role="tablist"][aria-label="Cinta de opciones"]');
  const tabsStrip = tablist?.parentElement;
  const panel = document.querySelector('#ribbon-tabpanel');
  const activeTab = document.querySelector('[role="tab"][aria-selected="true"]');
  const underline = activeTab?.querySelector('span[aria-hidden]');
  const bold = document.querySelector('button[aria-label="Negrita"]');
  const cs = (el, p) => (el ? getComputedStyle(el)[p] : null);
  const bb = (el) => (el ? { h: Math.round(el.getBoundingClientRect().height), w: Math.round(el.getBoundingClientRect().width) } : null);
  return {
    header: header ? { h: Math.round(header.getBoundingClientRect().height), bg: cs(header, 'backgroundColor') } : null,
    tabsStrip: tabsStrip ? { h: Math.round(tabsStrip.getBoundingClientRect().height) } : null,
    groupsRow: panel ? { h: Math.round(panel.getBoundingClientRect().height), overflowX: cs(panel, 'overflowX') } : null,
    page: r('.sytue-page'),
    activeTab: activeTab ? { text: activeTab.textContent.trim(), color: cs(activeTab, 'color'), weight: cs(activeTab, 'fontWeight') } : null,
    underline: underline ? { h: Math.round(underline.getBoundingClientRect().height), bg: cs(underline, 'backgroundColor') } : null,
    boldBtn: bold ? { ...bb(bold), title: bold.getAttribute('title'), aria: bold.getAttribute('aria-label'), pressed: bold.getAttribute('aria-pressed') } : null,
    tabs: [...document.querySelectorAll('[role="tab"]')].map((t) => t.textContent.trim()),
    archivo: (() => {
      const b = [...(tabsStrip?.querySelectorAll('button') ?? [])].find((x) => x.textContent.trim() === 'Archivo');
      return b ? { role: b.getAttribute('role'), hasPopup: b.getAttribute('aria-haspopup') } : null;
    })(),
    groupLabels: [...document.querySelectorAll('#ribbon-tabpanel [role="group"]')].map((g) => g.getAttribute('aria-label')),
    titles: [...document.querySelectorAll('#ribbon-tabpanel button[title]')].slice(0, 20).map((b) => b.getAttribute('title')),
    selects: [...document.querySelectorAll('#ribbon-tabpanel select')].map((s) => ({ label: s.getAttribute('aria-label'), h: Math.round(s.getBoundingClientRect().height), w: Math.round(s.getBoundingClientRect().width), value: s.value })),
    canvasBg: cs(document.querySelector('.sytue-page')?.parentElement?.parentElement, 'backgroundColor'),
  };
});

// --- 1440: estado inicial (Inicio) ---
await page.screenshot({ path: out('1440-inicio'), clip: CLIP });

// --- Escribir + seleccionar + negrita (estado pressed real) + hover sobre Cursiva ---
await page.click('.sytue-page');
await page.keyboard.type('Verificación de la cinta de opciones del editor.');
await page.keyboard.press('Enter');
await page.keyboard.type('Texto con énfasis para estados.');
// seleccionar la palabra «énfasis»
await page.dblclick('.sytue-page p:nth-child(2) >> nth=0').catch(() => {});
const word = await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let n;
  while ((n = walker.nextNode())) {
    const i = n.textContent.indexOf('énfasis');
    if (i >= 0) {
      const range = document.createRange();
      range.setStart(n, i);
      range.setEnd(n, i + 'énfasis'.length);
      const sel = getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      return true;
    }
  }
  return false;
});
report.checks.push(['selección de palabra', word]);
await page.click('button[aria-label="Negrita"]');
await page.waitForTimeout(300); // throttle 100ms del selectionchange
const boldPressed = await page.getAttribute('button[aria-label="Negrita"]', 'aria-pressed');
report.checks.push(['Negrita aria-pressed tras aplicar', boldPressed]);
await page.hover('button[aria-label="Cursiva"]');
await page.waitForTimeout(120);
await page.screenshot({ path: out('1440-inicio-pressed-hover'), clip: CLIP });

// --- Chip de guardado: esperar «Guardado» ---
const chipOk = await page
  .waitForFunction(() => {
    const chips = [...document.querySelectorAll('[aria-live="polite"]')];
    return chips.some((el) => /Guardado/.test(el.textContent || '') && !/Guardando/.test(el.textContent || ''));
  }, { timeout: 20_000 })
  .then(() => true).catch(() => false);
report.checks.push(['chip Guardado tras autosave', chipOk]);
await page.screenshot({ path: out('1440-header-guardado'), clip: { ...CLIP, height: 120 } });

// --- Popover de color (paleta) ---
await page.click('button[aria-label="Color de texto"]');
await page.waitForTimeout(150);
await page.screenshot({ path: out('1440-popover-color'), clip: { ...CLIP, height: 340 } });
await page.keyboard.press('Escape');

// --- Menú Archivo ---
await page.click('text=Archivo');
await page.waitForTimeout(150);
await page.screenshot({ path: out('1440-menu-archivo'), clip: { ...CLIP, height: 420 } });
report.measures.archivoMenu = await page.evaluate(() =>
  [...document.querySelectorAll('[role="menu"][aria-label="Archivo"] [role="menuitem"]')].map((i) => i.textContent.trim()),
);
await page.keyboard.press('Escape');

// --- Resto de pestañas (clic real) ---
for (const tab of ['insertar', 'diseno', 'referencias', 'revisar', 'vista']) {
  await page.click(`#ribbon-tab-${tab}`);
  await page.waitForTimeout(200);
  report.measures[`groups-${tab}`] = await page.evaluate(() => ({
    labels: [...document.querySelectorAll('#ribbon-tabpanel [role="group"]')].map((g) => g.getAttribute('aria-label')),
    panelH: Math.round(document.querySelector('#ribbon-tabpanel').getBoundingClientRect().height),
    overflow: (() => { const p = document.querySelector('#ribbon-tabpanel'); return p.scrollWidth - p.clientWidth; })(),
  }));
  await page.screenshot({ path: out(`1440-${tab}`), clip: CLIP });
}

// --- Popover de tabla (Insertar) ---
await page.click('#ribbon-tab-insertar');
await page.waitForTimeout(150);
await page.click('button[aria-label="Insertar tabla"]');
await page.waitForTimeout(150);
await page.screenshot({ path: out('1440-popover-tabla'), clip: { ...CLIP, height: 420 } });
await page.keyboard.press('Escape');

// --- Teclado: foco roving del tablist ---
await page.click('#ribbon-tab-inicio');
await page.keyboard.press('ArrowRight'); // → Insertar
await page.waitForTimeout(150);
report.measures.roving = await page.evaluate(() => ({
  focused: document.activeElement?.id,
  selected: document.querySelector('[role="tab"][aria-selected="true"]')?.id,
}));
await page.screenshot({ path: out('1440-focus-tab'), clip: { ...CLIP, height: 140 } });

// --- Foco visible en botón del ribbon (Tab desde la pestaña hasta el panel) ---
await page.focus('button[aria-label="Insertar tabla"]');
await page.waitForTimeout(100);
await page.screenshot({ path: out('1440-focus-boton'), clip: CLIP });

// --- 1280×800: overflow por pestaña ---
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(300);
report.measures.overflow1280 = {};
for (const tab of ['inicio', 'insertar', 'diseno', 'referencias', 'revisar', 'vista']) {
  await page.click(`#ribbon-tab-${tab}`);
  await page.waitForTimeout(200);
  report.measures.overflow1280[tab] = await page.evaluate(() => {
    const p = document.querySelector('#ribbon-tabpanel');
    const header = document.querySelector('#editor-title-input')?.closest('header');
    return {
      panelOverflowPx: p.scrollWidth - p.clientWidth,
      headerOverflowPx: header ? header.scrollWidth - header.clientWidth : null,
    };
  });
  if (tab === 'inicio' || tab === 'disposicion') {
    await page.screenshot({ path: out(`1280-${tab}`), clip: { ...CLIP, width: 1280 } });
  }
}
report.measures.overflow1280.tabsStrip = await page.evaluate(() => {
  const strip = document.querySelector('[role="tablist"]')?.parentElement;
  return strip ? strip.scrollWidth - strip.clientWidth : null;
});
await page.screenshot({ path: out('1280-header'), clip: { x: 0, y: 0, width: 1280, height: 120 } });

console.log(JSON.stringify(report, null, 2));
await browser.close();
