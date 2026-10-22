// Humo del editor nativo de Sytue Docs (integración BLUEPRINT.md §11).
// Uso: cd test/e2e && bun smoke-docs-editor.mjs
// 1) login IdP → 2) POST /api/create → 3) /edit?id= renderiza header+ribbon+
// página+statusbar → 4) escribir + Negrita → 5) chip «Guardado» → 6) reload →
// el texto persiste y el fragmento en negrita conserva font-weight.
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';

const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
const page = await context.newPage();

const consoleErrors = [];
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text());
});
const docsApiHits = [];
page.on('request', (r) => {
  if (r.url().includes('docs-api.sytue.localhost')) docsApiHits.push(r.url());
});

function fail(msg) {
  console.error(`HUMO KO: ${msg}`);
  process.exitCode = 1;
}

// --- 1. Login (lockout suave del IdP: reintento tras el refill, ver helpers.ts) ---
let logged = false;
for (let intento = 1; intento <= 5 && !logged; intento++) {
  if (intento > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' });
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
  await page.click('button[type=submit]');
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', {
      timeout: 90_000,
      waitUntil: 'domcontentloaded',
    })
    .then(() => true)
    .catch(() => false);
}
if (!logged) {
  fail('no se pudo iniciar sesión en el IdP');
  await browser.close();
  process.exit(1);
}
console.log('1. Sesión iniciada.');

// --- 2. Crear documento ---
const title = `humo-editor-${Date.now()}`;
const created = await page.request.post(`${DOCS}/api/create`, {
  data: { kind: 'document', title },
});
if (created.status() !== 201) {
  fail(`POST /api/create → ${created.status()}: ${await created.text()}`);
  await browser.close();
  process.exit(1);
}
const { id } = await created.json();
console.log(`2. Documento creado: ${id}`);

// --- 3. Abrir el editor y comprobar las cuatro zonas ---
await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
const zonas = [
  ['cabecera (título)', '#editor-title-input'],
  ['ribbon (tablist)', '[role="tablist"][aria-label="Cinta de opciones"]'],
  ['página (folio)', '.sytue-page'],
  ['barra de estado (zoom)', 'input[type="range"][aria-label="Zoom"]'],
];
for (const [nombre, sel] of zonas) {
  try {
    await page.waitForSelector(sel, { state: 'visible', timeout: 90_000 });
    console.log(`3. Zona visible: ${nombre}`);
  } catch {
    fail(`zona NO visible: ${nombre} (${sel})`);
  }
}

// --- 4. Escribir + Negrita ---
await page.click('.sytue-page');
await page.keyboard.type('Hola mundo persistente. ');
await page.click('button[aria-label="Negrita"]');
await page.keyboard.type('énfasis');
console.log('4. Texto escrito con fragmento en negrita.');

// --- 5. Autosave: respuesta 200 de POST /api/ops y chip «Guardado» ---
const opsOk = await page
  .waitForResponse(
    (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
    { timeout: 20_000 },
  )
  .then(() => true)
  .catch(() => false);
if (!opsOk) fail('el autosave no llegó (sin POST /api/ops 200)');
const chipOk = await page
  .waitForFunction(
    () => {
      const chips = Array.from(document.querySelectorAll('[aria-live="polite"]'));
      return chips.some((el) => /^\s*Guardado(\s|$)/.test(el.textContent || ''));
    },
    { timeout: 15_000 },
  )
  .then(() => true)
  .catch(() => false);
if (!chipOk) fail('el chip «Guardado» no apareció');
else console.log('5. Autosave confirmado (POST /api/ops 200 + chip «Guardado»).');

// --- 6. Recargar y comprobar persistencia ---
await page.reload({ waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
const persistencia = await page
  .waitForFunction(
    () => {
      const root = document.querySelector('.sytue-page');
      if (!root || !root.textContent?.includes('Hola mundo persistente.')) return false;
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      let node;
      while ((node = walker.nextNode())) {
        if (node.textContent && node.textContent.includes('énfasis')) {
          const w = getComputedStyle(node.parentElement).fontWeight;
          return w === 'bold' || parseInt(w, 10) >= 600;
        }
      }
      return false;
    },
    { timeout: 20_000 },
  )
  .then(() => true)
  .catch(() => false);
if (!persistencia) {
  const html = await page.$eval('.sytue-page', (el) => el.innerHTML).catch(() => '(sin folio)');
  fail(`tras recargar no persiste el texto o la negrita. innerHTML: ${html.slice(0, 500)}`);
} else {
  console.log('6. Persistencia OK: texto y negrita sobreviven a la recarga.');
}

// --- 7. Captura + salud ---
await page.screenshot({ path: 'shots/docs-editor-smoke.png' });
console.log('7. Captura: shots/docs-editor-smoke.png');
if (docsApiHits.length > 0) fail(`peticiones directas a docs-api (prohibidas): ${docsApiHits.join(', ')}`);
if (consoleErrors.length > 0) console.log(`AVISO consola (${consoleErrors.length}): ${consoleErrors.slice(0, 5).join(' | ')}`);
else console.log('8. Sin errores de consola.');

await browser.close();
console.log(process.exitCode ? 'HUMO: FALLOS — ver arriba' : 'HUMO: TODO EN VERDE');
