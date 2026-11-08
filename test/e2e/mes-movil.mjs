// SPDX-License-Identifier: Apache-2.0
// El mes compacto del teléfono de Calendar, con los componentes REALES y el CSS que
// genera Tailwind, sin servidor ni sesión: 390×844 a ×3, táctil, en oscuro y en claro, y
// el cajón abierto. Afirma la geometría y lo que se lee, y guarda las PNG en
// MES_MOVIL_OUT para compararlas con la captura del dueño (y con las de Fyne).
//
// Dos bancos: el de los componentes (mes-movil.tsx) y la APP entera (mes-movil-app.tsx,
// CalendarApp con la API contestada aquí): la integración —sin cabecera ni botón
// flotante, la barra y el aviso en AppShell, `n`/`p` con el paso de lo que se ve, nada de
// escritorio en el primer pintado—, el selector del cajón en los 26 idiomas y el título
// largo a 320.
//
//   tailwindcss -i apps/frontend/apps/calendar/src/styles/app.css -o "$S/mes-movil.css"
//   MES_MOVIL_JS="$S/mes-movil.js" MES_MOVIL_APP_JS="$S/mes-movil-app.js" \
//     bun run test/e2e/compilar-mes-movil.mjs
//   MES_MOVIL_CSS=… MES_MOVIL_JS=… MES_MOVIL_APP_JS=… MES_MOVIL_OUT=… node test/e2e/mes-movil.mjs
//
// Playwright sale de test/e2e/node_modules en el CI y, en local, del global
// (NODE_PATH=/opt/node22/lib/node_modules): no es una dependencia nueva.
import { createRequire } from 'node:module';
import { mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const { chromium } = createRequire(import.meta.url)('playwright');
const css = readFileSync(process.env.MES_MOVIL_CSS || '/tmp/mes-movil.css', 'utf8');
const js = readFileSync(process.env.MES_MOVIL_JS || '/tmp/mes-movil.js', 'utf8');
const appJs = readFileSync(process.env.MES_MOVIL_APP_JS || '/tmp/mes-movil-app.js', 'utf8');
const out = process.env.MES_MOVIL_OUT || '/tmp/mes-movil';
mkdirSync(out, { recursive: true });

let fallos = 0;
function exigir(condicion, mensaje) {
  if (condicion) return;
  fallos++;
  console.error(`✗ ${mensaje}`);
}

const browser = await chromium.launch();

async function abrir(tema, viewport = { width: 390, height: 844 }) {
  const movil = viewport.width < 640;
  const page = await browser.newPage({
    viewport,
    deviceScaleFactor: movil ? 3 : 1,
    hasTouch: movil,
    isMobile: movil,
  });
  const errores = [];
  page.on('pageerror', (e) => errores.push(e.message));
  // El bundle lee las cookies de idioma: about:blank tiene origen opaco. La navegación
  // se resuelve en memoria, sin servidor ni tráfico externo.
  const url = 'http://mes-movil.sytue.test/';
  await page.route(url, (route) =>
    route.fulfill({
      contentType: 'text/html',
      body: `<!doctype html><html lang="es" class="h-full" data-theme="${tema}" data-color="app" data-app="calendar"><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body class="bg-bg text-text h-full antialiased"></body></html>`,
    }),
  );
  await page.goto(url);
  await page.addStyleTag({ content: css });
  await page.addScriptTag({ content: js, type: 'module' });
  try {
    await page.locator('[data-banco-mes]').waitFor({ timeout: 10000 });
  } catch (error) {
    throw new Error(`no se montó el banco: ${errores.join('; ') || error.message}`);
  }
  exigir(errores.length === 0, `errores del banco: ${errores.join('; ')}`);
  return page;
}

for (const [tema, nombre] of [
  ['dark', 'oscuro'],
  ['light', 'claro'],
]) {
  const page = await abrir(tema);
  const m = await page.evaluate(() => {
    const rejilla = document.querySelector('[role="group"][id^="sytue-mes-"]');
    const filas = [...rejilla.children];
    const dia = (n) =>
      [...rejilla.querySelectorAll('button')].find((b) => b.textContent.trim() === String(n));
    const seis = dia(6).firstElementChild;
    const prim = getComputedStyle(document.documentElement).getPropertyValue('--sytue-primary');
    const muestra = document.createElement('span');
    muestra.style.backgroundColor = prim;
    document.body.appendChild(muestra);
    const primario = getComputedStyle(muestra).backgroundColor;
    muestra.remove();
    const barra = document.querySelector('.sytue-mobilebar');
    return {
      filas: filas.map((f) => Math.round(f.getBoundingClientRect().height)),
      semana: Math.round(filas[0].firstElementChild.getBoundingClientRect().width),
      numeros: filas.map((f) => f.firstElementChild.textContent.replace(/\D+.*$/, '')),
      ancho: document.documentElement.scrollWidth,
      fondoSeis: getComputedStyle(seis).backgroundColor,
      primario,
      puntos2: dia(2).querySelectorAll('[data-punto]').length,
      puntos9: dia(9).querySelectorAll('[data-punto]').length,
      titulo: barra.querySelector('h1')?.textContent,
      colorTitulo: getComputedStyle(barra.querySelector('h1')).color,
      colorBarra: getComputedStyle(barra).color,
      lista: document.querySelector('section h2')?.textContent,
      filasLista: [...document.querySelectorAll('section li')].map((li) => li.innerText),
      barraDisplay: getComputedStyle(barra).display,
    };
  });
  exigir(
    m.filas.length === 6 && m.filas.every((h) => h === 40),
    `${nombre}: seis filas de 40 (${m.filas})`,
  );
  exigir(m.semana === 44, `${nombre}: columna de semana de 44 (${m.semana})`);
  exigir(m.numeros.join() === '44,45,46,47,48,49', `${nombre}: semanas 44…49 (${m.numeros})`);
  exigir(m.ancho <= 390, `${nombre}: sin desborde horizontal (${m.ancho})`);
  exigir(
    m.fondoSeis === m.primario,
    `${nombre}: el 6 en primario (${m.fondoSeis} ≠ ${m.primario})`,
  );
  exigir(
    m.puntos2 === 1 && m.puntos9 === 2,
    `${nombre}: puntos 1 y 2 (${m.puntos2}, ${m.puntos9})`,
  );
  exigir(m.titulo === 'noviembre', `${nombre}: título «noviembre» (${m.titulo})`);
  // El título lleva el color de la barra: en «Marco» la barra es el marco de color, y un
  // `text-text` ahí era tinta oscura sobre el primario.
  exigir(
    m.colorTitulo === m.colorBarra,
    `${nombre}: el título con el color de la barra (${m.colorTitulo} ≠ ${m.colorBarra})`,
  );
  exigir(
    m.lista === 'viernes, 6 de noviembre de 2026',
    `${nombre}: título de la lista (${m.lista})`,
  );
  exigir(
    m.filasLista.length === 1 && /14:00 – 15:00\s+Tenis/.test(m.filasLista[0]),
    `${nombre}: la fila de Tenis (${JSON.stringify(m.filasLista)})`,
  );
  exigir(
    m.barraDisplay === 'grid',
    `${nombre}: la barra móvil es la rejilla de tres columnas (${m.barraDisplay})`,
  );
  await page.screenshot({ path: join(out, `calendar-mes-movil-${nombre}.png`) });

  // El asa: un toque pliega a la semana, Fin la lleva a la agenda, y arrastrarla mueve la
  // hoja con el dedo. Cada posición se mide (alto de la hoja, filas a la vista, lo de
  // debajo) y se fotografía, y también a medio arrastre.
  const asa = page.getByRole('slider', { name: 'Cambiar entre mes, semana y agenda' });
  const hoja = () =>
    page.evaluate(() => {
      const s = document.querySelector('[role="slider"]');
      const caja = s.previousElementSibling.getBoundingClientRect();
      return {
        alto: Math.round(caja.height),
        asaY: Math.round(s.getBoundingClientRect().top),
        valor: s.getAttribute('aria-valuetext'),
        filas: document.querySelectorAll('[id^="sytue-mes-"] > div:not([inert])').length,
        lista: document.querySelector('section h2')?.textContent ?? null,
        buscador: document.querySelector('input[type="search"]') !== null,
        cajon: [...document.querySelectorAll('[role="group"] [aria-pressed="true"]')]
          .map((b) => b.textContent)
          .find((x) => ['Mes', 'Semana', 'Día', 'Agenda'].includes(x)),
      };
    });
  // La hoja se anima 200 ms al llegar a su sitio.
  const asentar = () => page.waitForTimeout(300);
  await asa.click();
  await asentar();
  const sem = await hoja();
  exigir(
    sem.valor === 'Semana' && sem.alto === 64 && sem.filas === 1 && sem.cajon === 'Semana',
    `${nombre}: la semana (${JSON.stringify(sem)})`,
  );
  exigir(sem.lista === 'viernes, 6 de noviembre de 2026', `${nombre}: semana con la lista`);
  await page.screenshot({ path: join(out, `calendar-asa-semana-${nombre}.png`) });
  await asa.press('End');
  await asentar();
  const age = await hoja();
  exigir(
    age.valor === 'Agenda' && age.alto === 0 && age.buscador && age.lista === null,
    `${nombre}: la agenda (${JSON.stringify(age)})`,
  );
  exigir(age.cajon === 'Agenda', `${nombre}: el selector del cajón marca la agenda`);
  await page.screenshot({ path: join(out, `calendar-asa-agenda-${nombre}.png`) });
  await asa.press('Home');
  await asentar();
  // A medio arrastre, del mes hacia la semana: 100 px hacia arriba, sin soltar.
  const caja = await asa.boundingBox();
  const cx = caja.x + caja.width / 2;
  const cy = caja.y + caja.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) {
    await page.mouse.move(cx, cy - 10 * i);
    await page.waitForTimeout(30);
  }
  const medio = await hoja();
  exigir(
    medio.alto === 164 && medio.filas === 6,
    `${nombre}: a medio arrastre la hoja sigue al dedo (${JSON.stringify(medio)})`,
  );
  await page.screenshot({ path: join(out, `calendar-asa-arrastrando-${nombre}.png`) });
  // Soltar quieto a 144: más cerca de la semana.
  await page.mouse.move(cx, cy - 120);
  await page.waitForTimeout(150);
  await page.mouse.up();
  await asentar();
  const suelta = await hoja();
  exigir(suelta.valor === 'Semana' && suelta.alto === 64, `${nombre}: soltar a 144 lleva a la semana`);
  await asa.press('Home');
  await asentar();

  if (tema === 'dark') {
    // El cajón, con el bloque de vista y el aviso de pendientes en ☰.
    const menu = page.getByRole('button', { name: 'Abrir menú, 3 pendientes' });
    exigir((await menu.count()) === 1, 'el ☰ dice «Abrir menú, 3 pendientes»');
    await menu.click();
    await page.getByRole('button', { name: 'Recordatorios' }).click();
    await page.waitForTimeout(250); // el galón de la rama gira en 150 ms
    await page.screenshot({ path: join(out, `calendar-mes-movil-cajon-${nombre}.png`) });
  }
  await page.close();
}

