// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Auditoría de ACCESIBILIDAD de Sytue Docus, Calcus, Diapos y CertiPDF (web), con axe-core
// y pruebas de teclado, de zoom y de tamaño táctil. Complementa a `cinta.mjs` (maquetación
// de la cinta): aquí se mide lo que un lector de pantalla, un teclado o un dedo encuentran.
//
// Mide los bancos de pruebas `/dev/editor` y `/dev/pdf` de apps/docus (solo en desarrollo,
// 404 en producción). No hay sesión ni servicio detrás: la prueba contesta a `/api/*` con
// `page.route`, así que lo que se audita es el editor entero (cabecera, cinta, lienzo,
// rail, estado) con un documento de ejemplo, sin depender de datos.
//
// Uso (desde el contenedor del frontal; el servidor dev, antes, en otro proceso):
//   cd apps/frontend/apps/docus && bun --bun astro dev --port 4338 &
//   cd test/e2e && ACC_URL=http://127.0.0.1:4338/docus/prueba bun run accesibilidad-office.mjs
//   (o `bash correr-accesibilidad.sh`, que levanta el servidor y lanza este y cinta.mjs)
// Salida: informe en consola, `informe.json` y capturas de lo que falle en ACC_OUT (por
// defecto /tmp/accesibilidad). Código de salida 1 si algo grave falla.
//
// Qué comprueba, por producto, ancho (1280, 390, 320) y esquema (claro, oscuro):
//   1. axe-core (WCAG 2.0/2.1/2.2 A y AA) sobre la página y con cada pestaña de la cinta
//      abierta. Lo `serious` y `critical` FALLA; lo demás se lista como menor.
//   2. Zoom al 200 %: a 1280 px de ventana, 640 px CSS. El documento no desborda y la
//      cinta sigue entera (axe incluido).
//   3. Teclado (a 1280, claro): Tab llega a la cinta, las flechas mueven las pestañas y
//      la banda, F6 salta entre cinta y lienzo, Escape cierra un popover devolviendo el
//      foco a su disparador, y todo lo enfocable muestra un foco visible.
//   4. Objetivos táctiles (a 390 y 320, con puntero táctil): cada control de cabecera,
//      cinta y barra de estado mide al menos 44 × 44 px (WCAG 2.5.5; el AA de 2.5.8 son 24).
//   5. Lector de pantalla: la cinta es un `tablist` con `tabpanel`; toda región con rol
//      tiene nombre; hay una región `aria-live` que anuncia guardado y errores.
//
// ponytail: script de verificación, no forma parte de la suite de specs de @playwright/test
// (esa pide la pila dev entera con sesión). Un fallo es un fallo de la página, no del script.
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { mkdirSync, writeFileSync } from 'node:fs';
import { es } from '../../apps/frontend/packages/office/src/i18n/messages/es.ts';
import { pruebasDeInicio } from './inicio-office-scroll.mjs';

const BASE = process.env.ACC_URL || 'http://127.0.0.1:4338/docus/prueba';
const OUT = process.env.ACC_OUT || '/tmp/accesibilidad';
const SOLO = (process.env.ACC_SOLO || '').split(',').filter(Boolean);
const IDIOMA = process.env.ACC_LANG || 'es';
mkdirSync(OUT, { recursive: true });

const PRODUCTOS = [
  { id: 'docus', kind: 'document', ruta: '/dev/editor?kind=document' },
  { id: 'calcus', kind: 'spreadsheet', ruta: '/dev/editor?kind=spreadsheet' },
  { id: 'diapos', kind: 'presentation', ruta: '/dev/editor?kind=presentation' },
  { id: 'certipdf', kind: 'pdf', ruta: '/dev/pdf' },
  // La pantalla de abrir un fichero (sin PDF): lo que ve quien llega sin documento.
  { id: 'portada', kind: 'portada', ruta: '/dev/pdf?portada=1' },
].filter((p) => SOLO.length === 0 || SOLO.includes(p.id));
const ANCHOS = [
  { w: 1280, h: 800, movil: false },
  { w: 390, h: 844, movil: true },
  { w: 320, h: 640, movil: true },
];
const ESQUEMAS = ['light', 'dark'];
/** La URL del banco de pruebas de un producto, en el idioma de la auditoría. */
const urlDe = (prod) => `${BASE}${prod.ruta}${prod.ruta.includes('?') ? '&' : '?'}lang=${IDIOMA}`;
const ETIQUETAS_AXE = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];
const GRAVES = new Set(['serious', 'critical']);
const LADO_TACTIL = 44;

/* ---------- Informe ---------- */
const fallos = [];
const menores = new Map(); // «regla · producto» → { ayuda, impacto, ocurrencias, donde }
const resumen = { axe: 0, teclado: 0, zoom: 0, tactil: 0, lector: 0, scroll: 0 };
const falla = (area, ctx, msg) => {
  resumen[area] += 1;
  fallos.push(`[${area}] ${ctx}: ${msg}`);
  console.log(`  ✗ ${ctx} [${area}] ${msg}`);
};
const ok = (ctx, msg) => console.log(`  ✓ ${ctx} ${msg}`);

