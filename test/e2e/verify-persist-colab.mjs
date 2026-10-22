// Verificador de la zona «persistencia-colab» del editor Sytue Docs.
// Uso: cd test/e2e && bun verify-persist-colab.mjs
// Comprueba con interacción real: estados de guardado, escribir→recargar,
// renombrar→recargar, presencia+adopción LWW con 2 contextos, comentario
// creado aparece (y persiste), exportar (API + UI honesta). Capturas 1440x900
// y 1280x800 en shots/persist-colab-*.png para crítica visual posterior.
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';
const TS = Date.now();

const results = [];
const nota = (ok, name, detail = '') => {
  results.push({ ok, name, detail });
  console.log(`${ok ? 'OK ' : 'KO '} ${name}${detail ? ` — ${detail}` : ''}`);
};

const browser = await chromium.launch();
const ctxA = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  acceptDownloads: true,
});
const page = await ctxA.newPage();
page.setDefaultTimeout(90_000); // la cadena OIDC del dev puede tardar >30 s

const consoleErrors = [];
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200));
});
const docsApiHits = [];
page.on('request', (r) => {
  if (r.url().includes('docs-api.sytue.localhost')) docsApiHits.push(r.url());
});
const opsFallidas = [];
page.on('response', (r) => {
  if (r.url().includes('/api/ops') && r.request().method() === 'POST' && !r.ok())
    opsFallidas.push(`${r.status()} ${r.url()}`);
});

// --- 1. Login (lockout suave del IdP: reintentos tras refill) ---
let logged = false;
for (let intento = 1; intento <= 5 && !logged; intento++) {
  if (intento > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded' });
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') { logged = true; break; }
  await page.waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 }).catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  await page.fill('input[name=email]', EMAIL);
  await page.fill('input[name=password]', PASS);
  await page.click('button[type=submit]');
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', { timeout: 90_000, waitUntil: 'domcontentloaded' })
    .then(() => true)
    .catch(() => false);
}
if (!logged) { console.error('KO login IdP'); await browser.close(); process.exit(1); }
console.log('· Sesión iniciada.');

// --- 2. Crear documento (kind document: el representativo de la zona) ---
const created = await page.request.post(`${DOCS}/api/create`, {
  data: { kind: 'document', title: `verif-colab-${TS}` },
});
if (created.status() !== 201) {
  console.error(`KO create → ${created.status()}: ${await created.text()}`);
  await browser.close(); process.exit(1);
}
const { id } = await created.json();
console.log(`· Documento: ${id}`);

await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 60_000 });

// --- 3. Estados de guardado: observar transiciones del chip aria-live ---
await page.evaluate(() => {
  const chip = document.querySelector('header [aria-live="polite"]');
  window.__chipLog = [chip?.textContent?.trim() ?? '(sin chip)'];
  if (!chip) return;
  const obs = new MutationObserver(() => {
    const t = chip.textContent?.trim() ?? '';
    if (t !== window.__chipLog[window.__chipLog.length - 1]) window.__chipLog.push(t);
  });
  obs.observe(chip, { childList: true, characterData: true, subtree: true });
});
await page.click('.sytue-page');
const textoA = `Hola persistencia ${TS}. `;
await page.keyboard.type(textoA, { delay: 15 });
const chipGuardado = await page
  .waitForFunction(
    () => /^Guardado/.test(document.querySelector('header [aria-live="polite"]')?.textContent?.trim() ?? ''),
    { timeout: 20_000 },
  )
  .then(() => true).catch(() => false);
const chipLog = await page.evaluate(() => window.__chipLog ?? []);
nota(chipGuardado, 'guardado vuelve a «Guardado» tras escribir', `secuencia chip: ${JSON.stringify(chipLog)}`);
nota(
  chipLog.some((t) => t === 'Cambios sin guardar') && chipLog.some((t) => t === 'Guardando…'),
  'chip muestra estados intermedios (dirty y saving)',
  `vistos: ${JSON.stringify(chipLog)}`,
);

