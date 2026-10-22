// ¿Están los iconos de la barra superior donde parece que están?
//
// POR QUÉ EXISTE. La topbar es la pieza que se ve en las trece apps, y sus
// desajustes son de los que nadie sabe nombrar pero todo el mundo nota: el logo
// pegado al cristal y el avatar con aire, un botón dos píxeles más arriba que su
// vecino, una caja de 36px dentro de una barra de 56 que no reparte igual arriba
// que abajo.
//
// Mide en la suite DESPLEGADA, que es donde importa, y saca números en vez de
// impresiones: para cada elemento de la barra, cuánto hay de él a cada borde y
// cuánto mide su caja. Lo que se mira:
//
//   · VERTICAL   — |arriba − abajo| debe ser 0. Si no, el elemento está alto o
//                  bajo dentro de la barra.
//   · CANALÓN    — el contenido visible del primer y del último elemento debe
//                  estar a la MISMA distancia de su borde. Un botón icon-only
//                  lleva su icono metido hacia dentro por su propio relleno; una
//                  imagen suelta no. Alinear las CAJAS deja los CONTENIDOS
//                  descuadrados, y es el contenido lo que se ve.
//   · ALTURAS    — botones de alturas distintas en la misma fila.
//
// Uso:
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… bun run auditar-topbar.mjs [app…]
import { chromium } from "playwright";
import { activarOrganizacion, completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_SHOT_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const BASE = `https://${TENANT ? `${TENANT}.${DOMINIO}` : DOMINIO}`;

const CRED = {
  email: process.env.SYTUE_SHOT_EMAIL || process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_SHOT_PASS || process.env.SYTUE_AUDIT_PASS || "",
};
if (!CRED.email || !CRED.pass) {
  console.error("Faltan credenciales: SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.");
  process.exit(2);
}

/** Las pantallas donde vive la barra. El portal va sin sesión. */
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

/**
 * Mide la barra y lo que lleva dentro.
 *
 * El «contenido» de un elemento no es su caja: un botón de 36px con un icono de
 * 20 tiene 8px de relleno a cada lado, y esos 8px cuentan a la hora de mirar si
 * el icono está alineado con la marca. Por eso se mide también el primer hijo
 * pintado (el `svg` o el `img`), que es lo que el ojo ve.
 */
const MEDIR = () => {
  const barra = document.querySelector('header[role="banner"]');
  if (!barra) return { error: "sin <header role=banner>" };
  const rb = barra.getBoundingClientRect();
  const cs = getComputedStyle(barra);

  const visible = (el) => {
    const r = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    return (
      r.width > 0 &&
      r.height > 0 &&
      s.visibility !== "hidden" &&
      s.opacity !== "0"
    );
  };

  const pintado = (el) => {
    // Lo que se ve dentro del control: el icono o la imagen. Si no hay, la caja.
    const dentro = el.querySelector("svg, img");
    return dentro && visible(dentro) ? dentro : el;
  };

  const items = [
    ...barra.querySelectorAll("button, a, img, input, [role='button']"),
  ]
    .filter(visible)
    // Solo el nivel de arriba: un <img> dentro de un <a> lo trae ya `pintado`.
    .filter(
      (el) => !el.parentElement?.closest("button, a[href], [role='button']"),
    )
    .map((el) => {
      const r = el.getBoundingClientRect();
      const p = pintado(el).getBoundingClientRect();
      const etiqueta =
        el.getAttribute("aria-label") ||
        el.getAttribute("alt") ||
        el.querySelector("img")?.alt ||
        (el.textContent || "").trim().slice(0, 18) ||
        el.tagName.toLowerCase();
      return {
        etiqueta,
        tag: el.tagName.toLowerCase(),
        // Caja
        arriba: +(r.top - rb.top).toFixed(1),
        abajo: +(rb.bottom - r.bottom).toFixed(1),
        izq: +(r.left - rb.left).toFixed(1),
        der: +(rb.right - r.right).toFixed(1),
        alto: +r.height.toFixed(1),
        ancho: +r.width.toFixed(1),
        // Lo que se ve
        vIzq: +(p.left - rb.left).toFixed(1),
        vDer: +(rb.right - p.right).toFixed(1),
        vArriba: +(p.top - rb.top).toFixed(1),
        vAbajo: +(rb.bottom - p.bottom).toFixed(1),
      };
    });

  return {
    // Imágenes ROTAS en la barra: terminadas y sin píxeles. Es el avatar del
    // 2026-09-24, que salía como icono de imagen rota junto a la campana en las
    // trece apps porque su URL daba 404. Un `<img>` que aún carga no está
    // `complete`, así que esto no cuenta los diferidos.
    rotas: [...barra.querySelectorAll("img")]
      .filter((i) => i.complete && i.naturalWidth === 0)
      .map((i) => `${i.alt || "(sin alt)"} ← ${i.currentSrc || i.src}`),
    alto: +rb.height.toFixed(1),
    ancho: +rb.width.toFixed(1),
    padL: +parseFloat(cs.paddingLeft).toFixed(1),
    padR: +parseFloat(cs.paddingRight).toFixed(1),
    items,
  };
};

/**
 * El MISMO defecto, pero en las demás filas de la app.
 *
 * El patrón que desnivela la topbar —altura fija + borde, y el borde comiéndose
 * un píxel del reparto— se repite en cada cabecera de lista, cada barra de
 * herramientas y cada pie. Aquí se buscan TODAS: cualquier caja que centre a sus
 * hijos, tenga altura fija y línea arriba o abajo. Se informa solo de las que
 * salgan torcidas, que si no la lista es la mitad del DOM.
 */
const MEDIR_FILAS = () => {
  const fuera = [];
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    return (
      r.width > 8 &&
      r.height > 8 &&
      s.visibility !== "hidden" &&
      s.opacity !== "0"
    );
  };
  for (const el of document.querySelectorAll(
    "div, header, footer, nav, section, ul",
  )) {
    const s = getComputedStyle(el);
    if (!/flex|grid/.test(s.display) || s.alignItems !== "center") continue;
    const bordeArriba = parseFloat(s.borderTopWidth) || 0;
    const bordeAbajo = parseFloat(s.borderBottomWidth) || 0;
    if (bordeArriba + bordeAbajo === 0) continue;
    const r = el.getBoundingClientRect();
    if (r.height < 24 || r.height > 96 || !visible(el)) continue;
    // Solo filas con altura IMPUESTA: si el alto lo marca el contenido, el borde
    // no le quita nada a nadie.
    const alturaFija =
      s.height !== "auto" && (s.flexShrink === "0" || /px/.test(s.height));
    if (!alturaFija) continue;

    const hijos = [...el.children].filter(visible);
    if (hijos.length === 0) continue;
    for (const h of hijos) {
      const rh = h.getBoundingClientRect();
      const arriba = +(rh.top - r.top).toFixed(1);
      const abajo = +(r.bottom - rh.bottom).toFixed(1);
      if (Math.abs(arriba - abajo) < 0.9) continue;
      if (rh.height >= r.height - 1) continue; // ocupa toda la fila: no aplica
      fuera.push({
        fila: (el.className || el.tagName).toString().slice(0, 42),
        alto: +r.height.toFixed(1),
        hijo: (
          h.getAttribute("aria-label") ||
          (h.textContent || "").trim() ||
          h.tagName
        ).slice(0, 22),
        desnivel: +(arriba - abajo).toFixed(1),
      });
      break; // uno por fila basta para señalarla
    }
  }
  return fuera;
};