/* ---------- Datos de mentira ---------- */

const HTML_DOC =
  '<h1>Informe trimestral</h1><p>Texto de ejemplo con <strong>negrita</strong> y un ' +
  '<a href="https://sytue.eu">enlace</a>.</p><ul><li>Primero</li><li>Segundo</li></ul>' +
  '<table><tbody><tr><td>A</td><td>B</td></tr><tr><td>1</td><td>2</td></tr></tbody></table>';
const HOJA = JSON.stringify({
  active: 0,
  sheets: [
    {
      name: 'Ventas',
      cells: { A1: 'Mes', B1: 'Importe', A2: 'Enero', B2: '120', A3: 'Febrero', B3: '135', B4: '=SUM(B2:B3)' },
    },
  ],
});
const DIAPOS = JSON.stringify({
  slides: [
    {
      id: 's1',
      boxes: [
        { id: 'b1', x: 8, y: 10, w: 84, h: 20, kind: 'text', html: '<h1>Título de la presentación</h1>' },
        { id: 'b2', x: 8, y: 40, w: 84, h: 40, kind: 'text', html: '<p>Un párrafo de ejemplo.</p>' },
      ],
    },
    { id: 's2', boxes: [{ id: 'b3', x: 8, y: 10, w: 84, h: 20, kind: 'text', html: '<h1>Segunda</h1>' }] },
  ],
});
const CONTENIDO = { document: HTML_DOC, spreadsheet: HOJA, presentation: DIAPOS };
const CLAVE = { document: 'doc:html', spreadsheet: 'doc:sheet', presentation: 'doc:slides' };

const json = (route, cuerpo, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(cuerpo) });

/** Contesta a la API del editor (BFF) con un documento de ejemplo del tipo pedido. */
async function simularEditor(page, kind) {
  let seq = 0;
  await page.route('**/auth/session', (r) =>
    json(r, { authenticated: true, user: { id: 'demo-user', name: 'Demo', email: 'demo@sytue.test', tenant: 'prueba' } }),
  );
  await page.route(/\/api\/(documents|state|ops|presence|collab|comments|versions|rename|people|sharing|star|edit-requests|llamada|blocks)(\?|$)/, async (route) => {
    const u = new URL(route.request().url());
    const metodo = route.request().method();
    const que = u.pathname.split('/').pop();
    if (que === 'documents') {
      return json(route, {
        document: { id: 'demo', kind, title: 'Documento de ejemplo', owner_sub: 'demo-user' },
        role: 'owner',
      });
    }
    if (que === 'state') {
      return json(route, {
        seq,
        state: {
          seq,
          lamport: 1,
          elements: null,
          registers: { [CLAVE[kind]]: { value: CONTENIDO[kind], lamport: 1, dot: { a: 'x', c: 1 } } },
        },
      });
    }
    if (que === 'ops') return json(route, metodo === 'POST' ? { seq: ++seq, ops: [] } : { ops: [] });
    if (que === 'presence') return json(route, { members: [] });
    if (que === 'collab') return json(route, { error: 'sin_ws' }, 503);
    if (que === 'comments') return json(route, { comments: [] });
    if (que === 'versions') return json(route, { versions: [] });
    // Lo que la cabecera y la cinta piden al montarse: sin contestarlo, el servidor de
    // desarrollo (sin sesión) responde 401 y el grupo «Bloques» no se pinta ni se audita.
    if (que === 'llamada') return json(route, { available: false });
    if (que === 'blocks') return json(route, { blocks: [{ key: 'firmas', name: 'Tabla de firmas' }], org_blocks: [] });
    return json(route, {});
  });
}

const SVG_PAGINA = (n) =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 595 842" width="595" height="842">` +
  `<rect width="595" height="842" fill="#fff"/><text x="50" y="100" font-size="24" fill="#111">Página ${n} de ejemplo</text></svg>`;

