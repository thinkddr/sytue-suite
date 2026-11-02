// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Auditoría de ACCESIBILIDAD de la rejilla de Sytue Bases (web): axe-core, teclado, zoom
// y tamaño táctil. Mide el banco de pruebas `/bases/dev/rejilla` (solo en desarrollo, 404
// en producción), que monta la rejilla entera con datos de ejemplo y sin servicio detrás.
//
// Uso (desde el contenedor del frontal):
//   bash correr-accesibilidad-bases.sh      (levanta el servidor dev y lanza esto)
// o a mano:
//   cd apps/frontend/apps/bases && bun --bun astro dev --port 4337 &
//   cd test/e2e && ACC_URL=http://127.0.0.1:4337/bases bun run accesibilidad-bases.mjs
// Salida: consola, results.json y capturas en ACC_OUT (o un directorio temporal
// privado). Código de salida 1 si algo falla.
//
// Qué comprueba, a 1280, 390 y 320 px y en claro y oscuro:
//   1. axe-core (WCAG 2.0/2.1/2.2 A y AA): `serious` y `critical` FALLAN.
//   2. Teclado: la rejilla es UNA parada de Tab; flechas, Inicio/Fin, Ctrl+Inicio/Fin
//      mueven el foco; Intro abre el editor y escribir lo abre con la tecla; Intro
//      guarda y baja; Escape cancela y devuelve el foco; Espacio marca una casilla;
//      el foco es visible.
//   3. Solo lectura: no hay editor ni botones de borrar.
//   4. Zoom al 200 %: la página no desborda de lado (la rejilla sí se desplaza dentro de
//      su contenedor).
//   5. Objetivos táctiles a 390 y 320 px: cada control mide al menos 44 × 44 px (WCAG 2.5.5,
//      el listón de `accesibilidad-office.mjs`); una celda de la rejilla, 24 × 24 (2.5.8: es
//      contenido, como la celda de una hoja).
//   8. PANTALLAS ENTERAS (`/bases/dev/app`: las islas reales con el marco, y la API
//      contestada aquí con `page.route`): lista de bases, una base abierta, su tablero y el
//      diálogo de compartir. axe, lector, táctil, Tab con foco visible y zoom al 200 %.
//      SUMAR UNA PANTALLA NUEVA ES UNA LÍNEA en `PANTALLAS` (y, si pide datos nuevos, su
//      respuesta en `simularApi`).
//   7. Tablero Kanban y formulario público (`/bases/dev/vistas`): axe, táctil, mover una
//      tarjeta sin arrastrar y etiquetas y avisos del formulario.
//   6. Lector de pantalla: la rejilla es un `grid` con nombre, cabeceras de columna y de fila,
//      y hay una región `aria-live` que anuncia lo guardado.
//
// ponytail: script de verificación, no forma parte de la suite de @playwright/test (esa pide
// la pila dev entera con sesión).
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { createServer, request as requestHttp } from 'node:http';
import { join } from 'node:path';
import { es } from '../../apps/frontend/apps/bases/src/i18n/messages/es.ts';
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { prepareFixtureTheme, assertFixtureTheme } from './lib/wizard-checks.mjs';

const ASTRO = new URL(process.env.ACC_URL || 'http://127.0.0.1:4337/bases');
let BASE = ASTRO.href.replace(/\/$/, '');
const OUT = createReportDirectory('accesibilidad-bases', process.env.ACC_OUT);
const IDIOMA = process.env.ACC_LANG || 'es';

const ANCHOS = [
  { w: 1280, h: 800, movil: false },
  { w: 390, h: 844, movil: true },
  { w: 320, h: 640, movil: true },
];
const ESQUEMAS = ['light', 'dark'];
const ETIQUETAS_AXE = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];
const GRAVES = new Set(['serious', 'critical']);
const LADO_TACTIL = 44;
const LADO_CELDA = 24;

/**
 * Las pantallas enteras que se auditan. `abrir` son los nombres (en español, del catálogo)
 * de los botones que se pulsan en orden tras cargar, y `luego` lo que debe aparecer.
 * Una pantalla nueva = una línea aquí.
 */
const PANTALLAS = [
  { id: 'lista', ruta: '/dev/app?que=lista', espera: 'section[aria-labelledby="lista-bases"] li a' },
  { id: 'base', ruta: '/dev/app?que=base', espera: '[role="grid"]' },
  { id: 'base-tablero', ruta: '/dev/app?que=base', espera: '[role="grid"]', vista: `Por estado · ${es['view.kind.kanban']}`, luego: 'section[aria-label] select' },
  { id: 'compartir', ruta: '/dev/app?que=base', espera: '[role="grid"]', abrir: [es['share.open']], luego: '[role="dialog"]' },
  { id: 'scripts', ruta: '/dev/app?que=base', espera: '[role="grid"]', menuBase: es['scripts.open'], luego: '[role="dialog"] textarea' },
];

