// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Mide que el ACABADO de la rejilla de Calcus no cuesta pintado: sobre una hoja de FILAS filas
// (por defecto 20.000) en el banco de pruebas `/dev/superficies`, cronometra (a) el
// desplazamiento por pasos de la ventana virtualizada y (b) mover la selección con las
// flechas y ensancharla con Mayús. Imprime mediana y p95 en ms por paso. Se ejecuta antes y
// después de un cambio y se comparan las dos salidas.
//
// Uso: SUP_URL=http://127.0.0.1:4338/docus/prueba SUP_FILAS=20000 bun run superficies-rendimiento.mjs
//
// ponytail: script de verificación, no forma parte de la suite de specs.
import { chromium } from "playwright";

const BASE = process.env.SUP_URL || "http://127.0.0.1:4338/docus/prueba";
const FILAS = Number(process.env.SUP_FILAS || 20000);
const PASOS = Number(process.env.SUP_PASOS || 60);

const stat = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  const q = (p) => s[Math.min(s.length - 1, Math.floor(p * s.length))];
  return {
    mediana: +q(0.5).toFixed(1),
    p95: +q(0.95).toFixed(1),
    max: +s[s.length - 1].toFixed(1),
  };
};

const navegador = await chromium.launch();
const page = await navegador.newPage({
  viewport: { width: 1280, height: 800 },
});
page.setDefaultTimeout(240000);
await page.goto(
  `${BASE}/dev/superficies?kind=spreadsheet&lang=es&filas=${FILAS}`,
  { timeout: 240000 },
);
await page.waitForSelector('[role="grid"]');
await page.waitForTimeout(2000);

const frame = () =>
  page.evaluate(
    () =>
      new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))),
  );

// (a) desplazamiento
const scroll = [];
for (let i = 0; i < PASOS; i++) {
  const t = await page.evaluate(async () => {
    const g = document.querySelector('[role="grid"]');
    let el = g;
    while (
      el &&
      !(
        el.scrollHeight > el.clientHeight + 50 &&
        getComputedStyle(el).overflowY !== "visible"
      )
    )
      el = el.parentElement;
    const t0 = performance.now();
    el.scrollTop += 480;
    await new Promise((r) =>
      requestAnimationFrame(() => requestAnimationFrame(r)),
    );
    return performance.now() - t0;
  });
  scroll.push(t);
}

// (b) selección con teclado
await page.evaluate(() => {
  let el = document.querySelector('[role="grid"]');
  while (
    el &&
    !(
      el.scrollHeight > el.clientHeight + 50 &&
      getComputedStyle(el).overflowY !== "visible"
    )
  )
    el = el.parentElement;
  el.scrollTop = 0;
});
await page.waitForSelector('[data-ref="B3"]');
await page.locator('[data-ref="B3"]').first().click();
const teclado = [];
for (let i = 0; i < PASOS; i++) {
  const t0 = Date.now();
  const k = i % 2 ? "Shift+ArrowDown" : "ArrowDown";
  await page.keyboard.press(k);
  await frame();
  teclado.push(Date.now() - t0);
}
const celdas = await page.evaluate(
  () => document.querySelectorAll('[role="gridcell"]').length,
);
console.log(
  JSON.stringify({
    filas: FILAS,
    celdasPintadas: celdas,
    desplazamiento_ms: stat(scroll),
    seleccion_ms: stat(teclado),
  }),
);
await navegador.close();
