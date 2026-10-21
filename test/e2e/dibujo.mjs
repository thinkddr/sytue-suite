// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Prueba en NAVEGADOR REAL (Chromium) del bloque «Dibujo» de Sytue Docus: lo que un
// entorno sin layout (happy-dom) no puede medir —el puntero, las asas, el SVG pintado y la
// imagen de respaldo rasterizada— y la accesibilidad del diálogo con axe.
//
// Mide el banco de pruebas `/dev/editor?kind=document` de apps/docus (solo en desarrollo),
// contestando a `/api/*` con `page.route`, como `accesibilidad-office.mjs`. Se lanza con
// `bash correr-accesibilidad.sh`, que levanta el servidor y pasa este script entre otros,
// o a mano: `ACC_URL=http://127.0.0.1:4338/docus/prueba bun run dibujo.mjs`.
//
// Qué comprueba (claro y oscuro):
//   1. La cinta inserta un dibujo: abre el diálogo con el rol, el nombre y el foco bien.
//   2. Puntero: dibujar arrastrando, mover arrastrando, redimensionar con un asa, marco de
//      selección, y que cada gesto es UN paso de deshacer.
//   3. Teclado: herramienta armada + Intro, flechas (con Mayús), «[» y «]», Supr.
//   4. Al insertar, el folio lleva `figure[data-dibujo]` con la escena, el SVG y una imagen
//      de respaldo PNG REAL (firma y tamaño correctos) que leerá el exportador del servicio.
//   5. Doble clic y Intro sobre el dibujo lo reabren con su escena.
//   6. axe-core (WCAG 2.x A y AA) sobre el diálogo: lo serio y lo crítico FALLAN.
//
// ponytail: script de verificación, no un spec de @playwright/test (que pide la pila dev
// entera con sesión). Un fallo es un fallo de la página, no del script.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { mkdirSync } from 'node:fs';

const BASE = process.env.ACC_URL || 'http://127.0.0.1:4338/docus/prueba';
const OUT = process.env.ACC_OUT || '/tmp/dibujo';
mkdirSync(OUT, { recursive: true });

const fallos = [];
const ok = (m) => console.log(`  ✓ ${m}`);
const falla = (m) => {
  fallos.push(m);
  console.log(`  ✗ ${m}`);
};
const afirma = (cond, bien, mal) => (cond ? ok(bien) : falla(mal ?? bien));

const json = (route, cuerpo, status = 200) =>
  route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(cuerpo),
  });

/** Contesta a la API del editor (BFF) con un documento de ejemplo. */
async function simularEditor(page) {
  let seq = 0;
  await page.route('**/auth/session', (r) =>
    json(r, {
      authenticated: true,
      user: {
        id: 'demo-user',
        name: 'Demo',
        email: 'demo@sytue.test',
        tenant: 'prueba',
      },
    }),
  );
  await page.route(
    /\/api\/(documents|state|ops|presence|collab|comments|versions|rename|people|sharing|star|edit-requests|llamada|blocks)(\?|$)/,
    async (route) => {
      const que = new URL(route.request().url()).pathname.split('/').pop();
      const metodo = route.request().method();
      if (que === 'documents')
        return json(route, {
          document: {
            id: 'demo',
            kind: 'document',
            title: 'Documento de ejemplo',
            owner_sub: 'demo-user',
          },
          role: 'owner',
        });
      if (que === 'state')
        return json(route, {
          seq,
          state: {
            seq,
            lamport: 1,
            elements: null,
            registers: {
              'doc:html': {
                value: '<p>Texto de ejemplo.</p>',
                lamport: 1,
                dot: { a: 'x', c: 1 },
              },
            },
          },
        });
      if (que === 'ops')
        return json(route, metodo === 'POST' ? { seq: ++seq, ops: [] } : { ops: [] });
      if (que === 'presence') return json(route, { members: [] });
      if (que === 'collab') return json(route, { error: 'sin_ws' }, 503);
      if (que === 'comments') return json(route, { comments: [] });
      if (que === 'versions') return json(route, { versions: [] });
      // Lo que la cabecera y la cinta piden por su cuenta al montarse. Sin contestarlo aquí
      // llega al servidor de desarrollo, que no tiene sesión y responde 401: la isla lo
      // tolera, pero el navegador lo deja como error en la consola.
      if (que === 'llamada') return json(route, { available: false });
      if (que === 'blocks')
        return json(route, { blocks: [{ key: 'firmas', name: 'Signature table' }], org_blocks: [] });
      return json(route, {});
    },
  );
}

