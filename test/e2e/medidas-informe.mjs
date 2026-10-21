/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * Convierte los volcados de `medidas-web.mjs` en un inventario legible: un
 * fichero por app con TODO lo suyo (geometría, tipografía, color por token,
 * iconos, filas, foco, diálogos, estrecho, oscuro y estados), más un índice.
 *
 * No mide nada: solo formatea. Si un número parece raro, se vuelve a medir con
 * `medidas-web.mjs <app>` y esto se regenera. Cada fila lleva el SELECTOR del
 * que salió y la CAPTURA donde se ve, para poder comprobarlo sin re-medir.
 *
 *   bun run medidas-informe.mjs [<dir-medidas>]   # por defecto ~/medidas
 */
import { readdirSync, readFileSync, writeFileSync, existsSync } from "node:fs";

const DIR = process.argv[2] || `${process.env.HOME}/medidas`;
const lee = (f) => JSON.parse(readFileSync(f, "utf8"));
const n2 = (v) => (v == null ? "—" : typeof v === "number" ? String(Math.round(v * 100) / 100) : String(v));
const esc = (s) => String(s ?? "—").replace(/\|/g, "\\|").replace(/\n/g, " ");
const tabla = (cabeceras, filas) =>
  filas.length
    ? [`| ${cabeceras.join(" | ")} |`, `|${cabeceras.map(() => "---").join("|")}|`, ...filas.map((f) => `| ${f.map(esc).join(" | ")} |`)].join("\n")
    : "_sin datos_";

const NOMBRES = {
  portal: "Sytue Hub (portal)", mail: "Sytue Mail", drive: "Sytue Drive", calendar: "Sytue Calendar",
  contacts: "Sytue Contacts", notes: "Sytue Notepads", account: "Mi cuenta (account)",
  admin: "Consola de plataforma (console)", avisos: "Avisos / notificaciones", docs: "Docus (documentos)",
  sheets: "Calcus (hojas)", slides: "Diapos (presentaciones)", pdf: "CertiPDF", erp: "Sytue ERP",
  photos: "SyPhotos", rooms: "Sytue Rooms", cast: "Sytue Cast", pages: "Sytue Web",
};