/** Contesta a la API de CertiPDF con un PDF de dos páginas. */
async function simularPdf(page) {
  await page.route('**/auth/session', (r) =>
    json(r, { authenticated: true, user: { id: 'demo-user', name: 'Demo', email: 'demo@sytue.test', tenant: 'prueba' } }),
  );
  await page.route(/\/api\/pdf(-[a-z]+)?(\?|$)/, async (route) => {
    const u = new URL(route.request().url());
    const que = u.pathname.split('/').pop();
    if (que === 'pdf-firmas') return json(route, { sent: [], received: [] });
    if (que === 'pdf-recientes')
      return json(route, {
        items: [{ name: 'Contrato de ejemplo.pdf', path: '/PDF/Contrato de ejemplo.pdf', modifiedAt: '2026-09-01T10:00:00Z', size: 120000 }],
      });
    if (que !== 'pdf') return json(route, {});
    const cuerpo = JSON.parse(route.request().postData() || '{}');
    if (cuerpo.op === 'contenido') {
      const pag = (n) => ({ numero: n, ancho: 595, alto: 842, rotacion: 0, caja: [0, 0, 595, 842], extracto: `Página ${n} de ejemplo` });
      return json(route, {
        paginas: [pag(1), pag(2)],
        total: 2,
        indice: null,
        enlaces: null,
        adjuntos: null,
        anotaciones: [],
        campos: [],
        firmas: [],
        cifrado: false,
        permisos: {
          Imprimir: true, Modificar: true, Copiar: true, Anotar: true, RellenarCampos: true,
          Accesibilidad: true, Ensamblar: true, ImprimirEnAlta: true, SoloConClaveDeDueno: false,
        },
        huella: {
          Autor: 'Demo', Creador: '', Productor: '', Titulo: 'Ejemplo', Asunto: '', Palabras: '',
          Creado: '', Modificado: '', ConXMP: false, ConAdjuntos: false, ConJS: false,
        },
      });
    }
    if (cuerpo.op === 'render') return json(route, { paginas: { 1: SVG_PAGINA(1), 2: SVG_PAGINA(2) }, total: 2 });
    if (cuerpo.op === 'texto') return json(route, { paginas: [[{ texto: 'Página 1 de ejemplo', x: 50, y: 742, tamano: 24 }], null] });
    return json(route, {});
  });
}

/* ---------- Utilidades de medida (se ejecutan en la página) ---------- */

const ENFOCABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"]),[role="tab"],[role="button"]';

/** Controles visibles de cabecera, cinta y barra de estado con su medida. */
const medirObjetivos = () => {
  const sel =
    'button,a[href],input:not([type=hidden]),select,[role="tab"],[role="button"],[role="menuitem"],[role="switch"],[role="checkbox"],[role="radio"]';
  const visible = (e) => {
    const r = e.getBoundingClientRect();
    const cs = getComputedStyle(e);
    return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none';
  };
  // Un control a medio desplazar en la banda (su ancho se corta con el borde del
  // contenedor) sigue siendo el mismo botón: se mide su caja, no lo que asoma.
  const fuera = [];
  for (const e of document.querySelectorAll(sel)) {
    if (!visible(e) || e.closest('[aria-hidden="true"]') || e.closest('.sr-only')) continue;
    // Un enlace dentro de un párrafo («en línea») queda exento en WCAG 2.5.8; aquí solo
    // hay controles de cromo.
    const r = e.getBoundingClientRect();
    if (r.width < 44 || r.height < 44) {
      fuera.push({
        nombre: (e.getAttribute('aria-label') || e.textContent || e.getAttribute('title') || e.tagName).trim().slice(0, 40),
        rol: e.getAttribute('role') || e.tagName.toLowerCase(),
        w: Math.round(r.width),
        h: Math.round(r.height),
        zona: e.closest('[data-ribbon]')
          ? 'cinta'
          : e.closest('header,[data-editor-header]')
            ? 'cabecera'
            : e.closest('[data-editor-main]')
              ? 'lienzo'
              : 'otro',
      });
    }
  }
  return fuera;
};

async function esperarEditor(page, producto) {
  if (producto.kind === 'portada') {
    await page.waitForSelector('main button', { timeout: 60000 });
    await page.waitForTimeout(600);
    return;
  }
  if (producto.kind === 'pdf') {
    // El visor entero (cinta y hojas), no el esqueleto de «Abriendo el documento…».
    await page.waitForSelector('[role="tablist"]', { timeout: 60000 });
    await page.waitForSelector('[data-pagina]', { timeout: 60000 });
    await page.waitForTimeout(600);
    return;
  }
  await page.waitForSelector('[role="tablist"]', { timeout: 90000 });
  await page.waitForSelector('[data-editor-main]', { timeout: 30000 });
  await page.waitForTimeout(500);
}

async function abrirHerramientas(page) {
  // En <768 px la fila de grupos empieza plegada: el botón de herramientas la abre.
  const b = page.locator('button[aria-controls="ribbon-tabpanel"][aria-expanded]');
  if ((await b.count()) && (await b.first().isVisible())) {
    if ((await b.first().getAttribute('aria-expanded')) === 'false') await b.first().click();
  }
}

/* ---------- axe ---------- */

/**
 * Excepciones de WCAG 2.5.8 que axe no conoce, cada una con la prueba de que se cumple.
 * NO es una lista de reglas apagadas: una excepción solo vale mientras su `equivalente`
 * siga existiendo, y la comprobación falla si desaparece.
 *
 * Los tres marcadores de sangría de la regla de Docus (14 × 9 px) son un atajo de
 * puntero: la sangría izquierda, la derecha y la de primera línea se fijan también con
 * campos numéricos del panel de ajustes, operables con teclado, voz y dedo. Es la
 * excepción «Equivalente» de 2.5.8 (otro control da la misma función).
 */