// ---- La APP entera (CalendarApp con la API contestada aquí) ----

const LOCALES = [
  'en',
  'bg',
  'cs',
  'da',
  'de',
  'el',
  'es',
  'et',
  'fi',
  'fr',
  'ga',
  'hr',
  'hu',
].concat(['is', 'it', 'lt', 'lv', 'mt', 'nb', 'nl', 'pl', 'pt', 'ro', 'sk', 'sl', 'sv']);
const evApi = (d, h, summary) => ({
  ical: '',
  href: `/cal/${d}-${h}.ics`,
  uid: `${d}-${h}`,
  summary,
  start: `202611${String(d).padStart(2, '0')}T${String(h).padStart(2, '0')}0000`,
  end: `202611${String(d).padStart(2, '0')}T${String(h + 1).padStart(2, '0')}0000`,
});
const EVENTOS_API = [
  ...[2, 3, 10, 13, 17, 19, 20, 21, 23, 24, 26, 27, 30].map((d) => evApi(d, 10, 'Reunión')),
  evApi(6, 14, 'Tenis'),
  evApi(9, 9, 'Dentista'),
  evApi(9, 18, 'Cena'),
  evApi(16, 9, 'Revisión'),
  evApi(16, 17, 'Clase'),
];

async function abrirApp({ l = 'es', w = 390, h = 844, tema = 'dark' } = {}) {
  const movil = w < 640;
  const ctx = await browser.newContext({
    viewport: { width: w, height: h },
    deviceScaleFactor: movil ? 3 : 1,
    hasTouch: movil,
    isMobile: movil,
    locale: l,
    timezoneId: 'Europe/Madrid',
  });
  const page = await ctx.newPage();
  await page.clock.setFixedTime(new Date('2026-11-06T19:36:00+01:00'));
  // El tema de la app sale de su preferencia guardada, no del `data-theme` del HTML.
  await page.addInitScript((t) => {
    try {
      localStorage.setItem('sytue-theme', t);
    } catch {
      // sin almacenamiento, el tema por defecto
    }
  }, tema);
  const errores = [];
  page.on('pageerror', (e) => errores.push(e.message));
  const base = 'http://mes-movil.sytue.test/';
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.origin + u.pathname === base) {
      return route.fulfill({
        contentType: 'text/html',
        body: `<!doctype html><html lang="${l}" class="h-full" data-theme="${tema}" data-color="app" data-app="calendar"><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body class="bg-bg text-text h-full antialiased"></body></html>`,
      });
    }
    const json = (o) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(o) });
    if (u.pathname === '/api/calendars') {
      return json({
        calendars: [
          { id: 'c1', url: '/cal/', displayName: 'Personal', color: '#3d8fd6', isDefault: true },
        ],
      });
    }
    if (u.pathname === '/api/events') {
      // Lo del rango pedido, como el servicio: la agenda pide [cursor, +30 días). Las
      // horas del banco son flotantes y el rango va en UTC; para un banco, basta.
      const desde = (u.searchParams.get('start') ?? '').slice(0, 15);
      const hasta = (u.searchParams.get('end') ?? '99999999T999999').slice(0, 15);
      return json({ events: EVENTOS_API.filter((e) => e.start >= desde && e.start < hasta) });
    }
    if (u.pathname === '/api/invitations') {
      return json({ invitations: [{ id: 'i1', summary: 'X', status: 'NEEDS-ACTION' }] });
    }
    if (u.pathname === '/api/reminders') return json({ reminders: [], next_cursor: '' });
    if (u.pathname === '/api/shared') return json({ calendars: [] });
    return json({});
  });
  await page.goto(`${base}?l=${l}`);
  // La marca de la organización en la topbar de la Sytue Shell es un SVG en línea sin
  // tamaño propio que en el banco (sin el CSS de la Shell) mide 29 000 px de ancho. No es
  // de lo que se prueba aquí: se encaja en su cuadrado para que no desborde la página.
  await page.addStyleTag({
    content: `${css}\n.sytue-esquina-marca span.inline-flex{max-width:28px;overflow:hidden;color:transparent}`,
  });
  await page.addScriptTag({ content: appJs, type: 'module' });
  try {
    await page.waitForFunction(
      (compacto) =>
        compacto
          ? document.querySelectorAll('[id^="sytue-mes-"] [data-punto]').length > 0
          : document.querySelector('header[data-scroll-x]') !== null &&
            !document.querySelector('[role="status"][aria-busy="true"]'),
      movil,
      { timeout: 15000 },
    );
  } catch (error) {
    throw new Error(`no se cargó la app (${l}, ${w}): ${errores.join('; ') || error.message}`);
  }
  exigir(errores.length === 0, `app ${l}: errores ${errores.join('; ')}`);
  return { page, ctx };
}