function seccionPantalla(app, pant, d, estrecho, oscuro, estados) {
  const L = [];
  L.push(`\n## ${pant}\n`);
  L.push(`- **URL medida**: \`${d.url}\``);
  L.push(`- **Captura**: \`${app}/${pant}.ancho.png\` (1280×800, es-ES, claro, DSF 1) · estrecho \`${pant}.estrecho.png\` (390×844) · oscuro \`${pant}.oscuro.png\``);
  L.push(`- **Raíz**: 1 rem = **${d.rem} px** · ${d.elementos} elementos visibles medidos`);
  if (d.erroresPagina?.length) L.push(`- ⚠ la pantalla carga con ruido: ${d.erroresPagina.slice(0, 4).map((e) => `\`${esc(e)}\``).join(", ")}`);

  L.push(`\n### Esqueleto (regiones)\n`);
  L.push(tabla(
    ["selector", "caja x,y,w,h", "fondo", "padding", "borde", "radio", "gap / columnas", "sombra"],
    d.regiones.slice(0, 18).map((r) => [
      `\`${r.sel}\``,
      `${n2(r.caja.x)},${n2(r.caja.y)} · **${n2(r.caja.w)}×${n2(r.caja.h)}**`,
      r.est.bgToken ? `${r.est.bg} \`${r.est.bgToken}\`` : r.est.bg,
      r.est.padding, r.est.bordeToken ? `${r.est.borde} \`${r.est.bordeToken}\`` : r.est.borde,
      r.est.radio, r.est.columnas && r.est.columnas !== "none" ? `${r.est.gap} / ${r.est.columnas}` : r.est.gap,
      r.est.sombra,
    ]),
  ));

  L.push(`\n### Tipografía\n`);
  L.push(`> \`line-height\` es el alto de la **caja** de línea; el glifo va centrado dentro. La columna px es el valor computado.\n`);
  L.push(tabla(
    ["px (rem)", "peso", "interlineado", "espaciado", "caja", "color", "usos", "ejemplo — selector"],
    d.tipografia.slice(0, 18).map((t) => [
      `**${n2(t.px)}** (${n2(t.rem)}rem)`, t.peso, `${t.interlineado}${t.interlineadoPx ? ` = ${n2(t.interlineadoPx)}px` : ""}`,
      t.espaciado, t.caja === "none" ? "—" : t.caja,
      t.colorToken ? `${t.color} \`${t.colorToken}\`` : t.color, t.n,
      `“${esc(t.ejemplos[0]?.texto ?? "")}” — \`${t.ejemplos[0]?.sel ?? ""}\``,
    ]),
  ));

  L.push(`\n### Color por token\n`);
  L.push(tabla(
    ["uso", "valor", "token", "veces", "ejemplo — selector"],
    d.colores.slice(0, 26).map((c) => [c.prop, `\`${c.valor}\``, c.token ? `\`${c.token}\`` : "—", c.n, `\`${c.ejemplos[0]?.sel ?? ""}\``]),
  ));

  L.push(`\n### Espaciado, radios, bordes y sombras\n`);
  L.push(`**Padding** (t r b l): ${d.espaciado.padding.slice(0, 10).map((p) => `\`${p.valor}\` ×${p.n}`).join(" · ") || "—"}`);
  L.push(`\n**Gap** (flex/grid): ${d.espaciado.gap.slice(0, 10).map((p) => `\`${p.valor}\` ×${p.n}`).join(" · ") || "—"}`);
  L.push(`\n**Radios**: ${d.radios.slice(0, 10).map((p) => `\`${p.valor}\` ×${p.n}`).join(" · ") || "—"}`);
  L.push(`\n**Bordes**: ${d.bordes.slice(0, 8).map((p) => `\`${p.valor}\` ×${p.n}`).join(" · ") || "—"}`);
  L.push(`\n**Sombras**: ${d.sombras.slice(0, 6).map((p) => `\`${p.valor}\` ×${p.n}`).join(" · ") || "—"}`);

  if (d.repeticiones?.length) {
    L.push(`\n### Filas, tarjetas y rejillas repetidas\n`);
    L.push(tabla(
      ["contenedor", "n", "disposición", "alto item", "ancho item", "separación real", "padding item", "radio / borde", "fondo", "fuente"],
      d.repeticiones.slice(0, 10).map((r) => [
        `\`${r.contenedor}\``, r.n,
        r.columnas && r.columnas !== "none" ? `grid ${r.columnas}` : `${r.disposicion} gap:${r.gapDeclarado}`,
        `**${n2(r.alto.moda)}**${r.alto.min !== r.alto.max ? ` (${n2(r.alto.min)}–${n2(r.alto.max)})` : ""}`,
        r.ancho.min === r.ancho.max ? n2(r.ancho.min) : `${n2(r.ancho.min)}–${n2(r.ancho.max)}`,
        r.separacion.map(n2).join(", ") || "0",
        r.item.padding, `${r.item.radio} / ${r.item.borde}${r.item.bordeToken ? ` \`${r.item.bordeToken}\`` : ""}`,
        r.item.bgToken ? `${r.item.bg} \`${r.item.bgToken}\`` : r.item.bg, r.item.fuente,
      ]),
    ));
  }

  if (d.controles?.length) {
    L.push(`\n### Controles\n`);
    // Se agrupan por huella: 40 botones iguales son UN control, no cuarenta.
    const m = new Map();
    for (const c of d.controles) {
      const k = `${c.tag}|${c.tipo}|${Math.round(c.est.alto)}|${c.est.bg}|${c.est.radio}|${c.est.fuente}|${c.est.padding}`;
      if (!m.has(k)) m.set(k, { ...c, n: 0, textos: [] });
      const g = m.get(k);
      g.n++;
      if (g.textos.length < 3 && c.texto) g.textos.push(c.texto);
    }
    L.push(tabla(
      ["control", "n", "alto", "ancho", "padding", "fuente", "texto/fondo", "radio", "borde", "transición", "ejemplos — selector"],
      [...m.values()].sort((a, b) => b.n - a.n).slice(0, 14).map((c) => [
        `${c.tag}${c.tipo ? `[${c.tipo}]` : ""}`, c.n, `**${n2(c.est.alto)}**`, n2(c.caja.w), c.est.padding, c.est.fuente,
        `${c.est.fgToken ? `\`${c.est.fgToken}\`` : c.est.fg} / ${c.est.bgToken ? `\`${c.est.bgToken}\`` : c.est.bg}`,
        c.est.radio, c.est.borde, c.est.transicion,
        `${c.textos.map((t) => `“${esc(t)}”`).join(", ")} — \`${c.sel}\``,
      ]),
    ));
  }

  if (d.iconos?.length) {
    L.push(`\n### Iconos\n`);
    L.push(tabla(["talla", "trazo", "color", "veces", "ejemplo — selector"],
      d.iconos.slice(0, 10).map((i) => [`**${n2(i.w)}×${n2(i.h)}**`, i.trazo || "—", i.colorToken ? `${i.color} \`${i.colorToken}\`` : i.color, i.n, `\`${i.ejemplos[0]?.sel ?? ""}\``])));
  }

  if (d.animaciones?.length) {
    L.push(`\n### Animaciones\n`);
    L.push(tabla(["propiedad · duración · curva", "veces", "ejemplo — selector"],
      d.animaciones.slice(0, 10).map((a) => [`\`${esc(a.clave)}\``, a.n, `\`${a.ejemplos[0]?.sel ?? ""}\``])));
  }

  if (d.foco?.length) {
    L.push(`\n### Foco (medido enfocando, no supuesto)\n`);
    L.push(tabla(["elemento", "caja", "qué cambia al enfocar"],
      d.foco.map((f) => [`\`${f.q}\` “${esc(f.texto)}”`, `${n2(f.caja.w)}×${n2(f.caja.h)}`,
        Object.keys(f.cambios).length ? Object.entries(f.cambios).map(([k, v]) => `${k}: ${v.reposo} → **${v.foco}**`).join("; ") : (f.anillo || "—")])));
  }

  if (d.dialogos?.length) {
    L.push(`\n### Diálogos\n`);
    for (const dl of d.dialogos) {
      L.push(`\n#### «${esc(dl.titulo || dl.disparador)}» — abre desde “${esc(dl.disparador)}”\n`);
      L.push(`- Captura: \`${app}/${dl.captura}\``);
      L.push(`- **Caja**: ${n2(dl.caja.w)}×${n2(dl.caja.h)} en (${n2(dl.caja.x)}, ${n2(dl.caja.y)}) · \`width:${dl.est.width}\` \`max-width:${dl.est.maxWidth}\` \`max-height:${dl.est.maxHeight}\` · \`position:${dl.est.pos}\` z:${dl.est.z}`);
      L.push(`- **Superficie**: fondo \`${dl.est.bg}\`, radio \`${dl.est.radio}\`, borde \`${dl.est.borde}\`, padding \`${dl.est.padding}\`, sombra \`${dl.est.sombra}\``);
      L.push(`- **Velo**: ${dl.velo ? `${n2(dl.velo.caja.w)}×${n2(dl.velo.caja.h)} · fondo \`${dl.velo.bg}\` · desenfoque \`${dl.velo.desenfoque}\` · z:${dl.velo.z} · anim \`${dl.velo.animacion}\`` : "**no se detectó velo**"}`);
      L.push(`- **Animación**: entrada \`${dl.est.animacion}\` · transición \`${dl.est.transicion}\``);
      L.push(`- **Cierre (X)**: ${dl.cierre ? `${n2(dl.cierre.caja.w)}×${n2(dl.cierre.caja.h)}, icono ${dl.cierre.icono ? `${n2(dl.cierre.icono.w)}×${n2(dl.cierre.icono.h)}` : "—"}, aria \`${dl.cierre.aria}\`` : "no hay X"}`);
      L.push(`- **Foco al abrir**: ${dl.foco.dentro ? `dentro, en \`${dl.foco.sel}\` “${esc(dl.foco.texto)}”` : `**fuera** (activo: ${dl.foco.activo})`}`);
      L.push(`\n${tabla(["parte", "caja", "padding", "borde", "gap", "fondo", "texto"],
        dl.partes.map((p) => [`${p.rol} (${p.tag})`, `${n2(p.caja.w)}×${n2(p.caja.h)}`, p.padding, p.borde, p.gap, p.bg, `“${esc(p.texto)}”`]))}`);
      if (dl.botones.length) {
        L.push(`\n**Botonera del pie** (en orden del DOM, alineada a la derecha; el primario es el de fondo sólido y va el último):\n`);
        L.push(tabla(["texto", "caja", "x", "fondo", "texto", "radio", "borde", "padding", "fuente"],
          dl.botones.map((b) => [`“${esc(b.texto)}”`, `${n2(b.caja.w)}×${n2(b.caja.h)}`, n2(b.caja.x), b.bg, b.fg, b.radio, b.borde, b.padding, b.fuente])));
      }
      if (dl.otrosControles?.length) {
        L.push(`\n**Otros controles del diálogo** (barra de formato, pestañas, iconos del cuerpo):\n`);
        L.push(tabla(["texto", "caja", "icono", "radio", "color"],
          dl.otrosControles.map((b) => [`“${esc(b.texto)}”`, `${n2(b.caja.w)}×${n2(b.caja.h)}`, b.icono ? `${n2(b.icono.w)}×${n2(b.icono.h)}` : "—", b.radio, b.fg])));
      }
    }
  }

  if (estrecho) {
    L.push(`\n### Al estrechar (390×844)\n`);
    L.push(`- Desborde horizontal: **${estrecho.desbordaX > 1 ? `${estrecho.desbordaX} px — DESBORDA` : "ninguno"}**`);
    const anchas = new Map(d.regiones.map((r) => [r.sel, r.caja]));
    const filas = [];
    for (const r of estrecho.regiones.slice(0, 16)) {
      const a = anchas.get(r.sel);
      filas.push([`\`${r.sel}\``, a ? `${n2(a.w)}×${n2(a.h)}` : "no estaba", `${n2(r.caja.w)}×${n2(r.caja.h)}`, r.est.display, r.est.columnas !== "none" ? r.est.columnas : r.est.padding]);
    }
    for (const r of d.regiones.slice(0, 16)) if (!estrecho.regiones.some((e) => e.sel === r.sel)) filas.push([`\`${r.sel}\``, `${n2(r.caja.w)}×${n2(r.caja.h)}`, "**desaparece**", "—", "—"]);
    L.push(tabla(["región", "a 1280", "a 390", "display", "columnas/padding"], filas));
    const tipA = new Set(d.tipografia.map((t) => `${t.px}/${t.peso}`));
    const nuevas = estrecho.tipografia.filter((t) => !tipA.has(`${t.px}/${t.peso}`)).slice(0, 6);
    if (nuevas.length) L.push(`\n**Texto que solo existe en estrecho**: ${nuevas.map((t) => `${n2(t.px)}px/${t.peso} “${esc(t.ejemplos[0]?.texto)}”`).join(" · ")}`);
  }

  if (oscuro) {
    L.push(`\n### Modo oscuro\n`);
    const dif = Object.entries(oscuro.tokens).filter(([k, v]) => d.tokens[k] !== v);
    L.push(tabla(["token", "claro", "oscuro"], dif.slice(0, 30).map(([k, v]) => [`\`${k}\``, d.tokens[k] ?? "—", v])));
    if (dif.length > 30) L.push(`\n_(${dif.length} tokens cambian; los 30 primeros arriba, la lista entera en \`${pant}.oscuro.json\`)_`);
    if (!dif.length) L.push(`_Ningún token cambia: esta pantalla **no responde a \`prefers-color-scheme\`** (o el tema va forzado por atributo)._`);
  }

  if (estados) {
    L.push(`\n### Estados\n`);
    L.push(`- **Carga**: ${estados.carga?.length ? estados.carga.map((c) => `\`${c.sel}\` ${n2(c.caja.w)}×${n2(c.caja.h)} fondo \`${c.bg}\` radio \`${c.radio}\` anim \`${c.animacion}\``).join(" · ") : "no se detectó esqueleto ni indicador (captura `" + pant + ".carga.png`)"}`);
    L.push(`- **Sin conexión** (captura \`${pant}.sin-conexion.png\`): ${estados.sinConexion ? `título “${esc(estados.sinConexion.titulo)}” — “${esc(estados.sinConexion.texto?.slice(0, 180))}”` : "la página no respondió"}`);
    L.push(`- **Ruta inexistente** (captura \`${pant}.error.png\`): ${estados.rutaInexistente ? `\`${estados.rutaInexistente.url}\` → título “${esc(estados.rutaInexistente.titulo)}” — “${esc(estados.rutaInexistente.texto?.slice(0, 180))}”` : "sin respuesta"}`);
  }
  return L.join("\n");
}

// Las carpetas que empiezan por `_` NO son apps: ahí se aparta lo que se midió
// mal (p. ej. `/admin` redirigiendo a `/account`), y se guarda como prueba de
// por qué no cuenta. Sin esta línea salía un `MEDIDAS-_descartado.md` con
// pinta de informe bueno.
const apps = readdirSync(DIR, { withFileTypes: true })
  .filter((e) => e.isDirectory() && !e.name.startsWith("_"))
  .map((e) => e.name)
  .sort();
const indice = [];
for (const app of apps) {
  const dir = `${DIR}/${app}`;
  const pantallas = [...new Set(readdirSync(dir).filter((f) => f.endsWith(".ancho.json")).map((f) => f.replace(".ancho.json", "")))].sort();
  if (!pantallas.length) continue;
  const doc = [
    `# Medidas de ${NOMBRES[app] || app}`,
    ``,
    `Medido contra **producción** (\`corporate.sytue.eu\`, v3.3.9) con \`test/e2e/medidas-web.mjs\`.`,
    `Números del DOM (\`getBoundingClientRect\` + \`getComputedStyle\`), no de la captura a ojo.`,
    `Se regenera entero con \`bun run medidas-web.mjs ${app}\` + \`bun run medidas-informe.mjs\`.`,
    ``,
    `Pantallas: ${pantallas.map((p) => `[${p}](#${p.replace(/[^a-z0-9]+/gi, "-").toLowerCase()})`).join(" · ")}`,
  ];
  for (const p of pantallas) {
    const f = (v) => (existsSync(`${dir}/${p}.${v}.json`) ? lee(`${dir}/${p}.${v}.json`) : null);
    doc.push(seccionPantalla(app, p, lee(`${dir}/${p}.ancho.json`), f("estrecho"), f("oscuro"), f("estados")));
  }
  writeFileSync(`${DIR}/MEDIDAS-${app}.md`, doc.join("\n") + "\n");
  indice.push({ app, pantallas });
  console.log(`MEDIDAS-${app}.md · ${pantallas.length} pantallas`);
}
writeFileSync(`${DIR}/indice.json`, JSON.stringify(indice, null, 1));