const EXENCIONES = [
  {
    regla: 'target-size',
    nodo: (n) => /role="slider"/.test(n.html) && /aria-orientation="horizontal"/.test(n.html),
    motivo: 'marcadores de sangría de la regla: equivalente en el panel de ajustes (2.5.8, «Equivalente»)',
    // Los campos del panel de ajustes que hacen lo mismo que los marcadores.
    equivalente: async (page, t) => {
      const abrir = page.locator(`button[aria-label="${t('view.settingsPanel')}"]`);
      if ((await page.locator(`[aria-label="${t('settings.indent.left')}"],label:has-text("${t('settings.indent.left')}")`).count()) === 0) {
        await page.locator('[role="tab"]').last().click();
        if (await abrir.count()) await abrir.first().click();
        await page.waitForTimeout(200);
      }
      const cuenta = async (clave) =>
        (await page.getByLabel(t(clave), { exact: false }).count()) > 0;
      return (await cuenta('settings.indent.left')) && (await cuenta('settings.indent.right')) && (await cuenta('settings.indent.firstLine'));
    },
  },
  {
    // Las cajas de una diapositiva son CONTENIDO a escala: con 320 px de pantalla y la cinta
    // abierta el escenario mide 83 px de ancho y una caja de texto, 70 × 9. Su tamaño es el
    // de la diapositiva (excepción «Esencial» de 2.5.8, como la celda de una hoja), y se
    // agrandan con el zoom de la barra de estado. La excepción vale mientras ese zoom exista.
    regla: 'target-size',
    nodo: (n) => /data-slidebox=/.test(n.html),
    motivo: 'cajas de la diapositiva: contenido a escala (2.5.8, «Esencial»), se agrandan con el zoom de la barra de estado',
    equivalente: async (page, t) => (await page.getByLabel(t('status.zoom'), { exact: true }).count()) > 0,
  },
];
const exentosVistos = new Set();
const vistos = new Set(); // regla + sitio ya contados en este contexto: una pestaña más no es otro hallazgo

async function pasarAxe(page, ctx, producto, etiqueta) {
  let res;
  try {
    res = await new AxeBuilder({ page }).withTags(ETIQUETAS_AXE).analyze();
  } catch (e) {
    falla('axe', ctx, `${etiqueta}: axe no pudo analizar (${String(e.message).slice(0, 120)})`);
    return;
  }
  for (const v of res.violations) {
    let nodos = v.nodes;
    for (const ex of EXENCIONES) {
      if (ex.regla !== v.id) continue;
      const exentos = nodos.filter(ex.nodo);
      if (exentos.length === 0) continue;
      if (!exentosVistos.has(`${ctx}|${ex.regla}`)) {
        exentosVistos.add(`${ctx}|${ex.regla}`);
        const hay = await ex.equivalente(page, (k) => es[k]);
        if (!hay) falla('axe', ctx, `${etiqueta}: ${v.id} exento (${ex.motivo}) pero el equivalente ya no existe`);
        else console.log(`  · ${ctx} exento ${v.id}: ${exentos.length} nodos, ${ex.motivo}`);
      }
      nodos = nodos.filter((n) => !ex.nodo(n));
    }
    if (nodos.length === 0) continue;
    const donde = nodos
      .slice(0, 3)
      .map((n) => n.target.join(' '))
      .join(' | ');
    if (GRAVES.has(v.impact)) {
      const clave = `${ctx}|${v.id}|${donde}`;
      if (vistos.has(clave)) continue;
      vistos.add(clave);
      falla('axe', ctx, `${etiqueta}: ${v.id} (${v.impact}, ${nodos.length}) ${v.help} → ${donde}`);
    } else {
      const k = `${v.id} · ${producto.id}`;
      const previo = menores.get(k);
      if (previo) previo.n += nodos.length;
      else menores.set(k, { n: nodos.length, impacto: v.impact, ayuda: v.help, donde });
    }
  }
}

/* ---------- Teclado ---------- */

/** ¿Tiene el elemento con foco un indicador visible (contorno o sombra)? */
const focoVisible = () => {
  const e = document.activeElement;
  if (!e || e === document.body) return { ok: false, que: 'body' };
  const cs = getComputedStyle(e);
  const contorno = cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0 && cs.outlineColor !== 'rgba(0, 0, 0, 0)';
  const sombra = cs.boxShadow !== 'none' && !/^rgba?\(0, 0, 0, 0\)/.test(cs.boxShadow);
  // El anillo de la casa (.sytue-focus-ring) pinta el foco con ::after / ::before.
  const pseudo = ['::after', '::before'].some((p) => {
    const ps = getComputedStyle(e, p);
    return ps.content !== 'none' && (ps.boxShadow !== 'none' || (ps.outlineStyle !== 'none' && parseFloat(ps.outlineWidth) > 0) || parseFloat(ps.borderTopWidth) > 0);
  });
  return {
    ok: contorno || sombra || pseudo,
    que: `${e.tagName.toLowerCase()}${e.getAttribute('role') ? `[${e.getAttribute('role')}]` : ''} ${(e.getAttribute('aria-label') || e.textContent || '').trim().slice(0, 30)}`,
  };
};

