// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// PUERTA DE PR del motor de render de Sytue PDF: comprueba en un navegador de
// verdad que las páginas SE PINTAN.
//
// POR QUÉ HACE FALTA. El motor traduce PDF a SVG (ADR-0033) y sus tests en Go
// comprueban la CADENA del SVG: que ponga `fill="#ff0000"`, que la matriz sea la
// esperada. Eso no dice que se pinte. Un SVG puede ser correcto carácter a
// carácter y salir EN BLANCO —un `clipPath` que recorta todo, una fuente que no
// carga, un `viewBox` que deja el contenido fuera— y ninguno de esos fallos tiene
// forma de aparecer en un `strings.Contains`. Solo el motor del navegador, que es
// justo a quien el ADR-0033 delegó el trabajo, puede decir si salió algo.
//
// Y ESTO CORRE EN LA NUBE, no en el portátil: el navegador arranca en los
// runners de la forja, que están en Scaleway. La regla es de la casa y vale para
// todo lo que abra un Chromium, apunte a donde apunte.
//
// Sin entorno, sin credenciales y sin producción: las muestras las escribe un
// test de Go (`pdfmuestras_test.go`) en un directorio, y aquí se abren como
// fichero local.
//
// Uso: bun run audit-pdf-render.mjs <directorio-de-muestras>
import { chromium } from "playwright";
import { readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const DIR = resolve(process.argv[2] || "/tmp/sytue-pdf-muestras");

/**
 * Qué se exige de cada muestra.
 *
 * `tinta` es la fracción MÍNIMA de píxeles que no son del papel. Los umbrales
 * son flojos a propósito: esto es una puerta contra la página en blanco y contra
 * el contenido en el sitio equivocado, no una comparación de imagen — un
 * comparador de píxeles se rompe con cada cambio de versión del navegador y
 * acaba desactivado, que es la peor puerta de todas.
 */
const CASOS = {
  texto: { tinta: 0.001, texto: "Sytue PDF", fuente: true },
  "abajo-izquierda": {
    tinta: 0.05,
    cuadrante: "abajo-izquierda",
    color: [255, 0, 0],
  },
  recorte: { tinta: 0.2, cuadrante: "arriba" },
  degradado: { tinta: 0.9, degradado: true },
  "ocr-invisible": { tinta: 0, texto: "texto oculto", invisible: true },
  imagen: { tinta: 0.3 },
  // El campo creado y relleno: su recuadro y su valor tienen que estar. El
  // fondo del campo es casi blanco (250) y NO cuenta como tinta, así que lo que
  // se mide es el borde más las letras.
  //
  // OJO CON ESTE UMBRAL, que no se puede comparar con los de arriba: esta muestra
  // es la ÚNICA que no mide 100×100 puntos. Se construye con `docDePrueba`, que
  // pasa por `newPDFWriter` y emite A4 (595×842) — 50 veces más área—, así que el
  // mismo dibujo da un porcentaje 50 veces menor. Estaba en 0.008 (0.8 %), que
  // sobre A4 exige 4.008 pt² de tinta cuando el campo ENTERO mide 80×20 = 1.600 y
  // casi todo es fondo que no cuenta: inalcanzable pintándolo perfectamente.
  //
  // Medido en el CI con el campo bien pintado: 0.08 %. Con el campo ausente solo
  // quedaría la palabra «impreso» del documento base, en torno al 0.02 %. El
  // umbral va en medio, con margen a los dos lados.
  "campo-relleno": { tinta: 0.0005, texto: "Ana" },
};

const ANCHO = 400; // px de la captura; las muestras son de 100×100 puntos

let fallos = 0;
const mal = (caso, msg) => {
  console.error(`not ok - ${caso}: ${msg}`);
  fallos++;
};

const navegador = await chromium.launch();
const pagina = await navegador.newPage({
  viewport: { width: ANCHO, height: ANCHO },
});

// Un error de consola al cargar un SVG es casi siempre un `data:` mal formado o
// una fuente que no entra, y los dos dejan la página coja SIN fallar.
const errores = [];
pagina.on("pageerror", (e) => errores.push(String(e)));
pagina.on("console", (m) => m.type() === "error" && errores.push(m.text()));

const ficheros = readdirSync(DIR).filter((f) => f.endsWith(".svg"));
if (ficheros.length === 0) {
  console.error(`not ok - no hay ninguna muestra en ${DIR}`);
  process.exit(1);
}

for (const fichero of ficheros.sort()) {
  const caso = fichero.replace(/\.svg$/, "");
  const esperado = CASOS[caso];
  if (!esperado) {
    // Una muestra nueva sin criterio es una puerta que no vigila: mejor que
    // falle a que pase por defecto.
    mal(caso, "muestra sin criterio en CASOS; añádelo");
    continue;
  }
  errores.length = 0;

  // El SVG se abre como documento y se estira al viewport. `page.goto` sobre un
  // fichero, no `setContent`: así se ejerce el mismo camino que un `<img>` o un
  // visor, incluidas las restricciones de recursos externos.
  await pagina.goto(pathToFileURL(join(DIR, fichero)).href);
  // El estilo va COLGADO DEL `<svg>`, no con `addStyleTag`. Un SVG abierto como
  // documento no tiene `<head>` —la raíz es el propio `<svg>`—, así que
  // `addStyleTag` moría con «Cannot read properties of null (reading
  // 'appendChild')» y tumbaba el job entero antes de auditar la primera muestra.
  // Por lo mismo se cae `html,body` del CSS: aquí esos elementos no existen.
  await pagina.evaluate((css) => {
    const estilo = document.createElementNS(
      "http://www.w3.org/2000/svg",
      "style",
    );
    estilo.textContent = css;
    document.documentElement.appendChild(estilo);
  }, `svg{width:${ANCHO}px;height:${ANCHO}px;display:block;background:#fff}`);
  await pagina.evaluate(() => document.fonts.ready);

  if (errores.length > 0) {
    mal(caso, `errores en la consola: ${errores.slice(0, 2).join(" | ")}`);
  }

  const captura = await pagina.screenshot({ type: "png" });
  const medida = await medirPixeles(pagina, ANCHO);

  if (medida.tinta < esperado.tinta) {
    mal(
      caso,
      `la página sale casi en blanco (${(medida.tinta * 100).toFixed(2)} % de tinta, ` +
        `mínimo ${(esperado.tinta * 100).toFixed(2)} %)`,
    );
  }
  if (esperado.tinta === 0 && medida.tinta > 0.005) {
    mal(
      caso,
      `debía ser invisible y pinta un ${(medida.tinta * 100).toFixed(2)} %`,
    );
  }

  // El cuadrante es LA comprobación del eje volteado: en PDF el origen está
  // abajo y en SVG arriba, y un signo cambiado en la matriz da un SVG que sigue
  // pareciendo correcto en el texto pero pinta la página del revés.
  if (esperado.cuadrante && medida.cuadrante !== esperado.cuadrante) {
    mal(
      caso,
      `la tinta cayó en «${medida.cuadrante}» y debía caer en «${esperado.cuadrante}»`,
    );
  }
  if (esperado.color && !mismoColor(medida.dominante, esperado.color)) {
    mal(
      caso,
      `color dominante rgb(${medida.dominante}) en vez de rgb(${esperado.color})`,
    );
  }
  // Un degradado que se tradujo como color plano tiene UN solo tono.
  if (esperado.degradado && medida.tonos < 8) {
    mal(caso, `solo ${medida.tonos} tono(s): el degradado salió plano`);
  }

  if (esperado.texto) {
    const texto = await pagina.evaluate(() =>
      Array.from(document.querySelectorAll("text"))
        .map((n) => n.textContent)
        .join(" "),
    );
    if (!texto.includes(esperado.texto)) {
      mal(
        caso,
        `el texto no está en el documento: ${JSON.stringify(texto.slice(0, 60))}`,
      );
    }
    // Que el glifo tenga ANCHURA es lo que separa «hay un <text>» de «se pintó
    // algo»: con una fuente que no carga o un tamaño 0, la longitud es cero.
    const ancho = await pagina.evaluate(() => {
      const n = document.querySelector("text");
      return n && typeof n.getComputedTextLength === "function"
        ? n.getComputedTextLength()
        : -1;
    });
    if (!(ancho > 0)) {
      mal(caso, `el texto no ocupa nada (getComputedTextLength = ${ancho})`);
    }
  }
  if (captura.length < 100) {
    mal(caso, "la captura salió vacía");
  }
  if (!fallos) console.log(`ok - ${caso}`);
}

await navegador.close();

if (fallos > 0) {
  console.error(`\n✗ ${fallos} comprobación(es) del render en rojo.`);
  process.exit(1);
}
console.log(
  `\n✓ ${ficheros.length} muestra(s) del render se pintan como deben.`,
);

/**
 * Mide la página ya pintada: cuánta tinta hay, dónde cae y de qué color.
 *
 * Se hace DENTRO del navegador con un `canvas` en vez de decodificar el PNG
 * aquí: evita traerse una dependencia de imagen solo para contar píxeles.
 */
async function medirPixeles(pagina, lado) {
  return await pagina.evaluate(async (n) => {
    const svg = document.querySelector("svg");
    const serie = new XMLSerializer().serializeToString(svg);
    const img = new Image();
    await new Promise((ok, ko) => {
      img.onload = ok;
      img.onerror = ko;
      img.src =
        "data:image/svg+xml;base64," +
        btoa(unescape(encodeURIComponent(serie)));
    });
    // `createElementNS` con el namespace de XHTML, no `createElement`: el
    // documento aquí es un SVG, así que `createElement` crea el elemento en el
    // namespace del DOCUMENTO —el de SVG— y sale un `<canvas>` que no es un
    // HTMLCanvasElement. No falla al crearlo: falla después, con
    // «c.getContext is not a function», que no se parece en nada a la causa.
    const c = document.createElementNS(
      "http://www.w3.org/1999/xhtml",
      "canvas",
    );
    c.width = c.height = n;
    const ctx = c.getContext("2d");
    ctx.fillStyle = "#fff";
    ctx.fillRect(0, 0, n, n);
    ctx.drawImage(img, 0, 0, n, n);
    const d = ctx.getImageData(0, 0, n, n).data;

    let conTinta = 0;
    const cuadrantes = {
      "arriba-izquierda": 0,
      "arriba-derecha": 0,
      "abajo-izquierda": 0,
      "abajo-derecha": 0,
    };
    const cuenta = new Map();
    for (let y = 0; y < n; y++) {
      for (let x = 0; x < n; x++) {
        const i = (y * n + x) * 4;
        const [r, g, b] = [d[i], d[i + 1], d[i + 2]];
        // «Tinta» es todo lo que no es el papel. El margen de 12 absorbe el
        // antialiasing del borde, que si no cuenta como contenido.
        if (r > 243 && g > 243 && b > 243) continue;
        conTinta++;
        const v = y < n / 2 ? "arriba" : "abajo";
        const h = x < n / 2 ? "izquierda" : "derecha";
        cuadrantes[`${v}-${h}`]++;
        const clave = `${r >> 4},${g >> 4},${b >> 4}`;
        cuenta.set(clave, (cuenta.get(clave) || 0) + 1);
      }
    }

    const orden = Object.entries(cuadrantes).sort((a, b) => b[1] - a[1]);
    let cuadrante = orden[0][0];
    // Si la mitad de arriba concentra casi todo, el cuadrante exacto da igual y
    // lo que importa es la BANDA: es lo que pide el caso del recorte.
    const arriba =
      cuadrantes["arriba-izquierda"] + cuadrantes["arriba-derecha"];
    const abajo = cuadrantes["abajo-izquierda"] + cuadrantes["abajo-derecha"];
    if (conTinta > 0 && arriba / conTinta > 0.9) cuadrante = "arriba";
    if (conTinta > 0 && abajo / conTinta > 0.9 && orden[0][1] / conTinta < 0.9)
      cuadrante = "abajo";

    const dominanteClave = [...cuenta.entries()].sort((a, b) => b[1] - a[1])[0];
    const dominante = dominanteClave
      ? dominanteClave[0].split(",").map((v) => Number(v) * 16 + 8)
      : [255, 255, 255];

    return {
      tinta: conTinta / (n * n),
      cuadrante,
      dominante,
      tonos: cuenta.size,
    };
  }, lado);
}

/** Compara con holgura: el antialiasing y el perfil de color mueven unos puntos. */
function mismoColor(a, b) {
  return a.every((v, i) => Math.abs(v - b[i]) <= 40);
}