// --- 4. Renombrar (input de título → POST /api/rename) ---
const titulo = `Colab verificada ${TS}`;
const renameResp = page
  .waitForResponse((r) => r.url().includes('/api/rename') && r.request().method() === 'POST', { timeout: 10_000 })
  .catch(() => null);
await page.fill('#editor-title-input', titulo);
await page.keyboard.press('Enter');
const rr = await renameResp;
nota(rr !== null && rr.ok(), 'renombrar dispara POST /api/rename 2xx', rr ? `status ${rr.status()}` : 'sin petición');

// --- 5. Recargar: texto y título persisten ---
await page.reload({ waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 60_000 });
const textoPersiste = await page
  .waitForFunction((t) => document.querySelector('.sytue-page')?.textContent?.includes(t), textoA.trim(), { timeout: 15_000 })
  .then(() => true).catch(() => false);
nota(textoPersiste, 'escribir → recargar: el texto persiste');
const tituloInput = await page.inputValue('#editor-title-input').catch(() => '(sin input)');
const docTitle = await page.title();
nota(tituloInput === titulo, 'renombrar → recargar: el título persiste en el input', `input="${tituloInput}"`);
nota(docTitle.startsWith(titulo), 'document.title refleja el título', `"${docTitle}"`);

// --- 6. Export API (verdad del backend, antes de la UI) ---
for (const format of ['pdf', 'html']) {
  const res = await page.request.post(`${DOCS}/api/export?id=${id}`, { data: { format } });
  const body = (await res.text()).slice(0, 300);
  console.log(`· export ${format} → ${res.status()}: ${body}`);
  results.push({ ok: res.ok(), name: `API export ${format}`, detail: `${res.status()} ${body}` });
}

// --- 7. Comentario: abrir panel, crear, aparece ---
await page.click('button[aria-label="Comentarios"]');
await page.waitForSelector('textarea[aria-label="Nuevo comentario"]', { state: 'visible', timeout: 10_000 });
const cuerpo = `Comentario e2e ${TS}`;
await page.fill('textarea[aria-label="Nuevo comentario"]', cuerpo);
await page.click('button:has-text("Comentar")');
const comentarioAparece = await page
  .waitForSelector(`article:has-text("${cuerpo}")`, { state: 'visible', timeout: 10_000 })
  .then(() => true).catch(() => false);
nota(comentarioAparece, 'comentario creado aparece en el panel');

// --- 8. Presencia + adopción LWW con 2.º contexto ---
const storage = await ctxA.storageState();
const ctxB = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1280, height: 800 },
  storageState: storage,
});
const pageB = await ctxB.newPage();
await pageB.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await pageB.waitForSelector('.sytue-page', { state: 'visible', timeout: 60_000 });
await pageB.click('.sytue-page');
await pageB.keyboard.press('Control+End').catch(() => {});
const textoB = ` Texto desde B ${TS}.`;
await pageB.keyboard.type(textoB, { delay: 15 });
const bGuardado = await pageB
  .waitForFunction(
    () => /^Guardado/.test(document.querySelector('header [aria-live="polite"]')?.textContent?.trim() ?? ''),
    { timeout: 20_000 },
  )
  .then(() => true).catch(() => false);
nota(bGuardado, 'contexto B guarda sus cambios');

// En A: avatar de colaborador (polling 4 s) y adopción del contenido de B.
const avatarEnA = await page
  .waitForSelector('span[title="Colaborador conectado"]', { state: 'visible', timeout: 20_000 })
  .then(() => true).catch(() => false);
nota(avatarEnA, 'presencia: avatar de colaborador visible en A (2 contextos)');
const adopcionEnA = await page
  .waitForFunction((t) => document.querySelector('.sytue-page')?.textContent?.includes(t), textoB.trim(), { timeout: 25_000 })
  .then(() => true).catch(() => false);
