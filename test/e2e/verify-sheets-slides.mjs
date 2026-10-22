// Verificador de la zona sheets-slides (B11) — script desechable, NO es un spec.
// Uso: cd test/e2e && bun verify-sheets-slides.mjs
// Crea hoja de cálculo y presentación, interactúa de verdad (celdas, fx, ribbon,
// miniaturas, cajas), captura 1440x900 y 1280x800 y comprueba persistencia.
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';

const findings = [];
const ok = (msg) => console.log(`OK  ${msg}`);
const ko = (msg) => {
  findings.push(msg);
  console.log(`KO  ${msg}`);
};

import { existsSync } from 'node:fs';
const STATE = '/tmp/sytue-verify-state.json';
const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  ...(existsSync(STATE) ? { storageState: STATE } : {}),
});
context.setDefaultNavigationTimeout(120_000); // dev-server frío: la primera compilación tarda
context.setDefaultTimeout(30_000);
const page = await context.newPage();

const consoleErrors = [];
page.on('console', (m) => {
  const t = m.text();
  if (m.type() === 'error' && !/@fs\/|\.vite\/deps|ERR_ABORTED/.test(t)) consoleErrors.push(t);
});
page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`));
page.on('dialog', (d) => void d.accept().catch(() => {}));
const docsApiHits = [];
page.on('request', (r) => {
  if (r.url().includes('docs-api.sytue.localhost')) docsApiHits.push(r.url());
});

// --- Login (lockout suave del IdP: reintentos con recarga del bucket) ---
let logged = false;
for (let intento = 1; intento <= 6 && !logged; intento++) {
  if (intento > 1) await page.waitForTimeout(14_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' }).catch(() => {});
  if (page.url() === 'about:blank') continue;
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  const email = page.locator('input[name=email]');
  if (!(await email.isVisible().catch(() => false))) continue; // lockout: esperar recarga del bucket
  await email.fill(EMAIL);
  await page.fill('input[name=password]', PASS);
  await page.click('button[type=submit]', { noWaitAfter: true });
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' })
    .then(() => true)
    .catch(() => false);
}
if (!logged) { console.error('FATAL: sin sesión'); process.exit(1); }
await context.storageState({ path: STATE });
ok('sesión iniciada');

async function crear(kind, title) {
  const res = await page.request.post(`${DOCS}/api/create`, { data: { kind, title } });
  if (res.status() !== 201) throw new Error(`create ${kind} → ${res.status()}`);
  return (await res.json()).id;
}

const esperarGuardado = async () => {
  await page
    .waitForFunction(
      () => {
        const chips = Array.from(document.querySelectorAll('[aria-live="polite"]'));
        return chips.some((el) => /^\s*Guardado(\s|$)/.test(el.textContent || ''));
      },
      { timeout: 20_000 },
    )
    .then(() => ok('chip «Guardado»'))
    .catch(() => ko('el chip «Guardado» no llegó tras editar'));
};

const textoStatus = async () =>
  (await page.locator('.sytue-editor > div:last-child, [class*="editor-status"]').first().textContent().catch(() => '')) ||
  (await page.locator('input[aria-label="Zoom"]').locator('xpath=ancestor::div[contains(@class,"border-t")][1]').textContent().catch(() => ''));

/* =========================================================================
 * 1. HOJA DE CÁLCULO
 * ========================================================================= */
const sheetId = await crear('spreadsheet', `verif-hoja-${Date.now()}`);
ok(`hoja creada ${sheetId}`);
await page.goto(`${DOCS}/edit?id=${sheetId}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('[role="grid"]', { state: 'visible', timeout: 60_000 });

// Chrome: pestañas visibles según kind
const tabs = await page.locator('[role="tab"]').allTextContents();
const esperadasSheet = ['Inicio', 'Colaboración', 'Vista'];
if (JSON.stringify(tabs) !== JSON.stringify(esperadasSheet))
  ko(`pestañas de hoja: ${JSON.stringify(tabs)} (esperado ${JSON.stringify(esperadasSheet)})`);
else ok(`pestañas hoja: ${tabs.join(', ')}`);
for (const sel of ['#editor-title-input', 'input[aria-label="Zoom"]', 'button[aria-label="Buscar"]', 'input[aria-label="Barra de fórmulas"]']) {
  if (!(await page.locator(sel).first().isVisible().catch(() => false))) ko(`hoja: falta zona del chrome: ${sel}`);
}
const st1 = await textoStatus();
if (!/Hoja\s+1\s+de\s+1/.test(st1)) ko(`barra de estado hoja: «${st1.trim().slice(0, 80)}» (esperado «Hoja 1 de 1»)`);
else ok('barra de estado: Hoja 1 de 1');