/** El anillo de foco de la casa es un `box-shadow` y casi todos los controles llevan
 *  `transition`: justo tras recibir el foco vale 0, y medirlo entonces da un falso «sin foco».
 *  Se espera lo que dura la transición más larga (150 ms) y se salta la barra de
 *  desarrollo de Astro, que no es de la página. */
async function focoTrasTransicion(page) {
  await page.waitForTimeout(200);
  return page.evaluate(focoVisible);
}

async function pruebasDeTeclado(page, ctx, producto) {
  if (producto.kind === 'portada') return;
  if (producto.kind === 'pdf') return pruebasDeTecladoPdf(page, ctx);
  // 1. Tab desde el principio llega a una pestaña de la cinta en pocas pulsaciones y cada
  //    parada tiene foco visible.
  await page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : null));
  await page.mouse.click(2, 2).catch(() => undefined);
  await page.evaluate(() => document.body.focus());
  let llegoATab = false;
  const sinFoco = [];
  const paradas = [];
  for (let i = 0; i < 40; i++) {
    await page.keyboard.press('Tab');
    const f = await focoTrasTransicion(page);
    const rol = await page.evaluate(() => document.activeElement?.getAttribute('role'));
    paradas.push(f.que);
    if (!f.ok) sinFoco.push(f.que);
    if (rol === 'tab') {
      llegoATab = true;
      break;
    }
  }
  if (!llegoATab) falla('teclado', ctx, `Tab no llega a una pestaña de la cinta en 40 pulsaciones (${paradas.slice(0, 6).join(' > ')})`);
  else ok(ctx, `Tab llega a la cinta en ${paradas.length} pulsaciones`);
  if (sinFoco.length) falla('teclado', ctx, `sin foco visible: ${[...new Set(sinFoco)].slice(0, 4).join(' | ')}`);

  // 2. Flechas del tablist: mueven la selección.
  const antes = await page.evaluate(() => document.activeElement?.id);
  await page.keyboard.press('ArrowRight');
  const despues = await page.evaluate(() => ({ id: document.activeElement?.id, sel: document.activeElement?.getAttribute('aria-selected') }));
  if (despues.id === antes || despues.sel !== 'true') falla('teclado', ctx, 'ArrowRight no mueve la pestaña activa de la cinta');
  await page.keyboard.press('End');
  const fin = await page.evaluate(() => document.activeElement?.id);
  if (fin === despues.id) falla('teclado', ctx, 'End no lleva a la última pestaña');
  await page.keyboard.press('Home');
  const ini = await page.evaluate(() => document.activeElement?.id);
  if (ini !== 'ribbon-tab-inicio') falla('teclado', ctx, `Home no lleva a «Inicio» (${ini})`);

  // 3. Tab desde la pestaña entra en la banda (UNA parada) y las flechas la recorren.
  //    Su primer control puede ser un desplegable (la fuente), que se queda las flechas
  //    para él; las flechas se prueban sobre el primer BOTÓN.
  await abrirHerramientas(page);
  await page.keyboard.press('Tab');
  const enBanda = await page.evaluate(() => !!document.activeElement?.closest('#ribbon-banda'));
  if (!enBanda) falla('teclado', ctx, 'Tab desde las pestañas no entra en la banda de grupos');
  else {
    const f0 = await focoTrasTransicion(page);
    if (!f0.ok) falla('teclado', ctx, `la parada de la banda no tiene foco visible: ${f0.que}`);
    const boton = page.locator('#ribbon-banda button:not([disabled])').first();
    await boton.focus();
    const a = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80));
    await page.keyboard.press('ArrowRight');
    const b = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80));
    if (a === b) falla('teclado', ctx, 'ArrowRight no recorre los controles de la banda');
    const f = await focoTrasTransicion(page);
    if (!f.ok) falla('teclado', ctx, `control de la banda sin foco visible: ${f.que}`);
    // Fin e Inicio se pulsan SIEMPRE desde el botón. La flecha puede haber dejado el foco
    // en un desplegable, y ahí esas teclas son suyas (la barra no las intercepta en un
    // campo): en Diapos cambiaban la disposición de la diapositiva. La prueba editaba el
    // documento sin querer, el autoguardado saltaba 1,5 s después y, al recargarse el
    // lienzo, el F6 de más abajo encontraba el foco en el <body> según la carga del runner.
    await boton.focus();
    await page.keyboard.press('End');
    const f2 = await focoTrasTransicion(page);
    if (!f2.ok) falla('teclado', ctx, `último control de la banda sin foco visible: ${f2.que}`);
    await boton.focus();
    await page.keyboard.press('Home');
    const f3 = await page.evaluate(() => !!document.activeElement?.closest('#ribbon-banda'));
    if (!f3) falla('teclado', ctx, 'Inicio saca el foco de la banda');
  }

  // 4. F6: banda → lienzo → pestaña.
  await page.keyboard.press('F6');
  const enLienzo = await page.evaluate(() => !!document.activeElement?.closest('[data-editor-main]'));
  if (!enLienzo) falla('teclado', ctx, 'F6 desde la cinta no lleva al lienzo');
  await page.keyboard.press('F6');
  const enTab = await page.evaluate(() => document.activeElement?.getAttribute('role') === 'tab');
  if (!enTab) falla('teclado', ctx, 'F6 desde el lienzo no lleva a la pestaña activa');

  // 5. Escape cierra un popover y devuelve el foco al disparador.
  const disparador = page.locator('#ribbon-banda button[aria-haspopup]:not([disabled])').first();
  if (await disparador.count()) {
    await disparador.scrollIntoViewIfNeeded();
    await disparador.focus();
    await page.keyboard.press('Enter');
    await page.waitForTimeout(150);
    const abierto = await page.evaluate(() => !!document.querySelector('.ribbon-popover'));
    if (!abierto) falla('teclado', ctx, 'Enter en un disparador no abre su popover');
    else {
      // El foco entra en el popover o se queda en el disparador; en ambos casos Escape cierra.
      await page.keyboard.press('Escape');
      await page.waitForTimeout(150);
      const cerrado = await page.evaluate(() => !document.querySelector('.ribbon-popover'));
      const foco = await page.evaluate(() => document.activeElement?.hasAttribute('aria-haspopup') ?? false);
      if (!cerrado) falla('teclado', ctx, 'Escape no cierra el popover');
      else if (!foco) falla('teclado', ctx, 'al cerrar el popover con Escape el foco no vuelve a su disparador');
      else ok(ctx, 'Escape cierra el popover y devuelve el foco');
    }
  }

  // 6. El menú «Archivo»: se abre con Enter, las flechas recorren, Escape devuelve el foco.
  const archivo = page.locator('button[aria-haspopup="menu"]').first();
  if (await archivo.count()) {
    await archivo.focus();
    await page.keyboard.press('Enter');
    await page.waitForTimeout(100);
    const dentro = await page.evaluate(() => document.activeElement?.getAttribute('role') === 'menuitem');
    if (!dentro) falla('teclado', ctx, 'el menú Archivo no pasa el foco a su primera opción');
    await page.keyboard.press('Escape');
    const vuelve = await page.evaluate(() => document.activeElement?.getAttribute('aria-haspopup') === 'menu');
    if (!vuelve) falla('teclado', ctx, 'Escape en el menú Archivo no devuelve el foco al botón');
  }
}

