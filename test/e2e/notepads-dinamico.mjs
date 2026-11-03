// SPDX-License-Identifier: Apache-2.0
// Pantalla real: arrastre táctil/ratón, un deshacer, persistencia y captura fuera del scroll.
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { chromium } from 'playwright';
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { notepadsFixtureEndpoint, routeNotepadsFixture } from './lib/notepads-fixture.mjs';
import {
  assertFixtureTheme, describeWizardState, prepareFixtureTheme,
} from './lib/wizard-checks.mjs';
import { es } from '../../apps/frontend/apps/notes/src/i18n/messages/es.ts';

const URL = process.env.NOTEPADS_UX_URL;
if (!URL) throw new Error('NOTEPADS_UX_URL es obligatorio');
const fixtureOrigin = new globalThis.URL(URL).origin;
const OUT = createReportDirectory('notepads-ux', process.env.NOTEPADS_UX_OUT);
const browser = await chromium.launch();
let comprobaciones = 0;
const variantes = [];
let contextoActual = null;

async function hasta(condicion) {
  for (let i = 0; i < 400; i++) {
    if (condicion()) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error('La escritura no alcanzó el estado esperado');
}

async function gesto(page, source, target, touch, durante) {
  await source.waitFor({ state: 'visible' });
  await target.waitFor({ state: 'visible' });
  // CDP/mouse no tienen el auto-wait de click: el drawer puede ser visible
  // mientras aún desliza sus asas fuera del viewport. Esta acción espera Stable.
  await source.scrollIntoViewIfNeeded();
  await target.scrollIntoViewIfNeeded();
  const asa = await source.elementHandle();
  const destino = await target.elementHandle();
  assert(asa && destino, 'fuente y destino deben estar montados');
  let cdp = null;
  let apoyado = false;
  try {
    await page.waitForFunction((el) => !el.disabled, asa);
    const medir = (encuadrar = false) => asa.evaluate((el, { destino, encuadrar }) => {
      const scroller = el.closest('.overflow-auto, .overflow-y-auto');
      if (!scroller || !destino || !scroller.contains(destino))
        throw new Error('El asa y el destino deben compartir el desplazador del recorrido');
      const estado = () => {
        const a = el.getBoundingClientRect();
        const b = destino.getBoundingClientRect();
        const caja = scroller.getBoundingClientRect();
        const centro = { x: a.left + a.width / 2, y: a.top + a.height / 2 };
        const puntos = [
          centro,
          { x: a.left + Math.min(4, a.width / 2), y: a.top + Math.min(4, a.height / 2) },
          { x: a.right - Math.min(4, a.width / 2), y: a.top + Math.min(4, a.height / 2) },
          { x: a.left + Math.min(4, a.width / 2), y: a.bottom - Math.min(4, a.height / 2) },
          { x: a.right - Math.min(4, a.width / 2), y: a.bottom - Math.min(4, a.height / 2) },
        ];
        const source =
          puntos.find((p) => el.contains(document.elementFromPoint(p.x, p.y))) ?? centro;
        const target = { x: b.left + Math.min(b.width / 2, 80), y: b.top + 2 };
        const top = caja.top + scroller.clientTop;
        const left = caja.left + scroller.clientLeft;
        return {
          source,
          target,
          panel: {
            top, left,
            bottom: top + scroller.clientHeight,
            right: left + scroller.clientWidth,
          },
          scrollTop: scroller.scrollTop,
          maxScroll: Math.max(0, scroller.scrollHeight - scroller.clientHeight),
          sourceHit: el.contains(document.elementFromPoint(source.x, source.y)),
          targetHit: destino.contains(document.elementFromPoint(target.x, target.y)),
        };
      };
      let actual = estado();
      if (encuadrar) {
        // Solo ANTES de apoyar: encuadrar ambos puntos cuando caben. Si no, se
        // empieza por el asa y se alcanza el destino con el autoscroll real.
        const { source, target, panel } = actual;
        const minimo = Math.max(source.y - panel.bottom + 8, target.y - panel.bottom + 48);
        const maximo = Math.min(source.y - panel.top - 8, target.y - panel.top - 48);
        scroller.scrollTop +=
          minimo <= maximo
            ? Math.max(minimo, Math.min(0, maximo))
            : source.y - (panel.top + panel.bottom) / 2;
        // scrollTop se acota a 0/max: mandan las medidas realmente aplicadas.
        actual = estado();
        if (!actual.sourceHit) {
          scroller.scrollTop += actual.source.y - (actual.panel.top + actual.panel.bottom) / 2;
          actual = estado();
        }
      }
      return actual;
    }, { destino, encuadrar });
    let actual = await medir(true);
    assert(actual.sourceHit, `El asa debe recibir el puntero: ${JSON.stringify(actual)}`);
    let puntero = actual.source;
    cdp = touch ? await page.context().newCDPSession(page) : null;
    const mover = async ({ x, y }) => {
      if (cdp)
        await cdp.send('Input.dispatchTouchEvent', {
          type: 'touchMove', touchPoints: [{ id: 1, x, y }],
        });
      else await page.mouse.move(x, y);
      puntero = { x, y };
    };
    const sentido = ({ target, panel, scrollTop, maxScroll }) => {
      if (target.y < panel.top + 48 && scrollTop > 0.5) return -1;
      if (target.y > panel.bottom - 48 && scrollTop < maxScroll - 0.5) return 1;
      return 0;
    };
    const punto = (estado) => {
      const direccion = sentido(estado);
      return direccion
        ? {
            x: Math.max(estado.panel.left + 8, Math.min(estado.target.x, estado.panel.right - 8)),
            y: direccion < 0 ? estado.panel.top + 8 : estado.panel.bottom - 8,
          }
        : estado.target;
    };
    const alcanzar = async () => {
      const limite = Date.now() + 10_000;
      let sinAvance = 0;
      for (;;) {
        actual = await medir();
        if (!sentido(actual)) {
          // En 0/max no se exige un margen imposible: sí recorte e hit test.
          assert(
            actual.targetHit,
            `El destino debe recibir el puntero: ${JSON.stringify(actual)}`,
          );
          await mover(actual.target);
          return;
        }
        if (Date.now() >= limite || sinAvance >= 40)
          throw new Error(`El autoscroll no alcanza el destino: ${JSON.stringify(actual)}`);
        await mover(punto(actual));
        const anterior = actual.scrollTop;
        // El dedo sigue apoyado en el borde; sólo el RAF del componente desplaza.
        await page.waitForTimeout(50);
        actual = await medir();
        sinAvance = Math.abs(actual.scrollTop - anterior) < 0.5 ? sinAvance + 1 : 0;
      }
    };
    apoyado = true;
    if (cdp)
      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchStart', touchPoints: [{ id: 1, ...puntero }],
      });
    else {
      await page.mouse.move(puntero.x, puntero.y);
      await page.mouse.down();
    }
    const inicio = puntero;
    const fin = punto(actual);
    for (let i = 1; i <= 8; i++)
      await mover({
        x: inicio.x + ((fin.x - inicio.x) * i) / 8,
        y: inicio.y + ((fin.y - inicio.y) * i) / 8,
      });
    await alcanzar();
    await durante?.();
    await alcanzar();
    assert(
      await destino.evaluate(
        (el, { x, y }) => el.contains(document.elementFromPoint(x, y)),
        puntero,
      ),
      `El drop debe caer dentro del destino: ${JSON.stringify(actual)}`,
    );
    if (cdp) await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    else await page.mouse.up();
    apoyado = false;
  } catch (failure) {
    if (apoyado) {
      if (cdp)
        await cdp
          .send('Input.dispatchTouchEvent', { type: 'touchCancel', touchPoints: [] })
          .catch(() => {});
      else {
        await page.keyboard.press('Escape').catch(() => {});
        await page.mouse.up().catch(() => {});
      }
    }
    throw failure;
  } finally {
    if (cdp) await cdp.detach().catch(() => {});
    await asa.dispose().catch(() => {});
    await destino.dispose().catch(() => {});
  }
}