const navegador = await chromium.launch();
const contexto = await navegador.newContext({
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
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

// Lo que contestó cada petición de un avatar, para saber si la foto de la barra
// llegó. No basta con mirar el `<img>`: si falla, <Avatar> lo cambia por las
// iniciales y en el DOM ya no queda nada roto que ver.
const avatares = new Map();
page.on("response", (r) => {
  if (/\/auth\/avatar\b|\/avatar\?v=/.test(r.url()))
    avatares.set(r.url(), r.status());
});

const pedidas = process.argv.slice(2).filter((a) => !a.startsWith("-"));
const apps = pedidas.length ? pedidas : Object.keys(PANTALLAS);
const hallazgos = [];

for (const app of apps) {
  const url = PANTALLAS[app];
  if (!url) {
    console.log(`✗ ${app}: no sé dónde mirar`);
    continue;
  }
  try {
    avatares.clear();
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60_000 });
    await asegurarSesion();
    await page
      .waitForLoadState("networkidle", { timeout: 12_000 })
      .catch(() => {});
    await page.waitForTimeout(1_200);

    const m = await page.evaluate(MEDIR);
    if (m.error) {
      console.log(`✗ ${app.padEnd(9)} ${m.error}`);
      continue;
    }

    console.log(
      `\n### ${app} — barra ${m.alto}px, relleno ${m.padL}/${m.padR}`,
    );
    for (const it of m.items) {
      const desnivel = +(it.arriba - it.abajo).toFixed(1);
      const marca = Math.abs(desnivel) > 0.5 ? " ⟵ DESNIVELADO" : "";
      console.log(
        `  ${it.etiqueta.padEnd(20)} ${String(it.ancho).padStart(5)}×${String(it.alto).padEnd(5)}` +
          ` arriba ${String(it.arriba).padStart(5)} abajo ${String(it.abajo).padStart(5)}${marca}`,
      );
      if (Math.abs(desnivel) > 0.5) {
        hallazgos.push(
          `${app}: «${it.etiqueta}» ${desnivel > 0 ? "bajo" : "alto"} ${Math.abs(desnivel)}px`,
        );
      }
    }

    // Canalón: lo que se VE del primero contra lo que se VE del último.
    const primero = m.items[0];
    const ultimo = m.items[m.items.length - 1];
    if (primero && ultimo && primero !== ultimo) {
      const izq = primero.vIzq;
      const der = ultimo.vDer;
      const dif = +(izq - der).toFixed(1);
      console.log(
        `  canalón visible: izquierda ${izq}px («${primero.etiqueta}»), derecha ${der}px («${ultimo.etiqueta}»)` +
          (Math.abs(dif) > 1 ? `  ⟵ ${Math.abs(dif)}px de diferencia` : "  ✓"),
      );
      if (Math.abs(dif) > 1) {
        hallazgos.push(
          `${app}: canalón ${izq} vs ${der} (${Math.abs(dif)}px de diferencia)`,
        );
      }
    }

    for (const r of m.rotas) {
      console.log(`  imagen rota: ${r}  ⟵`);
      hallazgos.push(`${app}: imagen rota en la barra (${r})`);
    }
    // Con foto en Sytue ID, la barra tiene que haberla traído: la URL es la que
    // dice la sesión y su respuesta un 200 o un 304.
    const prefijo = new URL(url).pathname.split("/")[1];
    if (prefijo) {
      const sesion = await page
        .evaluate(async (p) => {
          const r = await fetch(`/${p}/auth/session`);
          return r.ok ? r.json() : null;
        }, prefijo)
        .catch(() => null);
      const foto = sesion?.user?.picture;
      if (foto) {
        const pedida = new URL(foto, page.url()).href;
        const estado = avatares.get(pedida);
        // Servida de la caché del navegador no hay respuesta que ver: vale que
        // el `<img>` esté pintado con píxeles.
        const pintada = await page.evaluate(
          (src) =>
            [...document.images].some(
              (i) => i.src === src && i.naturalWidth > 0,
            ),
          pedida,
        );
        const bien = estado === 200 || estado === 304 || pintada;
        console.log(
          `  avatar: ${foto} → ${estado ?? "no se pidió"}${bien ? "  ✓" : "  ⟵"}`,
        );
        if (!bien)
          hallazgos.push(
            `${app}: el avatar no carga (${foto} → ${estado ?? "no se pidió"})`,
          );
      }
    }

    // El mismo defecto en las demás filas de la app.
    const filas = await page.evaluate(MEDIR_FILAS);
    if (filas.length) {
      console.log(`  otras filas torcidas: ${filas.length}`);
      for (const f of filas.slice(0, 6)) {
        console.log(
          `    ${f.alto}px «${f.hijo}» ${f.desnivel > 0 ? "bajo" : "alto"} ${Math.abs(f.desnivel)}px — ${f.fila}`,
        );
      }
      hallazgos.push(
        `${app}: ${filas.length} fila(s) más con el contenido descentrado`,
      );
    }

    // Alturas distintas en la misma fila.
    const alturas = [
      ...new Set(m.items.filter((i) => i.tag === "button").map((i) => i.alto)),
    ];
    if (alturas.length > 1) {
      console.log(`  alturas de botón distintas: ${alturas.join(", ")}px  ⟵`);
      hallazgos.push(
        `${app}: botones de ${alturas.join(", ")}px en la misma barra`,
      );
    }
  } catch (e) {
    console.log(`✗ ${app}: ${e.message.split("\n")[0]}`);
  }
}

await navegador.close();
console.log(`\n${hallazgos.length} hallazgos`);
for (const h of hallazgos) console.log(`  · ${h}`);
