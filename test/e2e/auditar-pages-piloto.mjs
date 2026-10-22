// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Auditoría de aceptación de Sytue Web contra el piloto. Corre solo cuando
// existe una publicación real: no suplanta DNS/TLS con un servidor local.
//
// Uso:
//   SYTUE_PAGES_AUDIT_URL=https://demo.sites.sytue.eu \
//   SYTUE_PAGES_DOMAIN_URL=https://dominio-de-prueba.example \
//   bun run auditar-pages-piloto.mjs
//
// SYTUE_PAGES_DOMAIN_URL es opcional; si se facilita, exige también que TLS y
// la ruta pública funcionen por el dominio propio.
import { chromium, devices } from "playwright";
import { DETECTOR, linea } from "./detector-desbordamiento.mjs";

const sitio = process.env.SYTUE_PAGES_AUDIT_URL;
const dominio = process.env.SYTUE_PAGES_DOMAIN_URL;

if (!sitio) {
  console.error("falta SYTUE_PAGES_AUDIT_URL (URL HTTPS de un sitio Pages publicado)");
  process.exit(2);
}

for (const url of [sitio, dominio].filter(Boolean)) {
  let normalizada;
  try {
    normalizada = new URL(url);
  } catch {
    console.error(`URL inválida: ${url}`);
    process.exit(2);
  }
  if (normalizada.protocol !== "https:") {
    console.error(`la auditoría exige HTTPS: ${url}`);
    process.exit(2);
  }
}

const browser = await chromium.launch();
let fallos = 0;

async function medir(url, nombre) {
  const contexto = await browser.newContext(devices["iPhone 12"]);
  const pagina = await contexto.newPage();
  const consola = [];
  pagina.on("console", (evento) => {
    if (evento.type() === "error") consola.push(evento.text());
  });
  const respuesta = await pagina.goto(url, { waitUntil: "networkidle", timeout: 45_000 });
  if (!respuesta || respuesta.status() !== 200) {
    console.error(`${nombre}: HTTP ${respuesta?.status() ?? "sin respuesta"}`);
    fallos++;
    await contexto.close();
    return;
  }
  await pagina.evaluate(() => document.fonts.ready);
  const h1 = await pagina.locator("h1").count();
  if (h1 !== 1) {
    console.error(`${nombre}: se esperaban exactamente un h1 y hay ${h1}`);
    fallos++;
  }
  const resultado = await pagina.evaluate(DETECTOR);
  if (resultado.hallazgos.length > 0 || resultado.izq.length > 0) {
    console.error(`${nombre}: desbordamiento móvil: ${linea(resultado)}`);
    fallos++;
  }
  if (consola.length > 0) {
    console.error(`${nombre}: errores de consola: ${consola.join(" | ")}`);
    fallos++;
  }
  console.log(`${nombre}: HTTP 200, h1=${h1}, ${linea(resultado)}`);
  await contexto.close();
}

await medir(sitio, "sites wildcard");
if (dominio) await medir(dominio, "dominio propio");
await browser.close();

if (fallos > 0) process.exit(1);
