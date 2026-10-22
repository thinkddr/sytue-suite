/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * INVENTARIO DE MEDIDAS de la web, contra producción, para portar la interfaz
 * nativa (Fyne) con paridad 1:1 a nivel de píxel.
 *
 * POR QUÉ ES GENÉRICA Y NO UNA SONDA POR APP. Hasta hoy cada agente que medía
 * una pantalla escribía su propia sonda con sus propios selectores
 * (`con-medir.mjs`, `photos-medir.mjs`, `docus-ref.mjs`, `diapos-ref.mjs`…):
 * dieciséis productos medidos con dieciséis criterios distintos, y nada
 * comparable entre sí. Esto recorre el DOM entero y saca **inventarios**
 * (tipografía, color, espaciado, radios, sombras, controles, iconos, filas
 * repetidas, animaciones, foco, diálogos) con el MISMO criterio en todas.
 *
 * NO MIDE LA CAPTURA A OJO: cada número sale de `getBoundingClientRect()` o de
 * `getComputedStyle()`, y cada entrada lleva el selector del que salió y la
 * captura donde se ve. Se puede volver a generar entera con este guion.
 *
 * Uso (EN EL PILOTO, nunca en el portátil — 4 GB):
 *   cd ~/sytue-one/test/e2e
 *   set -a; . ~/.config/sytue/audit.env; set +a
 *   bun run medidas-web.mjs                    # las 16 apps
 *   bun run medidas-web.mjs photos rooms       # solo esas
 *   SALIDA=~/medidas bun run medidas-web.mjs
 *   OSCURO=1 SALIDA=~/medidas-oscuro bun run medidas-web.mjs   # el barrido entero en oscuro
 *
 * Salida: <SALIDA>/<app>/<pantalla>.<variante>.json + .png
 * El informe legible lo arma `medidas-informe.mjs` a partir de esos JSON.
 */
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { mkdirSync, writeFileSync } from "node:fs";
import { activarOrganizacion, completarMFASiHaceFalta, porQueNoEntra } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_MEDIDAS_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const BASE = `https://${TENANT ? `${TENANT}.` : ""}${DOMINIO}`;
/** La base de una pantalla: su host propio si lo pide, y si no el del tenant. */
const baseDe = (scr) => (scr.h ? `https://${scr.h}.${DOMINIO}` : BASE);
const SALIDA = process.env.SALIDA || `${process.env.HOME}/medidas`;
const STORAGE = fileURLToPath(new URL(".auth/prod-audit.json", import.meta.url));
const CRED = { email: process.env.SYTUE_AUDIT_EMAIL || "", pass: process.env.SYTUE_AUDIT_PASS || "" };

const VP_ANCHO = { width: 1280, height: 800 };
// `OSCURO=1` hace el barrido ENTERO en oscuro —pantallas, diálogos y estrecho—, no solo
// la pasada 4, que mide la pantalla de entrada y ningún diálogo.
const ESQUEMA = process.env.OSCURO ? "dark" : "light";
const VP_ESTRECHO = { width: 390, height: 844 };

/*
 * Catálogo de pantallas. Las rutas SALEN DEL ÁRBOL DE PÁGINAS
 * (`apps/frontend/apps/<app>/src/pages/`), no de la memoria, y llevan el
 * prefijo `base` de cada app porque el tenant sirve todas bajo el mismo host.
 *
 * `click` abre un estado que la URL no alcanza (un editor, un canal). Es
 * SIEMPRE una acción de lectura: aquí se mide contra producción y no se
 * escribe nada que no se pueda deshacer solo.
 * `estados: true` añade las pasadas de sin-conexión y de ruta inexistente; va
 * en una pantalla por app, que es donde el chrome de estado se ve igual.
 * `h` cambia el host: casi todo se sirve bajo el tenant, pero la consola de
 * plataforma NO —`corporate.<dom>/admin` redirige a `/account`, y sin esto se
 * medía «Mi cuenta» creyendo que era la consola—, así que va por `admin.<dom>`.
 */