async function pruebasDeTecladoPdf(page, ctx) {
  const sinFoco = [];
  let paradas = 0;
  await page.evaluate(() => document.body.focus());
  for (let i = 0; i < 40; i++) {
    await page.keyboard.press('Tab');
    const f = await focoTrasTransicion(page);
    paradas++;
    if (!f.ok && f.que !== 'body' && !f.que.startsWith('astro-dev-toolbar')) sinFoco.push(f.que);
  }
  if (sinFoco.length) falla('teclado', ctx, `${sinFoco.length} paradas de Tab sin foco visible: ${[...new Set(sinFoco)].slice(0, 5).join(' | ')}`);
  else ok(ctx, `${paradas} paradas de Tab, todas con foco visible`);
  // La barra de herramientas del visor: flechas.
  const barra = page.locator('[role="toolbar"]').first();
  if (await barra.count()) {
    await barra.locator(ENFOCABLE).first().focus();
    const a = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 60));
    await page.keyboard.press('ArrowRight');
    const b = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 60));
    if (a === b) falla('teclado', ctx, 'las flechas no recorren la barra de herramientas del visor');
  } else {
    falla('teclado', ctx, 'el visor no tiene una barra de herramientas con role="toolbar"');
  }
}

/* ---------- Lector de pantalla ---------- */

