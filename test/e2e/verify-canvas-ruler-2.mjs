// Verificador lienzo+regla — pasada 2: TDC visible, multipágina (líneas de
// corte, Página X de Y), regla al hacer scroll, h-scroll real a 200 %,
// apaisado, margen 48 y experimento arrastre+Ctrl+Z.
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const DOCS = 'https://docus.sytue.localhost';
const OUT = 'shots/verify-canvas';
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 1,
});
const page = await context.newPage();
const metrics = {};

// --- login ---
let logged = false;
for (let intento = 1; intento <= 6 && !logged; intento++) {
  if (intento > 1) await page.waitForTimeout(13_000);
  await page.goto(`${DOCS}/`, { waitUntil: 'domcontentloaded', timeout: 45_000 }).catch(() => {});
  if (new URL(page.url()).hostname === 'docus.sytue.localhost') {
    logged = true;
    break;
  }
  await page
    .waitForURL((u) => u.hostname === 'id.sytue.localhost', { timeout: 20_000 })
    .catch(() => {});
  if (new URL(page.url()).hostname !== 'id.sytue.localhost') continue;
  try {
    await page.fill('input[name=email]', 'admin@sytue.localhost', { timeout: 20_000 });
    await page.fill('input[name=password]', 'sytue-dev-admin', { timeout: 10_000 });
    await page.click('button[type=submit]', { noWaitAfter: true, timeout: 10_000 });
  } catch {
    continue;
  }
  logged = await page
    .waitForURL((u) => u.hostname === 'docus.sytue.localhost', {
      timeout: 90_000,
      waitUntil: 'domcontentloaded',
    })
    .then(() => true)
    .catch(() => false);
}
if (!logged) {
  console.error('KO login');
  process.exit(1);
}
let id = null;
for (let i = 1; i <= 4 && !id; i++) {
  const created = await page.request
    .post(`${DOCS}/api/create`, {
      data: { kind: 'document', title: `verif-lienzo2-${Date.now()}` },
      timeout: 60_000,
    })
    .catch(() => null);
  if (created && created.status() === 201) ({ id } = await created.json());
  else await page.waitForTimeout(8_000);
}
if (!id) {
  console.error('KO create');
  process.exit(1);
}
await page.goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('.sytue-page', { state: 'visible', timeout: 90_000 });
console.log('1. Editor abierto:', id);

const tab = (name) => page.click(`[role="tab"]:has-text("${name}")`);
const scrollTo = (top, left = 0) =>
  page.evaluate(
    ([t, l]) => {
      const s = document.querySelector('.sytue-page').closest('.overflow-auto');
      s.scrollTop = t;
      s.scrollLeft = l;
    },
    [top, left],
  );
const shoot = async (name, clipEl) => {
  const box = await page.evaluate((sel) => {
    const el = sel
      ? document.querySelector(sel)
      : document.querySelector('.sytue-page').closest('.overflow-auto');
    const r = (sel ? el : el).getBoundingClientRect();
    return {
      x: Math.max(0, r.x - 20),
      y: Math.max(0, r.y - 8),
      width: Math.min(1440 - Math.max(0, r.x - 20), r.width + 40),
      height: sel ? 150 : r.height,
    };
  }, clipEl || null);
  await page.screenshot({ path: `${OUT}/${name}.png`, clip: box });
};

// --- 2. Contenido con títulos + TDC al principio ---
await page.click('.sytue-page');
await page.keyboard.type('Manual del lienzo');
await page.click('button[aria-label="Estilo Título 1"]');
await page.keyboard.press('End');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Título 2"]');
await page.keyboard.type('Primera sección');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Normal"]');
await page.keyboard.type('Texto de la primera sección para dar cuerpo al documento.');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Título 2"]');
await page.keyboard.type('Segunda sección');
await page.keyboard.press('Enter');
await page.click('button[aria-label="Estilo Normal"]');
await page.keyboard.type('Texto de la segunda sección.');
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
await scrollTo(0);
await page.waitForTimeout(300);
metrics.toc = await page.evaluate(() => {
  const toc = document.querySelector('.sytue-page nav[data-toc]');
  return {
    border: getComputedStyle(toc).border,
    links: Array.from(toc.querySelectorAll('a')).map((a) => ({
      text: a.textContent,
      href: a.getAttribute('href'),
      level: a.closest('[data-level]')?.getAttribute('data-level'),
    })),
  };
});
await shoot('p2-1440-toc-top');
await shoot('p2-1440-ruler-close', '[aria-label="Regla horizontal"]');
console.log('2. TDC:', JSON.stringify(metrics.toc));

