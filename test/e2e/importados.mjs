// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Pinta en el banco de pruebas del editor (`/dev/editor` de apps/docus, que monta
// `@sytue/office` editor/dev/EditorDemo.tsx) los
// modelos que deja el importador (TestCorpusTerceros con SYTUE_CORPUS_MODELOS), una captura
// por hoja o por diapositiva, para ponerlas al lado de la referencia (el PDF que saca
// LibreOffice del fichero original, usado como caja negra: docs/security.md §4).
//
// Uso: correr-importados.sh, que levanta y apaga el servidor. A mano (en el contenedor del
// frontal, con las variables de OIDC de mentira de correr-importados.sh):
//   cd apps/frontend/apps/docus && bun --bun astro dev --port 4351 &
//   cd test/e2e && IMPORTADOS_DIR=/ruta/modelos IMPORTADOS_OUT=/ruta/capturas \
//     IMPORTADOS_SOLO=fichero1.pptx,fichero2.xlsx bun run importados.mjs
//
// ponytail: script de verificación visual, no forma parte de la suite de specs.
import { chromium } from "playwright";
import { mkdirSync, readFileSync, readdirSync } from "node:fs";

const BASE = process.env.IMPORTADOS_URL || "http://127.0.0.1:4351/docus/prueba";
const DIR = process.env.IMPORTADOS_DIR;
const OUT = process.env.IMPORTADOS_OUT || "/tmp/importados";
const SOLO = (process.env.IMPORTADOS_SOLO || "").split(",").filter(Boolean);
const MAX_PAGINAS = Number(process.env.IMPORTADOS_PAGINAS || 3);
if (!DIR) throw new Error("falta IMPORTADOS_DIR");
mkdirSync(OUT, { recursive: true });

const navegador = await chromium.launch();
for (const f of readdirSync(DIR).filter((f) => f.endsWith(".json"))) {
  const original = f.slice(0, -".json".length);
  if (SOLO.length && !SOLO.some((s) => original.includes(s))) continue;
  const kind = /\.xls[xm]$/i.test(original) ? "spreadsheet" : "presentation";
  const contenido = readFileSync(`${DIR}/${f}`, "utf8");
  const ctx = await navegador.newContext({ viewport: { width: 1280, height: 800 }, colorScheme: "light" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log(`  ${original}: pageerror: ${e.message}`));
  await page.addInitScript((c) => {
    globalThis.__SYTUE_DEMO_CONTENIDO__ = c;
  }, contenido);
  await page.goto(`${BASE}/dev/editor?demo=1&kind=${kind}&lang=es`);
  await page.addStyleTag({ content: "astro-dev-toolbar{display:none!important}" });
  await page.waitForSelector('[role="tablist"]', { timeout: 240000 });
  await page.waitForTimeout(1500);
  // Una captura por hoja (sus pestañas) o por diapositiva (sus miniaturas).
  const paginas =
    kind === "spreadsheet"
      ? page.locator("button[aria-pressed]").filter({ hasNotText: /^$/ })
      : page.locator('button[aria-label][draggable="true"][title]');
  const n = Math.min(MAX_PAGINAS, Math.max(1, await paginas.count()));
  for (let i = 0; i < n; i++) {
    if (i > 0) {
      await paginas.nth(i).click();
      await page.waitForTimeout(800);
    }
    await page.screenshot({ path: `${OUT}/${original}-${i + 1}.png` });
  }
  console.log(`${original}: ${n}`);
  await ctx.close();
}
await navegador.close();
