/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * INVENTARIO DE ORÍGENES por app web, contra producción y SOLO EN LECTURA, para
 * la CSP cerrada de cada app (`cabecerasCerradas`, FE-03 de la auditoría del
 * 2026-10-06). Recorre las pantallas de cada app y apunta a qué orígenes habla
 * el navegador, agrupados por la directiva que los gobierna (`connect-src`,
 * `img-src`, `media-src`, `frame-src`, `script-src`), junto con las violaciones
 * de CSP que el propio navegador informe (`securitypolicyviolation`): tras
 * desplegar, una app con su CSP cerrada tiene que salir con CERO violaciones.
 *
 * Uso (EN EL PILOTO):
 *   set -a; . ~/.config/sytue/audit.env; set +a
 *   bun run csp-origenes.mjs                 # todas
 *   bun run csp-origenes.mjs drive notes     # solo esas
 * Salida: JSON por la salida estándar.
 */
import { chromium } from "playwright";
import {
  activarOrganizacion,
  completarMFASiHaceFalta,
  porQueNoEntra,
} from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const IDP = `id.${DOMINIO}`;
const BASE = `https://corporate.${DOMINIO}`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_AUDIT_PASS || "",
};

// Pantallas de lectura por app. `click` abre un estado sin escribir nada.
const APPS = {
  portal: [
    "/one",
    "/one/buscar",
    "/one/actividad",
    "/one/notificaciones",
    "/one/datos",
  ],
  account: ["/account"],
  console: ["/admin", "/admin/users", "/admin/apps", "/admin/audit"],
  calendar: ["/calendar", "/calendar/events/new"],
  contacts: ["/contacts"],
  drive: [
    { u: "/drive", click: "[role=row] button, [role=gridcell], main li" },
  ],
  erp: ["/erp", "/erp?m=factura_venta"],
  notes: [{ u: "/notes", click: "main a[href*='/notes/']" }],
  photos: ["/photos", "/photos/albums", { u: "/photos", click: "main img" }],
  docus: [{ u: "/docus/", click: 'a[href*="/edit?id="]' }],
  calcus: [{ u: "/calcus/", click: 'a[href*="/edit?id="]' }],
  diapos: [{ u: "/diapos/", click: 'a[href*="/edit?id="]' }],
  certipdf: ["/certipdf/"],
  bases: ["/bases"],
  web: ["/web"],
  // Sin abrir un mensaje ni un canal: abrirlos los marca como leídos, y eso es escribir.
  mail: ["/mail"],
  rooms: ["/rooms"],
  cast: ["/cast"],
  cloud: ["/cloud"],
};

const DIRECTIVA = {
  fetch: "connect-src",
  xhr: "connect-src",
  eventsource: "connect-src",
  websocket: "connect-src",
  image: "img-src",
  media: "media-src",
  font: "font-src",
  script: "script-src",
  stylesheet: "style-src",
  document: "frame-src",
  manifest: "manifest-src",
};

const pedidas = process.argv.slice(2);
const plan = Object.entries(APPS).filter(
  ([a]) => !pedidas.length || pedidas.includes(a),
);

const navegador = await chromium.launch();
const ctx = await navegador.newContext({
  viewport: { width: 1280, height: 800 },
  locale: "es-ES",
});

async function sesion(p) {
  if (new URL(p.url()).hostname !== IDP) return;
  // «Mi cuenta» vive en el propio IdP: ahí no hay formulario que rellenar.
  if (!(await p.locator("input[name=email]").count())) return;
  const porque = porQueNoEntra(p.url());
  if (porque) throw new Error(`el login no llega: ${porque}`);
  await p.fill("input[name=email]", CRED.email);
  await p.fill("input[name=password]", CRED.pass);
  await p.click("button[type=submit]");
  await completarMFASiHaceFalta(p).catch(() => {});
  await p.waitForURL((u) => u.hostname !== IDP, { timeout: 90_000 });
  await activarOrganizacion(p);
}

const informe = {};
for (const [app, pantallas] of plan) {
  const origenes = {};
  const violaciones = new Set();
  const csp = new Set();
  const finales = new Set();
  const errores = [];
  const apunta = (tipo, url, propio) => {
    let o;
    try {
      o = new URL(url).origin;
    } catch {
      return;
    }
    if (o === "null" || url.startsWith("data:") || url.startsWith("blob:"))
      o = url.split(":")[0] + ":";
    const d = DIRECTIVA[tipo] ?? tipo;
    origenes[d] ??= {};
    const clave = o === propio ? "'self'" : o;
    origenes[d][clave] = (origenes[d][clave] ?? 0) + 1;
  };
  for (const scr of pantallas) {
    const { u, click } = typeof scr === "string" ? { u: scr } : scr;
    const p = await ctx.newPage();
    // El origen de la página EN ESE MOMENTO: el tenant redirige al host del producto.
    const propioAhora = () => {
      try {
        return new URL(p.mainFrame().url()).origin;
      } catch {
        return BASE;
      }
    };
    p.on("request", (r) => {
      const esMarco =
        r.resourceType() === "document" && r.frame() !== p.mainFrame();
      if (r.resourceType() === "document" && !esMarco) return;
      apunta(r.resourceType(), r.url(), propioAhora());
    });
    p.on("websocket", (ws) => apunta("websocket", ws.url(), propioAhora()));
    p.on("response", (r) => {
      if (
        r.request().resourceType() === "document" &&
        r.frame() === p.mainFrame()
      ) {
        const h = r.headers()["content-security-policy"];
        if (h) csp.add(h);
      }
      if (r.status() >= 500)
        errores.push(`${r.status()} ${r.url().slice(0, 120)}`);
    });
    // Los avisos de CSP del navegador no pasan por la consola de Playwright: se
    // escuchan en la página y se reenvían con una marca.
    await p.addInitScript(() => {
      addEventListener("securitypolicyviolation", (e) =>
        console.warn(
          `CSP-VIOLACION ${e.effectiveDirective} ${e.blockedURI} en ${location.origin}`,
        ),
      );
    });
    p.on("console", (m) => {
      if (m.text().startsWith("CSP-VIOLACION"))
        violaciones.add(m.text().slice(14, 254));
    });
    try {
      await p.goto(`${BASE}${u}`, {
        waitUntil: "domcontentloaded",
        timeout: 60_000,
      });
      await sesion(p);
      await p
        .waitForLoadState("networkidle", { timeout: 15_000 })
        .catch(() => {});
      if (click) {
        await p
          .locator(click)
          .first()
          .click({ timeout: 10_000 })
          .catch((e) => errores.push(`click: ${String(e).slice(0, 80)}`));
        await p
          .waitForLoadState("networkidle", { timeout: 15_000 })
          .catch(() => {});
      }
      await p.waitForTimeout(4000);
      finales.add(new URL(p.url()).origin);
      if (/\/login|\/authorize/.test(p.url()))
        errores.push(`sin sesión en ${p.url()}`);
    } catch (e) {
      errores.push(`${u}: ${String(e).slice(0, 160)}`);
    }
    await p.close();
  }
  informe[app] = {
    en: [...finales],
    origenes,
    csp: [...csp],
    violaciones: [...violaciones],
    errores,
  };
  console.error(`hecho ${app}`);
}
await navegador.close();
console.log(JSON.stringify(informe, null, 2));
// Una violación es una app que habla con un origen fuera de su lista: se sale en rojo.
process.exit(Object.values(informe).some((a) => a.violaciones.length) ? 1 : 0);