/* ---------- API de mentira para las pantallas enteras ---------- */
const B1 = '00000000-0000-4000-8000-0000000000b1';
const T1 = '00000000-0000-4000-8000-0000000000c1';
const FECHA = '2026-10-01T00:00:00Z';
const base = (id, name, role) => ({ id, name, description: '', owner_sub: 'demo-user', role, created_at: FECHA, updated_at: FECHA });
const CAMPOS_API = [
  { id: 'nombre', table_id: T1, name: 'Nombre', kind: 'text', position: 0 },
  { id: 'importe', table_id: T1, name: 'Importe', kind: 'number', position: 1 },
  { id: 'activo', table_id: T1, name: 'Activo', kind: 'checkbox', position: 2 },
  { id: 'estado', table_id: T1, name: 'Estado', kind: 'select', choices: ['nuevo', 'en curso', 'hecho'], position: 3 },
];
const registro = (n, data) => ({
  id: `00000000-0000-4000-8000-00000000000${n}`, table_id: T1, data, links: {}, rev: 1,
  created_by: 'demo-user', updated_by: 'demo-user', created_at: FECHA, updated_at: FECHA,
});
const REGISTROS_API = [
  registro(1, { nombre: 'Panadería Aurora', importe: 120, activo: true, estado: 'nuevo' }),
  registro(2, { nombre: 'Taller Bea', importe: 85.5, estado: 'hecho' }),
  registro(3, { nombre: 'Cleo y Gus' }),
];
// Chromium puede saltarse page.route al convertir una navegación en descarga
// (microsoft/playwright#27575/#27638). El origen visitado sirve el attachment
// directamente; el resto se reenvía al Astro efímero, incluido su HMR.
const rutaExportacion = `${ASTRO.pathname.replace(/\/$/, '')}/api/tables/${T1}/export`;
const exportacionesServidas = [];
let exportacionesInterceptadas = 0;
const erroresProxy = [];
const destinoAstro = (req) => {
  const entrada = new URL(req.url ?? '/', ASTRO);
  const destino = new URL(ASTRO);
  destino.pathname = entrada.pathname;
  destino.search = entrada.search;
  return destino;
};
const servidorFixture = createServer((req, res) => {
  const destino = destinoAstro(req);
  if (destino.pathname !== rutaExportacion) {
    const proxy = requestHttp(destino, {
      method: req.method,
      headers: { ...req.headers, host: ASTRO.host },
    }, (respuesta) => {
      res.writeHead(respuesta.statusCode ?? 502, respuesta.headers);
      respuesta.on('error', (error) => {
        erroresProxy.push({ path: destino.pathname, code: error.code ?? 'upstream_aborted' });
        res.destroy();
      });
      respuesta.pipe(res);
    });
    proxy.on('error', (error) => {
      erroresProxy.push({ path: destino.pathname, code: error.code ?? 'proxy_error' });
      if (!res.headersSent) res.writeHead(502);
      res.end();
    });
    req.pipe(proxy);
    return;
  }
  const formato = destino.searchParams.get('format');
  if (req.method !== 'GET' || !['csv', 'json'].includes(formato)) {
    res.writeHead(404).end();
    return;
  }
  exportacionesServidas.push(formato);
  res
    .writeHead(200, {
      "content-type":
        formato === "csv" ? "text/csv; charset=utf-8" : "application/json",
      "content-disposition": `attachment; filename="clientes.${formato}"`,
    })
    .end(
      formato === "csv"
        ? "Nombre,Importe\nPanadería Aurora,1200\n"
        : JSON.stringify(REGISTROS_API),
    );
});
// Vite abre el websocket contra el mismo origen del módulo /@vite/client.
servidorFixture.on('upgrade', (req, socket, head) => {
  const proxy = requestHttp(destinoAstro(req), {
    method: req.method,
    headers: { ...req.headers, host: ASTRO.host },
  });
  socket.once('close', () => proxy.destroy());
  proxy.on('upgrade', (respuesta, remoto, cabecera) => {
    socket.write(`HTTP/1.1 ${respuesta.statusCode} ${respuesta.statusMessage}\r\n`);
    for (let i = 0; i < respuesta.rawHeaders.length; i += 2)
      socket.write(`${respuesta.rawHeaders[i]}: ${respuesta.rawHeaders[i + 1]}\r\n`);
    socket.write('\r\n');
    if (head.length) remoto.write(head);
    if (cabecera.length) socket.write(cabecera);
    socket.on('error', () => remoto.destroy());
    remoto.on('error', () => socket.destroy());
    socket.on('close', () => remoto.destroy());
    remoto.on('close', () => socket.destroy());
    socket.pipe(remoto).pipe(socket);
  });
  proxy.on('error', () => socket.destroy());
  proxy.on('response', () => socket.destroy());
  proxy.end();
});
const conexionesFixture = new Set();
servidorFixture.on('connection', (socket) => {
  conexionesFixture.add(socket);
  socket.once('close', () => conexionesFixture.delete(socket));
});
const json = (route, cuerpo, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(cuerpo) });

/** Contesta al BFF de Bases y al Hub: ni sesión ni servicio detrás. */
async function simularApi(page, { rol = 'owner', vistasExtra = 0 } = {}) {
  const datos = { mutaciones: [], creados: [] };
  let tablaBorrada = false;
  // El Hub (notificaciones, catálogo del marco): fuera de alcance, contesta vacío.
  await page.route('http://127.0.0.1:9/**', (r) => json(r, {}));
  // El marco publica /bases; se aceptan ambos namespaces de autenticación del fixture.
  await page.route(/\/auth\/[a-z]+(\?|$)/, (r) =>
    json(r, { accessToken: 'demo', expiresAt: Date.now() + 3600e3, organizations: [], authenticated: true }),
  );
  await page.route('**/bases/api/**', (route) => {
    const url = new URL(route.request().url());
    const camino = url.pathname.split('/api/')[1] ?? '';
    const method = route.request().method();
    if (method !== 'GET') {
      datos.mutaciones.push({ method, camino });
      if (method === 'DELETE' && camino === `tables/${T1}`) tablaBorrada = true;
      if (method === 'POST' && camino === `tables/${T1}/records`) {
        const r = registro(9, {});
        datos.creados.push(r);
        return json(route, r, 201);
      }
      return json(route, {});
    }
    if (camino === 'bases') return json(route, [base(B1, 'Clientes y pedidos', rol), base('00000000-0000-4000-8000-0000000000b2', 'Inventario', 'viewer')]);
    if (/^bases\/[^/]+$/.test(camino)) return json(route, base(B1, 'Clientes y pedidos', rol));
    if (camino.endsWith('/tables')) return json(route, tablaBorrada ? [] : [{ id: T1, base_id: B1, name: 'Clientes', position: 0, role: rol, fields: CAMPOS_API }]);
    if (camino.endsWith('/records')) return json(route, { fields: CAMPOS_API, records: [...REGISTROS_API, ...datos.creados], role: rol });
    if (camino.endsWith('/export')) {
      exportacionesInterceptadas++;
      return route.continue();
    }
    if (camino.endsWith('/views'))
      return json(route, [
        { id: '00000000-0000-4000-8000-0000000000d1', table_id: T1, name: 'Por estado', kind: 'kanban', config: { group_by: 'estado' }, created_by: 'demo-user' },
        ...Array.from({ length: vistasExtra }, (_, i) => ({ id: `vista-${i}`, table_id: T1, name: `Vista del equipo ${i + 1}`, kind: 'grid', config: {}, created_by: 'demo-user' })),
      ]);
    if (camino.endsWith('/scripts'))
      return json(route, {
        can_review: true,
        scripts: [{ id: '00000000-0000-4000-8000-0000000000f1', base_id: B1, name: 'Resumen semanal', scope: 'shared', author_sub: 'demo-user', created_at: FECHA, latest_version: 1, approved_version: 1 }],
      });
    if (camino === 'script-quota')
      return json(route, { used: { user_runs: 3, org_runs: 3, user_ms: 900 }, limits: { user_runs: 60, org_runs: 600, user_ms: 600000 } });
    if (camino.endsWith('/permissions'))
      return json(route, [
        { id: '00000000-0000-4000-8000-0000000000e1', principal_kind: 'tenant', principal_id: 'prueba', role: 'viewer', created_by: 'demo-user', created_at: FECHA },
      ]);
    return json(route, []);
  });
  return datos;
}

