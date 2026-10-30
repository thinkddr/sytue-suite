// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Diagnóstico read-only: reutiliza la sesión del barrido y el detector real.
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { chromium, devices } from 'playwright';
import { DETECTOR } from './detector-desbordamiento.mjs';
import { esperarCajones } from './esperar-cajones.mjs';

const SOLO_BANCO = process.argv.includes('--banco');
const STORAGE = fileURLToPath(new URL('.auth/prod-audit.json', import.meta.url));
if (!SOLO_BANCO) assert.ok(existsSync(STORAGE), 'Falta la sesión remota del barrido móvil');
const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const destino = `https://bases.${DOMINIO}/bases`;
const browser = await chromium.launch();
const ctx = await browser.newContext({
  ...devices['Pixel 5'],
  ...(SOLO_BANCO ? {} : { storageState: STORAGE }),
  locale: 'de-DE',
  extraHTTPHeaders: { 'Accept-Language': 'de-DE,de;q=0.9' },
});
const page = await ctx.newPage();
const pruebas = [];
const cabecera = '<!doctype html><meta name="viewport" content="width=device-width, initial-scale=1">';

// El mismo evaluate recoge el transform y ejecuta el detector, sin un frame
// entre las dos medidas. La función importada se serializa, no se reimplementa.
async function medir(id) {
  return page.evaluate(`(() => {
    const el = document.getElementById(${JSON.stringify(id)});
    if (!el) throw new Error('El cajón desapareció');
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    return {
      left: r.left, width: r.width, right: r.right,
      transform: cs.transform, animationDuration: cs.animationDuration,
      animations: el.getAnimations().map(a => ({
        name: a.animationName || a.id, state: a.playState, pending: a.pending,
        currentTime: a.currentTime, playbackRate: a.playbackRate,
        progress: a.effect?.getComputedTiming().progress,
        duration: a.effect?.getTiming().duration
      })),
      detector: (${DETECTOR.toString()})()
    };
  })()`);
}

function sinRecorte(r, mensaje) {
  assert.ok(r.left >= -1, `${mensaje}: ${JSON.stringify(r)}`);
  assert.equal(r.detector.izq.length, 0, `${mensaje}: ${JSON.stringify(r.detector.izq)}`);
}