// Editar celdas: teclear sustituye, Enter confirma y baja
await page.locator('[data-ref="A1"]').click();
await page.keyboard.type('Producto');
await page.keyboard.press('Enter');
await page.keyboard.type('Manzanas');
await page.keyboard.press('Enter'); // A2 → A3
await page.locator('[data-ref="B1"]').click();
await page.keyboard.type('Precio');
await page.keyboard.press('Tab'); // → C1
await page.locator('[data-ref="B2"]').click();
await page.keyboard.type('1234,5');
await page.keyboard.press('Enter');

// Barra de fórmulas: escribir en la celda seleccionada (B3) y bajar
await page.locator('input[aria-label="Barra de fórmulas"]').fill('desde fx');
await page.locator('input[aria-label="Barra de fórmulas"]').press('Enter');
const b3 = await page.locator('[data-ref="B3"]').textContent();
if (b3?.trim() !== 'desde fx') ko(`fx→celda: B3 = «${b3}» (esperado «desde fx»)`);
else ok('barra de fórmulas escribe en la celda y baja');
// ¿La referencia de la barra refleja la celda? (tras Enter debe ser B4)
const refBox = await page.locator('[title="Celda activa"]').textContent();
if (refBox?.trim() !== 'B4') ko(`caja de referencia = «${refBox?.trim()}» tras Enter en fx (esperado B4)`);

// Negrita a nivel de celda desde el ribbon (celda A1 seleccionada, sin editar)
await page.locator('[data-ref="A1"]').click();
await page.locator('button[aria-label="Negrita"]').click();
const a1html = await page.locator('[data-ref="A1"]').innerHTML();
if (!/<b>/.test(a1html)) ko(`negrita de celda no aplicada: A1 = ${a1html}`);
else ok('negrita de celda desde el ribbon');

// Formato de número: B2 → moneda (es-ES sin agrupar 4 dígitos: «1234,50 €»)
await page.locator('[data-ref="B2"]').click();
await page.locator('select[aria-label="Formato de número"]').selectOption('currency');
const b2 = (await page.locator('[data-ref="B2"]').textContent())?.trim();
if (!/1234,50\s*€/.test(b2 || '')) ko(`formato moneda: B2 = «${b2}»`);
else ok(`formato moneda: B2 = «${b2}»`);

// Rango con Mayús+clic y cabeceras resaltadas
await page.locator('[data-ref="A1"]').click();
await page.locator('[data-ref="B2"]').click({ modifiers: ['Shift'] });
const refRango = await page.locator('[title="Celda activa"]').textContent();
if (refRango?.trim() !== 'A1:B2') ko(`rango Mayús+clic: referencia «${refRango?.trim()}» (esperado A1:B2)`);
else ok('rango Mayús+clic con referencia A1:B2');

// Captura 1440 (Hoja 1 con datos y rango seleccionado)
await esperarGuardado();
await page.screenshot({ path: 'shots/verify-sheet-1440.png' });

// Añadir hoja (activa pasa a la 2; la barra de estado debe seguirla)
await page.locator('button[aria-label="Añadir hoja"]').click();
const hoja2Status = await page
  .waitForFunction(
    () => /Hoja\s+2\s+de\s+2/.test(document.body.textContent || ''),
    { timeout: 5_000 },
  )
  .then(() => true)
  .catch(() => false);
if (!hoja2Status) ko(`tras añadir hoja la barra de estado no pasa a «Hoja 2 de 2» (dice «${(await textoStatus()).trim().slice(0, 60)}»)`);
else ok('añadir hoja → Hoja 2 de 2');

// Panel de ajustes (mismo chrome): Vista → Panel de ajustes
await page.getByRole('tab', { name: 'Vista' }).click();
const ajustesToggle = page.locator('button[aria-label="Panel de ajustes"], [aria-label="Panel de ajustes"]').first();
if (await ajustesToggle.isVisible().catch(() => false)) {
  await ajustesToggle.click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: 'shots/verify-sheet-vista-panel.png' });
  await ajustesToggle.click();
} else ko('hoja: no se encontró el toggle «Panel de ajustes» en Vista');
// ¿Regla ausente/deshabilitada en hoja?
const reglaSheet = await page.locator('[aria-label="Regla"]').count();
ok(`toggle Regla en hoja (debe estar ausente o deshabilitado): count=${reglaSheet}`);
await page.getByRole('tab', { name: 'Inicio' }).click();

// Zoom desde la barra de estado
await page.locator('button[aria-label="Ampliar"]').click();
await page.locator('button[aria-label="Ampliar"]').click();
const zoomTxt = await page.locator('input[aria-label="Zoom"]').inputValue();
if (zoomTxt !== '120') ko(`zoom: ${zoomTxt} (esperado 120)`);
else ok('zoom 120 % aplicado');
await page.waitForTimeout(400);
await page.screenshot({ path: 'shots/verify-sheet-zoom120.png' });
await page.locator('button[aria-label="Reducir"]').click();
await page.locator('button[aria-label="Reducir"]').click();