const fallos = [];
const resultados = new Map();
const caso = (ctx) => {
  if (!resultados.has(ctx))
    resultados.set(ctx, {
      context: ctx,
      passed: [],
      failures: [],
      downloads: [],
    });
  return resultados.get(ctx);
};
const falla = (area, ctx, msg) => {
  fallos.push(`[${area}] ${ctx}: ${msg}`);
  caso(ctx).failures.push({ area, message: msg });
  console.log(`  ✗ ${ctx} [${area}] ${msg}`);
};
const ok = (ctx, msg) => {
  caso(ctx).passed.push(msg);
  console.log(`  ✓ ${ctx} ${msg}`);
};

const url = (extra = '') => `${BASE}/dev/rejilla?lang=${IDIOMA}${extra}`;

async function abrir(browser, ancho, esquema, extra = '') {
  const ctx = await browser.newContext({
    viewport: { width: ancho.w, height: ancho.h },
    colorScheme: esquema,
    hasTouch: ancho.movil,
    isMobile: ancho.movil,
  });
  await prepareFixtureTheme(ctx, esquema);
  const page = await ctx.newPage();
  await page.goto(url(extra), { waitUntil: 'networkidle' });
  await page.waitForSelector('[role="grid"]', { timeout: 60000 });
  await page.waitForTimeout(300);
  await assertFixtureTheme(page, esquema);
  return { ctx, page };
}

async function focoVisible(page) {
  await page.waitForTimeout(200);
  return page.evaluate(() => {
    const e = document.activeElement;
    if (!e || e === document.body) return { ok: false, que: 'body' };
    const cs = getComputedStyle(e);
    const contorno = cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0;
    const sombra = cs.boxShadow !== 'none' && !/^rgba?\(0, 0, 0, 0\)/.test(cs.boxShadow);
    const pseudo = ['::after', '::before'].some((p) => {
      const ps = getComputedStyle(e, p);
      return ps.content !== 'none' && (ps.boxShadow !== 'none' || parseFloat(ps.outlineWidth) > 0);
    });
    return { ok: contorno || sombra || pseudo, que: e.tagName.toLowerCase() };
  });
}

const celda = (page, f, c) => page.locator(`[data-fila="${f}"][data-col="${c}"]`);
const enfocada = (page) =>
  page.evaluate(() => {
    const e = document.activeElement;
    return e?.dataset?.fila !== undefined ? [Number(e.dataset.fila), Number(e.dataset.col)] : null;
  });

// No espera a que llegue el foco: si sólo esperamos que exista el input, un efecto
// tardío puede recolocar el cursor después de Ctrl+A o de las primeras letras.
async function comprobarEditor(page, ctx, fila, col) {
  const estado = await page.evaluate((id) => {
    const el = document.getElementById(id);
    return {
      existe: !!el,
      foco: el === document.activeElement,
      texto: el instanceof HTMLInputElement && el.type === 'text',
      inicio: el?.selectionStart,
      fin: el?.selectionEnd,
      longitud: el?.value?.length,
    };
  }, `ed-${fila}-${col}`);
  if (!estado.existe) falla('teclado', ctx, `no se abrió el editor [${fila},${col}]`);
  else {
    if (!estado.foco) falla('teclado', ctx, `el editor [${fila},${col}] existe pero no recibe el teclado`);
    if (estado.texto && (estado.inicio !== estado.longitud || estado.fin !== estado.longitud)) {
      falla('teclado', ctx, `el cursor del editor [${fila},${col}] no está listo antes de escribir`);
    }
  }
  return estado.existe;
}

async function esperarFocoEditor(page, fila, col) {
  // Tras comprobar el contrato inmediato, continuar sólo con el editor enfocado.
  await page.waitForFunction((id) => document.activeElement?.id === id, `ed-${fila}-${col}`);
}

async function pasarAxe(page, ctx) {
  const res = await new AxeBuilder({ page }).withTags(ETIQUETAS_AXE).analyze();
  const graves = res.violations.filter((v) => GRAVES.has(v.impact));
  for (const v of graves) {
    const donde = v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ');
    falla('axe', ctx, `${v.id} (${v.impact}, ${v.nodes.length}) ${v.help} → ${donde}`);
  }
  if (graves.length === 0) ok(ctx, `axe sin fallos graves (${res.violations.length} menores)`);
}