try {
  // Una animación larga conserva el defecto del reloj fijo, pero NO un defecto
  // final de layout. Este banco no modifica producción ni copia su CSS.
  await page.setContent(`${cabecera}<style>body{margin:0}aside{position:fixed;left:0;top:40px;width:260px;height:400px;background:white}</style><aside id="fixture" class="sytue-drawer-in">Menü</aside>`);
  await page.locator('aside').evaluate((el) => {
    el.animate(
      [{ transform: 'translateX(-100%)' }, { transform: 'none' }],
      { duration: 900, fill: 'both' },
    );
  });
  await page.waitForTimeout(400);
  const transitorio = await medir('fixture');
  assert.ok(transitorio.left < -1 && transitorio.detector.izq.length > 0,
    'El control original de 400ms debe ver el movimiento transitorio');
  await esperarCajones(page);
  const final = await medir('fixture');
  sinRecorte(final, 'El banco debe terminar dentro del viewport');
  pruebas.push({ caso: 'reloj-fijo', transitorio, final });

  // El mismo detector debe seguir rechazando 3px REALES después de terminar.
  await page.locator('aside').evaluate((el) => { el.style.left = '-3px'; });
  await esperarCajones(page);
  const negativo = await medir('fixture');
  assert.equal(negativo.left, -3);
  assert.ok(negativo.detector.izq.some((h) => h.fuera === 3));
  pruebas.push({ caso: 'recorte-real-3px', medida: negativo });

  // Un movimiento que no termina es un fallo, no una medición aprobada.
  await page.setContent(`${cabecera}<aside class="sytue-drawer-in">Menü</aside>`);
  await page.locator('aside').evaluate((el) => {
    const a = el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 1_000 });
    a.pause();
  });
  await assert.rejects(esperarCajones(page, { limiteMs: 150 }), /no terminó/);
  await page.evaluate(() => document.getAnimations().forEach((a) => a.cancel()));
  pruebas.push({ caso: 'animacion-pausada', rechazada: true });

  await page.locator('aside').evaluate((el) => {
    el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 1_000 });
  });
  const cancelada = assert.rejects(esperarCajones(page), /AbortError/);
  await page.waitForTimeout(20);
  await page.evaluate(() => document.getAnimations().forEach((a) => a.cancel()));
  await cancelada;
  pruebas.push({ caso: 'animacion-cancelada', rechazada: true });

  if (!SOLO_BANCO) {
    // Medir la app desplegada en el mismo perfil/idioma del fallo. No se envían
    // formularios ni se crean datos; abrir/cerrar navegación sólo cambia esta UI.
    await page.goto(destino, { waitUntil: 'domcontentloaded' });
    assert.equal(new URL(page.url()).hostname, `bases.${DOMINIO}`, 'La sesión no llegó a Bases');
    await page.locator('aside.sytue-nav-drawer').waitFor({ state: 'attached' });
    await page.waitForTimeout(3_500);
    const drawer = page.locator('aside.sytue-nav-drawer').first();
    const id = await drawer.getAttribute('id');
    assert.ok(id, 'El cajón real debe tener un id');
    const toggle = page.locator(`button[aria-controls=${JSON.stringify(id)}]`);
    await toggle.evaluate((el) => el.click());
    await page.waitForFunction((id) => document.getElementById(id)?.classList.contains('sytue-drawer-in'), id);
    const entradaNatural = await medir(id);
    await esperarCajones(page);
    const finalNatural = await medir(id);
    pruebas.push({ caso: 'bases-entrada-natural', durante: entradaNatural, despues: finalNatural });
    sinRecorte(finalNatural, 'Bases tras su entrada CSS natural');
    await page.keyboard.press('Escape');
    await page.waitForFunction((id) => !document.getElementById(id)?.classList.contains('sytue-drawer-in'), id);
    await esperarCajones(page);
    const muestras = [];
    pruebas.push({ caso: 'bases-natural', muestras });
    for (let i = 0; i < 20; i++) {
      await toggle.evaluate((el) => el.click());
      await page.waitForTimeout(400); // Control SIN el arreglo, igual que el barrido.
      const antes = await medir(id);
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
      await esperarCajones(page);
      const despues = await medir(id);
      muestras.push({ ciclo: i, antes, despues });
      sinRecorte(despues, `Bases en reposo, ciclo ${i}`);
      await page.keyboard.press('Escape');
      await page.waitForFunction((id) => !document.getElementById(id)?.classList.contains('sytue-drawer-in'), id);
      await esperarCajones(page); // Pintar el cierre para que la próxima entrada reinicie CSS.
    }

    // Misma animación CSS REAL de producción, ralentizada sólo en este navegador
    // para comprobar que el reloj externo de 400ms no certifica que haya acabado.
    await toggle.evaluate((el) => el.click());
    await page.waitForFunction((id) => document.getElementById(id)?.classList.contains('sytue-drawer-in'), id);
    await drawer.evaluate((el) => {
      const animaciones = el.getAnimations();
      if (!animaciones.length) throw new Error('El cajón real no expone su animación');
      for (const a of animaciones) a.updatePlaybackRate(0.2);
    });
    await page.waitForTimeout(400);
    const durante = await medir(id);
    assert.ok(durante.left < -1 && durante.animations.some((a) => a.state === 'running'));
    await esperarCajones(page);
    const despues = await medir(id);
    sinRecorte(despues, 'Bases tras terminar la animación CSS real');
    pruebas.push({ caso: 'bases-reloj-ralentizado', durante, despues });

    const leftOriginal = await drawer.evaluate((el) => el.style.left);
    try {
      await drawer.evaluate((el) => { el.style.left = '-3px'; });
      await esperarCajones(page);
      const negativoReal = await medir(id);
      assert.equal(negativoReal.left, -3);
      assert.ok(negativoReal.detector.izq.some((h) => h.fuera === 3));
      pruebas.push({ caso: 'bases-recorte-real-3px', medida: negativoReal });
    } finally {
      await drawer.evaluate((el, left) => { el.style.left = left; }, leftOriginal);
    }
  }
} finally {
  console.log(JSON.stringify({ tipo: 'drawer-animation', modo: SOLO_BANCO ? 'banco' : 'produccion', destino: SOLO_BANCO ? 'fixture' : destino, perfil: 'Pixel 5', locale: 'de-DE', pruebas }));
  await browser.close();
}
