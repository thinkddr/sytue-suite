/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un PDF de una página en blanco, escrito a mano.
 *
 * Es lo que hace falta para que «PDF» sea un cuarto acceso de «Crear nuevo» junto
 * a documento, hoja y presentación: Sytue PDF se abre por RUTA de Drive
 * (`/pdf?path=…`), así que para empezar de cero hay que tener antes un fichero, y
 * el que ya existe para subirlo —`/api/pdf-abrir`— exige un PDF de verdad (mira
 * la cabecera `%PDF-` antes de guardar nada).
 *
 * Son cuatro objetos y una tabla xref: un PDF mínimo válido cabe en medio kilobyte
 * y no justifica una dependencia. Lo único que no se puede escribir a ojo son los
 * DESPLAZAMIENTOS de la xref —cada uno es la posición en bytes donde empieza su
 * objeto—, así que se calculan mientras se monta el fichero, que es justo lo que
 * comprueba `blanco.test.ts`.
 */

/** A4 en puntos PostScript (72 dpi), la misma página que usa el editor. */
const A4 = [595.28, 841.89] as const;

/**
 * PDF 1.4 de una página A4 vacía.
 *
 * Sin `/Contents`: una página sin flujo de contenido es válida (PDF 32000-1 §7.7.3.3,
 * «Contents» es opcional) y se dibuja en blanco. Ahorra el objeto del stream y su
 * `/Length`, que sería la única longitud que habría que mantener a mano.
 */
// `Uint8Array<ArrayBuffer>` y no `Uint8Array` a secas: el genérico por defecto es
// `ArrayBufferLike`, que incluye `SharedArrayBuffer` y no vale como `BlobPart`.
export function pdfEnBlanco(): Uint8Array<ArrayBuffer> {
  const objetos = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${A4[0]} ${A4[1]}] /Resources << >> >>`,
  ];

  let cuerpo = '%PDF-1.4\n';
  const offsets: number[] = [];
  objetos.forEach((obj, i) => {
    offsets.push(cuerpo.length);
    cuerpo += `${i + 1} 0 obj\n${obj}\nendobj\n`;
  });

  // xref: la entrada 0 es la cabecera obligatoria del objeto libre; el resto, cada
  // objeto en su posición, en 10 dígitos con ceros a la izquierda.
  const inicioXref = cuerpo.length;
  const entradas = offsets.map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('');
  cuerpo +=
    `xref\n0 ${objetos.length + 1}\n0000000000 65535 f \n${entradas}` +
    `trailer\n<< /Size ${objetos.length + 1} /Root 1 0 R >>\n` +
    `startxref\n${inicioXref}\n%%EOF\n`;

  // Latin-1 y no UTF-8 a propósito: la sintaxis del PDF es ASCII, y un
  // `TextEncoder` metería dos bytes en cuanto alguien añadiese un acento aquí,
  // descuadrando todos los desplazamientos que se acaban de calcular.
  const bytes = new Uint8Array(cuerpo.length);
  for (let i = 0; i < cuerpo.length; i++) bytes[i] = cuerpo.charCodeAt(i) & 0xff;
  return bytes;
}