async function teclado(page, ctx) {
  const errores = [];
  const alError = (e) => errores.push(e.message);
  page.on('pageerror', alError);
  // La rejilla es una sola parada de Tab: solo una celda tiene tabindex 0.
  const paradas = await page.locator('td[data-fila][tabindex="0"]').count();
  if (paradas !== 1) falla('teclado', ctx, `debe haber 1 celda en el orden de tabulación, hay ${paradas}`);

  await celda(page, 0, 0).focus();
  const f0 = await focoVisible(page);
  if (!f0.ok) falla('teclado', ctx, `la celda enfocada no muestra foco visible (${f0.que})`);

  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowDown');
  const a = await enfocada(page);
  if (!a || a[0] !== 1 || a[1] !== 1) falla('teclado', ctx, `flechas: foco en ${JSON.stringify(a)}, esperaba [1,1]`);
  await page.keyboard.press('End');
  const b = await enfocada(page);
  if (!b || b[1] !== 5) falla('teclado', ctx, `Fin no lleva a la última columna: ${JSON.stringify(b)}`);
  await page.keyboard.press('Control+Home');
  const c = await enfocada(page);
  if (!c || c[0] !== 0 || c[1] !== 0) falla('teclado', ctx, `Ctrl+Inicio: ${JSON.stringify(c)}`);
  await page.keyboard.press('Control+End');
  const d = await enfocada(page);
  if (!d || d[0] !== 2 || d[1] !== 5) falla('teclado', ctx, `Ctrl+Fin: ${JSON.stringify(d)}`);

  // Editar: Intro abre, escribir sustituye, Intro guarda y baja.
  await celda(page, 0, 0).focus();
  await page.keyboard.press('Enter');
  const editor = page.locator('#ed-0-0');
  if (await comprobarEditor(page, ctx, 0, 0)) {
    if ((await editor.getAttribute('aria-label')) !== 'Nombre' && IDIOMA === 'es') falla('lector', ctx, 'el editor no tiene nombre accesible');
    await esperarFocoEditor(page, 0, 0);
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.type('Nuevo nombre');
    if ((await editor.inputValue()) !== 'Nuevo nombre') falla('teclado', ctx, 'la escritura rápida perdió letras o la selección de Ctrl+A');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(150);
    const texto = (await celda(page, 0, 0).innerText()).trim();
    const pos = await enfocada(page);
    if (texto !== 'Nuevo nombre') falla('teclado', ctx, `Intro no guardó: «${texto}»`);
    if (!pos || pos[0] !== 1) falla('teclado', ctx, `tras guardar el foco debe bajar: ${JSON.stringify(pos)}`);
  }
  // Escape cancela y devuelve el foco a la celda.
  await celda(page, 1, 0).focus();
  await page.keyboard.press('F2');
  await comprobarEditor(page, ctx, 1, 0);
  await esperarFocoEditor(page, 1, 0);
  await page.keyboard.type('xx');
  await page.keyboard.press('Escape');
  const tras = (await celda(page, 1, 0).innerText()).trim();
  const pe = await enfocada(page);
  if (tras.includes('xx')) falla('teclado', ctx, 'Escape no cancela la edición');
  if (!pe || pe[0] !== 1 || pe[1] !== 0) falla('teclado', ctx, `Escape no devuelve el foco: ${JSON.stringify(pe)}`);
  // Escribir desde la celda abre el editor con esa tecla, sin perder las siguientes.
  await celda(page, 2, 0).focus();
  await page.keyboard.press('X');
  await comprobarEditor(page, ctx, 2, 0);
  await page.keyboard.type('yz');
  await page.keyboard.press('Enter');
  if ((await celda(page, 2, 0).innerText()).trim() !== 'Xyz') falla('teclado', ctx, 'escribir desde la celda perdió las primeras letras');
  // Fecha y selección reciben foco, aunque no admiten la API de selección de texto.
  for (const col of [2, 4]) {
    await celda(page, 0, col).focus();
    await page.keyboard.press('F2');
    await comprobarEditor(page, ctx, 0, col);
    await page.keyboard.press('Escape');
    const vuelta = await enfocada(page);
    if (!vuelta || vuelta[0] !== 0 || vuelta[1] !== col) falla('teclado', ctx, `Escape no devuelve el foco desde la columna ${col}`);
  }
  // Un valor que no encaja en el campo no se guarda y se anuncia.
  await celda(page, 0, 1).focus();
  await page.keyboard.press('Enter');
  await comprobarEditor(page, ctx, 0, 1);
  await esperarFocoEditor(page, 0, 1);
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type('no es un número');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(150);
  const aviso = (await page.locator('[role="status"]').first().innerText()).trim();
  if (!aviso) falla('lector', ctx, 'un valor inválido no se anuncia en la región aria-live');
  await page.keyboard.press('Escape');
  // Casilla: Espacio la marca.
  const antes = await celda(page, 1, 3).locator('input[type=checkbox]').isChecked();
  await celda(page, 1, 3).focus();
  await page.keyboard.press('Space');
  await page.waitForTimeout(150);
  const despues = await celda(page, 1, 3).locator('input[type=checkbox]').isChecked();
  if (antes === despues) falla('teclado', ctx, 'Espacio no marca la casilla');
  page.off('pageerror', alError);
  if (errores.length) falla('teclado', ctx, `el editor produjo errores: ${errores.slice(0, 2).join(' | ')}`);
  ok(ctx, 'teclado: foco itinerante, flechas, edición, Escape, validación y casilla');
}

async function lector(page, ctx) {
  const info = await page.evaluate(() => {
    const g = document.querySelector('[role="grid"]');
    return {
      nombre: g?.getAttribute('aria-label') || '',
      cols: document.querySelectorAll('th[scope="col"]').length,
      filas: document.querySelectorAll('th[scope="row"]').length,
      live: !!document.querySelector('[role="status"][aria-live="polite"]'),
      rowcount: g?.getAttribute('aria-rowcount'),
    };
  });
  if (!info.nombre) falla('lector', ctx, 'la rejilla no tiene nombre accesible');
  if (info.cols < 7) falla('lector', ctx, `faltan cabeceras de columna (${info.cols})`);
  if (info.filas < 3) falla('lector', ctx, `faltan cabeceras de fila (${info.filas})`);
  if (!info.live) falla('lector', ctx, 'no hay región aria-live para lo guardado');
  if (!info.rowcount) falla('lector', ctx, 'falta aria-rowcount');
}

async function tactil(page, ctx) {
  // La hoja móvil entra trasladándose: medir durante esa animación puede producir
  // alturas fraccionarias al restar coordenadas. Estas acciones esperan Stable,
  // sin pulsar controles ni sustituir el mínimo de 44 px por una tolerancia.
  const capas = page.locator('[role="menu"]:visible,[role="dialog"]:visible');
  for (const capa of await capas.all()) {
    await capa.scrollIntoViewIfNeeded();
    const ultimo = capa
      .locator('button:visible,a[href]:visible,input:not([type=hidden]):visible,select:visible')
      .last();
    if (await ultimo.count()) await ultimo.scrollIntoViewIfNeeded();
  }
  const { pequenos, medidas } = await page.evaluate(([ladoControl, ladoCelda]) => {
    const out = [];
    const medidas = [];
    for (const e of document.querySelectorAll('button,a[href],input:not([type=hidden]),select,td[data-fila]')) {
      // La cabecera es el marco común de la suite (`@sytue/shell`), no de Bases: su buscador
      // mide 36 px de alto en todas las apps y no se arregla desde aquí.
      if (e.closest('[aria-hidden="true"]') || e.closest('.sr-only') || e.closest('astro-dev-toolbar') || e.closest('header')) continue;
      const lado = e.matches('td[data-fila]') ? ladoCelda : ladoControl;
      const r = e.getBoundingClientRect();
      const cs = getComputedStyle(e);
      if (r.width === 0 || r.height === 0 || cs.visibility === 'hidden') continue;
      // La casilla de dentro de una celda es presentación: la diana es la celda.
      if (e.matches('input[type=checkbox]') && e.closest('td[data-fila]')) continue;
      // Una casilla con su etiqueta: la diana es la etiqueta entera (WCAG 2.5.8).
      const etiqueta = e.matches('input[type=checkbox],input[type=radio]') ? e.closest('label') : null;
      if (etiqueta) {
        const re = etiqueta.getBoundingClientRect();
        if (re.width >= lado && re.height >= lado) continue;
      }
      const medida = {
        tag: e.tagName.toLowerCase(),
        name: (e.getAttribute('aria-label') || e.textContent || '').trim().slice(0, 30),
        width: r.width,
        height: r.height,
        minimum: lado,
      };
      medidas.push(medida);
      if (r.width < lado || r.height < lado) out.push(medida);
    }
    return { pequenos: out, medidas };
  }, [LADO_TACTIL, LADO_CELDA]);
  caso(ctx).touchTargets = medidas;
  if (pequenos.length) {
    const detalle = pequenos.slice(0, 12)
      .map((m) => `${m.tag} ${m.name} ${m.width}×${m.height}`).join(' | ');
    falla('tactil', ctx, `${pequenos.length} controles por debajo de ${LADO_TACTIL}px: ${detalle}`);
  }
}