// Recargar: persistencia de hojas, hoja activa (2), y datos de la Hoja 1
await esperarGuardado();
await page.reload({ waitUntil: 'domcontentloaded' });
await page.waitForSelector('[role="grid"]', { state: 'visible', timeout: 60_000 });
const antesPersist = findings.length;
const hojas = await page.evaluate(() =>
  Array.from(document.querySelectorAll('button[aria-pressed]'))
    .filter((b) => /^Hoja \d/.test(b.textContent?.trim() || ''))
    .map((b) => `${b.textContent?.trim()}:${b.getAttribute('aria-pressed')}`),
);
if (hojas.length !== 2) ko(`persistencia: pestañas de hoja tras recarga = ${JSON.stringify(hojas)} (esperadas 2)`);
if (!hojas.some((h) => h.startsWith('Hoja 2') && h.endsWith('true')))
  ko(`persistencia: hoja activa tras recarga = ${JSON.stringify(hojas)} (se guardó con Hoja 2 activa)`);
// Volver a la Hoja 1 y comprobar los datos
await page.getByRole('button', { name: /^Hoja 1/ }).click();
await page.waitForTimeout(300);
const persist = await page.evaluate(() => {
  const q = (ref) => document.querySelector(`[data-ref="${ref}"]`);
  return {
    a1: q('A1')?.innerHTML ?? null,
    a2: q('A2')?.textContent ?? null,
    b1: q('B1')?.textContent ?? null,
    b2: q('B2')?.textContent ?? null,
    b3: q('B3')?.textContent ?? null,
  };
});
if (!/<b>Producto<\/b>/.test(persist.a1 || '')) ko(`persistencia: A1 = ${persist.a1} (esperado <b>Producto</b>)`);
if (persist.a2?.trim() !== 'Manzanas') ko(`persistencia: A2 = «${persist.a2}»`);
if (persist.b1?.trim() !== 'Precio') ko(`persistencia: B1 = «${persist.b1}»`);
if (!/1234,50\s*€/.test(persist.b2 || '')) ko(`persistencia: B2 = «${persist.b2}»`);
if (persist.b3?.trim() !== 'desde fx') ko(`persistencia: B3 = «${persist.b3}»`);
if (findings.length === antesPersist) ok('persistencia de la rejilla (datos, negrita, formato, hojas, activa) tras recarga');
await page.screenshot({ path: 'shots/verify-sheet-reload.png' });

// Captura 1280x800
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(400);
await page.screenshot({ path: 'shots/verify-sheet-1280.png' });
await page.setViewportSize({ width: 1440, height: 900 });

/* =========================================================================
 * 2. PRESENTACIÓN
 * ========================================================================= */
