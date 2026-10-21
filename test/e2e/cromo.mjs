// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Capturas del CROMO del editor de Docs (cabecera, cinta, franja y paneles, menú Archivo,
// diálogo de compartir, barra de estado) en claro y oscuro, a 1280 y 390 px, para
// documento, hoja y presentación. Mide el banco de pruebas `/dev/editor`
// (editor/dev/EditorDemo.tsx), servido por `astro dev` de apps/docus: solo desarrollo.
//
// Uso (desde el contenedor del frontal; el servidor dev, antes, en otro proceso):
//   cd apps/frontend/apps/docus && bun --bun astro dev --port 4338 &
//   cd test/e2e && CROMO_URL=http://127.0.0.1:4338/docus/prueba CROMO_OUT=/srv/agentes/capturas/cromo-antes bun run cromo.mjs
//
// ponytail: script de verificación visual, no forma parte de la suite de specs.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const BASE = process.env.CROMO_URL || "http://127.0.0.1:4338/docus/prueba";
const OUT = process.env.CROMO_OUT || "/tmp/cromo";
const SOLO = process.env.CROMO_SOLO || ""; // «general», «extras» o «paneles»; vacío = todo
mkdirSync(OUT, { recursive: true });

const KINDS = ["document", "spreadsheet", "presentation"];
const TEMAS = ["light", "dark"];
const ANCHOS = [
  { w: 1280, h: 800, movil: false },
  { w: 390, h: 844, movil: true },
];

const navegador = await chromium.launch();
const url = (kind, extra = "") => `${BASE}/dev/editor?demo=1&kind=${kind}&lang=es${extra}`;

async function abrir(kind, tema, { w, h, movil }, extra = "") {
  const ctx = await navegador.newContext({
    viewport: { width: w, height: h },
    colorScheme: tema,
    hasTouch: movil,
    isMobile: movil,
    deviceScaleFactor: 2,
  });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("  pageerror:", e.message));
  await page.goto(url(kind, extra));
  await page.addStyleTag({ content: "astro-dev-toolbar{display:none!important}" });
  await page.waitForSelector('[role="tablist"]', { timeout: 120000 });
  await page.waitForTimeout(700);
  return { ctx, page };
}

// Calentamiento (compila las dependencias de `astro dev`).
{
  const p = await navegador.newPage();
  await p.goto(url("document"));
  await p.waitForSelector('[role="tablist"]', { timeout: 240000 });
  await p.close();
}

for (const kind of KINDS) {
  for (const tema of TEMAS) {
    for (const a of ANCHOS) {
      const { ctx, page } = await abrir(kind, tema, a);
      const nombre = `${kind}-${tema}-${a.w}`;
      if (!SOLO || SOLO === "general") {
        await page.screenshot({ path: `${OUT}/${nombre}.png` });
        console.log(nombre);
      }
      // Solo en escritorio: pestañas, menús y diálogos.
      if (!a.movil && (!SOLO || SOLO === "extras")) {
        const tabs = page.locator('[role="tab"]');
        const n = await tabs.count();
        for (let i = 1; i < Math.min(n, 4); i++) {
          const nombreTab = ((await tabs.nth(i).getAttribute("id")) || `t${i}`).replace("ribbon-tab-", "");
          await tabs.nth(i).click();
          await page.waitForTimeout(250);
          await page.screenshot({
            path: `${OUT}/${nombre}-pestana-${nombreTab}.png`,
            clip: { x: 0, y: 0, width: a.w, height: 200 },
          });
        }
        await tabs.first().click();
        // Menú Archivo.
        await page.getByRole("button", { name: /^Archivo$/ }).click();
        await page.waitForTimeout(250);
        await page.screenshot({ path: `${OUT}/${nombre}-archivo.png`, clip: { x: 0, y: 0, width: 520, height: 520 } });
        await page.keyboard.press("Escape");
        // Diálogo Compartir.
        await page.getByRole("button", { name: /Compartir/ }).first().click();
        await page.waitForTimeout(500);
        await page.screenshot({ path: `${OUT}/${nombre}-compartir.png` });
        await page.keyboard.press("Escape");
        // Popover de la cinta (paleta de color) y banda desplegada con «Más».
        const swatch = page.locator('#ribbon-banda [aria-haspopup="true"]').first();
        if (await swatch.count()) {
          await swatch.click();
          await page.waitForTimeout(250);
          await page.screenshot({ path: `${OUT}/${nombre}-popover.png`, clip: { x: 0, y: 0, width: a.w, height: 360 } });
          await page.keyboard.press("Escape");
          await page.mouse.click(700, 500);
        }
        // Menú Descargar.
        await page.getByRole("button", { name: /Descargar/ }).first().click();
        await page.waitForTimeout(250);
        await page.screenshot({ path: `${OUT}/${nombre}-descargar.png`, clip: { x: a.w - 420, y: 0, width: 420, height: 320 } });
        await page.keyboard.press("Escape");
      }
      await ctx.close();
    }
  }
}

// Paneles y ajustes (solo documento; los paneles son los mismos en los tres).
for (const tema of !SOLO || SOLO === "paneles" ? TEMAS : []) {
  for (const panel of ["buscar", "esquema", "comentarios", "revision", "ia"]) {
    const { ctx, page } = await abrir("document", tema, ANCHOS[0], `&panel=${panel}`);
    await page.screenshot({ path: `${OUT}/panel-${panel}-${tema}-1280.png` });
    console.log(`panel-${panel}-${tema}`);
    await ctx.close();
  }
  const { ctx, page } = await abrir("document", tema, ANCHOS[0], `&ajustes=1`);
  await page.screenshot({ path: `${OUT}/ajustes-${tema}-1280.png` });
  await ctx.close();
  const m = await abrir("document", tema, ANCHOS[1], `&panel=comentarios`);
  await m.page.screenshot({ path: `${OUT}/panel-comentarios-${tema}-390.png` });
  await m.ctx.close();
}

await navegador.close();
console.log("hecho ->", OUT);
