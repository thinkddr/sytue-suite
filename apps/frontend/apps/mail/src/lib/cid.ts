/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * RESOLVER LAS IMÁGENES EN LÍNEA DE UN CORREO.
 *
 * Una imagen incrustada no viaja dentro del HTML: viaja como una parte MIME
 * aparte con su `Content-ID`, y el cuerpo la referencia con `<img src="cid:…">`
 * (RFC 2392). El navegador no sabe resolver `cid:`, así que hay que cambiarlo
 * por una URL que sí sepa pedir.
 *
 * Sytue Mail componía con `cid:` desde el primer día al ENVIAR, pero al LEER no
 * lo resolvía nadie: la imagen quedaba rota y el mensaje parecía venir vacío.
 */

/** Lo que hace falta de un adjunto para enlazarlo con el cuerpo. */
export interface ParteEnLinea {
  cid?: string | undefined;
}

/**
 * Cambia cada `src="cid:X"` por la URL de la parte que lo lleva.
 *
 * `urlDe` recibe el ÍNDICE ORIGINAL del adjunto dentro del mensaje, que es por
 * donde se descarga; renumerar aquí bajaría la parte equivocada.
 *
 * Lo que no encuentra se deja como está: un `cid:` sin parte es un mensaje mal
 * formado, y sustituirlo por una URL a ninguna parte cambiaría una imagen rota
 * por una petición inútil al servidor.
 */
export function resolverCidsEnLinea(
  html: string,
  partes: readonly ParteEnLinea[],
  urlDe: (indice: number) => string,
): string {
  const porCid = new Map<string, number>();
  partes.forEach((p, i) => {
    if (p.cid) porCid.set(p.cid.toLowerCase(), i);
  });
  if (porCid.size === 0) return html;

  // Solo dentro de un atributo `src`, y con la comilla que sea. Un `cid:` suelto
  // en el texto del correo no es una referencia y no se toca.
  return html.replace(
    /(\ssrc\s*=\s*)(["']?)cid:([^"'\s>]+)\2/gi,
    (entero, prefijo: string, comilla: string, cid: string) => {
      // Viaja percent-encoded cuando lleva caracteres raros (RFC 2392).
      let limpio = cid;
      try {
        limpio = decodeURIComponent(cid);
      } catch {
        /* un `%` suelto no es codificación: se usa tal cual */
      }
      const i = porCid.get(limpio.toLowerCase());
      if (i === undefined) return entero;
      const q = comilla || '"';
      return `${prefijo}${q}${urlDe(i)}${q}`;
    },
  );
}

/** Tipos que el navegador solo sabe decodificar como píxeles (los que el BFF
 *  acepta servir en línea). SVG fuera: puede llevar script. */
const IMAGEN_SEGURA = /^image\/(jpeg|png|gif|webp|avif|bmp|x-icon|vnd\.microsoft\.icon)$/;

/**
 * Las partes que hay que PINTAR en el cuerpo, no listar al pie.
 *
 * El iPhone manda las fotos en un `multipart/mixed` junto al texto, marcadas
 * `Content-Disposition: inline`, SIN parte HTML y SIN `Content-ID` que las cite
 * desde ningún sitio (medido sobre un mensaje real de iPhone Mail 23G83). No hay
 * nada que sustituir: o se pintan por su cuenta, o el mensaje se ve como un
 * saludo de una línea con tres ficheros colgando.
 *
 * Se conserva el ÍNDICE ORIGINAL porque es por donde se piden al servidor.
 */
export function imagenesEnLinea<T extends { type: string; inline?: boolean | undefined }>(
  partes: readonly T[],
): readonly (readonly [T, number])[] {
  return partes
    .map((a, i) => [a, i] as const)
    .filter(
      ([a]) => a.inline === true && IMAGEN_SEGURA.test(a.type.split(';')[0]!.trim().toLowerCase()),
    );
}