const slidesId = await crear('presentation', `verif-slides-${Date.now()}`);
ok(`presentación creada ${slidesId}`);
await page.goto(`${DOCS}/edit?id=${slidesId}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('[data-slidebox]', { state: 'visible', timeout: 60_000 });

const tabsP = await page.locator('[role="tab"]').allTextContents();
const esperadasP = ['Inicio', 'Insertar', 'Colaboración', 'Vista'];
if (JSON.stringify(tabsP) !== JSON.stringify(esperadasP))
  ko(`pestañas de presentación: ${JSON.stringify(tabsP)} (esperado ${JSON.stringify(esperadasP)})`);
else ok(`pestañas presentación: ${tabsP.join(', ')}`);
const stP = await textoStatus();
if (!/Diapositiva\s+1\s+de\s+1/.test(stP)) ko(`barra de estado presentación: «${stP.trim().slice(0, 80)}»`);
else ok('barra de estado: Diapositiva 1 de 1');

// Editar el título (doble clic = editar)
const caja = page.locator('[data-slidebox]').last();
await caja.dblclick();
await page.keyboard.press(process.platform === 'darwin' ? 'Meta+a' : 'Control+a');
await page.keyboard.type('Sytue One 2026');
await page.keyboard.press('Escape'); // commit
ok('título de diapositiva editado');

// Nueva diapositiva desde el carril (el ribbon Inicio tiene otro botón igual: scoped)
const rail = page.locator('div.w-40');
await rail.locator('button[aria-label="Nueva diapositiva"]').click();
const stP2 = await textoStatus();
if (!/Diapositiva\s+2\s+de\s+2/.test(stP2)) ko(`tras nueva diapositiva: «${stP2.trim().slice(0, 60)}»`);
else ok('nueva diapositiva → Diapositiva 2 de 2');

// Miniaturas: 2, la primera muestra el texto tecleado, clic en la 1 la activa
const thumbs = page.locator('button[aria-label^="Diapositiva"]');
if ((await thumbs.count()) !== 2) ko(`miniaturas: ${await thumbs.count()} (esperado 2)`);
const thumb1Text = await thumbs.first().textContent();
if (!thumb1Text?.includes('Sytue One 2026')) ko(`la miniatura 1 no refleja el texto editado: «${thumb1Text?.trim()}»`);
else ok('miniatura 1 refleja el texto editado');
await thumbs.first().click();
const stP3 = await textoStatus();
if (!/Diapositiva\s+1\s+de\s+2/.test(stP3)) ko(`clic en miniatura 1: «${stP3.trim().slice(0, 60)}» (esperado Diapositiva 1 de 2)`);
else ok('clic en miniatura activa la diapositiva');

// Cuadro de texto nuevo + selección (asas visibles) + mover con flechas
await thumbs.nth(1).click();
await page.locator('button[aria-label="Cuadro de texto"]').click();
const nueva = page.locator('[data-slidebox]').last();
await nueva.dblclick();
await page.keyboard.press(process.platform === 'darwin' ? 'Meta+a' : 'Control+a');
await page.keyboard.type('Caja móvil');
await page.keyboard.press('Escape');
// seleccionar con un clic (sin editar) → asas
await nueva.click();
const asas = await page.locator('[title^="Mover el cuadro"], [title^="Redimensionar el cuadro"]').count();
if (asas !== 2) ko(`asas de mover/redimensionar tras seleccionar: ${asas} (esperado 2)`);
else ok('asas de mover y redimensionar visibles');
// arrastrar el asa de mover
const asaMover = page.locator('[title^="Mover el cuadro"]');
const antes = await nueva.evaluate((el) => el.parentElement.style.left);
if (await asaMover.isVisible().catch(() => false)) {
  const bb = await asaMover.boundingBox();
  await page.mouse.move(bb.x + bb.width / 2, bb.y + bb.height / 2);
  await page.mouse.down();
  await page.mouse.move(bb.x + 120, bb.y + 60, { steps: 8 });
  await page.mouse.up();
  const despues = await nueva.evaluate((el) => el.parentElement.style.left);
  if (antes === despues) ko(`arrastre del asa de mover no movió la caja (left ${antes} → ${despues})`);
  else ok(`arrastre mueve la caja (left ${antes} → ${despues})`);
}

// Disposición dos columnas sobre la diapositiva 2 (select del ribbon Inicio)
await page.locator('select[aria-label="Disposición de la diapositiva"]').selectOption('dos-columnas');
await page.waitForTimeout(200);
const cajas2 = await page.locator('[data-slidebox]').count();
if (cajas2 !== 3) ko(`disposición «dos columnas»: ${cajas2} cajas (esperado 3)`);
else ok('disposición «dos columnas» aplica 3 cajas');

await esperarGuardado();
await page.screenshot({ path: 'shots/verify-slides-1440.png' });

// Duplicar y eliminar (confirm auto-aceptado) desde el carril
await rail.locator('button[aria-label="Duplicar diapositiva"]').click();
await page.waitForTimeout(200);
const nThumbs = await thumbs.count();
if (nThumbs !== 3) ko(`duplicar: ${nThumbs} miniaturas (esperado 3)`);
await rail.locator('button[aria-label="Eliminar diapositiva"]').click();
await page.waitForTimeout(200);
if ((await thumbs.count()) !== 2) ko(`eliminar: ${await thumbs.count()} miniaturas (esperado 2)`);
else ok('duplicar + eliminar diapositiva funcionan');

// Recarga: persistencia de diapositivas y texto
await esperarGuardado();
await page.reload({ waitUntil: 'domcontentloaded' });
await page.waitForSelector('[data-slidebox]', { state: 'visible', timeout: 60_000 });
const nThumbsR = await thumbs.count();
if (nThumbsR !== 2) ko(`persistencia slides: ${nThumbsR} miniaturas tras recarga (esperado 2)`);
const t1 = await thumbs.first().textContent();
if (!t1?.includes('Sytue One 2026')) ko(`persistencia slides: miniatura 1 sin el título editado («${t1?.trim().slice(0, 60)}»)`);
else ok('persistencia de presentación tras recarga');
await page.screenshot({ path: 'shots/verify-slides-reload.png' });

// 1280x800
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(400);
await page.screenshot({ path: 'shots/verify-slides-1280.png' });

/* ---------- salud ---------- */
if (docsApiHits.length > 0) ko(`peticiones directas a docs-api: ${docsApiHits.slice(0, 3).join(', ')}`);
if (consoleErrors.length > 0) ko(`errores de consola (${consoleErrors.length}): ${[...new Set(consoleErrors)].slice(0, 5).join(' | ')}`);
else ok('sin errores de consola');

await browser.close();
console.log('\n===== RESUMEN =====');
if (findings.length === 0) console.log('TODO EN VERDE');
else findings.forEach((f, i) => console.log(`${i + 1}. ${f}`));
