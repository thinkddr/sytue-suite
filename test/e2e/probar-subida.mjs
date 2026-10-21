// ¿Hasta qué tamaño deja subir una foto la galería?
//
// POR QUÉ EXISTE. Sembrando fotos de demostración, cuatro de dieciocho fallaron
// con un 502 y las otras catorce pasaron: el patrón no era el número de subidas
// ni la concurrencia, sino EL TAMAÑO. Este guion lo aísla —sube ficheros de
// tamaño creciente por el mismo camino que la galería (BFF → Drive → indexar) y
// dice qué contesta cada paso—, y así se vio que a partir de 1 MiB Drive
// respondía 413 y el BFF lo disfrazaba de 502.
//
// Queda porque un límite de subida es de las cosas que se rompen en silencio: no
// falla ningún test, no salta ninguna alerta, y lo descubre quien intenta subir
// la foto de su móvil.
//
// Uso:
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… bun run probar-subida.mjs [cuántas]
//
// Las imágenes se generan aquí con ImageMagick, con RUIDO y no con un degradado:
// un degradado de 1600×1067 comprime a 18 KB y no prueba nada.
//
// LO QUE SUBE SE QUEDA SUBIDO: los ficheros se llaman `PRUEBA-SUBIDA-n.jpg` para
// poder encontrarlos y borrarlos después.
import { chromium } from "playwright";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { activarOrganizacion, completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_SHOT_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const BASE = `https://${TENANT ? `${TENANT}.${DOMINIO}` : DOMINIO}`;
const CUANTAS = Number(process.argv[2] ?? 5);

const CRED = {
  email: process.env.SYTUE_SHOT_EMAIL || process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_SHOT_PASS || process.env.SYTUE_AUDIT_PASS || "",
};
if (!CRED.email || !CRED.pass) {
  console.error("Faltan credenciales: SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.");
  process.exit(2);
}

/** Lados que dan, con ruido y calidad 95, del orden de 0,5 / 1,4 / 5 / 14 / 28 MB. */
const LADOS = [
  [1200, 800],
  [1600, 1067],
  [3000, 2000],
  [5000, 3400],
  [7000, 4700],
];

const dir = mkdtempSync(join(tmpdir(), "sytue-subida-"));
const ficheros = [];
for (let i = 0; i < Math.min(CUANTAS, LADOS.length); i++) {
  const [w, h] = LADOS[i];
  const ruta = join(dir, `${i + 1}.jpg`);
  try {
    execFileSync("magick", [
      "-size",
      `${w}x${h}`,
      "xc:gray",
      "+noise",
      "Random",
      "-quality",
      "95",
      ruta,
    ]);
  } catch {
    console.error(
      "Hace falta ImageMagick (`magick`) para generar las imágenes.",
    );
    process.exit(2);
  }
  ficheros.push(ruta);
}

const navegador = await chromium.launch();
const contexto = await navegador.newContext({
  ignoreHTTPSErrors: true,
  locale: "es-ES",
});
const page = await contexto.newPage();
await page.goto(`${BASE}/photos`, {
  waitUntil: "domcontentloaded",
  timeout: 60_000,
});
if (new URL(page.url()).hostname === IDP) {
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  await completarMFASiHaceFalta(page).catch(() => {});
  await page.waitForURL((u) => u.hostname !== IDP, { timeout: 90_000 });
  await activarOrganizacion(page);
}
await page.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => {});

let mayorQuePasa = 0;
for (const [i, ruta] of ficheros.entries()) {
  const bytes = readFileSync(ruta);
  const kb = bytes.length / 1024;
  const r = await page.evaluate(
    async ({ b64, nombre }) => {
      const buf = Uint8Array.from(atob(b64), (ch) => ch.charCodeAt(0));
      const t = Date.now();
      const sub = await fetch(
        `/photos/api/drive-subir?nombre=${encodeURIComponent(nombre)}`,
        {
          method: "POST",
          credentials: "include",
          headers: { "content-type": "image/jpeg" },
          body: buf,
        },
      );
      const ms = Date.now() - t;
      if (!sub.ok)
        return {
          paso: "subir",
          status: sub.status,
          ms,
          cuerpo: (await sub.text()).slice(0, 120),
        };
      const { id } = await sub.json();
      const alta = await fetch("/photos/api/v1/assets", {
        method: "POST",
        credentials: "include",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ driveFileId: id }),
      });
      return { paso: alta.ok ? "ok" : "indexar", status: alta.status, ms };
    },
    { b64: bytes.toString("base64"), nombre: `PRUEBA-SUBIDA-${i + 1}.jpg` },
  );
  const marca = r.paso === "ok" ? "✓" : "✗";
  console.log(
    `${marca} ${kb.toFixed(0).padStart(6)} KB  ${r.paso.padEnd(8)} ${r.status}  ${String(r.ms).padStart(6)}ms` +
      (r.cuerpo ? `  ${r.cuerpo}` : ""),
  );
  if (r.paso === "ok") mayorQuePasa = Math.max(mayorQuePasa, kb);
}

await navegador.close();
console.log(
  `\nLo más grande que entra: ${mayorQuePasa.toFixed(0)} KB.` +
    "\nBorra lo subido cuando termines: `bun run quitar-foto.mjs PRUEBA-SUBIDA-1` …",
);