let t0Tam = '';
const titulo = (page) =>
  page.evaluate(() => {
    // En la app, la barra del teléfono es la topbar de la suite (barra única).
    const h1 = document.querySelector('header[role="banner"] [data-centrado] h1');
    return {
      texto: h1?.textContent,
      cabe: h1 ? h1.scrollWidth <= h1.clientWidth : false,
      tam: h1 ? getComputedStyle(h1).fontSize : '',
    };
  });

{
  // 390 en español: la integración.
  const { page, ctx } = await abrirApp();
  const m = await page.evaluate(() => ({
    primer: window.__primerPintado,
    cabecera: document.querySelector('header[data-scroll-x]') !== null,
    fab: [...document.querySelectorAll('button.fixed')].some(
      (b) => getComputedStyle(b).display !== 'none',
    ),
    menu: document
      .querySelector('header[role="banner"] button[aria-controls]')
      ?.getAttribute('aria-label'),
    barras: document.querySelectorAll('header[role="banner"], .sytue-mobilebar').length,
    ancho: document.documentElement.scrollWidth,
  }));
  exigir(
    !m.primer.cabecera && !m.primer.esqueletoMes && !m.primer.fab,
    `app: el primer pintado a 390 enseña escritorio (${JSON.stringify(m.primer)})`,
  );
  exigir(!m.cabecera, 'app: en el mes del teléfono no hay cabecera de vista');
  exigir(!m.fab, 'app: en el mes del teléfono no hay botón flotante');
  exigir(m.menu === 'Abrir menú, 1 pendiente', `app: el ☰ avisa de lo pendiente (${m.menu})`);
  exigir(m.ancho <= 390, `app: sin desborde horizontal (${m.ancho})`);
  // Una sola barra: la de 44 de AppShell ya no se pinta debajo de la topbar.
  exigir(m.barras === 1, `app: una sola barra arriba (${m.barras})`);
  const t0 = await titulo(page);
  t0Tam = t0.tam;
  exigir(t0.texto === 'noviembre' && t0.cabe, `app: el título (${JSON.stringify(t0)})`);
  await page.screenshot({ path: join(out, 'calendar-app-mes-movil-oscuro.png') });
  // `n`/`p`: un mes, y con el asa en la semana, una semana (el paso de deslizar).
  await page.keyboard.press('n');
  exigir((await titulo(page)).texto === 'diciembre', 'app: «n» pasa a diciembre');
  await page.keyboard.press('p');
  const asaApp = page.getByRole('slider', { name: 'Cambiar entre mes, semana y agenda' });
  await asaApp.click();
  // El oyente de teclas se vuelve a enlazar en un efecto, que corre tras pintar.
  await page.waitForTimeout(200);
  await page.keyboard.press('n');
  const elegido = await page.evaluate(
    () =>
      document
        .querySelector('[id^="sytue-mes-"] [aria-pressed="true"]')
        ?.getAttribute('aria-label') ?? '',
  );
  exigir(/13 de noviembre/.test(elegido), `app: en la semana, «n» avanza una semana (${elegido})`);
  // Con un día ENFOCADO, los atajos siguen y el foco va con el elegido (cada día lleva
  // `key` de su fecha: sin seguirlo, `n` lo dejaba en <body> y las flechas no iban).
  const foco = () =>
    page.evaluate(() => {
      const a = document.activeElement;
      return a?.getAttribute('aria-pressed') === 'true' ? a.getAttribute('aria-label') : a?.tagName;
    });
  const pulsar = async (tecla, quiero, donde) => {
    await page.keyboard.press(tecla);
    await page.waitForTimeout(100);
    const f = await foco();
    exigir(quiero.test(f ?? ''), `app: ${donde}, «${tecla}» deja el foco en ${f}`);
  };
  await page.locator('[id^="sytue-mes-"] [aria-pressed="true"]').focus();
  await pulsar('n', /20 de noviembre/, 'en la semana y con foco');
  await pulsar('ArrowRight', /21 de noviembre/, 'en la semana y con foco');
  // `w` y `a` mueven el asa (la vista ES su posición); `m` la devuelve al mes.
  await page.keyboard.press('a');
  await page.waitForTimeout(300);
  const valor = await asaApp.getAttribute('aria-valuetext');
  exigir(valor === 'Agenda', `app: «a» lleva el asa a la agenda (${valor})`);
  await page.screenshot({ path: join(out, 'calendar-app-asa-agenda-oscuro.png') });
  await page.keyboard.press('m');
  await page.waitForTimeout(300);
  await page.locator('[id^="sytue-mes-"] [aria-pressed="true"]').focus();
  await page.waitForTimeout(200);
  await pulsar('n', /21 de diciembre/, 'con foco');
  await pulsar('t', /6 de noviembre/, 'con foco');
  await pulsar('ArrowDown', /13 de noviembre/, 'con foco');
  await page.screenshot({ path: join(out, 'calendar-app-foco-tras-atajo-oscuro.png') });
  await ctx.close();
}