const APPS = {
  // --- Las seis ya medidas por otros agentes: van con el mismo criterio para
  // que el inventario sea comparable de punta a punta.
  portal: [{ n: "inicio", u: "/one", estados: true }, { n: "buscar", u: "/one/buscar" }, { n: "actividad", u: "/one/actividad" }, { n: "datos", u: "/one/datos" }, { n: "dispositivos", u: "/one/dispositivos" }, { n: "descargas", u: "/one/descargas" }],
  mail: [{ n: "bandeja", u: "/mail", estados: true }],
  drive: [{ n: "archivos", u: "/drive", estados: true }],
  calendar: [{ n: "mes", u: "/calendar", estados: true }, { n: "evento-nuevo", u: "/calendar/events/new" }],
  contacts: [{ n: "lista", u: "/contacts", estados: true }],
  notes: [{ n: "lista", u: "/notes", estados: true }],
  // --- Las diez que faltaban.
  account: [{ n: "portada", u: "/account", estados: true }],
  admin: [
    { n: "portada", u: "/admin", h: "admin", estados: true },
    { n: "usuarios", u: "/admin/users", h: "admin" },
    { n: "roles", u: "/admin/roles", h: "admin" },
    { n: "auditoria", u: "/admin/audit", h: "admin" },
    { n: "aplicaciones", u: "/admin/apps", h: "admin" },
    { n: "dispositivos", u: "/admin/devices", h: "admin" },
    { n: "grupos", u: "/admin/groups", h: "admin" },
    { n: "cumplimiento", u: "/admin/compliance", h: "admin" },
    { n: "sso", u: "/admin/sso", h: "admin" },
    { n: "etiquetas-sensibilidad", u: "/admin/sensitivity-labels", h: "admin" },
  ],
  avisos: [{ n: "notificaciones", u: "/one/notificaciones", estados: true }],
  // El editor se abre con `a[href*="/edit?id="]`: desde el ADR 0056 el enlace lleva la
  // organización (`/calcus/<org>/edit?id=…`) y un `/calcus/edit` exacto no casaría —las
  // tres pasadas del editor morían por tiempo (medido el 2026-09-27)—. Desde el ADR 0076
  // cada producto tiene su app y su prefijo.
  docs: [
    { n: "portada", u: "/docus/", estados: true },
    { n: "editor", u: "/docus/", click: 'a[href*="/edit?id="]' },
  ],
  sheets: [
    { n: "portada", u: "/calcus/", estados: true },
    { n: "editor", u: "/calcus/", click: 'a[href*="/edit?id="]' },
  ],
  slides: [
    { n: "portada", u: "/diapos/", estados: true },
    { n: "editor", u: "/diapos/", click: 'a[href*="/edit?id="]' },
  ],
  pdf: [{ n: "portada", u: "/certipdf/", estados: true }],
  // `?m=` es la CLAVE DE UN MODELO del catálogo de `services/syerp`
  // (`factura_venta`, `cliente`…), no el nombre de un módulo en inglés: con una
  // clave que no existe el ERP abre su portada y se mide dos veces la misma
  // pantalla creyendo que son dos.
  erp: [{ n: "portada", u: "/erp", estados: true }, { n: "facturas", u: "/erp?m=factura_venta" }, { n: "clientes", u: "/erp?m=cliente" }],
  // Sytue Web: «Mis sitios», el asistente y los ajustes de la organización. Solo mide
  // cuando el piloto la instala (`PAGES_ENABLED`); antes, sus tres rutas darían 404.
  pages: [{ n: "sitios", u: "/web", estados: true }, { n: "crear", u: "/web/new" }, { n: "ajustes", u: "/web/settings" }],
  photos: [
    { n: "cuadricula", u: "/photos", estados: true },
    { n: "albumes", u: "/photos/albums" },
    { n: "favoritos", u: "/photos/favorites" },
    { n: "papelera", u: "/photos/trash" },
    { n: "duplicados", u: "/photos/duplicates" },
    { n: "visor", u: "/photos", click: "main img, [role=grid] img, a[href*='/photos/p/']" },
  ],
  rooms: [
    { n: "portada", u: "/rooms", estados: true },
    // El canal se abre desde la lista lateral, que son BOTONES con `title`
    // (`ChannelRow.tsx:90`), no enlaces: la ruta `/rooms/channels/[channelId]`
    // existe pero la lista no la usa.
    { n: "canal", u: "/rooms", click: "nav button[title], aside button[title]" },
    { n: "compartir", u: "/rooms/share" },
  ],
};

const pedidas = process.argv.slice(2).filter((a) => !a.startsWith("-"));
const plan = Object.entries(APPS).filter(([a]) => !pedidas.length || pedidas.includes(a));

// ---------------------------------------------------------------------------
// La medida. Todo lo que sigue corre DENTRO de la página.
// ---------------------------------------------------------------------------

/**
 * Vuelca el DOM entero en inventarios agrupados.
 *
 * Agrupa por huella (tamaño+peso+color+…) en vez de listar mil elementos: lo
 * que un port necesita saber es «cuántos estilos de texto distintos hay y
 * cuáles», no el árbol. Cada grupo guarda hasta tres ejemplos con su selector.
 */
