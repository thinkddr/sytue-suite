/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Captura — lo que se SUELTA o se PEGA sobre el cuaderno se convierte en nota.
 *
 * Es el gesto que define a BasKet Note Pads: el cuaderno es donde se tiran las
 * cosas, no un formulario. Un enlace se vuelve nota de enlace; un texto, nota de
 * texto. Aquí está solo la interpretación, sin DOM, para poder probarla.
 *
 * SOLO TEXTO PLANO, nunca `text/html`: es la misma política que el editor de la
 * nota (ver la cabecera de RichText). El HTML del portapapeles de un navegador
 * trae estilos y estructura ajenos, y el servicio lo sanearía, pero lo que
 * quedaría no se parece a lo que la persona vio. El texto es lo que quiso pegar.
 */

export type Captura =
  { kind: 'link'; title: string; data: { url: string } } | { kind: 'text'; content: string };

/** El mismo criterio que `ValidWebURL` en el servicio: http/https con host. */
export function esURLWeb(v: string): boolean {
  try {
    const u = new URL(v);
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.hostname !== '';
  } catch {
    return false;
  }
}

/**
 * Interpreta un portapapeles o un arrastre. `uriList` es `text/uri-list` —lo que
 * pone el navegador al arrastrar un enlace o un marcador— y manda sobre el texto
 * porque es la URL sin adornos; el texto plano de ese mismo arrastre suele ser
 * el título del enlace, no la dirección.
 */
export function interpretarCaptura(texto: string, uriList = ''): Captura | null {
  // `text/uri-list` (RFC 2483): una URL por línea, y las que empiezan por `#`
  // son comentarios. Se toma la primera que valga.
  for (const linea of uriList.split(/\r?\n/)) {
    const l = linea.trim();
    if (l && !l.startsWith('#') && esURLWeb(l)) return enlace(l);
  }
  const t = texto.trim();
  if (!t) return null;
  // Un texto que es UNA sola URL es un enlace, no un párrafo con una URL dentro.
  if (!/\s/.test(t) && esURLWeb(t)) return enlace(t);
  return { kind: 'text', content: textoAHtml(t) };
}

function enlace(url: string): Captura {
  // El título es el host: es lo que se lee de un vistazo en la tarjeta, y la
  // URL entera ya sale en el cuerpo, entera y pulsable.
  return { kind: 'link', title: new URL(url).hostname, data: { url } };
}

/** Texto plano a HTML de párrafos: línea en blanco separa párrafos, salto simple es `<br>`. */
export function textoAHtml(texto: string): string {
  return texto
    .split(/\n{2,}/)
    .map((p) => `<p>${escapar(p.trim()).replaceAll('\n', '<br>')}</p>`)
    .join('');
}

/** Lo mínimo para que un texto sea HTML inerte. El servicio vuelve a sanear al guardar. */
export function escapar(s: string): string {
  return s
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}
