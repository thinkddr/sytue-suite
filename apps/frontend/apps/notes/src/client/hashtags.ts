/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hashtags — `#palabra` escrita en una nota, pulsable al pintarla.
 *
 * QUIÉN ES HASHTAG LO DECIDE EL SERVICIO (`domain.Hashtags`, services/notes): la
 * lista con su recuento y la búsqueda por `#palabra` salen de allí. Esto es su
 * espejo para PINTAR —la misma regla, los mismos casos en la prueba— y nada de
 * lo que hace se guarda: el contenido de la nota no cambia.
 */

// `#` que empieza palabra y va seguido de letra: ni `texto#ancla`, ni `/#ancla`,
// ni `#1`, ni la entidad `&#39;`, ni el `##` de un título de Markdown.
const HASHTAG = /(^|[^\p{L}\p{N}_&/#])#(\p{L}[\p{L}\p{N}_-]*)/gu;

/** La etiqueta sin el guion de cola, o `null` si es un color (`#ff0000`) o no cabe. */
function etiquetaDe(cruda: string): string | null {
  const tag = cruda.replace(/[-_]+$/, '');
  if (tag.length > 64 || (/^[0-9a-f]{6}$/i.test(tag) && /\d/.test(tag))) return null;
  return tag;
}

/**
 * Aplica `fn` a los trozos de TEXTO de un HTML. Dentro de un enlace, de `code` o de
 * `pre` no se toca: ahí un `#` o una `@` son parte de una URL o de un trozo de código.
 */
function enTexto(html: string, fn: (texto: string) => string): string {
  let fuera = 0;
  return html.replace(
    /<(\/?)(a|code|pre)\b[^>]*>|<[^>]*>|[^<]+/gi,
    (trozo, cierre?: string, etiqueta?: string) => {
      if (etiqueta) fuera = Math.max(0, fuera + (cierre ? -1 : 1));
      return fuera > 0 || trozo.startsWith('<') ? trozo : fn(trozo);
    },
  );
}

/**
 * El mismo HTML con cada hashtag envuelto en un enlace `a[data-hashtag]`. Se hace
 * al PINTAR, como `conAnclas`.
 */
export function conHashtags(html: string): string {
  return enTexto(html, (texto) =>
    texto.replace(HASHTAG, (todo, antes: string, cruda: string) => {
      const tag = etiquetaDe(cruda);
      if (!tag) return todo;
      // `tag` solo lleva letras, cifras, `_` y `-`: no hay nada que escapar.
      return `${antes}<a class="notes-hashtag" data-hashtag="${tag}" href="#${tag}">#${tag}</a>${cruda.slice(tag.length)}`;
    }),
  );
}

// `@` y una dirección, sin nada pegado delante (`@ana@empresa.es`): el espejo de
// `hubnotify.Mencionados`, que es quien avisa a esa persona al guardar la nota.
const MENCION = /(^|[\s(])@([^\s@()<>,;]+@[^\s@()<>,;]+\.[^\s@()<>,;]+)/g;

/** El mismo HTML con cada mención resaltada (`span.notes-mention`), al pintar. */
export function conMenciones(html: string): string {
  return enTexto(html, (texto) =>
    texto.replace(MENCION, (_todo, antes: string, cruda: string) => {
      // El punto final de la frase no es de la dirección. Va en el contenido de un
      // elemento, no en un atributo, y ya venía escapado: no hay nada que escapar.
      const dir = cruda.replace(/\.+$/, '');
      return `${antes}<span class="notes-mention">@${dir}</span>${cruda.slice(dir.length)}`;
    }),
  );
}
