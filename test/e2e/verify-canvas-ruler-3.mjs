// Pasada 3: fiabilidad del arrastre de sangría (3 intentos, con diagnóstico de
// eventos de puntero) + contador de palabras tras «Guardado».
import { chromium } from 'playwright';

const DOCS = 'https://docus.sytue.localhost';
const browser = await chromium.launch();
const context = await browser.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});
const page = await context.newPage();

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
      data: { kind: 'document', title: `verif-lienzo3-${Date.now()}` },
      timeout: 60_000,
    })
    .catch(() => null);
  if (created && created.status() === 201) ({ id } = await created.json());
  else await page.waitForTimeout(8_000);
}
let abierto = false;
for (let i = 1; i <= 3 && !abierto; i++) {
  await page
    .goto(`${DOCS}/edit?id=${id}`, { waitUntil: 'domcontentloaded', timeout: 60_000 })
    .catch(() => {});
  abierto = await page
    .waitForSelector('.sytue-page', { state: 'visible', timeout: 60_000 })
    .then(() => true)
    .catch(() => false);
}
if (!abierto) {
  console.error('KO editor');
  process.exit(1);
}
console.log('editor abierto');

// Tres párrafos normales
await page.click('.sytue-page');
await page.keyboard.type('Primer párrafo del experimento de arrastre.');
await page.keyboard.press('Enter');
await page.keyboard.type('Segundo párrafo del experimento de arrastre.');
await page.keyboard.press('Enter');
await page.keyboard.type('Tercer párrafo del experimento de arrastre.');

// Diagnóstico: contar eventos de puntero que llegan a los marcadores
await page.evaluate(() => {
  window.__evt = { down: 0, move: 0, up: 0 };
  document.addEventListener(
    'pointerdown',
    (e) => {
      if (e.target.closest?.('[role="slider"]')) window.__evt.down++;
    },
    true,
  );
  document.addEventListener(
    'pointermove',
    (e) => {
      if (e.target.closest?.('[role="slider"]')) window.__evt.move++;
    },
    true,
  );
  document.addEventListener(
    'pointerup',
    (e) => {
      if (e.target.closest?.('[role="slider"]')) window.__evt.up++;
    },
    true,
  );
});

const resultados = [];
for (let i = 0; i < 3; i++) {
  await page.locator('.sytue-page > p').nth(i).click();
  await page.waitForTimeout(200);
  const marker = page.locator('[role="slider"][aria-label="Sangría de primera línea"]');
  const mb = await marker.boundingBox();
  await page.mouse.move(mb.x + mb.width / 2, mb.y + mb.height / 2);
  await page.mouse.down();
  await page.mouse.move(mb.x + mb.width / 2 + 40, mb.y + mb.height / 2, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  const r = await page.evaluate((idx) => {
    const p = document.querySelectorAll('.sytue-page > p')[idx];
    return {
      indent: p.style.textIndent || '(vacío)',
      eventos: { ...window.__evt },
    };
  }, i);
  resultados.push(r);
  console.log(`intento ${i + 1}:`, JSON.stringify(r));
}

// Teclado sobre el marcador (AA): foco + flecha derecha
await page.locator('.sytue-page > p').nth(0).click();
await page.locator('[role="slider"][aria-label="Sangría derecha"]').focus();
await page.keyboard.press('ArrowLeft');
const kb = await page.evaluate(
  () => document.querySelectorAll('.sytue-page > p')[0].style.marginRight || '(vacío)',
);
console.log('teclado marginRight:', kb);

// Palabras tras Guardado
await page
  .waitForFunction(
    () =>
      Array.from(document.querySelectorAll('[aria-live="polite"]')).some((el) =>
        /^\s*Guardado(\s|$)/.test(el.textContent || ''),
      ),
    { timeout: 25_000 },
  )
  .catch(() => console.log('AVISO: sin chip Guardado'));
const words = await page.evaluate(
  () =>
    document
      .querySelector('.sytue-editor > .shrink-0:last-child')
      ?.textContent.match(/Palabras:\s*(\d+)/)?.[1],
);
console.log('palabras tras guardado:', words);
await browser.close();
console.log('LISTO3');
