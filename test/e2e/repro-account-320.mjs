// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// De dónde salen los +6px de `id.sytue.eu/account` a 320px.
//
// El barrido móvil los encontró en cuanto Sytue ID entró en la lista (iPhone SE y
// Galaxy S9+, los dos perfiles de 320), pero su línea dice `← ? «»`: no señala
// culpable. El detector busca al descendiente cuyo borde derecho rebasa el del
// contenedor y descarta lo que quede tapado por un scroller intermedio, y aquí
// no encaja ninguno — o sea que lo que sobra no es una caja que se sale, sino
// algo que el rectángulo no cuenta (un ancho mínimo de contenido, una columna
// que no encoge). Esto lo mira sin ese filtro y con más señal por elemento.
//
// SOLO LEE: entra, abre /account y mide. No pulsa nada.
//
// Se lanza en la forja, NO en un portátil (workflow mobile.yml, input
// `reproductor`): lo que decide dónde corre una prueba es dónde arranca el
// Chromium, no contra qué URL apunta.
//
// ponytail: sonda de un hallazgo concreto, no una puerta. Cuando el +6px esté
// arreglado y medido, se borra.
import { chromium, devices } from "playwright";
import { completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const EMAIL = process.env.SYTUE_AUDIT_EMAIL;
const PASS = process.env.SYTUE_AUDIT_PASS;
if (!EMAIL || !PASS) {
  console.error("Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.");
  process.exit(2);
}

const navegador = await chromium.launch();
const ctx = await navegador.newContext({
  ...devices["iPhone SE"],
  locale: "de-DE",
  extraHTTPHeaders: { "Accept-Language": "de-DE,de;q=0.9" },
});
const page = await ctx.newPage();

await page.goto(`https://id.${DOMINIO}/account`, {
  waitUntil: "domcontentloaded",
  timeout: 45_000,
});
if (new URL(page.url()).pathname.replace(/\/+$/, "") === "/login") {
  await page.fill("input[name=email]", EMAIL);
  await page.fill("input[name=password]", PASS);
  await page.click("button[type=submit]");
  await completarMFASiHaceFalta(page);
  await page.waitForURL((u) => u.pathname.replace(/\/+$/, "") === "/account", {
    timeout: 60_000,
  });
}
await page.waitForTimeout(1500);

const r = await page.evaluate(() => {
  const raiz = document.scrollingElement || document.documentElement;
  const vw = document.documentElement.clientWidth;
  const sel = (el) => {
    const partes = [];
    for (let n = el; n && n.nodeType === 1 && partes.length < 4; n = n.parentElement) {
      let s = n.tagName.toLowerCase();
      if (n.id) s += `#${n.id}`;
      else if (typeof n.className === "string" && n.className.trim())
        s += `.${n.className.trim().split(/\s+/).slice(0, 3).join(".")}`;
      partes.unshift(s);
    }
    return partes.join(" > ");
  };
  const fuera = [];
  const aprieta = [];
  for (const d of document.querySelectorAll("body *, body")) {
    const c = d.getBoundingClientRect();
    // Se sale por la derecha del VIEWPORT, sin descontar scrollers intermedios:
    // así aparece también lo que el detector descarta por quedar tapado.
    if (Math.round(c.right - vw) > 0)
      fuera.push({
        sel: sel(d),
        sobra: Math.round(c.right - vw),
        w: Math.round(c.width),
        txt: (d.textContent || "").trim().slice(0, 40),
      });
    // Y lo que NO cabe en su propia caja: `scrollWidth > clientWidth` con
    // `overflow-x: visible` lo hereda el ancestro y el detector lo salta a
    // propósito para no repetir la misma causa; aquí interesa justo eso.
    const ex = d.scrollWidth - d.clientWidth;
    if (ex > 1)
      aprieta.push({
        sel: sel(d),
        exceso: ex,
        overflowX: getComputedStyle(d).overflowX,
        cw: d.clientWidth,
        sw: d.scrollWidth,
        txt: (d.textContent || "").trim().slice(0, 40),
      });
  }
  return {
    vw,
    docExceso: raiz.scrollWidth - raiz.clientWidth,
    fuera: fuera.slice(0, 12),
    aprieta: aprieta.slice(0, 12),
  };
});

console.log(`viewport ${r.vw}px · el documento sobra ${r.docExceso}px\n`);
console.log("SE SALEN DEL VIEWPORT POR LA DERECHA:");
for (const f of r.fuera)
  console.log(`  +${f.sobra}px  w=${f.w}  ${f.sel}  «${f.txt}»`);
if (!r.fuera.length) console.log("  (ninguno: no es una caja que se salga)");
console.log("\nNO CABEN EN SU PROPIA CAJA (scrollWidth > clientWidth):");
for (const a of r.aprieta)
  console.log(
    `  +${a.exceso}px  ${a.cw}→${a.sw}  overflow-x:${a.overflowX}  ${a.sel}  «${a.txt}»`,
  );
if (!r.aprieta.length) console.log("  (ninguno)");

await navegador.close();