async function pruebasDeLector(page, ctx, producto) {
  const r = await page.evaluate(() => {
    const vivas = [...document.querySelectorAll('[aria-live],[role="status"],[role="alert"]')].map((e) => ({
      live: e.getAttribute('aria-live') || (e.getAttribute('role') === 'alert' ? 'assertive' : 'polite'),
      texto: (e.textContent || '').trim().slice(0, 40),
    }));
    const sinNombre = [...document.querySelectorAll('[role="tablist"],[role="toolbar"],[role="tabpanel"],[role="menu"],[role="dialog"],nav,aside')]
      .filter((e) => !(e.getAttribute('aria-label') || e.getAttribute('aria-labelledby')))
      .map((e) => e.getAttribute('role') || e.tagName.toLowerCase());
    const imgSinAlt = [...document.querySelectorAll('img:not([alt])')].length;
    const btnSinNombre = [...document.querySelectorAll('button,[role="button"],[role="tab"],a[href]')]
      .filter((e) => {
        const r = e.getBoundingClientRect();
        if (r.width === 0 || r.height === 0 || e.closest('[aria-hidden="true"]')) return false;
        return !((e.getAttribute('aria-label') || e.getAttribute('aria-labelledby') || e.getAttribute('title') || e.textContent || '').trim());
      })
      .map((e) => e.outerHTML.slice(0, 80));
    return { vivas, sinNombre, imgSinAlt, btnSinNombre, lang: document.documentElement.lang, titulo: document.title };
  });
  if (r.vivas.length === 0 && producto.kind !== 'portada') falla('lector', ctx, 'no hay ninguna región aria-live/status/alert: el guardado y los errores no se anuncian');
  if (r.sinNombre.length) falla('lector', ctx, `regiones sin nombre accesible: ${[...new Set(r.sinNombre)].join(', ')}`);
  if (r.imgSinAlt) falla('lector', ctx, `${r.imgSinAlt} <img> sin alt`);
  if (r.btnSinNombre.length) falla('lector', ctx, `controles sin nombre accesible: ${r.btnSinNombre.slice(0, 3).join(' | ')}`);
  if (!r.lang) falla('lector', ctx, 'el documento no declara idioma');
  if (producto.kind !== 'pdf' && producto.kind !== 'portada' && r.vivas.length > 0) {
    const guardado = await page.evaluate(() => [...document.querySelectorAll('[aria-live],[role="status"]')].some((e) => /guard|saved|gespeichert|enregistr/i.test(e.textContent || '')));
    if (!guardado) falla('lector', ctx, 'ninguna región viva dice el estado de guardado');
  }
}

/**
 * Un fallo de guardado se ANUNCIA: con el servicio caído (POST /api/ops → 500) la edición
 * deja el texto «Error al guardar» en una región viva (el chip de la cabecera es
 * `aria-live`, y el aviso un `role="alert"`). Un lector de pantalla que no oye el fallo
 * sigue escribiendo sobre un documento que no se guarda.
 */
async function pruebaDeErrorDeGuardado(page, ctx, producto) {
  if (producto.kind !== 'document') return;
  await page.route(/\/api\/ops(\?|$)/, (r) =>
    r.request().method() === 'POST' ? json(r, { error: 'caido' }, 500) : json(r, { ops: [] }),
  );
  const lienzo = page.locator('[data-editor-main] [contenteditable="true"]').first();
  if (!(await lienzo.count())) return falla('lector', ctx, 'no hay lienzo editable para probar el aviso de guardado');
  await lienzo.click();
  await page.keyboard.type('x');
  // El autoguardado espera 1500 ms; se sondea en vez de dormir un tiempo fijo, que con el
  // runner cargado se quedaba corto (1 de 9 pasadas en el frontal).
  const anunciado = await page
    .waitForFunction(
      () =>
        [...document.querySelectorAll('[aria-live],[role="status"],[role="alert"]')].some((e) =>
          /error|no se (pudo|ha podido)|fall/i.test(e.textContent || ''),
        ),
      undefined,
      { timeout: 15000 },
    )
    .then(() => true)
    .catch(() => false);
  if (!anunciado) falla('lector', ctx, 'un fallo de guardado no se anuncia en ninguna región viva');
  else ok(ctx, 'un fallo de guardado se anuncia en una región viva');
}

/* ---------- Principal ---------- */

const navegador = await chromium.launch();

// Calentamiento: la primera carga de `astro dev` compila y optimiza dependencias.
{
  const p = await navegador.newPage();
  for (const prod of PRODUCTOS) {
    if (prod.kind === 'pdf' || prod.kind === 'portada') await simularPdf(p);
    else await simularEditor(p, prod.kind);
    await p.goto(urlDe(prod), { timeout: 240000 }).catch(() => undefined);
    await esperarEditor(p, prod).catch(() => undefined);
    await p.unrouteAll({ behavior: 'ignoreErrors' });
  }
  await p.close();
}