const VOLCADO = function volcado() {
  const px = (v) => (v == null || v === "" ? null : Math.round(parseFloat(v) * 100) / 100);
  const txt = (el) => (el.textContent || "").trim().replace(/\s+/g, " ").slice(0, 70) || null;

  /** Selector corto y estable: id / data-testid / aria-label / tag.clase, con abuelo. */
  const sel = (el) => {
    const uno = (e) => {
      if (!e || e.nodeType !== 1) return "";
      if (e.id) return `#${e.id}`;
      const t = e.getAttribute?.("data-testid");
      if (t) return `[data-testid="${t}"]`;
      const al = e.getAttribute?.("aria-label");
      if (al) return `${e.tagName.toLowerCase()}[aria-label="${al.slice(0, 32)}"]`;
      const r = e.getAttribute?.("role");
      const cls = [...(e.classList || [])].filter((c) => !/^(svelte|astro)-/.test(c)).slice(0, 3).join(".");
      return e.tagName.toLowerCase() + (r ? `[role=${r}]` : "") + (cls ? `.${cls}` : "");
    };
    const partes = [];
    for (let e = el, i = 0; e && i < 3; e = e.parentElement, i++) partes.unshift(uno(e));
    return partes.filter(Boolean).join(" > ");
  };

  // --- Tokens de diseño resueltos. Salen de las hojas de estilo, no de una
  // lista escrita a mano: si mañana hay un token nuevo, sale solo.
  const raiz = getComputedStyle(document.documentElement);
  const nombres = new Set();
  for (const hoja of document.styleSheets) {
    let reglas;
    try { reglas = hoja.cssRules; } catch { continue; }
    for (const r of reglas || []) for (const m of (r.cssText || "").matchAll(/(--[\w-]+)\s*:/g)) nombres.add(m[1]);
  }
  const tokens = {};
  for (const n of [...nombres].sort()) {
    const v = raiz.getPropertyValue(n).trim();
    if (v) tokens[n] = v;
  }
  // Índice inverso valor→token, para anotar cada color con su token.
  const norm = (c) => (c || "").replace(/\s+/g, "").toLowerCase();
  const porValor = {};
  const sonda = document.createElement("span");
  sonda.style.cssText = "position:absolute;visibility:hidden";
  document.body.appendChild(sonda);
  for (const [n, v] of Object.entries(tokens)) {
    if (!/color|border|bg|fondo|ink|surface|text|accent|brand|chrome|header|sidebar|selected/i.test(n)) continue;
    sonda.style.color = "";
    sonda.style.color = `var(${n})`;
    const resuelto = norm(getComputedStyle(sonda).color);
    if (resuelto && !porValor[resuelto]) porValor[resuelto] = n;
    if (!porValor[norm(v)]) porValor[norm(v)] = n;
  }
  sonda.remove();
  const token = (c) => porValor[norm(c)] || null;

  const P = [
    "display", "position", "boxSizing", "width", "height", "minHeight", "maxWidth", "minWidth",
    "fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing", "textTransform", "textAlign",
    "color", "backgroundColor", "backgroundImage", "opacity",
    "borderTopWidth", "borderRightWidth", "borderBottomWidth", "borderLeftWidth",
    "borderTopColor", "borderLeftColor", "borderStyle", "borderRadius",
    "boxShadow", "backdropFilter", "outlineWidth", "outlineColor", "outlineStyle", "outlineOffset",
    "paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
    "marginTop", "marginBottom", "gap", "rowGap", "columnGap",
    "flexDirection", "flexWrap", "alignItems", "justifyContent", "gridTemplateColumns",
    "overflowX", "overflowY", "zIndex", "transitionProperty", "transitionDuration",
    "transitionTimingFunction", "transitionDelay", "animationName", "animationDuration",
    "animationTimingFunction", "objectFit", "cursor",
  ];

  const visible = (el, s, r) =>
    s.display !== "none" && s.visibility !== "hidden" && parseFloat(s.opacity) > 0.01 && r.width > 0 && r.height > 0;

  const todos = [];
  for (const el of document.querySelectorAll("body *")) {
    const tag = el.tagName.toLowerCase();
    if (tag === "script" || tag === "style" || tag === "noscript" || tag === "link" || tag === "meta") continue;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    if (!visible(el, s, r)) continue;
    const est = {};
    for (const k of P) est[k] = s[k];
    todos.push({ el, tag, s: est, r: { x: px(r.x), y: px(r.y), w: px(r.width), h: px(r.height) }, t: txt(el) });
  }

  /** Agrupa por clave y guarda hasta 3 ejemplos con selector, caja y texto. */
  const agrupar = (items, clave, extra) => {
    const m = new Map();
    for (const it of items) {
      const k = clave(it);
      if (k == null) continue;
      let g = m.get(k);
      if (!g) m.set(k, (g = { clave: k, n: 0, ejemplos: [], ...(extra ? extra(it) : {}) }));
      g.n++;
      if (g.ejemplos.length < 3) g.ejemplos.push({ sel: sel(it.el), caja: it.r, texto: it.t });
    }
    return [...m.values()].sort((a, b) => b.n - a.n);
  };

  const out = { url: location.href, rem: px(raiz.fontSize), viewport: { w: innerWidth, h: innerHeight }, tokens, elementos: todos.length };

  // --- Regiones: el esqueleto de la pantalla.
  const REG = "header,nav,aside,main,footer,[role=dialog],[role=navigation],[role=main],[role=banner],[role=toolbar],[role=tablist],[role=grid],[role=table],table,section";
  out.regiones = todos
    .filter((it) => it.el.matches(REG))
    .slice(0, 40)
    .map((it) => ({
      sel: sel(it.el), tag: it.tag, caja: it.r,
      est: {
        bg: it.s.backgroundColor, bgToken: token(it.s.backgroundColor),
        padding: `${it.s.paddingTop} ${it.s.paddingRight} ${it.s.paddingBottom} ${it.s.paddingLeft}`,
        borde: `${it.s.borderTopWidth}/${it.s.borderRightWidth ?? ""}/${it.s.borderBottomWidth}/${it.s.borderLeftWidth} ${it.s.borderTopColor}`,
        bordeToken: token(it.s.borderTopColor),
        radio: it.s.borderRadius, sombra: it.s.boxShadow, gap: it.s.gap,
        display: it.s.display, direccion: it.s.flexDirection, columnas: it.s.gridTemplateColumns,
        overflow: `${it.s.overflowX}/${it.s.overflowY}`, z: it.s.zIndex, pos: it.s.position,
      },
    }));

  // --- Tipografía. OJO: `lineHeight` es el alto de la CAJA de línea; el glifo
  // va centrado dentro. Se guarda también en px para no tener que convertir.
  out.tipografia = agrupar(
    todos.filter((it) => it.t && it.el.children.length === 0),
    (it) => `${px(it.s.fontSize)}px/${it.s.lineHeight} ${it.s.fontWeight} ${it.s.letterSpacing} ${it.s.textTransform} ${it.s.color}`,
    (it) => ({
      px: px(it.s.fontSize), rem: px(it.s.fontSize) / px(raiz.fontSize),
      peso: it.s.fontWeight, interlineado: it.s.lineHeight, interlineadoPx: px(it.s.lineHeight),
      espaciado: it.s.letterSpacing, caja: it.s.textTransform, color: it.s.color,
      colorToken: token(it.s.color), familia: it.s.fontFamily.split(",")[0].replace(/["']/g, ""),
    }),
  );

  // --- Colores en uso, con su token cuando lo hay.
  const colores = new Map();
  const anota = (prop, v, it) => {
    if (!v || v === "rgba(0, 0, 0, 0)" || v === "none" || v === "transparent") return;
    const k = `${prop}|${v}`;
    let g = colores.get(k);
    if (!g) colores.set(k, (g = { prop, valor: v, token: token(v), n: 0, ejemplos: [] }));
    g.n++;
    if (g.ejemplos.length < 2) g.ejemplos.push({ sel: sel(it.el), texto: it.t });
  };
  for (const it of todos) {
    anota("texto", it.s.color, it);
    anota("fondo", it.s.backgroundColor, it);
    if (parseFloat(it.s.borderTopWidth) > 0 || parseFloat(it.s.borderLeftWidth) > 0) anota("borde", it.s.borderTopColor, it);
  }
  out.colores = [...colores.values()].sort((a, b) => b.n - a.n).slice(0, 60);

  // --- Espaciado, radios, bordes, sombras: histogramas.
  const hist = (f) => {
    const m = new Map();
    for (const it of todos) {
      const v = f(it);
      if (!v || v === "0px" || v === "none" || v === "normal") continue;
      let g = m.get(v);
      if (!g) m.set(v, (g = { valor: v, n: 0, ejemplos: [] }));
      g.n++;
      if (g.ejemplos.length < 2) g.ejemplos.push({ sel: sel(it.el), caja: it.r, texto: it.t });
    }
    return [...m.values()].sort((a, b) => b.n - a.n).slice(0, 24);
  };
  out.espaciado = {
    padding: hist((it) => `${it.s.paddingTop} ${it.s.paddingRight} ${it.s.paddingBottom} ${it.s.paddingLeft}`),
    gap: hist((it) => (it.s.display.includes("flex") || it.s.display.includes("grid") ? it.s.gap : null)),
  };
  out.radios = hist((it) => it.s.borderRadius);
  out.bordes = hist((it) =>
    parseFloat(it.s.borderTopWidth) || parseFloat(it.s.borderLeftWidth) || parseFloat(it.s.borderBottomWidth)
      ? `${it.s.borderTopWidth}/${it.s.borderRightWidth}/${it.s.borderBottomWidth}/${it.s.borderLeftWidth} ${it.s.borderStyle} ${it.s.borderTopColor}`
      : null,
  );
  out.sombras = hist((it) => it.s.boxShadow);

  // --- Controles: botones, campos, pestañas, casillas.
  out.controles = todos
    .filter((it) => it.el.matches("button,a[role=button],input,select,textarea,[role=tab],[role=switch],[role=checkbox],[role=menuitem]"))
    .slice(0, 60)
    .map((it) => ({
      sel: sel(it.el), tag: it.tag, tipo: it.el.getAttribute("type") || it.el.getAttribute("role") || null,
      texto: it.t || it.el.getAttribute("aria-label") || it.el.getAttribute("placeholder"),
      caja: it.r,
      est: {
        alto: it.r.h, padding: `${it.s.paddingTop} ${it.s.paddingRight} ${it.s.paddingBottom} ${it.s.paddingLeft}`,
        fuente: `${px(it.s.fontSize)}px/${it.s.lineHeight} ${it.s.fontWeight}`,
        fg: it.s.color, fgToken: token(it.s.color), bg: it.s.backgroundColor, bgToken: token(it.s.backgroundColor),
        radio: it.s.borderRadius, borde: `${it.s.borderTopWidth} ${it.s.borderStyle} ${it.s.borderTopColor}`,
        sombra: it.s.boxShadow, gap: it.s.gap, transicion: `${it.s.transitionProperty} ${it.s.transitionDuration} ${it.s.transitionTimingFunction}`,
      },
    }));

  // --- Iconos: talla real y grosor de trazo.
  out.iconos = agrupar(
    todos.filter((it) => it.tag === "svg"),
    (it) => `${Math.round(it.r.w)}x${Math.round(it.r.h)} ${it.el.getAttribute("stroke-width") || getComputedStyle(it.el).strokeWidth || ""} ${it.s.color}`,
    (it) => ({ w: it.r.w, h: it.r.h, trazo: it.el.getAttribute("stroke-width") || getComputedStyle(it.el).strokeWidth, color: it.s.color, colorToken: token(it.s.color) }),
  );

  // --- Filas / tarjetas repetidas: lo que más se pregunta al portar.
  out.repeticiones = [];
  for (const it of todos) {
    const hijos = [...it.el.children].filter((c) => {
      const cs = getComputedStyle(c);
      const cr = c.getBoundingClientRect();
      return visible(c, cs, cr);
    });
    if (hijos.length < 3) continue;
    const huella = (c) => c.tagName + "." + [...c.classList].slice(0, 4).join(".");
    const cuenta = new Map();
    for (const c of hijos) cuenta.set(huella(c), (cuenta.get(huella(c)) || 0) + 1);
    const [mejor, veces] = [...cuenta.entries()].sort((a, b) => b[1] - a[1])[0];
    if (veces < 3) continue;
    const iguales = hijos.filter((c) => huella(c) === mejor);
    const cajas = iguales.map((c) => c.getBoundingClientRect());
    const altos = cajas.map((b) => px(b.height));
    const anchos = cajas.map((b) => px(b.width));
    // Separación real entre consecutivos (el gap declarado miente cuando hay margen).
    const seps = [];
    for (let i = 1; i < cajas.length; i++) {
      const d = cajas[i].y - (cajas[i - 1].y + cajas[i - 1].height);
      const dx = cajas[i].x - (cajas[i - 1].x + cajas[i - 1].width);
      seps.push(px(Math.abs(d) < Math.abs(dx) ? d : dx));
    }
    const c0 = getComputedStyle(iguales[0]);
    out.repeticiones.push({
      contenedor: sel(it.el), patron: mejor, n: iguales.length,
      disposicion: it.s.display, columnas: it.s.gridTemplateColumns, gapDeclarado: it.s.gap,
      alto: { min: Math.min(...altos), max: Math.max(...altos), moda: altos.sort((a, b) => altos.filter((v) => v === a).length - altos.filter((v) => v === b).length).pop() },
      ancho: { min: Math.min(...anchos), max: Math.max(...anchos) },
      separacion: seps.length ? [...new Set(seps)].slice(0, 5) : [],
      item: {
        sel: sel(iguales[0]), texto: txt(iguales[0]),
        padding: `${c0.paddingTop} ${c0.paddingRight} ${c0.paddingBottom} ${c0.paddingLeft}`,
        radio: c0.borderRadius, borde: `${c0.borderBottomWidth} ${c0.borderStyle} ${c0.borderBottomColor}`,
        bordeToken: token(c0.borderBottomColor), bg: c0.backgroundColor, bgToken: token(c0.backgroundColor),
        sombra: c0.boxShadow, fuente: `${px(c0.fontSize)}px/${c0.lineHeight} ${c0.fontWeight}`,
      },
    });
  }
  out.repeticiones = out.repeticiones.filter((r) => r.n >= 3).sort((a, b) => b.n - a.n).slice(0, 16);

  // --- Animaciones declaradas (duración y curva).
  out.animaciones = agrupar(
    todos.filter((it) => (it.s.transitionDuration && it.s.transitionDuration !== "0s") || (it.s.animationName && it.s.animationName !== "none")),
    (it) => `${it.s.transitionProperty} ${it.s.transitionDuration} ${it.s.transitionTimingFunction} ${it.s.transitionDelay} | anim:${it.s.animationName} ${it.s.animationDuration} ${it.s.animationTimingFunction}`,
  );

  return out;
};

/** Estado de foco: se enfoca y se compara lo que cambia. El anillo no se adivina. */
const FOCO = function foco() {
  const px = (v) => (v == null || v === "" ? null : Math.round(parseFloat(v) * 100) / 100);
  const P = ["outlineWidth", "outlineStyle", "outlineColor", "outlineOffset", "boxShadow", "borderTopColor", "backgroundColor", "color"];
  const lee = (el) => { const s = getComputedStyle(el); const o = {}; for (const k of P) o[k] = s[k]; return o; };
  const res = [];
  const sels = ["button", "a[href]", "input:not([type=hidden])", "textarea", "select", "[role=tab]", "[tabindex='0']"];
  for (const q of sels) {
    const el = [...document.querySelectorAll(q)].find((e) => {
      const r = e.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== "hidden";
    });
    if (!el) continue;
    const antes = lee(el);
    el.focus({ preventScroll: true });
    const despues = lee(el);
    const cambios = {};
    for (const k of P) if (antes[k] !== despues[k]) cambios[k] = { reposo: antes[k], foco: despues[k] };
    el.blur();
    const r = el.getBoundingClientRect();
    res.push({ q, texto: (el.textContent || el.getAttribute("aria-label") || "").trim().slice(0, 40), caja: { w: px(r.width), h: px(r.height) }, cambios, anillo: Object.keys(cambios).length ? null : "sin cambio computado (¿:focus-visible?)" });
  }
  return res;
};

/** Mide un diálogo abierto: velo, caja, cabecera/cuerpo/pie, botonera, X y animación. */
const DIALOGO = function dialogo() {
  const px = (v) => (v == null || v === "" ? null : Math.round(parseFloat(v) * 100) / 100);
  const caja = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { x: px(r.x), y: px(r.y), w: px(r.width), h: px(r.height) }; };
  const txt = (el) => (el?.textContent || "").trim().replace(/\s+/g, " ").slice(0, 70) || null;
  const d = document.querySelector("[role=dialog],[role=alertdialog],dialog[open]");
  if (!d) return null;
  const s = getComputedStyle(d);
  // El velo. NO se pide `position:fixed`: en `Modal.tsx` el velo es
  // `absolute inset-0` DENTRO de un contenedor fijo, así que exigir `fixed`
  // dejaba el campo vacío y el informe decía «no se detectó velo» en diálogos
  // que sí lo tienen. Lo que lo define es que cubra la ventana, no cómo se
  // posiciona.
  const velo = [...document.querySelectorAll("div,span")].find((e) => {
    if (e.contains(d) || e === d) return false;
    const es = getComputedStyle(e);
    if (es.position !== "fixed" && es.position !== "absolute") return false;
    const r = e.getBoundingClientRect();
    return r.width >= innerWidth * 0.95 && r.height >= innerHeight * 0.95 &&
      (es.backdropFilter !== "none" || es.backgroundColor !== "rgba(0, 0, 0, 0)");
  });
  const vs = velo ? getComputedStyle(velo) : null;
  const hijos = [...d.children];
  /*
   * Cabecera / cuerpo / pie POR POSICIÓN, que es el contrato de `Modal.tsx`:
   * primer hijo cabecera, último pie si hay más de dos, el resto cuerpo.
   *
   * Antes se deducía de la clase, y `/border-b/` daba positivo dentro de
   * `border-border-subtle` —la clase del PIE—, así que el pie salía rotulado
   * «cabecera» en todos los diálogos con botonera.
   */
  const rol = (e, i) => {
    if (e.tagName.toLowerCase() === "header") return "cabecera";
    if (e.tagName.toLowerCase() === "footer") return "pie";
    if (i === 0) return "cabecera";
    return hijos.length > 2 && i === hijos.length - 1 ? "pie" : "cuerpo";
  };
  // La BOTONERA es la del pie. Meter aquí los cuarenta iconos de la barra de
  // formato del redactor no informa de nada: lo que se porta es el orden de
  // «Guardar borrador / Enviar» y cuál de los dos es el primario.
  const pie = hijos.length > 2 ? hijos[hijos.length - 1] : null;
  const botones = [...(pie ?? d).querySelectorAll("button")].map((b) => {
    const bs = getComputedStyle(b);
    return {
      texto: txt(b) || b.getAttribute("aria-label"), caja: caja(b),
      bg: bs.backgroundColor, fg: bs.color, radio: bs.borderRadius,
      borde: `${bs.borderTopWidth} ${bs.borderStyle} ${bs.borderTopColor}`,
      padding: `${bs.paddingTop} ${bs.paddingRight} ${bs.paddingBottom} ${bs.paddingLeft}`,
      fuente: `${px(bs.fontSize)}px/${bs.lineHeight} ${bs.fontWeight}`,
    };
  });
  return {
    titulo: txt(d.querySelector("h1,h2,h3,[id*=title]")),
    caja: caja(d),
    est: {
      bg: s.backgroundColor, radio: s.borderRadius, sombra: s.boxShadow,
      borde: `${s.borderTopWidth} ${s.borderStyle} ${s.borderTopColor}`,
      padding: `${s.paddingTop} ${s.paddingRight} ${s.paddingBottom} ${s.paddingLeft}`,
      maxWidth: s.maxWidth, maxHeight: s.maxHeight, width: s.width, z: s.zIndex, pos: s.position,
      animacion: `${s.animationName} ${s.animationDuration} ${s.animationTimingFunction}`,
      transicion: `${s.transitionProperty} ${s.transitionDuration} ${s.transitionTimingFunction}`,
    },
    velo: velo ? { caja: caja(velo), bg: vs.backgroundColor, desenfoque: vs.backdropFilter, z: vs.zIndex, animacion: `${vs.animationName} ${vs.animationDuration} ${vs.animationTimingFunction}`, transicion: `${vs.transitionProperty} ${vs.transitionDuration}` } : null,
    partes: hijos.map((h, i) => {
      const hs = getComputedStyle(h);
      return { rol: rol(h, i), tag: h.tagName.toLowerCase(), caja: caja(h), texto: txt(h),
        padding: `${hs.paddingTop} ${hs.paddingRight} ${hs.paddingBottom} ${hs.paddingLeft}`,
        borde: `${hs.borderTopWidth}/${hs.borderBottomWidth} ${hs.borderTopColor}`, gap: hs.gap, bg: hs.backgroundColor };
    }),
    botones,
    // Lo que NO es la botonera del pie: la barra de formato del redactor, las
    // pestañas, los iconos del cuerpo. Sin estilo completo —no hace falta— pero
    // con caja y talla, que es lo que se necesita para reproducir la barra.
    otrosControles: [...d.querySelectorAll("button")].filter((b) => !pie || !pie.contains(b)).slice(0, 28).map((b) => {
      const bs = getComputedStyle(b);
      const i = b.querySelector("svg");
      return { texto: txt(b) || b.getAttribute("aria-label"), caja: caja(b), icono: caja(i), radio: bs.borderRadius, fg: bs.color };
    }),
    cierre: (() => {
      const x = [...d.querySelectorAll("button")].find((b) => /cerrar|close|✕|×/i.test((b.getAttribute("aria-label") || "") + (b.textContent || "")));
      return x ? { caja: caja(x), aria: x.getAttribute("aria-label"), icono: caja(x.querySelector("svg")) } : null;
    })(),
    foco: (() => {
      const a = document.activeElement;
      return a && d.contains(a) ? { dentro: true, sel: a.tagName.toLowerCase() + (a.getAttribute("aria-label") ? `[aria-label="${a.getAttribute("aria-label")}"]` : ""), texto: txt(a) } : { dentro: false, activo: document.activeElement?.tagName?.toLowerCase() };
    })(),
  };
};

// Abre solo lo que no escribe nada. El piloto ES producción.
const SEGUROS = /^(nuevo|nueva|crear|redactar|añadir|anadir|agregar|compartir|invitar|subir|importar|exportar|ajustes|configurac|filtrar|ordenar|buscar|mover|renombrar|etiquet|detalles|información|informacion|ayuda|atajos|preferenc|vista|columnas|descargar|imprimir)/i;
const PELIGROSOS = /(eliminar|borrar|enviar|guardar|salir|cerrar sesión|cerrar sesion|desactivar|revocar|expulsar|vaciar|pagar|suscri|confirmar|aceptar|publicar|archivar|bloquear|restablecer|restaurar|deshacer)/i;

// ---------------------------------------------------------------------------

mkdirSync(SALIDA, { recursive: true });
// `--font-render-hinting=none` NO es cosmética: sin ella este navegador —Linux con
// fontconfig— redondea el avance de CADA GLIFO a un píxel de CSS entero, y los anchos
// que esta sonda apunta salen entre 0 y 0,5 px por letra más largos que en cualquier
// macOS o Windows. Medido el 2026-09-22: «o» a 11,375 px son 6,5938 y la sonda apuntaba
// 7; «Colaboración», 67,67 contra 70 (+3,4 %). Con eso, comparar un rótulo del puerto a
// Fyne contra «lo medido en producción» daba un falso negativo que crece con la
// longitud de la palabra. Ver `docs/roadmap.md#texto-hinting-del-navegador`.
const navegador = await chromium.launch({ args: ['--font-render-hinting=none'] });
/*
 * SIN `storageState`: se entra de nuevo en cada barrido, a propósito.
 *
 * Reutilizar `.auth/prod-audit.json` medía basura SIN DECIRLO. El 2026-09-21 la
 * sesión guardada horas antes conservaba la cookie —el cromo pintaba entero,
 * con su avatar y su buscador— pero el token que la shell pide al BFF ya no
 * valía: todas las APIs contestaban 401 y la bandeja de Sytue Mail se midió con
 * «No se pudieron cargar las carpetas» (124 elementos). Con acceso nuevo, la
 * MISMA pantalla da 522. Un inventario de medidas sacado del estado de error es
 * peor que no tenerlo, porque nadie lo distingue de uno bueno.
 *
 * Cuesta UN acceso por barrido (el IdP admite 10/min), y al terminar se deja la
 * sesión fresca en el fichero para quien sí quiera reutilizarla.
 */
const ctx = await navegador.newContext({
  viewport: VP_ANCHO, deviceScaleFactor: 1, locale: "es-ES", colorScheme: ESQUEMA,
  ignoreHTTPSErrors: true,
});

async function sesion(p) {
  if (new URL(p.url()).hostname !== IDP) return;
  const porque = porQueNoEntra(p.url());
  if (porque) throw new Error(`el login no llega: ${porque}`);
  await p.fill("input[name=email]", CRED.email);
  await p.fill("input[name=password]", CRED.pass);
  await p.click("button[type=submit]");
  await completarMFASiHaceFalta(p).catch(() => {});
  await p.waitForURL((u) => u.hostname !== IDP, { timeout: 90_000 });
  await activarOrganizacion(p);
}

async function asentar(p) {
  await p.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => {});
  await p.evaluate(() => document.fonts?.ready).catch(() => {});
  await p.waitForTimeout(2000);
}

