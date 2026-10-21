// ¿Está cada icono centrado en su botón, y miden lo mismo los botones que
// comparten fila?
//
// POR QUÉ EXISTE. `auditar-topbar.mjs` mira la barra superior. Este mira TODO lo
// demás: cada botón visible de cada pantalla, en las once apps y en el portal.
// Son los desajustes que nadie sabe nombrar y todo el mundo nota —un icono medio
// píxel a la izquierda de su caja, dos botones vecinos de alturas distintas, un
// control de 24px donde el dedo necesita 44—, y a ojo no se encuentran: hay que
// medirlos.
//
// Lo que se mira, y por qué cada cosa:
//
//   · ICONO DESCENTRADO — en un botón de solo icono, el icono tiene que estar a
//     la misma distancia de los cuatro bordes. Si no, el velo del hover sale
//     torcido respecto a lo que se ve dentro.
//   · FILA DESPAREJA — botones que comparten fila y no comparten altura. El ojo
//     lee una fila como una unidad; una caja más alta que su vecina la rompe.
//   · DEDO — área táctil, con DOS listones que no significan lo mismo: 24×24 es
//     WCAG 2.5.8 (AA) y hay que cumplirlo; 44×44 es 2.5.5 (AAA) y es lo deseable
//     donde quepa. Mezclarlos convierte un fallo en una sugerencia y al revés.
//
// Uso:
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… bun run auditar-controles.mjs [app…]
//   … --movil   la misma pasada en un teléfono (390×844, puntero grueso)
import { chromium } from "playwright";
import { activarOrganizacion, completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_SHOT_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const BASE = `https://${TENANT ? `${TENANT}.${DOMINIO}` : DOMINIO}`;
/** Cuánto se tolera antes de llamarlo torcido. Medio píxel es redondeo. */
const TOLERANCIA = 1;

const CRED = {
  email: process.env.SYTUE_SHOT_EMAIL || process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_SHOT_PASS || process.env.SYTUE_AUDIT_PASS || "",
};
if (!CRED.email || !CRED.pass) {
  console.error("Faltan credenciales: SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.");
  process.exit(2);
}

const PANTALLAS = {
  one: `${BASE}/one`,
  mail: `${BASE}/mail`,
  drive: `${BASE}/drive`,
  docs: `${BASE}/docus`,
  sheets: `${BASE}/calcus`,
  slides: `${BASE}/diapos`,
  pdf: `${BASE}/certipdf`,
  calendar: `${BASE}/calendar`,
  contacts: `${BASE}/contacts`,
  notes: `${BASE}/notes`,
  photos: `${BASE}/photos`,
  rooms: `${BASE}/rooms`,
  cast: `${BASE}/cast`,
  pages: `${BASE}/web`,
  account: `${BASE}/account`,
  portal: `https://${DOMINIO}/`,
};

const MEDIR = (tolerancia) => {
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    return (
      r.width > 4 &&
      r.height > 4 &&
      r.top < innerHeight &&
      r.bottom > 0 &&
      s.visibility !== "hidden" &&
      s.opacity !== "0"
    );
  };

  const nombre = (el) =>
    (
      el.getAttribute("aria-label") ||
      el.getAttribute("title") ||
      (el.textContent || "").trim() ||
      el.querySelector("img")?.alt ||
      el.tagName
    )
      .toString()
      .replace(/\s+/g, " ")
      .slice(0, 26);

  const controles = [
    ...document.querySelectorAll("button, a[href], [role='button'], summary"),
  ]
    .filter(visible)
    // Un control dentro de otro se mide una vez: el de fuera es el que se pulsa.
    .filter((el) => !el.parentElement?.closest("button, [role='button']"));

  const descentrados = [];
  const pequenos = [];
  for (const el of controles) {
    const r = el.getBoundingClientRect();
    const texto = (el.textContent || "").trim();
    const icono = el.querySelector("svg, img");

    // Icono solo: el icono debe repartir igual a los cuatro lados.
    if (icono && !texto && visible(icono)) {
      const ri = icono.getBoundingClientRect();
      const dx = +(ri.left - r.left - (r.right - ri.right)).toFixed(1);
      const dy = +(ri.top - r.top - (r.bottom - ri.bottom)).toFixed(1);
      if (Math.abs(dx) > tolerancia || Math.abs(dy) > tolerancia) {
        descentrados.push({
          nombre: nombre(el),
          caja: `${r.width.toFixed(0)}×${r.height.toFixed(0)}`,
          dx,
          dy,
        });
      }
    }

    // Dedo: SOLO tiene sentido donde se toca con el dedo. Con ratón, 32px es un
    // objetivo cómodo y WCAG pide 24; medirlo en escritorio daba trescientos
    // «fallos» que no lo son. La pasada táctil es la que cuenta, y ahí las
    // utilidades `pointer-coarse:` del design system ya están aplicadas.
    //
    // DOS LISTONES, Y NO SIGNIFICAN LO MISMO:
    //
    //   · 24×24 es WCAG 2.5.8, nivel AA. Es el que hay que cumplir —EN 301 549,
    //     la norma que nos aplica, pide AA— y por debajo de ahí es un fallo, no
    //     una mejora pendiente.
    //   · 44×44 es WCAG 2.5.5, nivel AAA. Es lo deseable, y donde cabe se pone;
    //     pero forzarlo en la fila de siete acciones de un mensaje la sacaría de
    //     la pantalla de un teléfono. Se informa aparte para no confundir «se
    //     puede mejorar» con «está mal».
    if (matchMedia("(pointer: coarse)").matches) {
      const lado = Math.min(r.width, r.height);
      const caja = `${r.width.toFixed(0)}×${r.height.toFixed(0)}`;
      if (lado < 24) pequenos.push({ nombre: nombre(el), caja, nivel: "AA" });
      else if (lado < 44)
        pequenos.push({ nombre: nombre(el), caja, nivel: "AAA" });
    }
  }

  // Filas: controles que comparten CONTENEDOR y renglón.
  //
  // Agrupar solo por coordenada vertical juntaba cosas que nadie ve juntas —un
  // botón del rail izquierdo con una reacción de un mensaje del centro— y el
  // informe se llenaba de parejas que no son pareja. Una fila de verdad es la que
  // arma un contenedor: mismo padre flex, y además solapadas.
  const filaDe = (el) => {
    const alto = el.getBoundingClientRect().height;
    for (
      let p = el.parentElement;
      p && p !== document.body;
      p = p.parentElement
    ) {
      const s = getComputedStyle(p);
      if (!/flex|grid/.test(s.display) || s.flexDirection === "column")
        continue;
      // UNA FILA ES BAJA. El layout de dos columnas también es un flex en fila, y
      // por él se colaban parejas como «el enlace del rail izquierdo mide 33 y la
      // fila de la tabla del centro mide 51»: verdad, y no son fila. Si el
      // contenedor es mucho más alto que el control, es una zona, no un renglón.
      if (p.getBoundingClientRect().height > Math.max(alto * 2, 96))
        return null;
      return p;
    }
    return null;
  };
  const porContenedor = new Map();
  for (const el of controles) {
    const cont = filaDe(el);
    if (!cont) continue;
    if (!porContenedor.has(cont)) porContenedor.set(cont, []);
    porContenedor.get(cont).push(el);
  }
  const filas = [];
  for (const [, hermanos] of porContenedor) {
    for (const el of hermanos) {
      const r = el.getBoundingClientRect();
      const fila = filas.find((f) => {
        if (f.cont !== filaDe(el)) return false;
        const solape = Math.min(f.bottom, r.bottom) - Math.max(f.top, r.top);
        return solape > Math.min(f.bottom - f.top, r.height) * 0.6;
      });
      if (fila) {
        fila.top = Math.min(fila.top, r.top);
        fila.bottom = Math.max(fila.bottom, r.bottom);
        fila.items.push({ nombre: nombre(el), alto: +r.height.toFixed(1) });
      } else {
        filas.push({
          cont: filaDe(el),
          top: r.top,
          bottom: r.bottom,
          items: [{ nombre: nombre(el), alto: +r.height.toFixed(1) }],
        });
      }
    }
  }
  const desparejas = filas
    .filter((f) => f.items.length > 1)
    .map((f) => ({
      alturas: [...new Set(f.items.map((i) => i.alto))],
      items: f.items,
    }))
    .filter((f) => f.alturas.length > 1)
    // Diferencias de menos de 2px son redondeo de tipografía, no diseño.
    .filter((f) => Math.max(...f.alturas) - Math.min(...f.alturas) >= 2);

  return { total: controles.length, descentrados, pequenos, desparejas };
};