nota(adopcionEnA, 'adopción LWW: A recibe el texto de B sin recargar');

// ¿Y al revés? B también debería ver el avatar de A si A escribe.
await page.click('.sytue-page');
await page.keyboard.press('Control+End').catch(() => {});
await page.keyboard.type(` Réplica de A ${TS}.`, { delay: 15 });
const avatarEnB = await pageB
  .waitForSelector('span[title="Colaborador conectado"]', { state: 'visible', timeout: 20_000 })
  .then(() => true).catch(() => false);
nota(avatarEnB, 'presencia recíproca: avatar visible en B');

// --- 9. Interacción con pestañas + capturas 1440 ---
await page.waitForFunction(
  () => /^Guardado/.test(document.querySelector('header [aria-live="polite"]')?.textContent?.trim() ?? ''),
  { timeout: 20_000 },
).catch(() => {});
await page.click('[role="tab"]:has-text("Colaboración")').catch(() => nota(false, 'pestaña Colaboración clicable'));
await page.waitForTimeout(400);
await page.screenshot({ path: 'shots/persist-colab-1440-main.png' });
await page.screenshot({ path: 'shots/persist-colab-1440-header.png', clip: { x: 0, y: 0, width: 1440, height: 160 } });

// Menú Descargar abierto (para crítica visual del menú de exportación).
await page.click('button:has-text("Descargar")');
await page.waitForTimeout(350);
await page.screenshot({ path: 'shots/persist-colab-1440-export.png' });

// --- 10. Export UI honesta: clic en PDF → ¿pestaña/descarga o toast? ---
const popupP = ctxA.waitForEvent('page', { timeout: 8_000 }).catch(() => null);
const downloadP = page.waitForEvent('download', { timeout: 8_000 }).catch(() => null);
await page.click('[role="menuitem"]:has-text("PDF")');
const [popup, download] = await Promise.all([popupP, downloadP]);
let exportUi = '';
if (download) exportUi = `descarga: ${download.suggestedFilename()}`;
else if (popup) {
  await popup.waitForLoadState('domcontentloaded', { timeout: 10_000 }).catch(() => {});
  exportUi = `pestaña nueva: ${popup.url().slice(0, 160)}`;
} else {
  const toast = await page
    .waitForSelector(':text("No se pudo exportar")', { timeout: 4_000 })
    .then(() => true).catch(() => false);
  exportUi = toast ? 'toast «No se pudo exportar» (menú honesto en fallo)' : 'NADA: ni descarga, ni pestaña, ni toast';
}
nota(!exportUi.startsWith('NADA'), 'exportar PDF: descarga o feedback honesto', exportUi);
await page.screenshot({ path: 'shots/persist-colab-1440-postexport.png' });
if (popup) await popup.close().catch(() => {});
await page.keyboard.press('Escape');

// --- 11. Capturas 1280x800 (misma escena) ---
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(500);
await page.screenshot({ path: 'shots/persist-colab-1280-main.png' });
await page.click('button:has-text("Descargar")');
await page.waitForTimeout(350);
await page.screenshot({ path: 'shots/persist-colab-1280-export.png' });
await page.keyboard.press('Escape');

// Vista del contexto B (1280) con presencia, para la crítica.
await pageB.screenshot({ path: 'shots/persist-colab-1280-contextoB.png' });

// --- 12. Salud ---
nota(opsFallidas.length === 0, 'sin POST /api/ops fallidos', opsFallidas.slice(0, 3).join(' | '));
nota(docsApiHits.length === 0, 'sin peticiones directas a docs-api', docsApiHits.slice(0, 3).join(' | '));
nota(consoleErrors.length === 0, 'sin errores de consola', consoleErrors.slice(0, 5).join(' | '));

await browser.close();
const ko = results.filter((r) => !r.ok);
console.log(`\nRESUMEN: ${results.length - ko.length}/${results.length} OK${ko.length ? ` — fallan: ${ko.map((r) => r.name).join(' · ')}` : ''}`);
