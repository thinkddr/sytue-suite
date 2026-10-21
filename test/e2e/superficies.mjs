// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Capturas de las TRES superficies de edición de Docs (rejilla de Calcus, lienzo de Diapos,
// página de Docus) sobre el banco de pruebas `/dev/superficies` (editor/dev/SuperficiesDemo.tsx):
// en claro y oscuro, a 1280 y 390 px, y en sus estados de trabajo (selección de rango, filtro
// abierto, imagen o tabla seleccionadas, caja seleccionada…). Sirven para comparar el
// antes/después de un cambio de acabado; NO afirman nada, solo dejan los PNG.
//
// Uso (desde el contenedor del frontal; el servidor dev, antes, en otro proceso):
//   cd apps/frontend/apps/docus && bun --bun astro dev --port 4338 &
//   cd test/e2e && SUP_URL=http://127.0.0.1:4338/docus/prueba SUP_PREFIJO=antes bun run superficies.mjs
// Salida: PNG en SUP_OUT (por defecto /tmp/superficies), `<prefijo>-<escena>-<tema>-<ancho>.png`.
//
// ponytail: script de verificación, no forma parte de la suite de specs.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const BASE = process.env.SUP_URL || "http://127.0.0.1:4338/docus/prueba";
const OUT = process.env.SUP_OUT || "/tmp/superficies";
const PREFIJO = process.env.SUP_PREFIJO || "antes";
const SOLO = (process.env.SUP_SOLO || "").split(",").filter(Boolean);
const ANCHOS = [
  { w: 1280, h: 800, movil: false },
  { w: 390, h: 844, movil: true },
];
const TEMAS = ["light", "dark"];

mkdirSync(OUT, { recursive: true });

// Cada escena: tipo de documento, parámetros de URL y los gestos que la dejan en su estado.
const ESCENAS = [
  { id: "hoja-reposo", kind: "spreadsheet", gestos: async () => {} },
  {
    id: "hoja-rango",
    kind: "spreadsheet",
    gestos: async (p) => {
      await p.locator('[data-ref="B3"]').click();
      await p.locator('[data-ref="D7"]').click({ modifiers: ["Shift"] });
    },
  },
  {
    id: "hoja-filtro",
    kind: "spreadsheet",
    gestos: async (p) => {
      await p.locator('[data-ref="C3"]').click();
      await p
        .locator('button[aria-haspopup="listbox"]')
        .nth(1)
        .click({ timeout: 8000 });
    },
  },
  {
    id: "hoja-remotos",
    kind: "spreadsheet",
    query: "&remotos=1",
    gestos: async () => {},
  },
  { id: "diapos-reposo", kind: "presentation", gestos: async () => {} },
  {
    id: "diapos-caja",
    kind: "presentation",
    gestos: async (p) => {
      await p.locator("[data-scroll-x] button[draggable=true]").nth(1).click();
      await p.waitForTimeout(300);
      await p
        .locator("[data-boxframe]")
        .first()
        .click({ position: { x: 20, y: 20 } });
    },
  },
  {
    id: "diapos-remotos",
    kind: "presentation",
    query: "&remotos=1",
    gestos: async () => {},
  },
  { id: "doc-reposo", kind: "document", gestos: async () => {} },
  {
    id: "doc-imagen",
    kind: "document",
    gestos: async (p) => {
      await p.locator("img[data-sytue-image]").first().click();
    },
  },
  {
    id: "doc-tabla",
    kind: "document",
    gestos: async (p) => {
      await p.locator(".sytue-page td").nth(4).click();
    },
  },
  {
    id: "doc-remotos",
    kind: "document",
    query: "&remotos=1",
    gestos: async () => {},
  },
];

const navegador = await chromium.launch();
{
  const p = await navegador.newPage();
  await p.goto(`${BASE}/dev/superficies?kind=spreadsheet&lang=es`, {
    timeout: 240000,
  });
  await p.waitForSelector('[role="tablist"]', { timeout: 240000 });
  await p.close();
}

for (const esc of ESCENAS) {
  if (SOLO.length && !SOLO.some((s) => esc.id.includes(s))) continue;
  for (const tema of TEMAS) {
    for (const { w, h, movil } of ANCHOS) {
      const ctxB = await navegador.newContext({
        viewport: { width: w, height: h },
        hasTouch: movil,
        isMobile: movil,
        colorScheme: tema,
      });
      const page = await ctxB.newPage();
      const errores = [];
      page.on("pageerror", (e) => errores.push(e.message));
      await page.addInitScript((t) => {
        try {
          localStorage.setItem("sytue-theme", t);
        } catch {
          /* sin almacenamiento */
        }
      }, tema);
      await page.goto(
        `${BASE}/dev/superficies?kind=${esc.kind}&lang=es${esc.query ?? ""}`,
        { timeout: 120000 },
      );
      await page.evaluate(
        (t) => document.documentElement.setAttribute("data-theme", t),
        tema,
      );
      await page.waitForSelector('[role="tablist"]', { timeout: 60000 });
      await page.waitForTimeout(600);
      try {
        await esc.gestos(page);
      } catch (e) {
        console.log(
          `  gesto fallido ${esc.id}: ${String(e.message).split("\n")[0]}`,
        );
      }
      await page.waitForTimeout(300);
      const nombre = `${PREFIJO}-${esc.id}-${tema}-${w}.png`;
      await page.screenshot({ path: `${OUT}/${nombre}` });
      console.log(
        nombre,
        errores.length ? `ERRORES: ${errores.join(" | ")}` : "",
      );
      await ctxB.close();
    }
  }
}
await navegador.close();