// --- 3. Multipágina: párrafos hasta pasar de página (por el camino de input real) ---
await page.evaluate(() => {
  const root = document.querySelector('.sytue-page');
  const frag = Array.from(
    { length: 55 },
    (_, i) =>
      `<p>Párrafo de relleno ${i + 1} para forzar el salto de página: el folio continuo crece y debe pintar la línea de corte cada 1123 píxeles de maqueta, actualizando «Página X de Y» en la barra de estado.</p>`,
  ).join('');
  root.insertAdjacentHTML('beforeend', frag);
  root.dispatchEvent(new Event('input', { bubbles: true }));
});
await page.waitForTimeout(900); // throttle de medición 500 ms
metrics.multipage = await page.evaluate(() => {
  const pageEl = document.querySelector('.sytue-page');
  const status = document.querySelector('.sytue-editor > .shrink-0:last-child');
  return {
    pageOffsetH: pageEl.offsetHeight,
    cutLines: (getComputedStyle(pageEl).backgroundImage || '').includes('repeating-linear'),
    statusText: status.textContent.trim().replace(/\s+/g, ' ').slice(0, 60),
  };
});
console.log('3. Multipágina:', JSON.stringify(metrics.multipage));
// captura de la línea de corte (frontera página 1→2: y de maqueta 1123 + gap 32 + regla 20)
await scrollTo(1000);
await page.waitForTimeout(600);
metrics.statusAfterScroll = await page.evaluate(() => {
  const status = document.querySelector('.sytue-editor > .shrink-0:last-child');
  const ruler = document.querySelector('[aria-label="Regla horizontal"]');
  const rr = ruler.getBoundingClientRect();
  return {
    statusText: status.textContent.trim().replace(/\s+/g, ' ').slice(0, 40),
    rulerVisibleInViewport: rr.bottom > 0 && rr.top < innerHeight,
  };
});
await shoot('p2-1440-corte-pagina');
console.log('4. Tras scroll:', JSON.stringify(metrics.statusAfterScroll));

// --- 4. Zoom 200 en 1280: scroll horizontal real y alineación de la regla ---
await page.setViewportSize({ width: 1280, height: 800 });
await tab('Vista');
await page.selectOption('select[aria-label="Zoom"]', '200');
await page.waitForTimeout(500);
await scrollTo(0, 0);
await page.waitForTimeout(300);
metrics.z200 = await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  const pr = document.querySelector('.sytue-page').getBoundingClientRect();
  const rr = document.querySelector('[aria-label="Regla horizontal"]').getBoundingClientRect();
  return {
    clientW: s.clientWidth,
    scrollW: s.scrollWidth,
    hScroll: s.scrollWidth > s.clientWidth + 1,
    scrollLeft: s.scrollLeft,
    pageLeftVisible: pr.left >= s.getBoundingClientRect().left - 1,
    rulerLeftDelta: +(rr.left - pr.left).toFixed(1),
    rulerWidthDelta: +(rr.width - pr.width).toFixed(1),
  };
});
await shoot('p2-1280-z200-izq');
await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  s.scrollLeft = s.scrollWidth;
});
await page.waitForTimeout(300);
metrics.z200right = await page.evaluate(() => {
  const s = document.querySelector('.sytue-page').closest('.overflow-auto');
  const pr = document.querySelector('.sytue-page').getBoundingClientRect();
  const sr = s.getBoundingClientRect();
  return {
    scrollLeft: s.scrollLeft,
    pageRightVisible: pr.right <= sr.right + 1,
  };
});
await shoot('p2-1280-z200-dcha');
console.log('5. Zoom 200:', JSON.stringify(metrics.z200), JSON.stringify(metrics.z200right));

// --- 5. Apaisado + margen Estrecho (regla en 29,7 cm y sombreado 48) ---
await page.setViewportSize({ width: 1440, height: 900 });
await page.selectOption('select[aria-label="Zoom"]', '100');
await tab('Disposición');
await page.click('button[aria-label="Orientación horizontal"]');
await page.waitForTimeout(400);
await scrollTo(0);
metrics.landscape = await page.evaluate(() => {
  const pr = document.querySelector('.sytue-page').getBoundingClientRect();
  const ruler = document.querySelector('[aria-label="Regla horizontal"]');
  const labels = Array.from(ruler.querySelectorAll('svg text')).map((t) => t.textContent);
  return { pageW: +pr.width.toFixed(1), lastLabel: labels[labels.length - 1], nLabels: labels.length };
});
await shoot('p2-1440-apaisado');
await page.selectOption('select[aria-label="Márgenes de página"]', '48');
await page.waitForTimeout(300);
await shoot('p2-1440-apaisado-margen48', '[aria-label="Regla horizontal"]');
await page.click('button[aria-label="Orientación vertical"]');
await page.selectOption('select[aria-label="Márgenes de página"]', '96');
await page.waitForTimeout(300);
console.log('6. Apaisado:', JSON.stringify(metrics.landscape));

// --- 6. Experimento: escribir → arrastrar sangría → Ctrl+Z ---
await scrollTo(0);
await page.locator('.sytue-page p').first().click();
await page.keyboard.press('End');
await page.keyboard.type(' MARCADOR-UNDO.');
await page.waitForTimeout(300);
const marker = page.locator('[role="slider"][aria-label="Sangría de primera línea"]');
const mb = await marker.boundingBox();
await page.mouse.move(mb.x + mb.width / 2, mb.y + mb.height / 2);
await page.mouse.down();
await page.mouse.move(mb.x + mb.width / 2 + 38, mb.y + mb.height / 2, { steps: 6 });
await page.mouse.up();
const before = await page.evaluate(() => ({
  indent: document.querySelector('.sytue-page p')?.style.textIndent || '',
  hasMarker: document.querySelector('.sytue-page').textContent.includes('MARCADOR-UNDO'),
}));
await page.keyboard.press('ControlOrMeta+z');
await page.waitForTimeout(300);
const after = await page.evaluate(() => ({
  indent: document.querySelector('.sytue-page p')?.style.textIndent || '',
  hasMarker: document.querySelector('.sytue-page').textContent.includes('MARCADOR-UNDO'),
}));
metrics.undoExperiment = { before, after };
console.log('7. Arrastre+Ctrl+Z:', JSON.stringify(metrics.undoExperiment));

console.log('METRICS2 ' + JSON.stringify(metrics));
await browser.close();
console.log('LISTO');
