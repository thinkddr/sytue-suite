// Sonda de vista móvil: comprueba las TRES cosas que `mobile-audit.mjs` no mira.
//
// Aquella detecta desbordamiento horizontal —que la página no se salga— y con eso
// no se ve nada de lo que se reportó el 2026-09-07: campos que disparan el zoom
// de iOS, el menú lateral que se queda tapando lo que acabas de abrir, y rutas de
// sesión que devuelven 404 bajo el host de organización.
//
//   bun run sonda-movil.mjs [app...]      (sin args = todas)
//
// Credenciales: como el resto del harness, de SYTUE_AUDIT_EMAIL/PASS/TOTP. Se
// lanza con `./auditar-prod.sh` para que salgan del llavero y no de una línea de
// comando. Contra producción por defecto (SYTUE_BASE_DOMAIN para cambiarlo).
//
// SOLO LEE. Abre menús y mide; no envía, no crea y no borra nada.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium, devices } from "playwright";
import { fileURLToPath } from "node:url";
import { existsSync } from "node:fs";
import { codigoTOTP } from "./tests/helpers.ts";

const INFORME = `${createReportDirectory('sonda-movil')}/informe.json`;

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const IDP = `id.${DOMINIO}`;
const STORAGE = fileURLToPath(new URL(".auth/prod-audit.json", import.meta.url));

const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_AUDIT_PASS || "",
  totp: process.env.SYTUE_AUDIT_TOTP || "",
};

// Mismos prefijos que mobile-audit: bajo el host de organización cada app vive
// bajo el suyo, y es justo lo que rompía el logout.
const BASES = {
  portal: "/one",
  mail: "/mail",
  drive: "/drive",
  calendar: "/calendar",
  contacts: "/contacts",
  docus: "/docus",
  rooms: "/rooms",
  notes: "/notes",
};

const APPS = Object.keys(BASES);
const pedidas = process.argv.slice(2).filter((a) => APPS.includes(a));
const objetivo = pedidas.length ? pedidas : APPS;

/** iPhone SE: el más estrecho que se sigue usando, y donde todo se ve antes. */
const PERFIL = devices["iPhone SE"];

async function entrar(page) {
  if (new URL(page.url()).hostname !== IDP) return false;
  if ((await page.locator("input[name=password]").count()) === 0) return false;
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  const pideSegundo = () => /[?&]step_up=1/.test(page.url());
  await page.waitForURL(pideSegundo, { timeout: 15_000 }).catch(() => {});
  const totp = page.locator(
    'input[name=code], input[name=totp], input[autocomplete="one-time-code"]',
  );
  const hay = await totp
    .first()
    .waitFor({ timeout: pideSegundo() ? 30_000 : 5_000 })
    .then(() => true, () => false);
  if (hay) {
    await totp.first().fill(codigoTOTP(CRED.totp));
    await page.click("button[type=submit]");
    await page
      .waitForURL((u) => !/[?&]step_up=1/.test(u.href), { timeout: 20_000 })
      .catch(() => {});
  }
  return true;
}

/**
 * Campos que disparan el zoom de iOS.
 *
 * El umbral son 16 px ABSOLUTOS y `getComputedStyle` los da ya resueltos, así que
 * esto mide lo que el navegador aplica de verdad —no lo que dice la hoja—: es la
 * única forma de ver el fallo de `1rem` con la raíz en 14.
 */
const CAMPOS_QUE_AMPLIAN = `(() => {
  const malos = [];
  for (const el of document.querySelectorAll('input, select, textarea')) {
    const t = (el.getAttribute('type') || 'text').toLowerCase();
    if (['checkbox','radio','range','hidden','submit','button','file','image','color'].includes(t)) continue;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue;
    const px = parseFloat(getComputedStyle(el).fontSize);
    if (px < 16) {
      malos.push({
        px: Math.round(px * 100) / 100,
        etiqueta: el.tagName.toLowerCase() + (t !== 'text' ? '[' + t + ']' : ''),
        nombre: el.getAttribute('name') || el.getAttribute('aria-label') || el.id || '',
        clase: (el.getAttribute('class') || '').slice(0, 80),
      });
    }
  }
  return malos;
})()`;

/** El drawer, cuando está abierto y ocupando la pantalla. */
const DRAWER_ABIERTO = `(() => {
  const a = document.querySelector('aside.sytue-drawer-in');
  if (!a) return null;
  const r = a.getBoundingClientRect();
  return { ancho: Math.round(r.width), alto: Math.round(r.height) };
})()`;

const hallazgos = [];
function anotar(app, tipo, detalle) {
  hallazgos.push({ app, tipo, ...detalle });
}