// Los productos en paralelo: son independientes y cada combinación usa su propio contexto.
// En serie, cuatro productos × seis combinaciones × ~10 pasadas de axe pasan de la hora.
await Promise.all(
  PRODUCTOS.map(async (prod) => {
  for (const esquema of ESQUEMAS) {
    for (const { w, h, movil } of ANCHOS) {
      const id = `${prod.id}-${esquema}-${w}`;
      const ctxB = await navegador.newContext({
        viewport: { width: w, height: h },
        hasTouch: movil,
        isMobile: movil,
        colorScheme: esquema,
        locale: IDIOMA,
      });
      const page = await ctxB.newPage();
      const errores = [];
      page.on('pageerror', (e) => errores.push(e.message));
      if (prod.kind === 'pdf' || prod.kind === 'portada') await simularPdf(page);
      else await simularEditor(page, prod.kind);
      const url = urlDe(prod);
      try {
        await page.goto(url, { timeout: 120000 });
        await esperarEditor(page, prod);
      } catch (e) {
        falla('lector', id, `la página no llega a pintar el editor: ${String(e.message).slice(0, 120)}`);
        await page.screenshot({ path: `${OUT}/${id}-error.png` }).catch(() => undefined);
        await ctxB.close();
        continue;
      }

      // 0. El documento no desborda en horizontal.
      const doc = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, iw: window.innerWidth }));
      if (doc.sw > doc.iw) falla('zoom', id, `el documento desborda: ${doc.sw} > ${doc.iw}`);

      // 1. axe sobre la página y con cada pestaña de la cinta.
      await abrirHerramientas(page);
      await pasarAxe(page, id, prod, 'página');
      if (prod.kind !== 'pdf' && prod.kind !== 'portada') {
        const tabs = page.locator('[role="tab"]');
        const n = await tabs.count();
        for (let i = 0; i < n; i++) {
          const nombre = (await tabs.nth(i).innerText()).trim();
          await tabs.nth(i).click();
          await abrirHerramientas(page);
          await page.waitForTimeout(120);
          await pasarAxe(page, id, prod, `pestaña «${nombre}»`);
        }
        await tabs.first().click();
      }

      // 2. Lector de pantalla (una vez por producto y ancho, en claro).
      if (esquema === 'light') await pruebasDeLector(page, id, prod);

      // 3. Teclado (a escritorio, en claro) y el aviso de un fallo de guardado.
      if (esquema === 'light' && !movil) await pruebasDeTeclado(page, id, prod);
      if (esquema === 'light' && !movil) await pruebaDeErrorDeGuardado(page, id, prod);

      // 4. Objetivos táctiles (a móvil, en claro).
      if (movil && esquema === 'light') {
        await abrirHerramientas(page);
        const chicos = (await page.evaluate(medirObjetivos)).filter((x) => x.zona !== 'otro' || prod.kind === 'pdf' || prod.kind === 'portada');
        const mias = chicos.filter((x) => x.w < LADO_TACTIL || x.h < LADO_TACTIL);
        if (mias.length) {
          const resumenZonas = {};
          for (const x of mias) resumenZonas[x.zona] = (resumenZonas[x.zona] || 0) + 1;
          falla(
            'tactil',
            id,
            `${mias.length} controles por debajo de ${LADO_TACTIL} px (${JSON.stringify(resumenZonas)}): ` +
              mias.slice(0, 40).map((x) => `${x.nombre || x.rol} ${x.w}×${x.h}`).join(' | '),
          );
        } else ok(id, `todos los controles miden ≥ ${LADO_TACTIL} px`);
      }

      if (errores.length) falla('lector', id, `errores de página: ${errores.slice(0, 2).join(' | ')}`);
      await page.screenshot({ path: `${OUT}/${id}.png` });
      await ctxB.close();
    }
  }

  // 5. Zoom al 200 %: a 1280 de ventana, 640 px CSS (y la mitad de alto). Se mide también
  //    a 400 % (320 px), que es el listón de reflujo de WCAG 1.4.10, cubierto por el ancho
  //    de 320 de arriba.
  for (const esquema of ESQUEMAS) {
    const id = `${prod.id}-${esquema}-zoom200`;
    const ctxB = await navegador.newContext({ viewport: { width: 640, height: 400 }, colorScheme: esquema, locale: IDIOMA });
    const page = await ctxB.newPage();
    if (prod.kind === 'pdf' || prod.kind === 'portada') await simularPdf(page);
    else await simularEditor(page, prod.kind);
    try {
      await page.goto(urlDe(prod), { timeout: 120000 });
      await esperarEditor(page, prod);
    } catch (e) {
      falla('zoom', id, `la página no pinta a 640 px: ${String(e.message).slice(0, 120)}`);
      await ctxB.close();
      continue;
    }
    const doc = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, iw: window.innerWidth }));
    if (doc.sw > doc.iw) falla('zoom', id, `con zoom al 200 % el documento desborda: ${doc.sw} > ${doc.iw}`);
    else ok(id, 'sin desbordamiento horizontal al 200 %');
    await pasarAxe(page, id, prod, 'zoom 200 %');
    await page.screenshot({ path: `${OUT}/${id}.png` });
    await ctxB.close();
  }
  }),
);

await pruebasDeInicio(navegador, { BASE, OUT, SOLO, IDIOMA, falla, ok });
await navegador.close();

const listaMenores = [...menores.entries()].map(([k, v]) => ({ regla: k, impacto: v.impacto, nodos: v.n, ayuda: v.ayuda, donde: v.donde }));
writeFileSync(`${OUT}/informe.json`, JSON.stringify({ resumen, fallos, menores: listaMenores }, null, 2));
console.log('\n--- Menores (moderate/minor), no bloquean ---');
for (const m of listaMenores) console.log(` · ${m.regla} [${m.impacto}] ×${m.nodos}: ${m.ayuda} → ${m.donde}`);
console.log(`\n${fallos.length === 0 ? 'TODO EN VERDE' : `${fallos.length} FALLOS ${JSON.stringify(resumen)}`}`);
for (const f of fallos) console.log(` - ${f}`);
process.exit(fallos.length === 0 ? 0 : 1);
