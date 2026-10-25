// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Geometría REAL de los Modal/Popover compartidos: 320/375/390 px, alto normal y
// ventana baja, Marco/Carril. Usa el mismo servidor DEV sin backend que cinta.mjs.
// Comprueba que campos y acciones completos se alcanzan desplazando, no solo que
// el panel tiene una caja dentro de la pantalla. Una autocomprobación reintroduce
// el recorte para demostrar que el guardián lo detecta.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const BASE = process.env.ACC_URL || 'http://127.0.0.1:4338/docus/prueba';
const OUT = createReportDirectory('superposiciones', process.env.ACC_OUT ? `${process.env.ACC_OUT}/superposiciones` : undefined);
const fallos = [];
const medidas = [];
const compositores = [];
const falla = (ctx, mensaje) => {
  fallos.push({ ctx, mensaje });
  console.log(`  ✗ ${ctx}: ${mensaje}`);
};

/** Se mide tras la animación, cuando el panel tiene su tamaño definitivo. */
function medirPanel(el) {
  const r = el.getBoundingClientRect();
  const vv = window.visualViewport;
  return {
    left: r.left,
    top: r.top,
    right: r.right,
    bottom: r.bottom,
    ancho: r.width,
    alto: r.height,
    viewport: {
      left: vv?.offsetLeft ?? 0,
      top: vv?.offsetTop ?? 0,
      ancho: vv?.width ?? innerWidth,
      alto: vv?.height ?? innerHeight,
    },
    desbordaX: el.scrollWidth > el.clientWidth + 1,
    // Lo largo se recorre DENTRO del panel: el propio panel (el popover, o el diálogo
    // cuando ni el cuerpo cabe) o una zona suya con scroll. El diálogo de #730 deja
    // fijas la cabecera y el pie y desplaza solo el cuerpo; un contenido cortado sin
    // ningún scroll sigue siendo rojo.
    desplazaY:
      el.scrollHeight > el.clientHeight + 1 ||
      [...el.querySelectorAll('*')].some((d) => {
        const { overflowY } = getComputedStyle(d);
        return (overflowY === 'auto' || overflowY === 'scroll') && d.scrollHeight > d.clientHeight + 1;
      }),
  };
}

function defectosPanel(m) {
  const errores = [];
  if (
    m.left < m.viewport.left - 1 ||
    m.top < m.viewport.top - 1 ||
    m.right > m.viewport.left + m.viewport.ancho + 1 ||
    m.bottom > m.viewport.top + m.viewport.alto + 1
  )
    errores.push('el panel se sale del viewport');
  if (m.desbordaX) errores.push('el contenido del panel desborda horizontalmente');
  return errores;
}

/** Caja visible = viewport ∩ TODOS los ancestros que recortan, no solo el body. */
function medirControl(el) {
  const r = el.getBoundingClientRect();
  const vv = window.visualViewport;
  const visible = {
    left: vv?.offsetLeft ?? 0,
    top: vv?.offsetTop ?? 0,
    right: (vv?.offsetLeft ?? 0) + (vv?.width ?? innerWidth),
    bottom: (vv?.offsetTop ?? 0) + (vv?.height ?? innerHeight),
  };
  for (let padre = el.parentElement; padre; padre = padre.parentElement) {
    const cs = getComputedStyle(padre);
    const p = padre.getBoundingClientRect();
    if (cs.overflowX !== 'visible') {
      visible.left = Math.max(visible.left, p.left + padre.clientLeft);
      visible.right = Math.min(visible.right, p.left + padre.clientLeft + padre.clientWidth);
    }
    if (cs.overflowY !== 'visible') {
      visible.top = Math.max(visible.top, p.top + padre.clientTop);
      visible.bottom = Math.min(visible.bottom, p.top + padre.clientTop + padre.clientHeight);
    }
  }
  const tocado = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
  return {
    left: r.left,
    top: r.top,
    right: r.right,
    bottom: r.bottom,
    ancho: r.width,
    alto: r.height,
    visible,
    completo:
      r.width > 0 &&
      r.height > 0 &&
      r.left >= visible.left - 1 &&
      r.top >= visible.top - 1 &&
      r.right <= visible.right + 1 &&
      r.bottom <= visible.bottom + 1,
    pulsable: tocado === el || el.contains(tocado),
    radio: parseFloat(getComputedStyle(el).borderTopLeftRadius),
  };
}