{
  // 1280: la forma de escritorio sí se pinta (que el banco la sabe ver).
  const { page, ctx } = await abrirApp({ w: 1280, h: 800 });
  const primer = await page.evaluate(() => window.__primerPintado);
  exigir(
    primer.cabecera && primer.esqueletoMes,
    `app 1280: primer pintado (${JSON.stringify(primer)})`,
  );
  await ctx.close();
}

// El selector de vista del cajón en los 26 idiomas: ni se sale del cajón ni trunca.
for (const l of LOCALES) {
  const { page, ctx } = await abrirApp({ l });
  if (l === 'el') {
    const t = await titulo(page);
    exigir(t.texto === 'Νοέμβριος', `app el: el título en nominativo (${t.texto})`);
    await page.screenshot({ path: join(out, 'calendar-app-mes-movil-el-oscuro.png') });
  }
  await page.locator('header[role="banner"] button[aria-controls]').first().tap();
  await page.waitForTimeout(350);
  const g = await page.evaluate(() => {
    const grupo = [...document.querySelectorAll('[role="group"]')].find(
      (x) => !x.id && x.querySelector('[aria-pressed]'),
    );
    const cajon = grupo?.closest('[id^="sytue-app-nav-"]');
    if (!grupo || !cajon) return null;
    const botones = [...grupo.querySelectorAll('button')];
    return {
      cabe: grupo.scrollWidth <= grupo.clientWidth,
      dentro:
        Math.round(grupo.getBoundingClientRect().right) <=
        Math.round(cajon.getBoundingClientRect().right),
      truncados: botones.filter((b) => b.scrollWidth > b.clientWidth).map((b) => b.textContent),
      n: botones.length,
    };
  });
  exigir(
    g && g.n === 4 && g.cabe && g.dentro && g.truncados.length === 0,
    `cajón ${l}: el selector de vista ${JSON.stringify(g)}`,
  );
  if (['de', 'fi', 'el', 'bg', 'lt'].includes(l)) {
    await page.screenshot({ path: join(out, `calendar-app-cajon-${l}-oscuro.png`) });
  }
  await ctx.close();
}