const TACTIL = process.argv.includes("--movil");

const navegador = await chromium.launch();
const contexto = await navegador.newContext({
  ignoreHTTPSErrors: true,
  // Con `--movil`, un teléfono de verdad: viewport estrecho y puntero grueso, que
  // es lo que activa las reservas de 44px del design system.
  ...(TACTIL
    ? {
        viewport: { width: 390, height: 844 },
        hasTouch: true,
        isMobile: true,
        deviceScaleFactor: 3,
      }
    : { viewport: { width: 1440, height: 900 } }),
  colorScheme: "light",
  locale: "es-ES",
});
const page = await contexto.newPage();

async function asegurarSesion() {
  if (new URL(page.url()).hostname !== IDP) return;
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  await completarMFASiHaceFalta(page).catch(() => {});
  await page.waitForURL((u) => u.hostname !== IDP, { timeout: 90_000 });
  await activarOrganizacion(page);
}

const pedidas = process.argv.slice(2).filter((a) => !a.startsWith("-"));
const apps = pedidas.length ? pedidas : Object.keys(PANTALLAS);
let totalDescentrados = 0;
let totalDesparejas = 0;
let totalPequenos = 0;

for (const app of apps) {
  const url = PANTALLAS[app];
  if (!url) {
    console.log(`✗ ${app}: no sé dónde mirar`);
    continue;
  }
  try {
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60_000 });
    await asegurarSesion();
    await page
      .waitForLoadState("networkidle", { timeout: 12_000 })
      .catch(() => {});
    await page.waitForTimeout(1_200);

    const m = await page.evaluate(MEDIR, TOLERANCIA);
    console.log(`\n### ${app} — ${m.total} controles a la vista`);

    if (m.descentrados.length) {
      console.log(
        `  iconos descentrados en su botón: ${m.descentrados.length}`,
      );
      for (const d of m.descentrados.slice(0, 8)) {
        const ejes = [
          Math.abs(d.dx) > TOLERANCIA
            ? `${d.dx > 0 ? "derecha" : "izquierda"} ${Math.abs(d.dx)}px`
            : "",
          Math.abs(d.dy) > TOLERANCIA
            ? `${d.dy > 0 ? "abajo" : "arriba"} ${Math.abs(d.dy)}px`
            : "",
        ].filter(Boolean);
        console.log(`    «${d.nombre}» ${d.caja} — ${ejes.join(", ")}`);
      }
    }
    if (m.desparejas.length) {
      console.log(
        `  filas con botones de alturas distintas: ${m.desparejas.length}`,
      );
      for (const f of m.desparejas.slice(0, 6)) {
        console.log(
          `    ${f.alturas.join(" / ")}px — ${f.items
            .map((i) => `«${i.nombre}» ${i.alto}`)
            .slice(0, 4)
            .join(", ")}`,
        );
      }
    }
    if (m.pequenos.length) {
      const incumplen = m.pequenos.filter((p) => p.nivel === "AA");
      const mejorables = m.pequenos.filter((p) => p.nivel === "AAA");
      if (incumplen.length) {
        console.log(
          `  ✗ por debajo de 24px (WCAG 2.5.8 AA — esto es un fallo): ${incumplen.length}`,
        );
        for (const p of incumplen) console.log(`      «${p.nombre}» ${p.caja}`);
      }
      if (mejorables.length) {
        console.log(
          `  · entre 24 y 44px (WCAG 2.5.5 AAA — mejorable donde quepa): ${mejorables.length}`,
        );
        for (const p of mejorables.slice(0, 5))
          console.log(`      «${p.nombre}» ${p.caja}`);
      }
    }
    if (!m.descentrados.length && !m.desparejas.length && !m.pequenos.length) {
      console.log("  ✓ nada que señalar");
    }
    totalDescentrados += m.descentrados.length;
    totalDesparejas += m.desparejas.length;
    totalPequenos += m.pequenos.length;
  } catch (e) {
    console.log(`✗ ${app}: ${e.message.split("\n")[0]}`);
  }
}

await navegador.close();
console.log(
  `\nTOTAL: ${totalDescentrados} iconos descentrados, ${totalDesparejas} filas desparejas, ` +
    `${totalPequenos} controles pequeños para el dedo`,
);