/** Lo que un lector encuentra en una pantalla entera: nombres, idioma, título. */
async function lectorDePantalla(page, ctx) {
  const r = await page.evaluate(() => {
    const visible = (e) => {
      const b = e.getBoundingClientRect();
      return b.width > 0 && b.height > 0 && !e.closest('[aria-hidden="true"]') && !e.closest('astro-dev-toolbar');
    };
    const nombre = (e) => (e.getAttribute('aria-label') || e.getAttribute('aria-labelledby') || e.getAttribute('title') || e.textContent || '').trim();
    return {
      // Sin `nav` ni `aside`: son del marco común (`AppShell` de @sytue/ui), que no los nombra en
      // ninguna app; aquí se mide lo que Bases pinta.
      regiones: [...document.querySelectorAll('[role="tablist"],[role="toolbar"],[role="tabpanel"],[role="dialog"],[role="grid"],[role="group"]')]
        .filter((e) => visible(e) && !(e.getAttribute('aria-label') || e.getAttribute('aria-labelledby')))
        .map((e) => e.getAttribute('role') || e.tagName.toLowerCase()),
      controles: [...document.querySelectorAll('button,[role="button"],[role="tab"],a[href]')].filter((e) => visible(e) && !nombre(e)).map((e) => e.outerHTML.slice(0, 80)),
      campos: [...document.querySelectorAll('input:not([type=hidden]),select,textarea')].filter((e) => visible(e) && !(e.labels?.length || e.getAttribute('aria-label') || e.getAttribute('aria-labelledby'))).length,
      h1: document.querySelectorAll('h1').length,
      lang: document.documentElement.lang,
    };
  });
  if (r.regiones.length) falla('lector', ctx, `regiones sin nombre accesible: ${[...new Set(r.regiones)].join(', ')}`);
  if (r.controles.length) falla('lector', ctx, `controles sin nombre accesible: ${r.controles.slice(0, 3).join(' | ')}`);
  if (r.campos) falla('lector', ctx, `${r.campos} campos sin etiqueta`);
  if (r.h1 !== 1) falla('lector', ctx, `debe haber un h1, hay ${r.h1}`);
  if (!r.lang) falla('lector', ctx, 'el documento no declara idioma');
}

/** Tab recorre la pantalla y cada parada enseña el foco. */
async function tabulacion(page, ctx) {
  const sinFoco = new Set();
  for (let i = 0; i < 30; i++) {
    await page.keyboard.press('Tab');
    await page.waitForTimeout(180);
    const f = await page.evaluate(() => {
      const e = document.activeElement;
      if (!e || e === document.body || e.closest('astro-dev-toolbar') || e.tagName.includes('-')) return null;
      const cs = getComputedStyle(e);
      const contorno = cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0;
      const sombra = cs.boxShadow !== 'none' && !/^rgba?\(0, 0, 0, 0\)/.test(cs.boxShadow);
      const pseudo = ['::after', '::before'].some((p) => {
        const ps = getComputedStyle(e, p);
        return ps.content !== 'none' && (ps.boxShadow !== 'none' || parseFloat(ps.outlineWidth) > 0);
      });
      return contorno || sombra || pseudo ? null : `${e.tagName.toLowerCase()} ${(e.getAttribute('aria-label') || e.textContent || '').trim().slice(0, 30)}`;
    });
    if (f) sinFoco.add(f);
  }
  if (sinFoco.size) falla('teclado', ctx, `paradas de Tab sin foco visible: ${[...sinFoco].slice(0, 5).join(' | ')}`);
  else ok(ctx, 'Tab: todas las paradas con foco visible');
}

async function abrirPantalla(browser, p, ancho, esquema) {
  const c = await browser.newContext({
    viewport: { width: ancho.w, height: ancho.h },
    colorScheme: esquema,
    hasTouch: ancho.movil,
    isMobile: ancho.movil,
  });
  await prepareFixtureTheme(c, esquema);
  const page = await c.newPage();
  const errores = [];
  page.on('pageerror', (e) => errores.push(e.message));
  const datos = await simularApi(page, { rol: p.rol ?? 'owner', vistasExtra: ancho.movil ? 12 : 0 });
  await page.goto(`${BASE}${p.ruta}&lang=${IDIOMA}`, { timeout: 120000 });
  await page.waitForSelector(p.espera, { timeout: 90000 });
  await assertFixtureTheme(page, esquema);
  if (p.vista) {
    await selectorDeVistas(page).click();
    await page.getByRole('menuitem', { name: p.vista, exact: true }).click();
  }
  if (p.menuBase) {
    await menuBase(page).click();
    await page.getByRole('menuitem', { name: p.menuBase, exact: true }).click();
  }
  for (const nombre of p.abrir ?? []) await page.getByRole('button', { name: nombre, exact: true }).first().click();
  if (p.luego) await page.waitForSelector(p.luego, { timeout: 15000 });
  await page.waitForTimeout(300);
  return { c, page, errores, datos };
}

const selectorDeVistas = (page) =>
  page.getByRole('button', { name: new RegExp(`^${es['view.list']}:`) });
const menuBase = (page) =>
  page.getByRole('button', { name: `${es['actions.more']} · Clientes y pedidos`, exact: true });
const menuTabla = (page) =>
  page.getByRole('button', { name: `${es['actions.more']} · Clientes`, exact: true });

async function contenidoEnPrimeraPantalla(page, p, ancho, ctx) {
  if (!ancho.movil || !['base', 'base-tablero'].includes(p.id)) return;
  const primera =
    p.id === 'base'
      ? page.locator('[role="tabpanel"] tbody tr').first()
      : page.locator('[role="tabpanel"] section[aria-label] li').first();
  await primera.getByText('Panadería Aurora', { exact: true }).waitFor();
  const caja = await primera.boundingBox();
  if (!caja || caja.y < 0 || caja.y + caja.height > ancho.h - 8) {
    falla(
      'densidad',
      ctx,
      `el primer registro/tarjeta queda fuera de la primera pantalla (${JSON.stringify(caja)})`,
    );
  } else ok(ctx, 'primer registro/tarjeta completo sin desplazar');
}