const resumen = [];

for (const [app, pantallas] of plan) {
  const dir = `${SALIDA}/${app}`;
  mkdirSync(dir, { recursive: true });
  for (const scr of pantallas) {
    const p = await ctx.newPage();
    const errores = [];
    p.on("pageerror", (e) => errores.push(String(e).slice(0, 160)));
    p.on("response", (r) => { if (r.status() >= 400) errores.push(`${r.status()} ${r.url().slice(0, 110)}`); });
    const etiqueta = `${app}/${scr.n}`;
    try {
      await p.goto(baseDe(scr) + scr.u, { waitUntil: "domcontentloaded", timeout: 60_000 });
      await sesion(p);
      await asentar(p);
      if (scr.click) {
        const obj = p.locator(scr.click).first();
        await obj.waitFor({ state: "visible", timeout: 12_000 });
        await obj.click({ timeout: 10_000 });
        await asentar(p);
      }
      const base = `${dir}/${scr.n}`;
      const guarda = (v, d) => writeFileSync(`${base}.${v}.json`, JSON.stringify(d, null, 1));

      // 1) Ancho, claro.
      const ancho = await p.evaluate(VOLCADO);
      ancho.foco = await p.evaluate(FOCO);
      ancho.erroresPagina = [...new Set(errores)].slice(0, 12);
      await p.screenshot({ path: `${base}.ancho.png`, fullPage: true });

      // 2) Diálogos: se abren los seguros, se miden y se cierran con Escape.
      const disparadores = await p.evaluate(({ s, d }) => {
        const okS = new RegExp(s.source, s.flags); const okD = new RegExp(d.source, d.flags);
        return [...document.querySelectorAll("button,[role=button],a[role=button]")]
          .map((b, i) => ({ i, t: ((b.textContent || "") + " " + (b.getAttribute("aria-label") || "")).trim().replace(/\s+/g, " ").slice(0, 50), vis: b.getBoundingClientRect().width > 0 }))
          .filter((x) => x.vis && x.t && okS.test(x.t) && !okD.test(x.t))
          .slice(0, 6);
      }, { s: { source: SEGUROS.source, flags: SEGUROS.flags }, d: { source: PELIGROSOS.source, flags: PELIGROSOS.flags } });
      ancho.dialogos = [];
      for (const dsp of disparadores) {
        try {
          await p.locator("button,[role=button],a[role=button]").nth(dsp.i).click({ timeout: 6000 });
          await p.waitForTimeout(900);
          const med = await p.evaluate(DIALOGO);
          if (med) {
            med.disparador = dsp.t;
            const nom = dsp.t.toLowerCase().replace(/[^a-z0-9]+/g, "-").slice(0, 24);
            await p.screenshot({ path: `${base}.dlg-${nom}.png` });
            med.captura = `${scr.n}.dlg-${nom}.png`;
            ancho.dialogos.push(med);
          }
          await p.keyboard.press("Escape");
          await p.waitForTimeout(500);
          if (await p.locator("[role=dialog],dialog[open]").count()) { await p.goto(baseDe(scr) + scr.u, { waitUntil: "domcontentloaded" }); await asentar(p); }
        } catch { /* un disparador que no abre nada no es un hallazgo */ }
      }
      guarda("ancho", ancho);

      // 3) Estrecho (móvil): qué corte usa.
      await p.setViewportSize(VP_ESTRECHO);
      await p.waitForTimeout(1200);
      const estrecho = await p.evaluate(VOLCADO);
      estrecho.desbordaX = await p.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      await p.screenshot({ path: `${base}.estrecho.png`, fullPage: true });
      guarda("estrecho", estrecho);
      await p.setViewportSize(VP_ANCHO);

      // 4) Oscuro (solo tokens, colores y regiones: la geometría no cambia).
      await p.emulateMedia({ colorScheme: "dark" });
      await p.waitForTimeout(1200);
      const oscuro = await p.evaluate(VOLCADO);
      await p.screenshot({ path: `${base}.oscuro.png` });
      guarda("oscuro", { url: oscuro.url, tokens: oscuro.tokens, colores: oscuro.colores, regiones: oscuro.regiones, sombras: oscuro.sombras });
      await p.emulateMedia({ colorScheme: ESQUEMA });

      // 5) Estados, una pantalla por app.
      if (scr.estados) {
        const est = {};
        // Carga: se vuelve a entrar y se mira ANTES de que asiente.
        await p.goto(baseDe(scr) + scr.u, { waitUntil: "commit", timeout: 30_000 }).catch(() => {});
        await p.waitForTimeout(350);
        est.carga = await p.evaluate(() => {
          const px = (v) => Math.round(parseFloat(v) * 100) / 100;
          const esq = [...document.querySelectorAll("[class*=skeleton],[class*=animate-pulse],[role=progressbar],[aria-busy=true],[class*=spinner],[class*=cargando]")].slice(0, 8);
          return esq.map((e) => { const s = getComputedStyle(e); const r = e.getBoundingClientRect();
            return { sel: e.tagName.toLowerCase() + "." + [...e.classList].slice(0, 3).join("."), caja: { w: px(r.width), h: px(r.height) }, bg: s.backgroundColor, radio: s.borderRadius, animacion: `${s.animationName} ${s.animationDuration} ${s.animationTimingFunction}` }; });
        }).catch(() => []);
        await p.screenshot({ path: `${base}.carga.png` }).catch(() => {});
        await asentar(p);
        // Sin conexión.
        await ctx.setOffline(true);
        await p.reload({ waitUntil: "domcontentloaded", timeout: 30_000 }).catch(() => {});
        await p.waitForTimeout(2500);
        est.sinConexion = await p.evaluate(() => ({ titulo: document.querySelector("h1,h2")?.textContent?.trim().slice(0, 80) || null, texto: document.body.innerText.trim().replace(/\s+/g, " ").slice(0, 300) })).catch(() => null);
        await p.screenshot({ path: `${base}.sin-conexion.png` }).catch(() => {});
        await ctx.setOffline(false);
        // Ruta inexistente (error / vacío explicado).
        await p.goto(`${baseDe(scr)}${scr.u.split("?")[0].replace(/\/$/, "")}/no-existe-medidas-${Date.now()}`, { waitUntil: "domcontentloaded", timeout: 30_000 }).catch(() => {});
        await p.waitForTimeout(2000);
        est.rutaInexistente = await p.evaluate(() => ({ url: location.href, titulo: document.querySelector("h1,h2")?.textContent?.trim().slice(0, 80) || null, texto: document.body.innerText.trim().replace(/\s+/g, " ").slice(0, 300) })).catch(() => null);
        await p.screenshot({ path: `${base}.error.png` }).catch(() => {});
        guarda("estados", est);
      }

      // Una pantalla con 401/403 NO está medida: está midiendo su estado de
      // error. Se dice en voz alta en el log y queda marcado en el resumen.
      const auth = ancho.erroresPagina.filter((e) => /^(401|403) /.test(e));
      resumen.push({ app, pantalla: scr.n, url: ancho.url, elementos: ancho.elementos, dialogos: ancho.dialogos.length, sinAutorizar: auth.length, ok: true });
      console.log(
        `OK   ${etiqueta.padEnd(28)} ${ancho.elementos} elementos · ${ancho.tipografia.length} estilos de texto · ` +
          `${ancho.dialogos.length} diálogos${auth.length ? `  ⚠ ${auth.length} respuestas ${auth[0].slice(0, 3)}: lo medido es el ESTADO DE ERROR` : ""}`,
      );
    } catch (e) {
      resumen.push({ app, pantalla: scr.n, ok: false, error: String(e).slice(0, 180) });
      console.log(`FALLO ${etiqueta.padEnd(28)} ${String(e).slice(0, 140)}`);
    } finally {
      await p.close().catch(() => {});
    }
  }
}

await ctx.storageState({ path: STORAGE }).catch(() => {});
writeFileSync(`${SALIDA}/resumen.json`, JSON.stringify(resumen, null, 1));
await navegador.close();
console.log(`\n${resumen.filter((r) => r.ok).length}/${resumen.length} pantallas medidas en ${SALIDA}`);