const navegador = await chromium.launch();

async function sesion(esquema) {
  console.log(`\n=== Dibujo · esquema ${esquema} ===`);
  const contexto = await navegador.newContext({
    viewport: { width: 1280, height: 900 },
    hasTouch: true,
    colorScheme: esquema,
  });
  const page = await contexto.newPage();
  const consola = [];
  page.on('pageerror', (e) => consola.push(String(e)));
  page.on('console', (m) => m.type() === 'error' && consola.push(m.text()));
  page.on('response', (response) => {
    if (response.status() === 504)
      console.log(`  recurso 504: ${response.url()}`);
  });
  await simularEditor(page);
  await page.goto(`${BASE}/dev/editor?kind=document&lang=en`, {
    timeout: 240000,
  });
  await page.waitForSelector('[role="tablist"]', { timeout: 90000 });
  await page.waitForSelector('.sytue-page', { timeout: 30000 });
  await page.waitForTimeout(500);

  /* ----- 1. Insertar desde la cinta ----- */
  await page.locator('.sytue-page').click();
  await page.locator('#ribbon-tab-insertar').click();
  const boton = page.getByRole('button', { name: 'Insert or edit a drawing' });
  await boton.waitFor({ timeout: 10000 });
  await boton.click();
  const dialogo = page.getByRole('dialog', { name: 'Edit drawing' });
  await dialogo.waitFor({ timeout: 10000 });
  ok('la cinta abre el diálogo «Edit drawing»');
  const lienzo = dialogo.getByRole('application', { name: 'Drawing canvas' });
  const objetos = dialogo.locator('section[aria-labelledby="dib-objetos"]');
  const nObjetos = () => objetos.locator('li').count();

  /* ----- 2. Puntero ----- */
  // 640 × 400 unidades de escena mapeadas al ancho del lienzo. Se mide en cada gesto: el
  // panel lateral cambia de alto y con él puede aparecer una barra de desplazamiento.
  const mapa = async () => {
    const c = await lienzo.boundingBox();
    const k = c.width / 640;
    return { k, px: (x, y) => [c.x + x * k, c.y + y * k] };
  };

  await dialogo.getByRole('button', { name: 'Rectangle', exact: true }).click();
  let { k, px } = await mapa();
  const [x1, y1] = px(100, 80);
  const [x2, y2] = px(260, 170);
  await page.mouse.move(x1, y1);
  await page.mouse.down();
  await page.mouse.move((x1 + x2) / 2, (y1 + y2) / 2, { steps: 4 });
  await page.mouse.move(x2, y2, { steps: 4 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  afirma((await nObjetos()) === 1, 'arrastrar con la herramienta armada crea un rectángulo');
  const izq = dialogo.getByLabel('Left', { exact: true });
  const ancho = dialogo.getByLabel('Width', { exact: true });
  afirma(
    Math.abs(Number(await izq.inputValue()) - 100) <= 2,
    'nace donde se empezó a arrastrar',
    `x=${await izq.inputValue()}`,
  );
  afirma(
    Math.abs(Number(await ancho.inputValue()) - 160) <= 2,
    'con el ancho arrastrado',
    `w=${await ancho.inputValue()}`,
  );
  afirma(
    (await dialogo
      .getByRole('button', { name: 'Select', exact: true })
      .getAttribute('aria-pressed')) === 'true',
    'la herramienta vuelve a Seleccionar',
  );

  // Mover arrastrando el objeto.
  ({ k, px } = await mapa());
  const [mx, my] = px(180, 125);
  await page.mouse.move(mx, my);
  await page.mouse.down();
  await page.mouse.move(mx + 40 * k, my + 30 * k, { steps: 6 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  afirma(
    Math.abs(Number(await izq.inputValue()) - 140) <= 2,
    'arrastrar un objeto lo mueve',
    `x=${await izq.inputValue()}`,
  );

  // Un gesto = un paso: un solo «Deshacer» devuelve el objeto a donde estaba.
  await dialogo.getByRole('button', { name: 'Undo' }).click();
  await page.waitForTimeout(100);
  afirma(
    Math.abs(Number(await izq.inputValue()) - 100) <= 2,
    'deshacer revierte el movimiento ENTERO (un gesto, un paso)',
    `x=${await izq.inputValue()}`,
  );
  await dialogo.getByRole('button', { name: 'Redo' }).click();
  await page.waitForTimeout(100);

  // Redimensionar con el asa sureste.
  const asa = page.locator('[data-asa="se"]');
  const ab = await asa.boundingBox();
  await page.mouse.move(ab.x + ab.width / 2, ab.y + ab.height / 2);
  await page.mouse.down();
  await page.mouse.move(ab.x + ab.width / 2 + 40 * k, ab.y + ab.height / 2 + 20 * k, { steps: 5 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  afirma(
    Math.abs(Number(await ancho.inputValue()) - 200) <= 2,
    'el asa redimensiona',
    `w=${await ancho.inputValue()}`,
  );

  // Marco de selección: rodea el rectángulo y una elipse nueva.
  await dialogo.getByRole('button', { name: 'Ellipse', exact: true }).click();
  ({ k, px } = await mapa());
  const [ex1, ey1] = px(400, 200);
  const [ex2, ey2] = px(520, 280);
  await page.mouse.move(ex1, ey1);
  await page.mouse.down();
  await page.mouse.move(ex2, ey2, { steps: 5 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  afirma((await nObjetos()) === 2, 'segunda forma creada');
  ({ k, px } = await mapa());
  const [vx, vy] = px(20, 20);
  const [wx, wy] = px(630, 390);
  await page.mouse.move(vx, vy);
  await page.mouse.down();
  await page.mouse.move(wx, wy, { steps: 6 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  const seleccionados = await objetos.locator('button[aria-pressed="true"]').count();
  afirma(seleccionados === 2, 'el marco selecciona lo que toca', `seleccionados=${seleccionados}`);
  afirma(
    await dialogo.getByRole('button', { name: 'Connect the two selected objects' }).isEnabled(),
    'con dos seleccionados se puede conectar',
  );
  await dialogo.getByRole('button', { name: 'Connect the two selected objects' }).click();
  await page.waitForTimeout(150);
  afirma((await nObjetos()) === 3, 'el conector se añade como un objeto más');

  /* ----- 2b. Mano alzada: ratón, lápiz (CDP) y dedo (CDP) ----- */
  await dialogo.getByRole('button', { name: 'Pencil', exact: true }).click();
  const trazos = () => objetos.locator('button', { hasText: 'Freehand stroke' }).count();
  ({ k, px } = await mapa());
  const dibuja = async (pts) => {
    await page.mouse.move(...px(...pts[0]));
    await page.mouse.down();
    for (const q of pts.slice(1)) await page.mouse.move(...px(...q), { steps: 3 });
    await page.mouse.up();
    await page.waitForTimeout(120);
  };
  await dibuja([
    [30, 300],
    [80, 340],
    [140, 310],
    [200, 350],
  ]);
  afirma((await trazos()) === 1, 'el lápiz con ratón crea un trazo y sigue armado');
  const cdp = await page.context().newCDPSession(page);
  const [lx, ly] = px(260, 300);
  const evento = (type, x, y, extra = {}) =>
    cdp.send('Input.dispatchMouseEvent', {
      type,
      x,
      y,
      button: 'left',
      buttons: 1,
      pointerType: 'pen',
      ...extra,
    });
  await evento('mousePressed', lx, ly, { clickCount: 1 });
  for (let i = 1; i <= 6; i++) await evento('mouseMoved', lx + i * 12 * k, ly + (i % 2) * 20 * k);
  await evento('mouseReleased', lx + 72 * k, ly, { buttons: 0 });
  await page.waitForTimeout(150);
  afirma((await trazos()) === 2, 'el lápiz (pointerType pen) crea un trazo');
  const [tx, ty] = px(380, 300);
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: tx, y: ty }],
  });
  for (let i = 1; i <= 6; i++)
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: tx + i * 10 * k, y: ty + (i % 2) * 25 * k }],
    });
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await page.waitForTimeout(150);
  afirma((await trazos()) === 3, 'el dedo (táctil) crea un trazo');
  // La goma borra el trazo entero que toca.
  await dialogo.getByRole('button', { name: 'Eraser', exact: true }).click();
  ({ k, px } = await mapa());
  await page.mouse.move(...px(80, 340));
  await page.mouse.down();
  await page.mouse.move(...px(82, 338), { steps: 2 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  afirma((await trazos()) === 2, 'la goma borra el trazo entero que toca');
  await dialogo.getByRole('button', { name: 'Select', exact: true }).click();

  /* ----- 3. Teclado ----- */
  await lienzo.focus();
  await page.keyboard.press('Escape'); // quita la selección
  await dialogo.getByRole('button', { name: 'Star', exact: true }).click();
  await lienzo.focus();
  await page.keyboard.press('Enter');
  await page.waitForTimeout(150);
  const avisoLive = dialogo.locator('[role="status"]');
  afirma(
    (await avisoLive.textContent()) === 'Star 1 added',
    'Intro con la herramienta armada coloca el objeto y lo anuncia',
  );
  const antes = Number(await izq.inputValue());
  await page.keyboard.press('Shift+ArrowRight');
  await page.waitForTimeout(100);
  afirma(Number(await izq.inputValue()) === antes + 10, 'Mayús+flecha mueve 10 unidades');
  await page.keyboard.press('[');
  await page.waitForTimeout(100);
  afirma(
    /selected$/.test((await avisoLive.textContent()) ?? ''),
    '«[» recorre los objetos y lo anuncia',
  );
  await page.keyboard.press('Delete');
  await page.waitForTimeout(100);
  afirma(/deleted$/.test((await avisoLive.textContent()) ?? ''), 'Supr borra y lo anuncia');

  /* ----- 6. axe del diálogo ----- */
  const axe = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  const graves = axe.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
  for (const v of graves)
    falla(
      `axe ${v.id} (${v.impact}): ${v.help} · ${v.nodes.length} nodo(s): ${v.nodes[0]?.html.slice(0, 140)}`,
    );
  if (graves.length === 0) ok(`axe: sin violaciones graves (${axe.violations.length} menores)`);
  await page.screenshot({ path: `${OUT}/dibujo-editor-${esquema}.png` });

  /* ----- 4a. Descargas: SVG y PDF vectorial ----- */
  for (const [boton, firma] of [
    ['Download SVG', '<svg'],
    ['Download PDF', '%PDF-1.4'],
  ]) {
    const [descarga] = await Promise.all([
      page.waitForEvent('download'),
      dialogo.getByRole('button', { name: boton }).click(),
    ]);
    const ruta = await descarga.path();
    const { readFileSync } = await import('node:fs');
    const cuerpo = readFileSync(ruta).toString('latin1');
    afirma(
      cuerpo.startsWith(firma) && cuerpo.length > 800,
      `${boton}: fichero ${descarga.suggestedFilename()} válido`,
      `${boton}: ${cuerpo.slice(0, 30)}`,
    );
  }

  /* ----- 4. Insertar y comprobar el folio ----- */
  await dialogo.getByRole('button', { name: 'Insert', exact: true }).click();
  await dialogo.waitFor({ state: 'detached', timeout: 10000 });
  const fig = page.locator('.sytue-page figure[data-dibujo]');
  await fig.waitFor({ timeout: 10000 });
  ok('el folio recibe figure[data-dibujo]');
  // La imagen de respaldo se rasteriza antes de insertar: esperar a que esté.
  const info = await page.evaluate(async () => {
    const f = document.querySelector('.sytue-page figure[data-dibujo]');
    const img = f?.querySelector('img[data-dibujo-respaldo]');
    return {
      escena: f?.getAttribute('data-escena') ?? '',
      svg: !!f?.querySelector('svg'),
      rect: f?.querySelectorAll('svg rect').length ?? 0,
      src: img?.getAttribute('src') ?? '',
      oculto: img?.hasAttribute('hidden') ?? false,
      tabindex: f?.getAttribute('tabindex'),
      editable: f?.getAttribute('contenteditable'),
    };
  });
  afirma(
    info.svg && info.rect > 0,
    'lleva el SVG pintado',
    JSON.stringify({ svg: info.svg, rect: info.rect }),
  );
  afirma(info.escena.startsWith('{"v":1,'), 'lleva la escena serializada y versionada');
  afirma(info.editable === 'false' && info.tabindex === '0', 'es un bloque atómico y enfocable');
  const png = Buffer.from(info.src.replace(/^data:image\/png;base64,/, ''), 'base64');
  const firmaPng = png.subarray(0, 8).toString('hex') === '89504e470d0a1a0a';
  afirma(firmaPng, 'la imagen de respaldo es un PNG real', `src=${info.src.slice(0, 40)}`);
  if (firmaPng) {
    const w = png.readUInt32BE(16);
    const h = png.readUInt32BE(20);
    afirma(w === 1280 && h === 800, `con la resolución 2× de la escena (${w}×${h})`, `${w}×${h}`);
    // Pintado de verdad, no una imagen en blanco: el IDAT comprimido de un lienzo vacío es diminuto.
    afirma(
      png.length > 1500,
      `con contenido (${png.length} bytes)`,
      `solo ${png.length} bytes: ¿en blanco?`,
    );
  }
  afirma(info.oculto, 'el respaldo va oculto en el editor');

  /* ----- 5. Reabrir: doble clic y teclado ----- */
  await fig.dblclick();
  await dialogo.waitFor({ timeout: 10000 });
  afirma((await nObjetos()) >= 2, 'el doble clic reabre el dibujo con sus objetos');
  await dialogo.getByRole('button', { name: 'Cancel' }).click();
  await dialogo.waitFor({ state: 'detached', timeout: 10000 });
  await fig.focus();
  await page.keyboard.press('Enter');
  await dialogo.waitFor({ timeout: 10000 });
  ok('Intro sobre el dibujo enfocado lo reabre');
  // Sin cambios, cancelar no pregunta.
  await page.keyboard.press('Escape');
  await dialogo.waitFor({ state: 'detached', timeout: 10000 });
  ok('Escape sin cambios cierra sin preguntar');

  // Los 503 son del simulador (`/api/collab` contesta «sin_ws» a propósito), no de la página.
  const reales = consola.filter((m) => !/status of 503/.test(m));
  if (reales.length) falla(`errores de consola: ${reales.slice(0, 3).join(' | ')}`);
  else ok('sin errores de consola');
  await contexto.close();
}

for (const esquema of ['light', 'dark']) await sesion(esquema);
await navegador.close();

console.log(fallos.length ? `\n${fallos.length} fallo(s)` : '\nDibujo: todo en orden');
process.exit(fallos.length ? 1 : 0);
