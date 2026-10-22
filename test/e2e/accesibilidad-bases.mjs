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
// Salida: informe en consola y capturas de lo que falle en ACC_OUT (por defecto
// /tmp/accesibilidad-bases). Código de salida 1 si algo falla.
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
import { mkdirSync } from 'node:fs';
import { es } from '../../apps/frontend/apps/bases/src/i18n/messages/es.ts';

const BASE = process.env.ACC_URL || 'http://127.0.0.1:4337/bases';
const OUT = process.env.ACC_OUT || '/tmp/accesibilidad-bases';
const IDIOMA = process.env.ACC_LANG || 'es';
mkdirSync(OUT, { recursive: true });

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
  { id: 'base-tablero', ruta: '/dev/app?que=base', espera: '[role="grid"]', abrir: [`Por estado · ${es['view.kind.kanban']}`], luego: 'section[aria-label] select' },
  { id: 'compartir', ruta: '/dev/app?que=base', espera: '[role="grid"]', abrir: [es['share.open']], luego: '[role="dialog"]' },
  { id: 'scripts', ruta: '/dev/app?que=base', espera: '[role="grid"]', abrir: [es['scripts.open']], luego: '[role="dialog"] textarea' },
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
const json = (route, cuerpo, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(cuerpo) });

/** Contesta al BFF de Bases y al Hub: ni sesión ni servicio detrás. */
async function simularApi(page) {
  // El Hub (notificaciones, catálogo del marco): fuera de alcance, contesta vacío.
  await page.route('http://127.0.0.1:9/**', (r) => json(r, {}));
  // El banco no lleva el prefijo de la app en `/auth` (no hay `basePathScript`): los dos valen.
  await page.route(/\/auth\/[a-z]+(\?|$)/, (r) =>
    json(r, { accessToken: 'demo', expiresAt: Date.now() + 3600e3, organizations: [], authenticated: true }),
  );
  await page.route('**/bases/api/**', (route) => {
    const camino = new URL(route.request().url()).pathname.split('/api/')[1] ?? '';
    if (route.request().method() !== 'GET') return json(route, {});
    if (camino === 'bases') return json(route, [base(B1, 'Clientes y pedidos', 'owner'), base('00000000-0000-4000-8000-0000000000b2', 'Inventario', 'viewer')]);
    if (/^bases\/[^/]+$/.test(camino)) return json(route, base(B1, 'Clientes y pedidos', 'owner'));
    if (camino.endsWith('/tables')) return json(route, [{ id: T1, base_id: B1, name: 'Clientes', position: 0, role: 'owner', fields: CAMPOS_API }]);
    if (camino.endsWith('/records')) return json(route, { fields: CAMPOS_API, records: REGISTROS_API, role: 'owner' });
    if (camino.endsWith('/views'))
      return json(route, [
        { id: '00000000-0000-4000-8000-0000000000d1', table_id: T1, name: 'Por estado', kind: 'kanban', config: { group_by: 'estado' }, created_by: 'demo-user' },
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
}

const fallos = [];
const falla = (area, ctx, msg) => {
  fallos.push(`[${area}] ${ctx}: ${msg}`);
  console.log(`  ✗ ${ctx} [${area}] ${msg}`);
};
const ok = (ctx, msg) => console.log(`  ✓ ${ctx} ${msg}`);

const url = (extra = '') => `${BASE}/dev/rejilla?lang=${IDIOMA}${extra}`;

async function abrir(browser, ancho, esquema, extra = '') {
  const ctx = await browser.newContext({
    viewport: { width: ancho.w, height: ancho.h },
    colorScheme: esquema,
    hasTouch: ancho.movil,
    isMobile: ancho.movil,
  });
  const page = await ctx.newPage();
  await page.goto(url(extra), { waitUntil: 'networkidle' });
  await page.waitForSelector('[role="grid"]', { timeout: 60000 });
  await page.waitForTimeout(300);
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
  if ((await editor.count()) !== 1) falla('teclado', ctx, 'Intro no abre el editor de la celda');
  else {
    if ((await editor.getAttribute('aria-label')) !== 'Nombre' && IDIOMA === 'es') falla('lector', ctx, 'el editor no tiene nombre accesible');
    await page.keyboard.press('Control+A');
    await page.keyboard.type('Nuevo nombre');
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
  await page.keyboard.type('xx');
  await page.keyboard.press('Escape');
  const tras = (await celda(page, 1, 0).innerText()).trim();
  const pe = await enfocada(page);
  if (tras.includes('xx')) falla('teclado', ctx, 'Escape no cancela la edición');
  if (!pe || pe[0] !== 1 || pe[1] !== 0) falla('teclado', ctx, `Escape no devuelve el foco: ${JSON.stringify(pe)}`);
  // Un valor que no encaja en el campo no se guarda y se anuncia.
  await celda(page, 0, 1).focus();
  await page.keyboard.press('Enter');
  await page.keyboard.press('Control+A');
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
  const pequenos = await page.evaluate(([ladoControl, ladoCelda]) => {
    const out = [];
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
      if (r.width < lado || r.height < lado) {
        out.push(`${e.tagName.toLowerCase()} ${(e.getAttribute('aria-label') || e.textContent || '').trim().slice(0, 30)} ${Math.round(r.width)}×${Math.round(r.height)}`);
      }
    }
    return out;
  }, [LADO_TACTIL, LADO_CELDA]);
  if (pequenos.length) falla('tactil', ctx, `${pequenos.length} controles por debajo de ${LADO_TACTIL}px: ${pequenos.slice(0, 12).join(' | ')}`);
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
  const page = await c.newPage();
  const errores = [];
  page.on('pageerror', (e) => errores.push(e.message));
  await simularApi(page);
  await page.goto(`${BASE}${p.ruta}&lang=${IDIOMA}`, { timeout: 120000 });
  await page.waitForSelector(p.espera, { timeout: 90000 });
  for (const nombre of p.abrir ?? []) await page.getByRole('button', { name: nombre, exact: true }).first().click();
  if (p.luego) await page.waitForSelector(p.luego, { timeout: 15000 });
  await page.waitForTimeout(300);
  return { c, page, errores };
}

const browser = await chromium.launch();
try {
  // Pantallas enteras: la lista, la base, su tablero y compartir, con el marco.
  for (const p of PANTALLAS) {
    for (const ancho of [...ANCHOS, { w: 640, h: 400, movil: false, zoom: true }]) {
      for (const esquema of ESQUEMAS) {
        const ctx = `${p.id} ${ancho.zoom ? 'zoom200' : `${ancho.w}px`} ${esquema}`;
        let abierta;
        try {
          abierta = await abrirPantalla(browser, p, ancho, esquema);
        } catch (e) {
          falla('lector', ctx, `la pantalla no llega a pintarse: ${String(e.message).slice(0, 160)}`);
          continue;
        }
        const { c, page, errores } = abierta;
        const antes = fallos.length;
        await pasarAxe(page, ctx);
        const desborda = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
        if (desborda) falla('zoom', ctx, 'la página desborda de lado');
        if (esquema === 'light') await lectorDePantalla(page, ctx);
        if (ancho.movil && esquema === 'light') await tactil(page, ctx);
        if (ancho.w === 1280 && esquema === 'light') await tabulacion(page, ctx);
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
        const page = await c.newPage();
        await page.goto(`${BASE}/dev/vistas?que=${que}&lang=${IDIOMA}`, { waitUntil: 'networkidle' });
        await page.waitForSelector(que === 'tablero' ? 'section[aria-label]' : 'form', { timeout: 60000 });
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
} finally {
  await browser.close();
}

console.log(`\n${fallos.length === 0 ? 'OK: sin fallos' : `${fallos.length} fallo(s)`}`);
for (const f of fallos) console.log(` - ${f}`);
process.exit(fallos.length === 0 ? 0 : 1);