async function herramientasDelEditor(page, leerNotas) {
  const tarjeta = page.locator('article[data-note-id="nota-3"]');
  await tarjeta.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
  const editor = tarjeta.locator('[contenteditable="true"]');
  await editor.fill('Texto seleccionado y resto intacto.');
  const original = await editor.elementHandle();
  assert(original, 'se montó el editor');
  const sigueEditando = async () => {
    assert(
      await original.evaluate((el) => el.isConnected && el.isContentEditable),
      'las herramientas deben conservar el mismo editor, sin cerrarlo y remontarlo',
    );
    assert.equal(await editor.count(), 1);
  };
  const seleccionar = async () => {
    await sigueEditando();
    await editor.evaluate((el) => {
      el.focus();
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      let nodo;
      while ((nodo = walker.nextNode())) {
        const inicio = nodo.textContent.indexOf('seleccionado');
        if (inicio < 0) continue;
        const rango = document.createRange();
        rango.setStart(nodo, inicio);
        rango.setEnd(nodo, inicio + 'seleccionado'.length);
        const seleccion = window.getSelection();
        seleccion.removeAllRanges();
        seleccion.addRange(rango);
        document.dispatchEvent(new Event('selectionchange'));
        return;
      }
      throw new Error('No se encontró el fragmento que debe conservar su formato');
    });
  };
  const estilo = (fragmento, propiedad) =>
    editor.evaluate(
      (el, { fragmento, propiedad }) => {
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        let nodo;
        while ((nodo = walker.nextNode()))
          if (nodo.textContent.includes(fragmento))
            return getComputedStyle(nodo.parentElement)[propiedad];
        throw new Error(`No se encontró ${fragmento}`);
      },
      { fragmento, propiedad },
    );
  try {
    const barra = tarjeta.getByRole('toolbar').first();
    contextoActual.fase = 'editor-bloque';
    await seleccionar();
    await barra.getByRole('button', { name: es['editor.block'], exact: true }).click();
    const bloques = page.getByRole('menu', { name: es['editor.block'], exact: true });
    await bloques.waitFor();
    await sigueEditando();
    await bloques.getByRole('menuitem', { name: es['editor.h2'], exact: true }).click();
    await sigueEditando();
    assert.equal(await editor.locator('h2').textContent(), 'Texto seleccionado y resto intacto.');
    // El selector de estilo puede permanecer abierto; Escape cierra esa capa,
    // mientras que el Escape del editor, comprobado abajo, sí termina la edición.
    if (await bloques.isVisible()) {
      await bloques.getByRole('menuitem').first().focus();
      await page.keyboard.press('Escape');
    }
    await bloques.waitFor({ state: 'hidden' });
    await sigueEditando();

    contextoActual.fase = 'editor-teclado';
    await seleccionar();
    await barra.getByRole('button', { name: es['editor.italic'], exact: true }).press('Enter');
    await sigueEditando();
    assert.equal(await estilo('seleccionado', 'fontStyle'), 'italic');
    assert.notEqual(await estilo('Texto ', 'fontStyle'), 'italic');

    contextoActual.fase = 'editor-color';
    await seleccionar();
    await barra.getByRole('button', { name: es['editor.textColor'], exact: true }).click();
    const colores = page.getByRole('dialog', { name: es['editor.textColor'], exact: true });
    await colores.waitFor();
    await sigueEditando();
    await colores.getByRole('button', { name: '#b91c1c', exact: true }).click();
    await colores.waitFor({ state: 'hidden' });
    await sigueEditando();
    assert.equal(await estilo('seleccionado', 'color'), 'rgb(185, 28, 28)');
    assert.notEqual(await estilo('Texto ', 'color'), 'rgb(185, 28, 28)');

    contextoActual.fase = 'editor-enlace';
    await seleccionar();
    await barra.getByRole('button', { name: es['editor.linkTool'], exact: true }).click();
    const enlace = page.getByRole('dialog', { name: es['editor.linkTool'], exact: true });
    const url = 'https://sytue.example/editor';
    await enlace.getByRole('textbox', { name: es['editor.linkTool'], exact: true }).fill(url);
    await sigueEditando();
    await enlace
      .getByRole('button', {
        name: es['editor.linkToUrl'].replace('{url}', url),
        exact: true,
      })
      .click();
    await enlace.waitFor({ state: 'hidden' });
    await sigueEditando();
    assert.equal(await editor.locator('a').count(), 1);
    assert.equal(await editor.locator('a').textContent(), 'seleccionado');
    assert.equal(await editor.locator('a').getAttribute('href'), url);
    assert.equal(await editor.textContent(), 'Texto seleccionado y resto intacto.');

    contextoActual.fase = 'editor-barra-cuaderno';
    const cuaderno = page.locator('.notes-day-head');
    const filtro = cuaderno.getByRole('searchbox', { name: es['filter.placeholder'], exact: true });
    const abrirFiltro = async () => {
      if (!(await filtro.isVisible()))
        await cuaderno.getByRole('button', { name: es['filter.placeholder'], exact: true }).click();
      await filtro.focus();
    };
    await abrirFiltro();
    await sigueEditando();
    await cuaderno.getByRole('button', { name: es['export.label'], exact: true }).click();
    const exportar = page.getByRole('menu', { name: es['export.label'], exact: true });
    await exportar.waitFor();
    await sigueEditando();
    await page.keyboard.press('Escape');
    await exportar.waitFor({ state: 'hidden' });
    await sigueEditando();
    await cuaderno
      .getByRole('group', { name: es['filter.placeholder'], exact: true })
      .getByRole('button', { name: es['common.close'], exact: true })
      .click();
    await filtro.waitFor({ state: 'hidden' });
    await sigueEditando();
    await editor.focus();
    await editor.press('ControlOrMeta+End');
    await page.keyboard.type(' Escritura posterior.');
    await sigueEditando();
    assert((await editor.textContent()).includes(' y resto intacto. Escritura posterior.'));

    contextoActual.fase = 'editor-salir-a-otra-nota';
    const otra = page.locator('article[data-note-id="nota-4"]');
    await otra.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
    await editor.waitFor({ state: 'detached' });
    assert.equal(await editor.count(), 0, 'otra nota sí termina la sesión anterior');
    await hasta(() =>
      leerNotas()
        .find((n) => n.id === 'nota-3')
        ?.content.includes('Escritura posterior.'),
    );
    const guardado = leerNotas().find((n) => n.id === 'nota-3').content;
    assert(guardado.includes('<h2') && guardado.includes(url), 'se guarda el formato aplicado');
    contextoActual.fase = 'editor-escape';
    const segundo = otra.locator('[contenteditable="true"]');
    await segundo.fill('Salir con Escape también guarda.');
    await segundo.press('Escape');
    assert.equal(await segundo.count(), 0, 'Escape en el texto sí termina la edición');
    await hasta(() =>
      leerNotas()
        .find((n) => n.id === 'nota-4')
        ?.content.includes('Salir con Escape'),
    );
    contextoActual.fase = 'editor-filtro-otra-nota';
    await otra.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
    await segundo.fill('Salir desde el filtro a otra nota.');
    await abrirFiltro();
    assert.equal(await segundo.count(), 1, 'el filtro conserva la sesión');
    await tarjeta.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
    await segundo.waitFor({ state: 'detached' });
    assert.equal(
      await segundo.count(),
      0,
      'salir del filtro a otra nota cierra la sesión anterior',
    );
    await hasta(() =>
      leerNotas()
        .find((n) => n.id === 'nota-4')
        ?.content.includes('Salir desde el filtro'),
    );
    await editor.press('Escape');

    contextoActual.fase = 'editor-escape-desde-toolbar';
    await otra.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
    await segundo.fill('Escape desde la barra también guarda.');
    await abrirFiltro();
    assert.equal(await segundo.count(), 1, 'el foco del filtro no cierra el editor');
    await filtro.press('Escape');
    assert.equal(await segundo.count(), 0, 'Escape desde la barra sí termina la edición');
    await hasta(() =>
      leerNotas()
        .find((n) => n.id === 'nota-4')
        ?.content.includes('Escape desde la barra'),
    );
  } finally {
    await original.dispose();
  }
}