async function descargarExportacion(page, formato, ctx) {
  await menuTabla(page).click();
  const enlace = page.getByRole("menuitem", {
    name: es[`export.${formato}`],
    exact: true,
  });
  const destino = new URL(await enlace.getAttribute("href"), page.url());
  if (
    destino.origin !== new URL(page.url()).origin ||
    destino.pathname !== rutaExportacion ||
    destino.searchParams.get("format") !== formato ||
    (await enlace.getAttribute("download")) === null
  )
    falla("gestión", ctx, `falta el enlace de descarga ${formato}`);
  const antes = exportacionesServidas.filter((f) => f === formato).length;
  const interceptadasAntes = exportacionesInterceptadas;
  const diagnostico = {
    format: formato,
    path: destino.pathname,
    served: 0,
    intercepted: 0,
  };
  caso(ctx).downloads.push(diagnostico);
  try {
    const [descarga] = await Promise.all([
      page.waitForEvent("download"),
      enlace.click(),
    ]);
    diagnostico.downloadPath = new URL(descarga.url()).pathname;
    diagnostico.failure = await descarga.failure();
    if (diagnostico.failure)
      throw new Error(`la descarga ${formato} falla: ${diagnostico.failure}`);
    const archivo = await descarga.path();
    const contenido = archivo ? readFileSync(archivo, "utf8") : "";
    diagnostico.filename = descarga.suggestedFilename();
    diagnostico.bytes = Buffer.byteLength(contenido);
    if (diagnostico.filename !== `clientes.${formato}`)
      falla(
        "gestión",
        ctx,
        `la exportación ${formato} no termina como descarga`,
      );
    if (!contenido.includes("Panadería Aurora"))
      falla("gestión", ctx, `la descarga ${formato} está vacía`);
  } finally {
    diagnostico.served =
      exportacionesServidas.filter((f) => f === formato).length - antes;
    diagnostico.intercepted = exportacionesInterceptadas - interceptadasAntes;
    console.log(`  Descarga ${ctx}: ${JSON.stringify(diagnostico)}`);
  }
  if (diagnostico.served !== 1)
    falla(
      "gestión",
      ctx,
      `la descarga ${formato} no recibe exactamente una respuesta HTTP`,
    );
}

async function gestionCompacta(page, datos, ctx) {
  // Hay trece vistas guardadas: Inicio/Fin y flechas deben seguir llegando a la última.
  const selector = selectorDeVistas(page);
  await selector.focus();
  await selector.press('Enter');
  await page.getByRole('menu', { name: es['view.list'], exact: true }).waitFor();
  await page.waitForFunction(() => document.activeElement?.getAttribute('role') === 'menuitem');
  await page.keyboard.press('End');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Enter');
  await page.waitForFunction(
    (id) =>
      document.getElementById(id)?.getAttribute('aria-label')?.includes('Vista del equipo 12'),
    await selector.getAttribute('id'),
  );
  if (!(await selector.evaluate((el) => el === document.activeElement)))
    falla('teclado', ctx, 'elegir vista no devuelve el foco al selector');
  await selector.click();
  await page.getByRole('menuitem', { name: es['view.all'], exact: true }).click();

  await menuTabla(page).press('Enter');
  for (const clave of [
    'field.add',
    'table.rename',
    'table.delete',
    'table.add',
    'import.open',
    'export.csv',
    'export.json',
  ]) {
    if (!(await page.getByRole('menuitem', { name: es[clave], exact: true }).isVisible()))
      falla('gestión', ctx, `${clave} no es alcanzable en gestión de tabla`);
  }
  await tactil(page, `${ctx} menú tabla`);
  await page.keyboard.press('Escape');
  for (const formato of ['csv', 'json']) await descargarExportacion(page, formato, ctx);

  for (const clave of ['field.add', 'table.rename']) {
    await menuTabla(page).click();
    await page.getByRole('menuitem', { name: es[clave], exact: true }).click();
    const dialogo = page.getByRole('dialog');
    await dialogo.waitFor();
    if (
      clave === 'table.rename' &&
      (await dialogo.getByRole('textbox').inputValue()) !== 'Clientes'
    )
      falla('gestión', ctx, 'renombrar no conserva el nombre actual');
    await page.keyboard.press('Escape');
    await dialogo.waitFor({ state: 'hidden' });
  }
  await selector.click();
  await page.getByRole('menuitem', { name: es['view.add'], exact: true }).click();
  await page.getByRole('dialog').waitFor();
  await page.keyboard.press('Escape');
  await page.getByRole('dialog').waitFor({ state: 'hidden' });

  // Alta cotidiana disponible también con el tablero abierto, usando el endpoint real.
  await selector.click();
  await page
    .getByRole('menuitem', { name: `Por estado · ${es['view.kind.kanban']}`, exact: true })
    .click();
  const tarjetas = page.locator('[role="tabpanel"] section[aria-label] li');
  await tarjetas.first().waitFor();
  const antes = await tarjetas.count();
  await page.getByRole('button', { name: es['grid.addRecord'], exact: true }).first().click();
  await page.waitForFunction(
    (n) => document.querySelectorAll('[role="tabpanel"] section[aria-label] li').length === n,
    antes + 1,
  );
  if (datos.creados.length !== 1) falla('gestión', ctx, 'añadir registro no ejecuta una sola alta');

  // Abrir una acción destructiva no confirma; cancelar no debe emitir DELETE.
  await menuTabla(page).click();
  await page.getByRole('menuitem', { name: es['table.delete'], exact: true }).click();
  const confirmacion = page.getByRole('dialog', { name: es['common.delete'], exact: true });
  await confirmacion.waitFor();
  if (datos.mutaciones.some((m) => m.method === 'DELETE'))
    falla('gestión', ctx, 'borrar actúa antes de confirmar');
  await confirmacion.getByRole('button', { name: es['common.cancel'], exact: true }).click();
  await confirmacion.waitFor({ state: 'hidden' });
  if (datos.mutaciones.some((m) => m.method === 'DELETE'))
    falla('gestión', ctx, 'cancelar ha borrado la tabla');
  await menuTabla(page).click();
  await page.getByRole('menuitem', { name: es['table.delete'], exact: true }).click();
  await Promise.all([
    page.waitForResponse(
      (r) => r.request().method() === 'DELETE' && r.url().endsWith(`/tables/${T1}`),
    ),
    confirmacion.getByRole('button', { name: es['common.delete'], exact: true }).click(),
  ]);
  if (
    datos.mutaciones.filter((m) => m.method === 'DELETE' && m.camino === `tables/${T1}`).length !==
    1
  )
    falla('gestión', ctx, 'la confirmación no emite exactamente un borrado de tabla');
  await page.getByText(es['base.noTables'], { exact: true }).waitFor();
  ok(ctx, 'vistas, descargas, diálogos, alta y confirmación accesibles desde menús');
}

