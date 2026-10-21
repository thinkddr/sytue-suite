// Verificador de la zona «paneles-status» del editor Sytue Docs.
// Uso: cd test/e2e && bun verify-paneles-status.mjs
// Carril izquierdo + paneles (buscar/esquema/comentarios) + panel derecho +
// statusbar: aperturas/cierres, anchos, scroll interno, contadores, zoom.
// Capturas en shots/paneles-status/ a 1440x900 y 1280x800.
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = 'admin@sytue.localhost';
const PASS = 'sytue-dev-admin';
const OUT = 'shots/paneles-status';
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
const page = await context.newPage();
page.setDefaultNavigationTimeout(120_000);

const consoleErrors = [];
page.on('console', (m) => {
  const t = m.text();
  if (m.type() === 'error' && !/@fs\/|\.vite\/deps|ERR_ABORTED/.test(t)) consoleErrors.push(t);
});

const results = [];
function check(name, ok, detail = '') {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'OK ' : 'KO '} ${name}${detail ? ` — ${detail}` : ''}`);
}
async function shot(name, clip) {
  await page.screenshot({ path: `${OUT}/${name}.png`, ...(clip ? { clip } : {}) });
  console.log(`SHOT ${OUT}/${name}.png`);
}

// ---------- 1. Login ----------
let logged = false;
for (let i = 1; i <= 5 && !logged; i++) {
  if (i > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' });
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', EMAIL);
  await page.fill('input[name=password]', PASS);
  await page.click('button[type=submit]');
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' })
    .then(() => true).catch(() => false);
}
if (!logged) { console.error('KO login'); await browser.close(); process.exit(1); }
console.log('LOGIN ok');

// ---------- 2. Crear documento + sembrar comentarios ----------
const created = await page.request.post(`${DOCS}/api/create`, {
  data: { kind: 'document', title: `verify-paneles-${Date.now()}` },
});
if (created.status() !== 201) { console.error(`KO create ${created.status()}`); await browser.close(); process.exit(1); }
const { id } = await created.json();
console.log(`DOC ${id}`);

// 9 comentarios sembrados (probar scroll interno del panel); uno con ancla.
for (let i = 1; i <= 9; i++) {
  const body =
    i === 5
      ? 'Comentario largo para probar el ajuste de línea dentro de la tarjeta del panel: la caja no debería desbordar horizontalmente nunca, ni siquiera con palabras larguísimas como supercalifragilisticoespialidoso.'
      : `Comentario sembrado n.º ${i} para el panel.`;
  const anchor = i === 3 ? { text: 'lorem' } : null;
  const r = await page.request.post(`${DOCS}/api/comments?id=${id}`, { data: { body, anchor } });
  if (r.status() !== 201) console.log(`AVISO: comentario ${i} → ${r.status()}`);
}

// ---------- 3. Abrir editor ----------
await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
await page.waitForTimeout(800);

// Estado inicial de la statusbar (documento vacío).
const statusBar = page.locator('input[type="range"][aria-label="Zoom"]').locator('xpath=ancestor::div[contains(@class,"border-t")]').first();
const stTxt0 = (await statusBar.textContent())?.replace(/\s+/g, ' ').trim();
check('statusbar inicial', /Página 1 de 1/.test(stTxt0 ?? '') && /Palabras: 0/.test(stTxt0 ?? ''), stTxt0);
check('statusbar idioma', (await page.locator('select[aria-label="Idioma del documento"]').count()) === 1);

// Geometría del cromo.
const geom = await page.evaluate(() => {
  const railBtn = document.querySelector('button[aria-label="Buscar"]');
  const rail = railBtn?.parentElement;
  const status = document.querySelector('input[aria-label="Zoom"]')?.closest('div[class*="border-t"]');
  const strip = document.querySelector('button[aria-label="Panel de ajustes"]')?.parentElement;
  const r = (el) => (el ? { w: Math.round(el.getBoundingClientRect().width), h: Math.round(el.getBoundingClientRect().height) } : null);
  return { rail: r(rail), status: r(status), strip: r(strip) };
});
check('carril izq 40px', geom.rail?.w === 40, JSON.stringify(geom.rail));
check('statusbar 28px', geom.status?.h === 28, JSON.stringify(geom.status));
check('franja ajustes plegada 40px', geom.strip?.w === 40, JSON.stringify(geom.strip));

// ---------- 4. Contenido real: títulos + párrafos ----------
await page.click('.sytue-page');
await page.keyboard.type('Informe anual');
await page.getByRole('button', { name: 'Título 1' }).first().click();
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.getByRole('button', { name: 'Estilo Normal' }).first().click();
await page.keyboard.insertText('Primer párrafo con lorem de prueba y más texto corrido para el buscador, lorem otra vez.');
await page.keyboard.press('Enter');
await page.keyboard.type('Metodología');
await page.getByRole('button', { name: 'Título 2' }).first().click();
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.getByRole('button', { name: 'Estilo Normal' }).first().click();
await page.keyboard.insertText('Segundo párrafo, también con lorem dentro.');
await page.keyboard.press('Enter');
await page.keyboard.type('Detalle técnico');
await page.getByRole('button', { name: 'Título 3' }).first().click();
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.getByRole('button', { name: 'Estilo Normal' }).first().click();
// Relleno hasta superar una página A4 (1123px) para el contador de páginas.
for (let i = 0; i < 30; i++) {
  await page.keyboard.insertText(
    `Párrafo de relleno ${i + 1}: texto suficiente para crecer el folio y forzar la segunda página del contador de estado, con lorem incluido para sumar coincidencias al buscador y palabras al contador inferior de la barra de estado del editor.`,
  );
  await page.keyboard.press('Enter');
}

// Esperar autosave (chip «Guardado») para palabras reales.
await page.waitForFunction(
  () => Array.from(document.querySelectorAll('[aria-live="polite"]')).some((el) => /^\s*Guardado(\s|$)/.test(el.textContent || '')),
  { timeout: 20_000 },
).catch(() => check('chip Guardado tras teclear', false));

// Palabras: statusbar vs recuento directo del folio (misma fórmula del blueprint).
const wordsCmp = await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  const real = root.innerHTML.replace(/<[^>]+>/g, ' ').trim().split(/\s+/).filter(Boolean).length;
  const span = Array.from(document.querySelectorAll('span')).find((s) => /^Palabras:/.test(s.textContent || ''));
  return { real, shown: span ? span.textContent : null };
});
check('palabras reales', wordsCmp.shown === `Palabras: ${wordsCmp.real}`, JSON.stringify(wordsCmp));

// Contador de páginas: debería ser ≥ 2 ya; scroll al fondo → página actual = última.
await page.waitForTimeout(900); // throttle 500ms de measure
const pages1 = await page.evaluate(() => {
  const span = Array.from(document.querySelectorAll('span')).find((s) => /^Página \d+ de \d+/.test(s.textContent || ''));
  return span?.textContent ?? null;
});
check('páginas ≥ 2 tras relleno', /de ([2-9]|\d{2,})/.test(pages1 ?? ''), pages1);
const scrollInfo = await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  let sc = null;
  for (let n = root.parentElement; n; n = n.parentElement) {
    const s = getComputedStyle(n);
    if (/(auto|scroll)/.test(s.overflowY)) { sc = n; break; }
  }
  sc.scrollTop = sc.scrollHeight;
  return { canScroll: sc.scrollHeight > sc.clientHeight };
});
check('lienzo con scroll propio', scrollInfo.canScroll);
await page.waitForTimeout(400);
const pages2 = await page.evaluate(() => {
  const span = Array.from(document.querySelectorAll('span')).find((s) => /^Página \d+ de \d+/.test(s.textContent || ''));
  return span?.textContent ?? null;
});
check('página actual sigue al scroll', /^Página ([2-9]|\d{2,}) de \d+$/.test(pages2 ?? ''), pages2);
await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  for (let n = root.parentElement; n; n = n.parentElement) {
    const s = getComputedStyle(n);
    if (/(auto|scroll)/.test(s.overflowY)) { n.scrollTop = 0; break; }
  }
});

// ---------- 5. Zoom (statusbar) ----------
const slider = page.locator('input[type="range"][aria-label="Zoom"]');
await slider.focus();
await page.keyboard.press('ArrowRight'); // 110
await page.keyboard.press('ArrowRight'); // 120
await page.keyboard.press('ArrowRight'); // 130
await page.waitForTimeout(200);
const zoomA = await page.evaluate(() => {
  const label = Array.from(document.querySelectorAll('span')).find((s) => /^\d+ %$/.test((s.textContent || '').trim()));
  const root = document.querySelector('.sytue-page');
  let scale = null;
  for (let n = root.parentElement; n; n = n.parentElement) {
    const t = getComputedStyle(n).transform;
    if (t && t !== 'none') { scale = Math.round(new DOMMatrix(t).a * 100); break; }
  }
  return { label: label?.textContent?.trim(), scale };
});
check('slider→130% y folio escala', zoomA.label === '130 %' && zoomA.scale === 130, JSON.stringify(zoomA));
await page.keyboard.press('End'); // 200
await page.waitForTimeout(150);
check('botón Ampliar deshabilitado a 200', await page.locator('button[aria-label="Ampliar"]').isDisabled());
await page.keyboard.press('Home'); // 50
await page.waitForTimeout(150);
check('botón Reducir deshabilitado a 50', await page.locator('button[aria-label="Reducir"]').isDisabled());
// Botones − / + reales.
await page.locator('button[aria-label="Ampliar"]').click(); // 60
await page.locator('button[aria-label="Ampliar"]').click(); // 70
const zoomB = await page.evaluate(() =>
  Array.from(document.querySelectorAll('span')).find((s) => /^\d+ %$/.test((s.textContent || '').trim()))?.textContent?.trim(),
);
check('botones +/− mueven el zoom', zoomB === '70 %', zoomB);
// Volver a 100 con el slider.
await slider.focus();
for (let i = 0; i < 3; i++) await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);
await shot('1440-status', { x: 0, y: 900 - 34, width: 1440, height: 34 });

// ---------- 6. Panel Buscar ----------
await page.click('button[aria-label="Buscar"]');
const searchInput = page.locator('input[aria-label="Buscar en el documento"]');
check('panel Buscar abre', await searchInput.isVisible());
const panelGeom = await page.evaluate(() => {
  const aside = document.querySelector('aside[aria-label="Buscar"]');
  return aside ? Math.round(aside.getBoundingClientRect().width) : null;
});
check('panel izq 288px', panelGeom === 288, `${panelGeom}px`);
const searchFocused = await page.evaluate(() => document.activeElement?.getAttribute('aria-label') === 'Buscar en el documento');
check('input de búsqueda recibe foco al abrir', searchFocused);
await searchInput.fill('lorem');
await page.waitForTimeout(600); // debounce 300ms
const counter1 = await page.locator('aside[aria-label="Buscar"] p').first().textContent();
check('contador de coincidencias', /^1 de \d+$/.test((counter1 ?? '').trim()), counter1?.trim());
const highlights = await page.evaluate(() => (typeof Highlight !== 'undefined' && CSS.highlights ? Array.from(CSS.highlights.keys()) : []));
check('resaltado nativo pintado', highlights.includes('sytue-search'), highlights.join(','));
await page.locator('button[aria-label="Coincidencia siguiente"]').click();
await page.locator('button[aria-label="Coincidencia siguiente"]').click();
const counter2 = (await page.locator('aside[aria-label="Buscar"] p').first().textContent())?.trim();
check('navegación siguiente', /^3 de \d+$/.test(counter2 ?? ''), counter2);
await page.locator('button[aria-label="Coincidencia anterior"]').click();
const counter3 = (await page.locator('aside[aria-label="Buscar"] p').first().textContent())?.trim();
check('navegación anterior', /^2 de \d+$/.test(counter3 ?? ''), counter3);
// Enter en el input = siguiente.
await searchInput.focus();
await page.keyboard.press('Enter');
const counter4 = (await page.locator('aside[aria-label="Buscar"] p').first().textContent())?.trim();
check('Enter = siguiente', /^3 de \d+$/.test(counter4 ?? ''), counter4);
// Sin coincidencias.
await searchInput.fill('zzzznoexiste');
await page.waitForTimeout(600);
const counter5 = (await page.locator('aside[aria-label="Buscar"] p').first().textContent())?.trim();
check('mensaje sin coincidencias', counter5 === 'Sin coincidencias', counter5);
await searchInput.fill('lorem');
await page.waitForTimeout(600);
await shot('1440-panel-buscar', { x: 0, y: 0, width: 380, height: 900 });
await shot('1440-full-buscar');

// ---------- 7. Panel Esquema ----------
await page.click('button[aria-label="Esquema del documento"]');
await page.waitForTimeout(300);
const outline = await page.evaluate(() => {
  const aside = document.querySelector('aside[aria-label="Esquema"]');
  if (!aside) return null;
  const btns = Array.from(aside.querySelectorAll('nav button'));
  return btns.map((b) => ({ text: b.textContent?.trim(), pl: getComputedStyle(b).paddingLeft }));
});
check('esquema: 3 títulos con sangría', outline?.length === 3 && outline[0].pl === '8px' && outline[1].pl === '16px' && outline[2].pl === '24px', JSON.stringify(outline));
check('resaltado de búsqueda limpiado al cambiar de panel', await page.evaluate(() => !CSS.highlights?.has('sytue-search')));
// Clic en un título → scroll del lienzo.
const beforeScroll = await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  for (let n = root.parentElement; n; n = n.parentElement) {
    if (/(auto|scroll)/.test(getComputedStyle(n).overflowY)) return n.scrollTop;
  }
});
await page.locator('aside[aria-label="Esquema"] nav button').last().click();
await page.waitForTimeout(700);
const afterScroll = await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  for (let n = root.parentElement; n; n = n.parentElement) {
    if (/(auto|scroll)/.test(getComputedStyle(n).overflowY)) return n.scrollTop;
  }
});
check('clic en esquema desplaza el lienzo', afterScroll !== beforeScroll, `${beforeScroll}→${afterScroll}`);
await shot('1440-panel-esquema', { x: 0, y: 0, width: 380, height: 900 });

// ---------- 8. Panel Comentarios ----------
await page.click('button[aria-label="Comentarios"]');
await page.waitForTimeout(600);
const commentCards = await page.locator('aside[aria-label="Comentarios"] article').count();
check('comentarios sembrados visibles', commentCards === 9, `${commentCards} tarjetas`);
const commentsScroll = await page.evaluate(() => {
  const aside = document.querySelector('aside[aria-label="Comentarios"]');
  const body = aside?.querySelector('.overflow-y-auto');
  if (!body) return null;
  return { scrollable: body.scrollHeight > body.clientHeight, overflowX: body.scrollWidth > body.clientWidth + 1 };
});
check('panel comentarios: scroll interno', commentsScroll?.scrollable === true, JSON.stringify(commentsScroll));
check('panel comentarios: sin desborde horizontal', commentsScroll?.overflowX === false);
// Añadir comentario por UI.
await page.locator('textarea[aria-label="Nuevo comentario"]').fill('Comentario añadido desde la interfaz.');
await page.getByRole('button', { name: 'Comentar', exact: true }).click();
await page.waitForTimeout(1200);
const cardsAfterAdd = await page.locator('aside[aria-label="Comentarios"] article').count();
check('añadir comentario por UI', cardsAfterAdd === 10, `${cardsAfterAdd} tarjetas`);
// Resolver el primero.
await page.locator('aside[aria-label="Comentarios"] article').first().getByRole('button', { name: 'Resolver' }).click();
await page.waitForTimeout(1200);
const firstOpacity = await page.evaluate(() => {
  const a = document.querySelector('aside[aria-label="Comentarios"] article');
  return a ? getComputedStyle(a).opacity : null;
});
check('resolver atenúa la tarjeta', firstOpacity === '0.6', firstOpacity);
// Responder a un hilo.
await page.locator('aside[aria-label="Comentarios"] article').nth(1).getByRole('button', { name: 'Responder' }).first().click();
await page.locator('textarea[aria-label="Respuesta al comentario"]').fill('Respuesta de prueba al hilo.');
await page.locator('aside[aria-label="Comentarios"] article').nth(1).getByRole('button', { name: 'Responder' }).last().click();
await page.waitForTimeout(1200);
const replyVisible = await page.locator('aside[aria-label="Comentarios"]').getByText('Respuesta de prueba al hilo.').count();
check('respuesta anidada visible', replyVisible === 1);
// Scroll interno arriba para la captura.
await page.evaluate(() => {
  const body = document.querySelector('aside[aria-label="Comentarios"] .overflow-y-auto');
  if (body) body.scrollTop = 0;
});
await shot('1440-panel-comentarios', { x: 0, y: 0, width: 380, height: 900 });
// Eliminar respuesta (probar trash).
const trashCount0 = await page.locator('aside[aria-label="Comentarios"] article').count();
await page.locator('button[aria-label="Eliminar respuesta"]').first().click();
await page.waitForTimeout(1200);
check('eliminar respuesta', (await page.locator('aside[aria-label="Comentarios"]').getByText('Respuesta de prueba al hilo.').count()) === 0, `tarjetas raíz ${trashCount0}`);

// Cierre por botón X y por toggle del carril.
await page.click('button[aria-label="Cerrar panel"]');
await page.waitForTimeout(200);
check('cerrar con X', (await page.locator('aside[aria-label="Comentarios"]').count()) === 0);
await page.click('button[aria-label="Buscar"]');
await page.waitForTimeout(200);
await page.click('button[aria-label="Buscar"]'); // toggle cierra
await page.waitForTimeout(200);
check('toggle del carril cierra', (await page.locator('aside[aria-label="Buscar"]').count()) === 0);
// aria-pressed del carril.
await page.click('button[aria-label="Esquema del documento"]');
const pressed = await page.evaluate(() => ({
  esquema: document.querySelector('button[aria-label="Esquema del documento"]')?.getAttribute('aria-pressed'),
  buscar: document.querySelector('button[aria-label="Buscar"]')?.getAttribute('aria-pressed'),
}));
check('aria-pressed del carril', pressed.esquema === 'true' && pressed.buscar === 'false', JSON.stringify(pressed));
await page.click('button[aria-label="Esquema del documento"]');

// ---------- 9. Panel derecho de ajustes ----------
await page.click('button[aria-label="Panel de ajustes"]');
await page.waitForTimeout(300);
const settingsGeom = await page.evaluate(() => {
  const aside = document.querySelector('aside[aria-label="Ajustes"]');
  const h = aside?.querySelector('h2')?.textContent;
  return aside ? { w: Math.round(aside.getBoundingClientRect().width), h2: h } : null;
});
check('panel ajustes abre (párrafo)', settingsGeom?.h2 === 'Ajustes de párrafo', JSON.stringify(settingsGeom));
check('ancho ajustes vs panel izq', settingsGeom?.w === 288, `${settingsGeom?.w}px (izq: 288px)`);
// Caret en un párrafo → stepper habilitado; aplicar «Antes (pt)» +.
await page.locator('.sytue-page p').first().click();
await page.waitForTimeout(300);
const stepperDisabled = await page.locator('button[aria-label="Aumentar Antes (pt)"]').isDisabled();
check('steppers habilitados con caret en párrafo', !stepperDisabled);
const mtBefore = await page.evaluate(() => document.querySelector('.sytue-page p')?.style.marginTop || '');
await page.locator('button[aria-label="Aumentar Antes (pt)"]').click();
await page.waitForTimeout(300);
const mtAfter = await page.evaluate(() => document.querySelector('.sytue-page p')?.style.marginTop || '');
check('stepper aplica margen al párrafo', mtBefore !== mtAfter && mtAfter !== '', `«${mtBefore}»→«${mtAfter}»`);
await shot('1440-ajustes-parrafo', { x: 1440 - 340, y: 0, width: 340, height: 900 });

// Insertar tabla real (popover 6×6) y contexto de tabla.
await page.getByRole('tab', { name: 'Insertar' }).click();
await page.locator('button[aria-label="Insertar tabla"]').click();
await page.locator('button[aria-label="Tabla de 3 × 4"]').click();
await page.waitForTimeout(400);
const tableCount = await page.locator('.sytue-page table').count();
check('tabla insertada', tableCount === 1);
await page.locator('.sytue-page table td').first().click();
await page.waitForTimeout(400);
const settingsTable = await page.evaluate(() => document.querySelector('aside[aria-label="Ajustes"] h2')?.textContent);
check('panel cambia a Ajustes de tabla', settingsTable === 'Ajustes de tabla', settingsTable ?? 'null');
// Conmutador cabecera alternada.
await page.getByRole('switch', { name: 'Cabecera alternada' }).click();
await page.waitForTimeout(300);
const headerRow = await page.evaluate(() => {
  const tr = document.querySelector('.sytue-page table tr');
  return tr ? tr.style.backgroundColor : null;
});
check('cabecera alternada pinta la primera fila', headerRow !== null && headerRow !== '', headerRow ?? 'null');
// ¿Existen controles de filas/columnas en alguna parte de la interfaz?
const rowColControls = await page.evaluate(() => {
  const labels = Array.from(document.querySelectorAll('button, [role="menuitem"]')).map(
    (b) => (b.getAttribute('aria-label') || b.textContent || '').toLowerCase(),
  );
  return labels.filter((l) => /(insertar|eliminar).*(fila|columna)/.test(l));
});
check('controles de fila/columna de tabla presentes', rowColControls.length > 0, rowColControls.join(' | ') || 'NINGUNO en toda la interfaz');
await shot('1440-ajustes-tabla', { x: 1440 - 340, y: 0, width: 340, height: 900 });

// Sincronía con Vista → «Panel de ajustes» y colapso a franja.
await page.getByRole('tab', { name: 'Vista' }).click();
await page.waitForTimeout(200);
const vistaToggle = page.getByRole('checkbox', { name: 'Panel de ajustes' }).first();
const vistaChecked = await vistaToggle.isChecked().catch(() => null);
check('Vista refleja panel abierto', vistaChecked === true, String(vistaChecked));
await page.locator('aside[aria-label="Ajustes"] button[aria-label="Panel de ajustes"]').click();
await page.waitForTimeout(200);
check('colapso a franja con engranaje', (await page.locator('aside[aria-label="Ajustes"]').count()) === 0 && (await page.locator('button[aria-label="Panel de ajustes"]').count()) > 0);
const vistaChecked2 = await vistaToggle.isChecked().catch(() => null);
check('Vista refleja panel cerrado', vistaChecked2 === false, String(vistaChecked2));

// ---------- 10. Estado «busy» 1440: buscar + ajustes abiertos ----------
await page.click('button[aria-label="Buscar"]');
await page.locator('input[aria-label="Buscar en el documento"]').fill('lorem');
await page.click('button[aria-label="Panel de ajustes"]');
await page.getByRole('tab', { name: 'Inicio' }).click();
await page.waitForTimeout(700);
await shot('1440-full-busy');

// ---------- 11. 1280×800 ----------
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(600);
const fit1280 = await page.evaluate(() => {
  const status = document.querySelector('input[aria-label="Zoom"]')?.closest('div[class*="border-t"]');
  const root = document.querySelector('.sytue-page');
  let sc = null;
  for (let n = root.parentElement; n; n = n.parentElement) {
    if (/(auto|scroll)/.test(getComputedStyle(n).overflowY)) { sc = n; break; }
  }
  return {
    statusOverflow: status ? status.scrollWidth > status.clientWidth + 1 : null,
    canvasW: sc ? Math.round(sc.clientWidth) : null,
    canvasHScroll: sc ? sc.scrollWidth > sc.clientWidth : null,
    bodyHScroll: document.documentElement.scrollWidth > document.documentElement.clientWidth,
  };
});
check('1280: statusbar sin desborde', fit1280.statusOverflow === false, JSON.stringify(fit1280));
check('1280: sin scroll horizontal de página completa', fit1280.bodyHScroll === false);
console.log(`1280: lienzo ${fit1280.canvasW}px, hscroll=${fit1280.canvasHScroll}`);
await shot('1280-full-busy');
await shot('1280-status', { x: 0, y: 800 - 34, width: 1280, height: 34 });
// Cerrar buscar, dejar solo ajustes.
await page.click('button[aria-label="Cerrar panel"]');
await page.waitForTimeout(300);
await shot('1280-full-ajustes');

// ---------- 12. Statusbar de hoja de cálculo y presentación ----------
for (const kind of ['spreadsheet', 'presentation']) {
  const r = await page.request.post(`${DOCS}/api/create`, { data: { kind, title: `verify-${kind}-${Date.now()}` } });
  if (r.status() !== 201) { check(`crear ${kind}`, false, String(r.status())); continue; }
  const { id: kid } = await r.json();
  await page.goto(`${DOCS}/edit?id=${kid}`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('input[aria-label="Zoom"]', { state: 'visible', timeout: 60_000 });
  await page.waitForTimeout(1000);
  const st = await page.evaluate(() => {
    const bar = document.querySelector('input[aria-label="Zoom"]')?.closest('div[class*="border-t"]');
    return bar?.textContent?.replace(/\s+/g, ' ').trim();
  });
  const want = kind === 'spreadsheet' ? /^Hoja 1 de 1/ : /^Diapositiva 1 de 1/;
  check(`statusbar ${kind}`, want.test(st ?? ''), st);
  const noWords = !/Palabras/.test(st ?? '');
  check(`statusbar ${kind} sin palabras/idioma`, noWords, st);
  // Añadir unidad (hoja/diapositiva) y ver el contador.
  const addLabel = kind === 'spreadsheet' ? 'Añadir hoja' : /Nueva diapositiva/;
  const addBtn = page.locator(`button[aria-label="${kind === 'spreadsheet' ? 'Añadir hoja' : 'Nueva diapositiva'}"]`).first();
  if ((await addBtn.count()) > 0) {
    await addBtn.click();
    await page.waitForTimeout(800);
    const st2 = await page.evaluate(() => {
      const bar = document.querySelector('input[aria-label="Zoom"]')?.closest('div[class*="border-t"]');
      return bar?.textContent?.replace(/\s+/g, ' ').trim();
    });
    const want2 = kind === 'spreadsheet' ? /^Hoja 2 de 2/ : /^Diapositiva 2 de 2/;
    check(`statusbar ${kind} tras añadir`, want2.test(st2 ?? ''), st2);
  } else {
    check(`botón añadir en ${kind}`, false, 'no encontrado');
  }
  await shot(`1280-status-${kind}`, { x: 0, y: 800 - 34, width: 1280, height: 34 });
}

// ---------- Resumen ----------
console.log('\n===== RESUMEN =====');
for (const r of results) console.log(`${r.ok ? 'OK ' : 'KO '} ${r.name}${r.detail ? ` — ${r.detail}` : ''}`);
console.log(`\nErrores de consola (${consoleErrors.length}):`);
for (const e of consoleErrors.slice(0, 10)) console.log(`  · ${e.slice(0, 200)}`);
await browser.close();