try {
  for (const [width, height] of [
    [1280, 800],
    [390, 844],
    [320, 640],
    [640, 320],
  ]) {
    for (const colorScheme of ['light', 'dark']) {
      const touch = width < 768;
      const context = await browser.newContext({
        viewport: { width, height },
        colorScheme,
        hasTouch: touch,
        isMobile: touch,
      });
      await prepareFixtureTheme(context, colorScheme);
      const page = await context.newPage();
      const id = `${width}x${height}-${colorScheme}`;
      const pageErrors = [];
      const responses = [];
      const onPageError = (error) => pageErrors.push(error.message);
      const onResponse = (response) => {
        const url = new globalThis.URL(response.url());
        if (url.origin !== fixtureOrigin) return;
        const type = response.request().resourceType();
        if (type === 'script' || type === 'document' || notepadsFixtureEndpoint(url.pathname))
          responses.push({
            path: url.pathname,
            type,
            status: response.status(),
            mime: response.headers()['content-type'] ?? '',
          });
      };
      page.on('pageerror', onPageError);
      page.on('response', onResponse);
      contextoActual = { page, id, fase: 'inicio', pageErrors, responses };
      const fecha = '2026-11-02T10:00:00Z';
      const nb = (id, title, position) => ({
        id,
        title,
        position,
        owner_sub: 'fixture-owner',
        color: '#146a47',
        layout: 'list',
        visibility: 'private',
        indexable: false,
        encrypted: false,
        note_count: id === 'cuaderno-proyecto' ? 25 : 0,
        created_at: fecha,
        updated_at: fecha,
      });
      let notebooks = [
        nb('cuaderno-proyecto', 'Proyecto de noviembre', 0),
        nb('cuaderno-ideas', 'Ideas por desarrollar', 1),
      ];
      let notes = Array.from({ length: 24 }, (_, i) => ({
        id: `nota-${i}`,
        notebook_id: notebooks[0].id,
        owner_sub: 'fixture-owner',
        kind: 'text',
        title: ['Preparar la reunión', 'Recoger las propuestas del equipo', 'Revisar el borrador'][
          i % 3
        ],
        content: '<p>Una idea por línea, con sitio para escribir y ordenar.</p>',
        checked: false,
        collapsed: false,
        pinned: false,
        x: 0,
        y: 0,
        col: 0,
        position: i,
        group_id: null,
        width: 320,
        states: [],
        links: [],
        created_at: fecha,
        updated_at: fecha,
      }));
      notes.push({
        ...notes[0],
        id: 'nota-hija',
        group_id: 'nota-0',
        title: 'Detalle de la reunión',
        position: 0,
      });
      let movimientos = 0,
        altas = 0;
      let falloIncierto = false;
      let retrasarTraslado = false;
      let liberarTraslado = null;
      await routeNotepadsFixture(page, URL, (route, endpoint) => {
        const req = route.request();
        const json = (body, status = 200) =>
          route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
        if (endpoint.service === 'auth' && endpoint.path === 'token')
          return json({ accessToken: 'fixture', expiresAt: Date.now() + 3_600_000 });
        if (endpoint.service !== 'notes')
          return json({ apps: [], organizations: [], notifications: [] });
        const p = endpoint.path;
        const method = req.method();
        if (p === 'notebooks')
          return json({ notebooks: [...notebooks].sort((a, b) => a.position - b.position) });
        const lista = /^notebooks\/([^/]+)\/notes$/.exec(p);
        if (lista && method === 'GET')
          return json({
            notes: notes
              .filter((n) => n.notebook_id === lista[1])
              .sort((a, b) => a.position - b.position),
          });
        if (lista && method === 'POST') {
          const input = req.postDataJSON();
          const nueva = {
            ...notes[0],
            ...input,
            id: `captura-${++altas}`,
            position: notes.length,
            notebook_id: lista[1],
          };
          notes.push(nueva);
          notebooks = notebooks.map((n) =>
            n.id === lista[1] ? { ...n, note_count: n.note_count + 1 } : n,
          );
          return json(nueva, 201);
        }
        const mover = /^notes\/([^/]+)\/move$/.exec(p);
        if (mover) {
          movimientos++;
          const input = req.postDataJSON();
          if (input.notebook_id && retrasarTraslado) {
            retrasarTraslado = false;
            return new Promise((resolve) => {
              liberarTraslado = async () => {
                await json({ error: 'conflict' }, 409);
                resolve();
              };
            });
          }
          notes = notes.map((n) => (n.id === mover[1] ? { ...n, ...input } : n));
          if (falloIncierto) {
            falloIncierto = false;
            return route.abort('failed');
          }
          return route.fulfill({ status: 204 });
        }
        const editar = /^notes\/([^/]+)$/.exec(p);
        if (editar && method === 'PUT') {
          const input = req.postDataJSON();
          notes = notes.map((n) => (n.id === editar[1] ? { ...n, ...input } : n));
          return json(notes.find((n) => n.id === editar[1]));
        }
        const book = /^notebooks\/([^/]+)$/.exec(p);
        if (book && method === 'PUT') {
          const input = req.postDataJSON();
          notebooks = notebooks.map((n) => (n.id === book[1] ? { ...n, ...input } : n));
          return json(notebooks.find((n) => n.id === book[1]));
        }
        return json({
          tags: [],
          reminders: [],
          hashtags: [],
          notes: [],
          tasks: [],
          days: [],
          events: [],
          connections: [],
          requests: [],
        });
      });
      await page.goto(URL);
      await page.locator('article[data-note-id="nota-0"]').waitFor();
      contextoActual.fase = 'tema';
      await assertFixtureTheme(page, colorScheme);
      assert.equal(await page.locator('html').getAttribute('data-app'), 'notes');
      contextoActual.fase = 'lista';
      const dock = page.locator('.notes-capture-dock');
      const input = dock.locator('input[name="capture"]');
      await input.waitFor({ state: 'visible' });
      assert.equal(
        await page.locator('input[name="capture"]').count(),
        1,
        'un único campo de captura',
      );
      const caja = await dock.boundingBox();
      assert(caja && caja.y + caja.height <= height + 1, 'captura visible dentro del viewport');
      assert.equal(
        await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1),
        false,
      );
      await page.screenshot({ path: join(OUT, `${id}-lista.png`) });
      await herramientasDelEditor(page, () => notes);
      if (height >= 600) {
        const source = () =>
          page.locator('article[data-note-id="nota-2"]').getByRole('button', {
            name: `${es['history.move']}: Revisar el borrador`,
            exact: true,
          });
        const target = () => page.locator('article[data-note-id="nota-0"]');
        contextoActual.fase = 'arrastre-notas';
        await gesto(page, source(), target(), touch, () =>
          page.screenshot({ path: join(OUT, `${id}-arrastre.png`) }),
        );
        await page.waitForFunction(
          () =>
            document.querySelector('article[data-note-id]')?.getAttribute('data-note-id') ===
            'nota-2',
        );
        contextoActual.fase = 'deshacer';
        await page
          .locator('.notes-day-head')
          .getByRole('button', { name: es['common.more'], exact: true })
          .click();
        const accionesCuaderno = page.getByRole('menu', {
          name: es['common.more'],
          exact: true,
        });
        await accionesCuaderno
          .getByRole('menuitem', {
            name: es['history.undoOf'].replace('{action}', es['history.move']),
            exact: true,
          })
          .click();
        // Deshacer debe retirar el menú y su velo antes del siguiente gesto.
        await accionesCuaderno.waitFor({ state: 'hidden' });
        await page.waitForFunction(
          () =>
            document.querySelector('article[data-note-id]')?.getAttribute('data-note-id') ===
            'nota-0',
        );
        await hasta(
          () =>
            notes.find((n) => n.id === 'nota-0')?.position === 0 &&
            notes.find((n) => n.id === 'nota-2')?.position === 2,
        );
        contextoActual.fase = 'reordenar-notas';
        await gesto(page, source(), target(), touch);
        await page.waitForFunction(
          () =>
            document.querySelector('article[data-note-id]')?.getAttribute('data-note-id') ===
            'nota-2',
        );
        await hasta(
          () =>
            notes.find((n) => n.id === 'nota-0')?.position === 1 &&
            notes.find((n) => n.id === 'nota-1')?.position === 2 &&
            notes.find((n) => n.id === 'nota-2')?.position === 0,
        );
        assert(movimientos > 0, 'el gesto debe persistir posiciones');
        contextoActual.fase = 'recargar-orden';
        await page.reload();
        await page.waitForFunction(
          () =>
            document.querySelector('article[data-note-id]')?.getAttribute('data-note-id') ===
            'nota-2',
        );
        // El servicio aplica el cambio, pero el cliente pierde la respuesta.
        // Debe compensar también esa escritura incierta, no solo las confirmadas.
        const posiciones = Object.fromEntries(notes.map((n) => [n.id, n.position]));
        const previas = movimientos;
        contextoActual.fase = 'compensar-respuesta-perdida';
        falloIncierto = true;
        await gesto(
          page,
          page
            .locator('article[data-note-id="nota-0"]')
            .getByRole('button', {
              name: `${es['history.move']}: Preparar la reunión`,
              exact: true,
            }),
          page.locator('article[data-note-id="nota-2"]'),
          touch,
        );
        await hasta(() => movimientos >= previas + 2);
        assert.deepEqual(Object.fromEntries(notes.map((n) => [n.id, n.position])), posiciones);
        await page.waitForFunction(
          () => !document.querySelector('article button[title]')?.disabled,
        );
        contextoActual.fase = 'reordenar-cuadernos';
        if (touch) await page.locator('button[id^="sytue-app-menu-"]').click();
        await Promise.all([
          page.waitForResponse(
            (r) => r.request().method() === 'PUT' && r.url().endsWith('/notebooks/cuaderno-ideas'),
          ),
          gesto(
            page,
            page
              .locator('[data-notebook-id="cuaderno-ideas"]')
              .getByRole('button', {
                name: `${es['notebook.move']}: Ideas por desarrollar`,
                exact: true,
              }),
            page.locator('[data-notebook-id="cuaderno-proyecto"]'),
            touch,
          ),
        ]);
        assert.equal(notebooks.find((n) => n.id === 'cuaderno-ideas')?.layout, 'list');
        assert.equal(notebooks.find((n) => n.id === 'cuaderno-ideas')?.visibility, 'private');
        await page.waitForFunction(
          () =>
            document.querySelector('[data-notebook-id]')?.getAttribute('data-notebook-id') ===
            'cuaderno-ideas',
        );
        if (touch) await page.keyboard.press('Escape');
      }
      contextoActual.fase = 'captura';
      await input.fill('Una nota capturada desde el pie');
      await input.press('Enter');
      await page.waitForFunction(
        () => document.querySelector('input[name="capture"]')?.value === '',
      );
      assert.equal(altas, 1, 'capturar debe crear exactamente una nota');
      if (height >= 600) {
        // Mientras un traslado de una familia espera un 409, otra edición y otra
        // captura se guardan. Reponer la familia no puede pisar ninguna de ellas.
        contextoActual.fase = 'mover-familia';
        retrasarTraslado = true;
        const padre = page.locator('article[data-note-id="nota-0"]');
        await padre.getByRole('button', { name: es['common.more'], exact: true }).click();
        await page.getByRole('menuitem', { name: es['note.moveTo'], exact: true }).click();
        await page
          .getByRole('dialog', { name: es['move.title'], exact: true })
          .getByRole('button', { name: 'Ideas por desarrollar', exact: true })
          .click();
        await hasta(() => liberarTraslado !== null);
        await padre.waitFor({ state: 'detached' });
        await page.locator('article[data-note-id="nota-hija"]').waitFor({ state: 'detached' });
        assert.equal(
          await page
            .locator('article[data-note-id="nota-0"], article[data-note-id="nota-hija"]')
            .count(),
          0,
          'la familia entera sale de la vista',
        );
        const otra = page.locator('article[data-note-id="nota-1"]');
        contextoActual.fase = 'edicion-concurrente';
        await otra.getByRole('button', { name: es['note.placeholder'], exact: true }).click();
        await otra.getByRole('textbox').fill('Edición concurrente');
        await otra.getByRole('textbox').press('Escape');
        await hasta(() =>
          notes.find((n) => n.id === 'nota-1')?.content.includes('Edición concurrente'),
        );
        contextoActual.fase = 'captura-concurrente';
        await input.fill('Captura concurrente');
        await input.press('Enter');
        await hasta(() => altas === 2);
        contextoActual.fase = 'reponer-familia';
        await liberarTraslado();
        await page.locator('article[data-note-id="nota-hija"]').waitFor();
        assert((await otra.textContent()).includes('Edición concurrente'));
        assert(
          (await page.locator('article[data-note-id="captura-2"]').textContent()).includes(
            'Captura concurrente',
          ),
        );
      }
      comprobaciones++;
      variantes.push({ id, theme: colorScheme, width, height });
      page.off('pageerror', onPageError);
      page.off('response', onResponse);
      await context.close();
      contextoActual = null;
    }
  }
  assert.equal(comprobaciones, 8);
  writeReport(
    join(OUT, 'results.json'),
    JSON.stringify({ passed: comprobaciones, variants: variantes }),
  );
} catch (error) {
  if (contextoActual) {
    const { page, id, fase, pageErrors, responses } = contextoActual;
    // Estado estructural del fixture sintético, sin URL completa, cuerpos,
    // cabeceras de autenticación, stack, HTML ni variables de entorno.
    const state = await describeWizardState({ page }).catch(() => null);
    if (state) state.url = new globalThis.URL(state.url).pathname;
    const sinURLCompleta = (message) => String(message).replace(/https?:\/\/[^\s"'<>]+/g, (url) => {
      try {
        return new globalThis.URL(url).pathname;
      } catch {
        return '[URL]';
      }
    });
    const failure = {
      id,
      fase,
      error: { name: error.name, message: sinURLCompleta(error.message) },
      pageErrors: pageErrors.map(sinURLCompleta),
      responses,
      state,
    };
    writeReport(
      join(OUT, 'results.json'),
      JSON.stringify({ passed: comprobaciones, variants: variantes, failure }, null, 2),
    );
    console.error(JSON.stringify({ failure }));
    await page
      .screenshot({ path: join(OUT, `${id}-${fase}-failure.png`), timeout: 5000 })
      .catch(() => {});
  }
  throw error;
} finally {
  await browser.close();
}
