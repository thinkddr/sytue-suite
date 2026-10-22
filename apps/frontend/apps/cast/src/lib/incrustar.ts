/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El código de inserción de un vídeo: el mismo `<iframe>` que devuelve el oEmbed
 * del servicio (`services/cast/internal/http/handlers.go`, OEmbed), para que pegar
 * el código a mano y pegar el enlace en un sitio con oEmbed den lo mismo.
 */

function escapar(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/** 640 de ancho y el alto de la proporción del vídeo (16:9 si no se sabe). */
export function medidas(ancho: number | null, alto: number | null): { w: number; h: number } {
  const w = 640;
  if (!ancho || !alto || ancho <= 0 || alto <= 0) return { w, h: 360 };
  return { w, h: Math.round((w * alto) / ancho) };
}

export function codigoDeInsercion(
  urlIncrustado: string,
  titulo: string,
  ancho: number | null,
  alto: number | null,
): string {
  const { w, h } = medidas(ancho, alto);
  return (
    `<iframe src="${escapar(urlIncrustado)}" width="${w}" height="${h}" frameborder="0" ` +
    `allow="autoplay; fullscreen; picture-in-picture" allowfullscreen title="${escapar(titulo)}"></iframe>`
  );
}