// El título largo a 320 (iPhone SE): septiembre de 2027 entero, sin perder el año.
//
// Solo con idiomas cuyos datos de ICU trae el Chromium de Playwright: el irlandés no
// viene, Intl caía a otro locale y lo que se medía era «2027 M09» o «September 2027», no
// «Meán Fómhair 2027». Se exige antes de medir (Fyne genera sus tablas con el ICU
// completo de Bun y no tiene ese hueco). El largo que cabe es el finés: «syyskuu 2027».
for (const l of ['el', 'ro', 'de', 'fi', 'hu', 'lt']) {
  const { page, ctx } = await abrirApp({ l, w: 320, h: 640 });
  const icu = await page.evaluate(
    (x) => new Intl.DateTimeFormat(x, { month: 'long' }).resolvedOptions().locale,
    l,
  );
  if (!icu.startsWith(l)) {
    exigir(false, `320 ${l}: este Chromium no trae ICU de «${l}» (resuelve «${icu}»)`);
    await ctx.close();
    continue;
  }
  for (let i = 0; i < 10; i++) await page.keyboard.press('n');
  const t = await titulo(page);
  exigir(t.cabe && /2027/.test(t.texto ?? ''), `320 ${l}: el título entero (${JSON.stringify(t)})`);
  // El griego no cabe ni en el hueco en `text-md` y baja a `text-sm`; el finés cabe y se
  // queda en su talla. Era el rumano hasta la barra única: la entrada a la suite (44 px a
  // la derecha) le quitó el sitio, y «septembrie 2027» baja a `text-sm` entero, con el año.
  if (l === 'el') exigir(t.tam !== t0Tam, `320 el: baja de talla (${t.tam})`);
  if (l === 'fi') exigir(t.tam === t0Tam, `320 fi: conserva la talla (${t.tam})`);
  if (l === 'el' || l === 'ro') {
    await page.screenshot({ path: join(out, `calendar-app-sep2027-${l}-320.png`) });
  }
  await ctx.close();
}

await browser.close();
if (fallos > 0) {
  console.error(`mes-movil: ${fallos} comprobación(es) fallida(s)`);
  process.exit(1);
}
console.log(`mes-movil: todo en orden; capturas en ${out}`);