const navegador = await chromium.launch();
let autocomprobacionModal = false;
let autocomprobacionPopover = false;
let autocomprobacionCompositor = false;
try {
  for (const ancho of [320, 375, 390]) {
    for (const alto of [320, 640]) {
      for (const forma of ['marco', 'carril']) {
        for (const modo of ['center', 'right', 'sheet', 'popover']) {
          const ctx = `${modo}-${forma}-${ancho}x${alto}`;
          const antes = fallos.length;
          const contexto = await navegador.newContext({
            viewport: { width: ancho, height: alto },
            hasTouch: true,
            isMobile: true,
          });
          const page = await contexto.newPage();
          page.on('pageerror', (e) => falla(ctx, e.message));
          try {
            const viewportVisual =
              modo === 'popover' && ancho === 375 && alto === 640 && forma === 'marco';
            if (viewportVisual)
              await page.addInitScript(() => {
                // Solo se simula la API de teclado de Safari: el layout y todas las
                // cajas se siguen midiendo en Chromium real. Cambiar el tamaño de
                // la página no reproduce su viewport visual desplazado.
                const vv = Object.assign(new EventTarget(), {
                  width: innerWidth,
                  height: innerHeight,
                  offsetLeft: 0,
                  offsetTop: 0,
                });
                Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv });
              });
            await page.goto(`${BASE}/dev/superposiciones?modo=${modo}&forma=${forma}`);
            const panel = page.locator('[role="dialog"]');
            await panel.waitFor({ timeout: 180000 });
            await page.evaluate(() => document.fonts.ready);
            await page.waitForTimeout(220);
            if (viewportVisual) {
              await page.evaluate(() => {
                Object.assign(window.visualViewport, {
                  width: innerWidth - 24,
                  height: 256,
                  offsetLeft: 12,
                  offsetTop: 72,
                });
                window.visualViewport.dispatchEvent(new Event('resize'));
                window.visualViewport.dispatchEvent(new Event('scroll'));
              });
              await page.waitForTimeout(50);
            }

            if (modo === 'center' && !autocomprobacionModal) {
              const estilo = await page.addStyleTag({
                content:
                  '[role="dialog"][aria-modal="true"] > :last-child {flex-wrap:nowrap!important}',
              });
              // justify-end expulsa el primer botón por la IZQUIERDA: scrollWidth
              // solo denuncia el exceso por la derecha. Usar la misma medición
              // de recorte y alcance que vigila cada acción del caso real.
              const accion = panel.locator('[data-demo-action]').first();
              await accion.scrollIntoViewIfNeeded();
              const caja = await accion.evaluate(medirControl);
              if (caja.completo && caja.pulsable)
                falla(
                  ctx,
                  `el guardián no detecta el pie sin reflujo de la autocomprobación: ${JSON.stringify(caja)}`,
                );
              await estilo.evaluate((el) => el.remove());
              autocomprobacionModal = true;
            }
            if (modo === 'popover' && !autocomprobacionPopover) {
              const estilo = await page.addStyleTag({
                content: '[role="dialog"] {max-height:none!important}',
              });
              if (
                !defectosPanel(await panel.evaluate(medirPanel)).includes(
                  'el panel se sale del viewport',
                )
              )
                falla(
                  ctx,
                  'el guardián no detecta el panel sin tope de alto de la autocomprobación',
                );
              await estilo.evaluate((el) => el.remove());
              autocomprobacionPopover = true;
            }

            const m = await panel.evaluate(medirPanel);
            medidas.push({ ctx, ...m });
            for (const defecto of defectosPanel(m)) falla(ctx, `${defecto}: ${JSON.stringify(m)}`);
            if (!m.desplazaY)
              falla(ctx, 'la muestra larga debe poder desplazarse dentro del panel');

            const comprobar = async (control, nombre) => {
              await control.scrollIntoViewIfNeeded();
              const caja = await control.evaluate(medirControl);
              if (!caja.completo || !caja.pulsable) {
                falla(
                  ctx,
                  `${nombre} queda recortado o tapado tras desplazar: ${JSON.stringify(caja)}`,
                );
                return false;
              }
              return true;
            };
            const campos = panel.locator('[data-demo-field]');
            for (let i = 0; i < (await campos.count()); i++) {
              const campo = campos.nth(i);
              if (await comprobar(campo, `campo ${i + 1}`)) await campo.fill(`Field ${i + 1}`);
            }
            const acciones = panel.locator('[data-demo-action]');
            // En el Popover basta el primero y el último: llegar al último obliga a
            // desplazar su contenido y además demuestra que ese scroll no lo cierra.
            const indices =
              modo === 'popover'
                ? [0, (await acciones.count()) - 1]
                : Array.from({ length: await acciones.count() }, (_, i) => i);
            for (const i of indices) {
              const accion = acciones.nth(i);
              const id = await accion.getAttribute('data-demo-action');
              if (!(await comprobar(accion, `acción ${id}`))) continue;
              const caja = await accion.evaluate(medirControl);
              if (caja.ancho < 44 || caja.alto < 44 || caja.radio !== 4)
                falla(
                  ctx,
                  `acción ${id}: se esperan objetivo 44px y radio 4px: ${JSON.stringify(caja)}`,
                );
              await accion.click();
              await page.waitForFunction(
                (valor) =>
                  document.querySelector('[data-demo-result]')?.textContent?.trim() === valor,
                id,
              );
            }
            // El cierre también debe seguir siendo alcanzable al volver al principio.
            if (modo !== 'popover') {
              const cerrar = panel.locator('button').first();
              if (await comprobar(cerrar, 'cerrar')) await cerrar.click();
            } else {
              await page.keyboard.press('Escape');
            }
            await panel.waitFor({ state: 'detached' });
          } catch (error) {
            falla(ctx, error.message);
          } finally {
            if (fallos.length > antes)
              await page.screenshot({ path: `${OUT}/${ctx}.png` }).catch(() => {});
            else console.log(`  ✓ ${ctx}: panel dentro, campos y acciones alcanzables`);
            await contexto.close();
          }
        }
      }
    }
  }
  // El compositor es el mismo en canal e hilo. A 1024px los dos laterales
  // dejan solo 412px centrales: el breakpoint de la ventana no basta.
  for (const ancho of [320, 375, 390, 1024]) {
    for (const forma of ['marco', 'carril']) {
      for (const hilo of [false, true]) {
        for (const tactil of [true, false]) {
          const ctx = `compositor-${hilo ? 'hilo' : 'canal'}-${forma}-${ancho}-${tactil ? 'tactil' : 'raton'}`;
          const antes = fallos.length;
          const contexto = await navegador.newContext({
            viewport: { width: ancho, height: ancho === 1024 ? 1366 : 640 },
            hasTouch: tactil,
            isMobile: tactil,
          });
          const page = await contexto.newPage();
          page.on('pageerror', (e) => falla(ctx, e.message));
          try {
            await page.addInitScript(() => localStorage.setItem('sytue-rooms-formato', '1'));
            await page.goto(
              `${BASE}/dev/superposiciones?modo=compositor&forma=${forma}&paneles=${ancho === 1024 ? 1 : 0}&hilo=${hilo ? 1 : 0}`,
            );
            if ((await page.evaluate(() => document.compatMode)) !== 'CSS1Compat')
              falla(ctx, 'el banco debe medir en modo estándar, no en quirks');
            const compositor = page.locator('[data-demo-composer]');
            const texto = compositor.getByRole('combobox', { name: 'Message', exact: true });
            await texto.waitFor({ timeout: 180000 });
            await page.evaluate(() => document.fonts.ready);
            await texto.fill('A message');
            const acciones = compositor.locator('[data-scroll-x]');
            const enviar = compositor.getByRole('button', { name: 'Send', exact: true });
            const caja = await compositor.boundingBox();
            if (Math.abs(caja.width - (ancho === 1024 ? 412 : ancho)) > 1)
              falla(ctx, `el banco no reproduce el ancho útil: ${JSON.stringify(caja)}`);

            if (!autocomprobacionCompositor && tactil && !hilo && ancho === 320) {
              const estilo = await page.addStyleTag({
                content:
                  '[data-demo-composer] [data-scroll-x] {overflow:visible!important;min-width:max-content!important}',
              });
              if ((await enviar.evaluate(medirControl)).completo)
                falla(ctx, 'el guardián no detecta el envío expulsado por una barra sin scroll');
              await estilo.evaluate((el) => el.remove());
              autocomprobacionCompositor = true;
            }

            const formato = acciones.getByRole('button', { name: 'Formatting', exact: true });
            await formato.click();
            if ((await acciones.getByRole('button').count()) !== (hilo ? 3 : 5))
              falla(ctx, 'la barra de formato no se recoge');
            await formato.click();
            const botones = acciones.getByRole('button');
            const cuenta = await botones.count();
            if (cuenta !== (hilo ? 10 : 12))
              falla(ctx, `faltan herramientas con el formato desplegado: ${cuenta}`);
            for (let i = 0; i < cuenta; i++) {
              const boton = botones.nth(i);
              await boton.scrollIntoViewIfNeeded();
              const m = await boton.evaluate(medirControl);
              const envio = await enviar.evaluate(medirControl);
              const min = tactil ? 44 : 24;
              if (
                !m.completo ||
                !m.pulsable ||
                !envio.completo ||
                !envio.pulsable ||
                m.right > envio.left + 1 ||
                m.bottom > caja.y + caja.height + 1 ||
                m.ancho < min ||
                m.alto < min ||
                envio.ancho < min ||
                envio.alto < min ||
                m.radio !== 4 ||
                envio.radio !== 4
              )
                falla(
                  ctx,
                  `herramienta ${i}/enviar recortados, solapados o pequeños: ${JSON.stringify({ m, envio })}`,
                );
            }
            const negrita = acciones.getByRole('button', { name: 'Bold', exact: true });
            await negrita.scrollIntoViewIfNeeded();
            await negrita.click();
            if (!hilo) {
              const programar = acciones.getByRole('button', { name: 'Send later', exact: true });
              await programar.scrollIntoViewIfNeeded();
              await programar.click();
              const panel = page.getByRole('dialog', { name: 'Send later', exact: true });
              await panel.waitFor();
              await page.waitForTimeout(220);
              for (const defecto of defectosPanel(await panel.evaluate(medirPanel)))
                falla(ctx, `popover del compositor: ${defecto}`);
              await page.keyboard.press('Escape');
              await panel.waitFor({ state: 'detached' });
            }
            const cuerpo = await texto.inputValue();
            const envio = await enviar.evaluate(medirControl);
            if (!envio.completo || !envio.pulsable)
              falla(
                ctx,
                `Enviar no permanece accesible al final de la tira: ${JSON.stringify(envio)}`,
              );
            if (forma === 'marco' && tactil && !hilo && (ancho === 320 || ancho === 1024))
              await page.screenshot({ path: `${OUT}/${ctx}.png` });
            await enviar.click();
            await page.waitForFunction(
              (valor) => document.querySelector('[data-demo-result]')?.textContent === valor,
              cuerpo,
            );
            compositores.push({ ctx, anchoUtil: caja.width, herramientas: cuenta, envio });
          } catch (error) {
            falla(ctx, error.message);
          } finally {
            if (fallos.length > antes)
              await page.screenshot({ path: `${OUT}/${ctx}.png` }).catch(() => {});
            else console.log(`  ✓ ${ctx}: herramientas desplazables y envío fijo alcanzables`);
            await contexto.close();
          }
        }
      }
    }
  }
} finally {
  await navegador.close();
}
if (medidas.length !== 48)
  falla('barrido', `se esperaban 48 combinaciones medidas y solo hay ${medidas.length}`);
if (!autocomprobacionModal || !autocomprobacionPopover)
  falla('autocomprobación', 'deben ejecutarse los dos controles negativos de recorte');
if (compositores.length !== 32 || !autocomprobacionCompositor)
  falla(
    'compositor',
    `se esperaban 32 combinaciones y el control negativo; hay ${compositores.length}, control=${autocomprobacionCompositor}`,
  );
writeReport(`${OUT}/informe.json`, JSON.stringify({ medidas, compositores, fallos }, null, 2));
console.log(
  `Superposiciones: ${medidas.length} combinaciones, ${compositores.length} compositores, ${fallos.length} fallos; autocomprobaciones modal=${autocomprobacionModal}, popover=${autocomprobacionPopover}, compositor=${autocomprobacionCompositor}`,
);
if (fallos.length) process.exitCode = 1;