let browser;
try {
  await new Promise((resolve, reject) => {
    servidorFixture.once('error', reject);
    servidorFixture.listen(0, '127.0.0.1', resolve);
  });
  const origenFixture = new URL(ASTRO);
  origenFixture.host = `127.0.0.1:${servidorFixture.address().port}`;
  BASE = origenFixture.href.replace(/\/$/, '');
  browser = await chromium.launch();
  // Pantallas enteras: la lista, la base, su tablero y compartir, con el marco.
  for (const p of PANTALLAS) {
    for (const ancho of [...ANCHOS, { w: 640, h: 400, movil: false, zoom: true }]) {
      for (const esquema of ESQUEMAS) {
        const ctx = `${p.id} ${ancho.zoom ? 'zoom200' : `${ancho.w}px`} ${esquema}`;
        Object.assign(caso(ctx), { screen: p.id, width: ancho.w, height: ancho.h, theme: esquema });
        let abierta;
        try {
          abierta = await abrirPantalla(browser, p, ancho, esquema);
        } catch (e) {
          falla('lector', ctx, `la pantalla no llega a pintarse: ${String(e.message).slice(0, 160)}`);
          continue;
        }
        const { c, page, errores, datos } = abierta;
        const antes = fallos.length;
        try { await contenidoEnPrimeraPantalla(page, p, ancho, ctx); }
        catch (e) { falla('densidad', ctx, String(e.message).slice(0, 240)); }
        if (p.id === 'lista' && ancho.movil) {
          const desalineadas = await page.locator('section[aria-labelledby="lista-bases"] li a').evaluateAll((enlaces) => enlaces.filter((a) => Math.abs(a.getBoundingClientRect().width - a.parentElement.getBoundingClientRect().width) > 1).length);
          if (desalineadas) falla('densidad', ctx, 'las tarjetas no ocupan el mismo ancho de la lista');
        }
        if (['lista', 'base', 'base-tablero'].includes(p.id) && [320, 390, 1280].includes(ancho.w)) {
          await page.screenshot({ path: `${OUT}/muestra-${p.id}-${ancho.w}-${esquema}.png` });
        }
        await pasarAxe(page, ctx);
        const desborda = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
        if (desborda) falla('zoom', ctx, 'la página desborda de lado');
        if (esquema === 'light') await lectorDePantalla(page, ctx);
        if (ancho.movil && esquema === 'light') await tactil(page, ctx);
        if (ancho.w === 1280 && esquema === 'light') await tabulacion(page, ctx);
        if (p.id === 'base' && ancho.movil && esquema === 'light') {
          try { await gestionCompacta(page, datos, ctx); }
          catch (e) { falla('gestión', ctx, String(e.message).slice(0, 240)); }
        }
        if (p.id === 'base' && ancho.w === 1280 && esquema === 'light') {
          try {
            for (const formato of ['csv', 'json']) await descargarExportacion(page, formato, ctx);
            ok(ctx, 'descargas CSV y JSON reales también en escritorio');
          } catch (e) { falla('gestión', ctx, String(e.message).slice(0, 240)); }
        }
        if (errores.length) falla('lector', ctx, `errores de página: ${errores.slice(0, 2).join(' | ')}`);
        if (fallos.length > antes) await page.screenshot({ path: `${OUT}/${ctx.replace(/ /g, '-')}.png` });
        await c.close();
      }
    }
  }

  for (const ancho of ANCHOS) {
    for (const esquema of ESQUEMAS) {
      const ctx = `${ancho.w}px ${esquema}`;
      const { ctx: c, page } = await abrir(browser, ancho, esquema);
      await pasarAxe(page, ctx);
      await lector(page, ctx);
      if (ancho.movil) await tactil(page, ctx);
      if (ancho.w === 1280 && esquema === 'light') await teclado(page, ctx);
      // Con el diálogo de varios abierto (selección múltiple) también.
      await celda(page, 1, 5).focus();
      await page.keyboard.press('Enter');
      await page.waitForSelector('[role="dialog"]', { timeout: 5000 }).catch(() => undefined);
      if ((await page.locator('[role="dialog"]').count()) === 0) falla('teclado', ctx, 'la selección múltiple no abre su diálogo');
      else {
        // El diálogo entra con una transición de opacidad: medido a medio aparecer, axe ve
        // el botón primario mezclado con el fondo y da un contraste que no es el suyo.
        await page.waitForTimeout(400);
        await pasarAxe(page, `${ctx} (diálogo)`);
      }
      if (fallos.length) await page.screenshot({ path: `${OUT}/${ancho.w}-${esquema}.png`, fullPage: true });
      await c.close();
    }
  }
  // Tablero Kanban y formulario público (bancos de pruebas de `dev/vistas`).
  for (const ancho of ANCHOS) {
    for (const esquema of ESQUEMAS) {
      for (const que of ['tablero', 'formulario']) {
        const ctx = `${que} ${ancho.w}px ${esquema}`;
        const c = await browser.newContext({
          viewport: { width: ancho.w, height: ancho.h },
          colorScheme: esquema,
          hasTouch: ancho.movil,
          isMobile: ancho.movil,
        });
        await prepareFixtureTheme(c, esquema);
        const page = await c.newPage();
        await page.goto(`${BASE}/dev/vistas?que=${que}&lang=${IDIOMA}`, { waitUntil: 'networkidle' });
        await page.waitForSelector(que === 'tablero' ? 'section[aria-label]' : 'form', { timeout: 60000 });
        await assertFixtureTheme(page, esquema);
        await pasarAxe(page, ctx);
        if (ancho.movil) await tactil(page, ctx);
        const desborda = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1);
        if (desborda) falla('zoom', ctx, 'la página desborda de lado');
        if (que === 'tablero' && ancho.w === 1280 && esquema === 'light') {
          // Sin arrastrar: el selector «Mover a» de la tarjeta la cambia de columna.
          const cuenta = async () => page.locator('section[aria-label]').nth(1).locator('li').count();
          const antes = await cuenta();
          const sel = page.locator('section[aria-label]').first().locator('select').first();
          await sel.focus();
          const f = await focoVisible(page);
          if (!f.ok) falla('teclado', ctx, 'el selector «Mover a» no muestra foco visible');
          await sel.selectOption({ index: 1 });
          await page.waitForTimeout(200);
          if ((await cuenta()) !== antes + 1) falla('teclado', ctx, 'mover una tarjeta con el selector no la cambia de columna');
          else ok(ctx, 'la tarjeta se mueve sin arrastrar');
          if (!(await page.locator('[role="status"]').first().innerText()).trim()) falla('lector', ctx, 'mover una tarjeta no se anuncia');
          const origen = page.locator('[data-bases-column="en curso"]');
          const destino = page.locator('[data-bases-column="hecho"]');
          const asa = origen.locator('li button').first();
          const desde = await asa.boundingBox();
          const hasta = await destino.boundingBox();
          if (!desde || !hasta) falla('arrastre', ctx, 'falta el asa o su destino');
          else {
            const empezar = async () => {
              await page.mouse.move(desde.x + desde.width / 2, desde.y + desde.height / 2);
              await page.mouse.down();
              await page.mouse.move(hasta.x + hasta.width / 2, hasta.y + 60, { steps: 8 });
            };
            await empezar();
            await page.keyboard.press('Escape');
            await page.mouse.up();
            if (await origen.locator('li').count() !== antes + 1) falla('arrastre', ctx, 'Escape ha guardado el movimiento');
            const enDestino = await destino.locator('li').count();
            await empezar();
            await page.mouse.up();
            await page.waitForFunction((n) => document.querySelector('[data-bases-column="hecho"]')?.querySelectorAll('li').length === n, enDestino + 1);
            ok(ctx, 'el asa mueve al soltar y Escape conserva la columna');
          }
        }
        if (que === 'formulario' && ancho.w === 1280 && esquema === 'light') {
          // Cada control tiene etiqueta y un envío vacío se explica sin perder lo escrito.
          const sinNombre = await page.evaluate(() =>
            [...document.querySelectorAll('form input, form select')].filter((e) => !(e.labels?.length || e.getAttribute('aria-label'))).length,
          );
          if (sinNombre) falla('lector', ctx, `${sinNombre} controles del formulario sin etiqueta`);
          await page.locator('form button[type=submit]').click();
          await page.waitForTimeout(200);
          if ((await page.locator('[role="alert"]').count()) === 0) falla('lector', ctx, 'un envío vacío no avisa');
          else ok(ctx, 'el envío vacío avisa con role=alert');
        }
        await c.close();
      }
    }
  }
  // Zoom al 200 %: a 1280 px de ventana son 640 px CSS.
  {
    const { ctx: c, page } = await abrir(browser, { w: 640, h: 800, movil: false }, 'light');
    const desborda = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1);
    if (desborda) falla('zoom', '200 %', 'la página desborda de lado');
    else ok('200 %', 'sin desbordamiento de la página');
    await c.close();
  }
  // Solo lectura: sin editor ni borrar.
  {
    const { ctx: c, page } = await abrir(browser, ANCHOS[0], 'light', '&lectura=1');
    await celda(page, 0, 0).focus();
    await page.keyboard.press('Enter');
    if ((await page.locator('#ed-0-0').count()) !== 0) falla('lectura', 'solo lectura', 'se abre un editor en solo lectura');
    if ((await page.locator('tbody th button').count()) > 0) falla('lectura', 'solo lectura', 'hay botones de borrar en solo lectura');
    await c.close();
  }
  // La gestión compacta respeta el rol real de la base y de su tabla.
  {
    const { c, page } = await abrirPantalla(browser, { id: 'base-lector', ruta: '/dev/app?que=base', espera: '[role="grid"]', rol: 'viewer' }, ANCHOS[1], 'light');
    for (const clave of ['share.open', 'grid.addRecord']) {
      if (await page.getByRole('button', { name: es[clave], exact: true }).count()) falla('lectura', 'menús', `${clave} aparece para un lector`);
    }
    await menuTabla(page).click();
    for (const clave of ['field.add', 'table.rename', 'table.delete', 'table.add', 'import.open']) {
      if (await page.getByRole('menuitem', { name: es[clave], exact: true }).count()) falla('lectura', 'menús', `${clave} aparece para un lector`);
    }
    for (const clave of ['export.csv', 'export.json']) {
      if (!await page.getByRole('menuitem', { name: es[clave], exact: true }).isVisible()) falla('lectura', 'menús', `${clave} no está disponible para un lector`);
    }
    await page.keyboard.press('Escape');
    await selectorDeVistas(page).click();
    if (await page.getByRole('menuitem', { name: es['view.add'], exact: true }).count()) falla('lectura', 'menús', 'crear vista aparece para un lector');
    await page.getByRole('menuitem', { name: `Por estado · ${es['view.kind.kanban']}`, exact: true }).click();
    await selectorDeVistas(page).click();
    if (await page.getByRole('menuitem', { name: es['view.delete'], exact: true }).count()) falla('lectura', 'menús', 'borrar vista aparece para un lector');
    await page.keyboard.press('Escape');
    await menuBase(page).click();
    for (const clave of ['base.rename', 'base.delete', 'conn.open']) {
      if (await page.getByRole('menuitem', { name: es[clave], exact: true }).count()) falla('lectura', 'menús', `${clave} aparece para un lector`);
    }
    await c.close();
  }
} catch (error) {
  // Sólo contexto sintético: sin cabeceras, credenciales, entorno ni cuerpos de red.
  falla('arnés', 'fatal', String(error.message).slice(0, 400));
  throw error;
} finally {
  try {
    await browser?.close();
  } finally {
    for (const socket of conexionesFixture) socket.destroy();
    await new Promise((resolve) => servidorFixture.close(resolve));
    writeReport(join(OUT, 'results.json'), JSON.stringify({
      synthetic: true,
      ok: fallos.length === 0,
      results: [...resultados.values()].map((r) => ({ ...r, ok: r.failures.length === 0 })),
      transport: { served: exportacionesServidas, intercepted: exportacionesInterceptadas, errors: erroresProxy },
    }, null, 2));
  }
}

console.log(`\n${fallos.length === 0 ? 'OK: sin fallos' : `${fallos.length} fallo(s)`}`);
for (const f of fallos) console.log(` - ${f}`);
process.exit(fallos.length === 0 ? 0 : 1);