// `--host-resolver-rules` cuando el DNS del equipo no resuelve. No cambia nada
// de la máquina: solo le dice a ESTE Chromium a qué IP va cada host, y el `Host`
// y el SNI siguen siendo los de verdad. Se usó el 2026-09-07, con iCloud Private
// Relay dejando un resolver que no contestaba (`192.0.2.42` en un utun).
const reglasDNS = process.env.SYTUE_AUDIT_DNS_RULES || "";
const navegador = await chromium.launch({
  args: reglasDNS ? [`--host-resolver-rules=${reglasDNS}`] : [],
});
const ctx = await navegador.newContext({
  ...PERFIL,
  ...(existsSync(STORAGE) ? { storageState: STORAGE } : {}),
});
const page = await ctx.newPage();

for (const app of objetivo) {
  const url = `https://${app === "portal" ? "one" : app}.${DOMINIO}${BASES[app]}`;
  process.stdout.write(`\n▸ ${app.padEnd(10)} ${url}\n`);
  try {
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 45_000 });
    if (await entrar(page)) {
      await ctx.storageState({ path: STORAGE });
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 45_000 });
    }
    await page.waitForTimeout(3500); // hidratación de las islas

    // DÓNDE SE MIDIÓ, y no se da por hecho. Si la sesión no cuaja, la página
    // que hay delante es la del IdP y medirla como si fuera la app es el «ok»
    // falso que este harness existe para no dar. Se anota y se sigue: los
    // campos del IdP también cuentan —es la primera pantalla en un móvil— pero
    // etiquetados como suyos.
    const donde = new URL(page.url()).hostname === IDP ? "idp" : app;
    if (donde === "idp") console.log(`   ⚠ sin sesión: se mide el IdP, no ${app}`);

    // (1) Campos que amplían.
    const campos = await page.evaluate(CAMPOS_QUE_AMPLIAN);
    for (const c of campos) anotar(donde, "zoom-ios", { ...c, visto_en: app });
    console.log(`   campos < 16px: ${campos.length}`);

    // (2) El menú lateral, y si se cierra al usarlo.
    // El botón que abre el menú se identifica por CONTROLAR un <aside>, no por su
    // id: lo genera `useId` y no contiene nada reconocible.
    const toggle = page
      .locator('button[aria-controls][aria-expanded], header button[aria-controls]')
      .first();
    let estado = "sin-menu";
    if (await toggle.count()) {
      await toggle.click().catch(() => {});
      await page.waitForTimeout(600);
      const abierto = await page.evaluate(DRAWER_ABIERTO);
      if (abierto) {
        // Se pulsa el PRIMER control del menú que no sea el propio cierre.
        const dentro = page.locator(
          'aside.sytue-drawer-in a[href], aside.sytue-drawer-in button:not([aria-label*="err"])',
        );
        if (await dentro.count()) {
          await dentro.first().click({ timeout: 5_000 }).catch(() => {});
          await page.waitForTimeout(900);
          const sigue = await page.evaluate(DRAWER_ABIERTO);
          estado = sigue ? "NO-SE-CIERRA" : "se-cierra";
          if (sigue) anotar(app, "drawer-no-cierra", { ...sigue });
        } else estado = "menu-vacio";
        await page.keyboard.press("Escape").catch(() => {});
      } else estado = "no-abre";
    }
    console.log(`   menú lateral:  ${estado}`);

    // (3) Rutas de sesión bajo el prefijo de la app y sin él.
    for (const ruta of ["/auth/logout", "/auth/session"]) {
      const sin = await page.evaluate(
        async (u) => (await fetch(u, { method: "HEAD", redirect: "manual" })).status,
        ruta,
      );
      const con = await page.evaluate(
        async (u) => (await fetch(u, { method: "HEAD", redirect: "manual" })).status,
        BASES[app] + ruta,
      );
      console.log(`   ${ruta.padEnd(15)} sin prefijo: ${sin}   con prefijo: ${con}`);
      if (sin === 404 && con !== 404) anotar(app, "ruta-sin-prefijo", { ruta, sin, con });
    }
  } catch (e) {
    console.log(`   ERROR: ${String(e).split("\n")[0].slice(0, 100)}`);
    anotar(app, "error", { mensaje: String(e).split("\n")[0].slice(0, 160) });
  }
}

await navegador.close();
writeReport(INFORME, JSON.stringify(hallazgos, null, 2));
console.log(`\n${"═".repeat(60)}`);
const porTipo = {};
for (const h of hallazgos) porTipo[h.tipo] = (porTipo[h.tipo] || 0) + 1;
console.log("HALLAZGOS:", JSON.stringify(porTipo));
console.log(`Detalle en ${INFORME}`);
process.exit(hallazgos.length ? 1 : 0);
