// SPDX-License-Identifier: Apache-2.0
// Contraste de los componentes REALES y del CSS generado por Tailwind en el runner.
// No levanta aplicaciones ni usa sesión. Incluye una mutación: quitar la exclusión
// del tooltip tiene que reproducir la tinta oscura sobre fondo oscuro de Marco.
import { chromium } from 'playwright';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createReportDirectory, writeReport } from './lib/report-output.mjs';

const css = readFileSync(process.env.MENUS_CSS || '/tmp/menus-traslucidos.css', 'utf8');
const js = readFileSync(process.env.MENUS_JS || '/tmp/menus-traslucidos.js', 'utf8');
const colores = readFileSync(new URL('../../branding/tokens/colors.css', import.meta.url), 'utf8');
const apps = [...new Set([...colores.matchAll(/\[data-app='([^']+)'\]/g)].map((m) => m[1]))];
const paletas = [
  { color: 'petroleo', app: 'cast' },
  { color: 'cobalto', app: 'cast' },
  ...apps.map((app) => ({ color: 'app', app })),
];
const out = createReportDirectory('menus-traslucidos', process.env.MENUS_OUT);
const browser = await chromium.launch();
let comprobaciones = 0;

function exigir(condicion, mensaje) {
  comprobaciones++;
  if (!condicion) throw new Error(mensaje);
}

async function abrir(hoja, tema, forma, color, app = 'cast', opciones = {}) {
  const page = await browser.newPage({
    viewport: opciones.viewport || { width: 1280, height: 900 },
    hasTouch: Boolean(opciones.touch),
    isMobile: Boolean(opciones.touch),
  });
  const errores = [];
  page.on('pageerror', (error) => errores.push(error.message));
  // El bundle lee las cookies reales de idioma: about:blank tiene origen opaco.
  // La navegación se resuelve en memoria, sin servidor ni tráfico externo.
  const url = `http://menus.sytue.test/${opciones.nav ? `?nav=${opciones.nav}` : ''}`;
  await page.route(url, (route) =>
    route.fulfill({
      contentType: 'text/html',
      body: `<!doctype html><html lang="es" data-theme="${tema}" data-forma="${forma}" data-color="${color}" data-app="${app}"><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body></body></html>`,
    }),
  );
  await page.goto(url);
  if (opciones.safe)
    await page.evaluate((safe) => {
      for (const [lado, valor] of Object.entries(safe)) {
        document.documentElement.style.setProperty(`--sytue-safe-${lado}`, `${valor}px`);
      }
    }, opciones.safe);
  await page.addStyleTag({ content: hoja });
  await page.addScriptTag({ content: js, type: 'module' });
  try {
    await page
      .locator(opciones.nav ? '[data-nav-fixture]' : '[data-contexto="tarjeta"]')
      .waitFor({ timeout: 10000 });
  } catch (error) {
    throw new Error(`no se montó la fixture: ${errores.join('; ') || error.message}`);
  }
  exigir(errores.length === 0, `error del bundle de la fixture: ${errores.join('; ')}`);
  return page;
}

async function opcionesDeshabilitadas(select) {
  return select.evaluate((el) => {
    const normal = getComputedStyle(el.querySelector('option[value="es"]')).color;
    const opciones = [
      ...el.querySelectorAll(
        'option[value="bloqueada"], optgroup[disabled], optgroup[disabled] option',
      ),
    ];
    return opciones.map((opcion) => ({
      deshabilitada: opcion.matches(':disabled'),
      tintaDistinta: getComputedStyle(opcion).color !== normal,
    }));
  });
}

/** Colores que el navegador PINTA, con las transparencias de los ancestros compuestas. */
async function medir(locator) {
  return locator.evaluate((el) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    const color = (valor) => {
      ctx.clearRect(0, 0, 1, 1);
      ctx.fillStyle = valor;
      ctx.fillRect(0, 0, 1, 1);
      const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data;
      return [r, g, b, a / 255];
    };
    const sobre = (a, b) => {
      const alfa = a[3] + b[3] * (1 - a[3]);
      if (alfa === 0) return [0, 0, 0, 0];
      return [...a.slice(0, 3).map((c, i) => (c * a[3] + b[i] * b[3] * (1 - a[3])) / alfa), alfa];
    };
    const luminancia = (c) => {
      const rgb = c.slice(0, 3).map((n) => {
        const s = n / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      });
      return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
    };
    const estilo = getComputedStyle(el);
    const propio = color(estilo.backgroundColor);
    let fondo = propio;
    for (let padre = el.parentElement; padre && fondo[3] < 1; padre = padre.parentElement) {
      fondo = sobre(fondo, color(getComputedStyle(padre).backgroundColor));
    }
    // La hoja del navegador es blanca si ningún ancestro ha pintado el fondo.
    fondo = sobre(fondo, [255, 255, 255, 1]);
    const tinta = sobre(color(estilo.color), fondo);
    const a = luminancia(tinta);
    const b = luminancia(fondo);
    return {
      alfa: propio[3],
      fondo: estilo.backgroundColor,
      tinta: estilo.color,
      razon: (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05),
    };
  });
}

async function legible(locator, nombre, opaco = false) {
  const m = await medir(locator);
  exigir(!opaco || m.alfa === 1, `${nombre}: fondo transparente (${m.fondo})`);
  exigir(m.razon >= 4.5, `${nombre}: ${m.razon.toFixed(2)}:1 (${m.tinta} sobre ${m.fondo})`);
}

async function paneles(page, nombre) {
  for (const superficie of await page
    .locator('.sytue-menu-surface, .sytue-popover-surface')
    .all()) {
    await legible(superficie, `${nombre}, superficie`, true);
    for (const texto of await superficie
      .locator('.text-text, .text-text-muted, .text-primary-text, .sytue-meta')
      .all()) {
      await legible(texto, `${nombre}, texto de opción`);
    }
  }
}

/** Fondo REAL de PNG: medir() no compone las imágenes de fondo del marco. */
async function pixelesNavegacion(page, png, datos) {
  return page.evaluate(
    async ({ png, datos }) => {
      const imagen = new Image();
      imagen.src = `data:image/png;base64,${png}`;
      await imagen.decode();
      const canvas = document.createElement('canvas');
      canvas.width = imagen.width;
      canvas.height = imagen.height;
      const ctx = canvas.getContext('2d', { willReadFrequently: true });
      ctx.drawImage(imagen, 0, 0);
      const pixel = (x, y) => [...ctx.getImageData(Math.floor(x), Math.floor(y), 1, 1).data];
      const muestra = (rect, y) => [pixel(rect.x + 4, y), pixel(rect.right - 5, y)];
      // El borde derecho recibe la sombra legítima de la hoja contigua. Dos
      // puntos interiores limpian esa interferencia sin tolerar otra diferencia:
      // barra y menú se comparan siempre en las MISMAS coordenadas horizontales.
      const ejeMarco = [datos.nav.x + 4, datos.nav.x + datos.nav.width / 2];
      const superior = ejeMarco.map((x) => pixel(x, datos.topbar.y + 2));
      const menu = ejeMarco.map((x) => pixel(x, datos.nav.y + 2));
      const seleccion = muestra(datos.active, datos.active.y + datos.active.height / 2);
      const pie = muestra(datos.nav, datos.footer.y + datos.footer.height / 2);
      const color = (css) => {
        const c = document.createElement('canvas');
        c.width = c.height = 1;
        const p = c.getContext('2d');
        p.fillStyle = css;
        p.fillRect(0, 0, 1, 1);
        return [...p.getImageData(0, 0, 1, 1).data];
      };
      const luminancia = (rgb) =>
        rgb
          .slice(0, 3)
          .map((v) => {
            const s = v / 255;
            return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
          })
          .reduce((s, v, i) => s + v * [0.2126, 0.7152, 0.0722][i], 0);
      const contraste = (css, fondos) =>
        fondos.map((fondo) => {
          const tinta = color(css);
          const sobre = tinta
            .slice(0, 3)
            .map((v, i) => (v * tinta[3]) / 255 + fondo[i] * (1 - tinta[3] / 255));
          const a = luminancia(sobre),
            b = luminancia(fondo);
          return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
        });
      return {
        ejeMarco,
        superior,
        menu,
        diferencias: menu.map((rgb, i) =>
          rgb.slice(0, 3).map((v, c) => Math.abs(v - superior[i][c])),
        ),
        texto: contraste(datos.color, seleccion),
        pie: contraste(datos.footerColor, pie),
        foco: contraste(datos.focusColor, menu),
      };
    },
    { png: png.toString('base64'), datos },
  );
}

async function navegacion() {
  const modos = ['light', 'dark', 'translucent'].map((tema) => ({ tema, forma: 'marco' }));
  modos.push({ tema: 'dark', forma: 'carril' });
  const casos = modos.flatMap((modo) => [
    { ...modo, tipo: 'desktop', width: 1280, height: 800 },
    { ...modo, tipo: 'mobile', width: 390, height: 844 },
    { ...modo, tipo: 'erp', width: 1280, height: 800 },
  ]);
  casos.push({
    tema: 'translucent',
    forma: 'marco',
    tipo: 'mobile',
    width: 640,
    height: 320,
    safe: { left: 44, right: 44, bottom: 21 },
  });
  const resultados = [];
  for (const caso of casos) {
    const id = `nav-${caso.tipo}-${caso.tema}-${caso.forma}-${caso.width}`;
    const resultado = { id, ...caso, ok: false };
    let page;
    try {
      page = await abrir(css, caso.tema, caso.forma, 'app', 'notes', {
        nav: caso.tipo === 'erp' ? 'erp' : 'app',
        viewport: { width: caso.width, height: caso.height },
        touch: caso.tipo === 'mobile',
        safe: caso.safe,
      });
      const drawer = page.locator('[data-nav-fixture] .sytue-nav-drawer');
      const toggle = page.locator('[data-nav-fixture] button[aria-controls]').first();
      if (caso.tipo !== 'desktop') await toggle.click();
      await drawer.scrollIntoViewIfNeeded(); // Stable: incluye la entrada animada real.
      const nav = caso.tipo === 'erp' ? drawer : drawer.locator('nav.bg-sidebar');
      const active = nav.getByRole('button', { name: 'Archivo', exact: true });
      await active.click();
      if (caso.tipo !== 'desktop') await toggle.click();
      await drawer.scrollIntoViewIfNeeded();
      exigir(
        (await active.getAttribute('aria-current')) === 'page',
        `${id}: no cambió la selección`,
      );
      exigir((await nav.locator('.sytue-nav-current').count()) === 1, `${id}: selección duplicada`);
      await nav.getByRole('button', { name: 'Mi cuaderno', exact: true }).focus();
      const idle = await active.evaluate((el) => getComputedStyle(el).boxShadow);
      await active.focus();
      await active.press('Tab');
      await page.keyboard.press('Shift+Tab');
      const datos = await active.evaluate((el) => {
        const nav = el.closest('nav');
        const drawer = el.closest('.sytue-nav-drawer');
        const frame = el.closest('.sytue-frame');
        const topbar = document.querySelector('.sytue-topbar-carril');
        const footer = nav.querySelector('.sytue-sidebar-footer');
        const textoPie = nav.querySelector('[data-nav-footer]');
        const rect = (e) => e.getBoundingClientRect().toJSON();
        const fondo = getComputedStyle(
          drawer.classList.contains('sytue-drawer-in') ? drawer : frame,
        );
        // CSSOM puede conservar calc()/porcentajes; que el navegador resuelva el eje.
        const eje = document.createElement('span');
        eje.style.cssText = 'position:fixed;width:0;height:0;pointer-events:none';
        eje.style.left = fondo.backgroundPositionX;
        document.body.append(eje);
        const offset = eje.getBoundingClientRect().x;
        eje.remove();
        const estilo = getComputedStyle(el);
        const linea = getComputedStyle(footer, '::before');
        return {
          nav: rect(nav),
          active: rect(el),
          footer: rect(textoPie),
          topbar: rect(topbar),
          color: estilo.color,
          footerColor: getComputedStyle(textoPie).color,
          focusColor: estilo.getPropertyValue('--sytue-focus-ring'),
          focusVisible: el.matches(':focus-visible'),
          shadow: estilo.boxShadow,
          background: {
            image: fondo.backgroundImage,
            size: fondo.backgroundSize,
            x: fondo.backgroundPositionX,
            offset,
          },
          topbarImage: getComputedStyle(topbar).backgroundImage,
          footerLine: {
            border: getComputedStyle(footer).borderTopWidth,
            content: linea.content,
            width: linea.borderTopWidth,
            left: linea.left,
            right: linea.right,
          },
          coarse: matchMedia('(pointer:coarse)').matches,
        };
      });
      resultado.datos = datos;
      const safe = { left: 0, right: 0, bottom: 0, ...caso.safe };
      exigir(
        datos.nav.x === safe.left && datos.nav.right <= caso.width - safe.right,
        `${id}: el menú invade el área segura horizontal`,
      );
      exigir(
        datos.nav.y === datos.topbar.bottom && datos.nav.bottom <= caso.height - safe.bottom,
        `${id}: el menú invade la barra o el área segura inferior`,
      );
      exigir(
        datos.footer.bottom <= datos.nav.bottom && datos.footer.y >= datos.nav.y,
        `${id}: pie fuera del menú`,
      );
      exigir(
        datos.focusVisible && datos.shadow !== idle && /0px 0px 0px 2px/.test(datos.shadow),
        `${id}: selección oculta el aro de foco de2px (${datos.shadow})`,
      );
      if (caso.tipo === 'mobile')
        exigir(datos.coarse && datos.active.height >= 44, `${id}: entrada táctil inferior a44px`);
      if (caso.forma === 'marco') {
        exigir(
          idle.includes('inset') && idle.includes('1px'),
          `${id}: falta contorno de selección`,
        );
        exigir(
          datos.background.image === datos.topbarImage &&
            datos.background.image.includes('linear-gradient'),
          `${id}: el menú reinicia el fondo del marco`,
        );
        exigir(
          parseFloat(datos.background.size) === caso.width &&
            datos.background.offset + safe.left === 0,
          `${id}: fondo fuera del eje del viewport`,
        );
        exigir(
          datos.footerLine.border === '0px' &&
            datos.footerLine.width === '1px' &&
            parseFloat(datos.footerLine.left) > 0 &&
            datos.footerLine.left === datos.footerLine.right,
          `${id}: el separador del pie atraviesa todo el marco`,
        );
      } else
        exigir(
          datos.footerLine.border === '1px' && !idle.includes('inset'),
          `${id}: Carril heredó el contorno o el pie de Marco`,
        );
      const png = await page.screenshot({ path: join(out, `${id}.png`) });
      resultado.pixeles = await pixelesNavegacion(page, png, datos);
      if (caso.forma === 'marco')
        exigir(
          resultado.pixeles.diferencias.flat().every((v) => v <= 1),
          `${id}: discontinuidad real del gradiente ${JSON.stringify(resultado.pixeles.diferencias)}`,
        );
      exigir(
        Math.min(...resultado.pixeles.texto, ...resultado.pixeles.pie) >= 4.5,
        `${id}: texto sin contraste suficiente sobre el PNG real`,
      );
      exigir(
        Math.min(...resultado.pixeles.foco) >= 3,
        `${id}: aro de foco sin contraste suficiente`,
      );
      resultado.ok = true;
      console.log(`✓ ${id}: geometría, PNG, selección, foco y pie`);
    } catch (error) {
      resultado.error = String(error.message).slice(0, 500);
      if (page)
        await page.screenshot({ path: join(out, `${id}-fallo.png`) }).catch(() => undefined);
    } finally {
      resultados.push(resultado);
      if (page) await page.close();
    }
  }
  writeReport(join(out, 'results.json'), JSON.stringify({ results: resultados }, null, 2));
  const fallos = resultados.filter((r) => !r.ok);
  exigir(fallos.length === 0, `navegación: ${fallos.map((r) => `${r.id}: ${r.error}`).join('; ')}`);
}

try {
  // La guarda se prueba en ROJO sin cambiar el árbol ni publicar un commit roto.
  const guarda = ':not(.sytue-tooltip-surface)';
  const reglaDeshabilitadas = /select option:disabled,\s*select optgroup:disabled\s*\{[^}]+\}/g;
  const panelOpaco = /--sytue-panel-surface:\s*var\(--sytue-surface-raised\)/g;
  exigir(
    css.split(guarda).length === 2,
    'la guarda del tooltip debe existir UNA vez en el CSS generado',
  );
  exigir(
    apps.includes('cast') && apps.length > 15,
    'no se encontraron las paletas de las aplicaciones',
  );
  exigir(
    [...css.matchAll(reglaDeshabilitadas)].length === 1,
    'la regla de opciones deshabilitadas debe existir UNA vez en el CSS generado',
  );
  exigir([...css.matchAll(panelOpaco)].length === 1, 'no se encontró el panel opaco traslúcido');
  const mutada = await abrir(
    css
      .replace(guarda, '')
      .replace(reglaDeshabilitadas, '')
      .replace(panelOpaco, '--sytue-panel-surface:rgba(255,255,255,0.8)'),
    'translucent',
    'marco',
    'petroleo',
  );
  const opcionesRotas = await opcionesDeshabilitadas(mutada.getByRole('combobox').first());
  exigir(
    opcionesRotas.length === 3 &&
      opcionesRotas.every((opcion) => opcion.deshabilitada && !opcion.tintaDistinta),
    'la mutación NO detectó la pérdida de distinción visual de las opciones deshabilitadas',
  );
  console.log(
    '✓ Mutación detectada: opciones deshabilitadas con la misma tinta que las disponibles',
  );
  await mutada.getByRole('button', { name: 'Ayuda de tarjeta', exact: true }).hover();
  const ayudaRota = mutada.getByRole('tooltip', { name: 'Ayuda de tarjeta' });
  await ayudaRota.waitFor();
  const roto = await medir(ayudaRota);
  exigir(roto.razon < 4.5, `la mutación NO fue detectada: ${roto.razon.toFixed(2)}:1`);
  await mutada.screenshot({ path: join(out, 'mutacion-tooltip.png') });
  console.log(`✓ Mutación detectada: tooltip con ${roto.razon.toFixed(2)}:1`);
  await mutada.getByRole('button', { name: 'Abrir formulario', exact: true }).click();
  const modalRoto = mutada.getByRole('dialog', { name: 'Formulario de prueba' });
  await modalRoto.waitFor();
  exigir((await medir(modalRoto)).alfa < 1, 'la mutación NO detectó el panel modal transparente');
  await mutada.screenshot({ path: join(out, 'mutacion-modal.png') });
  console.log('✓ Mutación detectada: panel modal transparente');
  await mutada.close();

  const feedback = await abrir(css, 'translucent', 'marco', 'petroleo');
  for (const width of [320, 1280]) {
    await feedback.setViewportSize({ width, height: 900 });
    await feedback.getByRole('button', { name: 'Mostrar error persistente', exact: true }).click();
    await feedback.getByRole('button', { name: 'Abrir formulario', exact: true }).click();
    const dialog = feedback.getByRole('dialog', { name: 'Formulario de prueba' });
    await dialog.getByRole('alert').first().waitFor();
    exigir(
      await dialog
        .getByRole('alert')
        .first()
        .evaluate(
          (element) =>
            element.closest('[data-sytue-dialog-toasts]') !== null &&
            getComputedStyle(element.parentElement).position !== 'fixed',
        ),
      `${width}: el aviso sigue flotando sobre el diálogo`,
    );
    await dialog.getByRole('textbox', { name: 'Nombre' }).fill('Cuenta sin bloquear');
    await dialog.getByRole('button', { name: 'Guardar formulario', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    exigir(
      (await feedback.getByRole('alert').count()) > 0,
      'cerrar el formulario perdió el error persistente',
    );
  }
  await feedback.close();

  for (const tema of ['light', 'dark', 'translucent']) {
    for (const forma of ['marco', 'carril']) {
      for (const { color, app } of paletas) {
        const nombre = `${tema}/${forma}/${color}/${app}`;
        const page = await abrir(css, tema, forma, color, app);
        await paneles(page, nombre);
        // El panel REAL de Modal (no un menú con role=dialog) debe tapar lo de
        // detrás también cuando se convierte en hoja táctil.
        for (const width of [1280, 390]) {
          await page.setViewportSize({ width, height: 900 });
          await page.getByRole('button', { name: 'Abrir formulario', exact: true }).click();
          const modal = page.getByRole('dialog', { name: 'Formulario de prueba' });
          await modal.waitFor();
          await legible(modal, `${nombre}/${width}, modal`, true);
          for (const texto of await modal.locator('.text-text, .text-text-muted').all()) {
            await legible(texto, `${nombre}/${width}, texto de modal`);
          }
          await modal.getByRole('textbox', { name: 'Nombre' }).fill('Cuenta personal');
          await page.keyboard.press('Escape');
          await modal.waitFor({ state: 'hidden' });
        }
        await page.setViewportSize({ width: 1280, height: 900 });
        for (const opcion of await page.locator('select option, select optgroup').all()) {
          await legible(opcion, `${nombre}, opción nativa`, true);
        }
        for (const select of await page.getByRole('combobox').all()) {
          const opciones = await opcionesDeshabilitadas(select);
          exigir(
            opciones.length === 3 &&
              opciones.every((opcion) => opcion.deshabilitada && opcion.tintaDistinta),
            `${nombre}: una opción deshabilitada o su grupo pierde la distinción visual`,
          );
        }
        // Todas las rampas pasan por los menús anidados y su selección real. Los
        // caminos de apertura, foco, hover y portales se repiten con los tres colores
        // base; el comportamiento de esos componentes es común a cada producto.
        if (color === 'app' && app !== 'cast') {
          await page.getByRole('button', { name: 'Opciones de contenido', exact: true }).click();
          await page.getByRole('dialog', { name: 'Opciones de contenido' }).waitFor();
          // El enlace usa primary-text: cambia con la rampa de CADA producto.
          await paneles(page, nombre);
          await page.close();
          console.log(`✓ ${nombre}`);
          continue;
        }
        for (const contexto of ['contenido', 'cromo', 'tarjeta']) {
          const select = page.getByRole('combobox', { name: `Idioma de ${contexto}` });
          await select.press('Home');
          await select.press('ArrowDown');
          exigir(
            (await select.inputValue()) === 'de',
            `${nombre}: el teclado no salta la opción deshabilitada de ${contexto}`,
          );
          await select.press('ArrowDown');
          exigir(
            (await select.inputValue()) === 'en',
            `${nombre}: el teclado no salta el grupo deshabilitado de ${contexto}`,
          );
          await page.getByRole('button', { name: `Acciones de ${contexto}`, exact: true }).click();
          const menu = page.getByRole('menu', { name: `Acciones de ${contexto}` });
          await menu.waitFor();
          await legible(menu, `${nombre}, menú de ${contexto}`, true);
          for (const opcion of await menu.getByRole('menuitem').all()) {
            await opcion.hover();
            // Espera a que termine la transición real del estado bajo el puntero.
            await page.waitForTimeout(180);
            await legible(opcion, `${nombre}, acción bajo el puntero de ${contexto}`);
          }
          await page.keyboard.press('Escape');
          await menu.waitFor({ state: 'detached' });
          await page.getByRole('button', { name: `Opciones de ${contexto}`, exact: true }).click();
          const panel = page.getByRole('dialog', { name: `Opciones de ${contexto}` });
          await panel.waitFor();
          await paneles(page, nombre);
          await page.keyboard.press('Escape');
          await panel.waitFor({ state: 'detached' });
          await page.getByRole('button', { name: `Ayuda de ${contexto}`, exact: true }).hover();
          const ayuda = page.getByRole('tooltip', { name: `Ayuda de ${contexto}` });
          await ayuda.waitFor();
          await legible(ayuda, `${nombre}, ayuda de ${contexto}`, true);
          const flecha = await medir(ayuda.locator('.sytue-tooltip-arrow'));
          const caja = await medir(ayuda);
          exigir(
            flecha.fondo === caja.fondo,
            `${nombre}: la flecha hereda el color de ${contexto}`,
          );
          if (
            color === 'petroleo' &&
            forma === 'marco' &&
            contexto === 'tarjeta' &&
            tema !== 'light'
          ) {
            await page.screenshot({ path: join(out, `${tema}-marco.png`) });
          }
          await page.mouse.move(0, 0);
        }
        await page.close();
        console.log(`✓ ${nombre}`);
      }
    }
  }
  await navegacion();
  console.log(`✓ ${comprobaciones} comprobaciones de contraste, opacidad y herencia`);
} finally {
  await browser.close();
}
