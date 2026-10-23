// SPDX-License-Identifier: Apache-2.0
// Contraste de los componentes REALES y del CSS generado por Tailwind en el runner.
// No levanta aplicaciones ni usa sesión. Incluye una mutación: quitar la exclusión
// del tooltip tiene que reproducir la tinta oscura sobre fondo oscuro de Marco.
import { chromium } from 'playwright';
import { mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const css = readFileSync(process.env.MENUS_CSS || '/tmp/menus-traslucidos.css', 'utf8');
const js = readFileSync(process.env.MENUS_JS || '/tmp/menus-traslucidos.js', 'utf8');
const colores = readFileSync(new URL('../../branding/tokens/colors.css', import.meta.url), 'utf8');
const apps = [...new Set([...colores.matchAll(/\[data-app='([^']+)'\]/g)].map((m) => m[1]))];
const paletas = [
  { color: 'petroleo', app: 'cast' },
  { color: 'cobalto', app: 'cast' },
  ...apps.map((app) => ({ color: 'app', app })),
];
const out = process.env.MENUS_OUT || '/tmp/menus-traslucidos';
mkdirSync(out, { recursive: true });
const browser = await chromium.launch();
let comprobaciones = 0;

function exigir(condicion, mensaje) {
  comprobaciones++;
  if (!condicion) throw new Error(mensaje);
}

async function abrir(hoja, tema, forma, color, app = 'cast') {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const errores = [];
  page.on('pageerror', (error) => errores.push(error.message));
  // El bundle lee las cookies reales de idioma: about:blank tiene origen opaco.
  // La navegación se resuelve en memoria, sin servidor ni tráfico externo.
  const url = 'http://menus.sytue.test/';
  await page.route(url, (route) =>
    route.fulfill({
      contentType: 'text/html',
      body: `<!doctype html><html lang="es" data-theme="${tema}" data-forma="${forma}" data-color="${color}" data-app="${app}"><head></head><body></body></html>`,
    }),
  );
  await page.goto(url);
  await page.addStyleTag({ content: hoja });
  await page.addScriptTag({ content: js, type: 'module' });
  try {
    await page.locator('[data-contexto="tarjeta"]').waitFor({ timeout: 10000 });
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
  console.log(`✓ ${comprobaciones} comprobaciones de contraste, opacidad y herencia`);
} finally {
  await browser.close();
}
