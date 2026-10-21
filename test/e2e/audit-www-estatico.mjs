// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// PUERTA DE PR de desbordamiento móvil para `www`. Corre sobre el `dist` que el
// job de Lighthouse ya construye: sin entorno nuevo, sin credenciales, sin
// producción.
//
// Por qué solo www: es la ÚNICA app con `output: 'static'`. Las otras diez son
// `output: 'server'` y redirigen al IdP, así que no hay build que auditar y su
// puerta sigue siendo el barrido nocturno contra producción (mobile.yml). Y www
// es justo donde estuvo la regresión de agosto de 2026: 23 rutas localizadas
// desbordando el documento entero, que nadie paró hasta que ya estaba desplegado
// —el `preset: "desktop"` de .lighthouserc.json mira cuatro URLs sin localizar y
// ninguna a 320 px—.
//
// El detector es el MISMO módulo que usa el barrido nocturno, no una copia.
//
// Uso: bun run audit-www-estatico.mjs [ruta-al-dist]
import { chromium, devices } from "playwright";
import { fileURLToPath } from "node:url";
import { startServer } from "../../apps/frontend/static-serve.mjs";
import { DETECTOR, linea } from "./detector-desbordamiento.mjs";

const DIST =
  process.argv[2] ||
  fileURLToPath(new URL("../../apps/frontend/apps/www/dist", import.meta.url));

// El idioma va en la RUTA (i18n de Astro con `en` sin prefijo), así que no vale
// con negociar `Accept-Language`: hay que pedir cada una. El alemán es el idioma
// de palabras más largas y el que rompió los titulares.
// Las FICHAS de producto son la plantilla con el titular más largo del catálogo
// —el patrón exacto de esa regresión— y esta puerta no miraba ninguna de las 40.
// No hacen falta las 40: con el idioma de palabras más largas basta, que es el
// mismo criterio con el que ya están en el barrido nocturno.
const RUTAS = [
  "/",
  "/es",
  "/de",
  "/fr",
  "/it",
  "/de/terms",
  "/downloads",
  "/de/product/docs",
  "/fr/product/rooms",
];
// Los dos anchos que importan: el suelo real del catálogo y el teléfono más
// común. Por encima de 390 px nunca ha desbordado nada en 465 medidas.
const PERFILES = [
  ["iPhone SE", devices["iPhone SE"]], // 320 px
  ["iPhone 12", devices["iPhone 12"]], // 390 px
];

const server = startServer({ root: DIST, port: 0, hostname: "127.0.0.1" });
const base = `http://127.0.0.1:${server.port}`;

const browser = await chromium.launch();

let desbordes = 0;
let medidas = 0;
for (const [nombre, perfil] of PERFILES) {
  const ctx = await browser.newContext(perfil);
  const page = await ctx.newPage();
  for (const ruta of RUTAS) {
    const resp = await page.goto(`${base}${ruta}`, {
      waitUntil: "networkidle",
      timeout: 30_000,
    });
    const http = resp?.status() ?? 0;
    if (http >= 400) {
      console.log(
        `  SIN UI  ${nombre.padEnd(12)} ${ruta.padEnd(14)} HTTP ${http}`,
      );
      desbordes++; // una ruta que no existe en el dist es un fallo de la puerta
      continue;
    }
    // La tipografía cambia el ancho del texto: medir antes de que cargue da un
    // verde falso. Es autoalojada (CSP `font-src 'self'`), así que llega.
    await page.evaluate(() => document.fonts.ready);
    const r = await page.evaluate(DETECTOR);
    medidas++;
    const grave = r.hallazgos.length > 0 || r.izq.length > 0;
    if (grave) desbordes++;
    console.log(
      `${grave ? "DESBORDA" : "   ok   "} ${nombre.padEnd(12)} ${ruta.padEnd(14)} ${linea(r)}`,
    );
  }
  await ctx.close();
}
await browser.close();
server.stop(true);

console.log(
  `\n${desbordes} desbordadas de ${medidas} medidas (www, dist local).`,
);
if (medidas !== RUTAS.length * PERFILES.length) {
  console.log(
    "No se midieron todas las combinaciones: NO cuenta como aprobado.",
  );
  process.exit(1);
}
process.exit(desbordes > 0 ? 1 : 0);
