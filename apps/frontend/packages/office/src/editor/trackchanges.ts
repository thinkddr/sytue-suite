/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Control de cambios (track changes) — núcleo PURO (BLUEPRINT.md §3, encargo
 * «modo control de cambios»). EL HTML ES EL MODELO: una edición rastreada se
 * marca en el propio HTML del folio con `<ins data-track …>` (inserción) o
 * `<del data-track …>` (borrado), con autoría escapada y color por autor inline
 * (viaja en el HTML, sin registro externo). Aquí solo funciones sin DOM: generar
 * la marca (escapando el dato del autor, frontera de confianza) y NORMALIZAR
 * marcas adyacentes del mismo autor (colapsa una racha en una sola marca). El
 * pegamento DOM vivo (interceptar beforeinput, mover el caret) vive en
 * canvas/exec.ts: hornea las marcas con insertionHtml/deletionHtml y funde las
 * contiguas sobre nodos VIVOS (coalesceMark, O(1) y sin mover el caret); esta
 * mergeAdjacentTrackMarks es la especificación PURA de esa fusión (string→string),
 * cubierta por test/trackchanges.test.ts.
 *
 * Aquí viven además, como funciones puras y probadas: la PERSISTENCIA del modo
 * por documento (una sentinela invisible al frente del HTML — el modo VIVE en el
 * contenido, que ES el modelo, y viaja con el autosave) y la RESOLUCIÓN de las
 * marcas (aceptar/rechazar = consolidar el HTML como si el cambio se hubiera
 * aplicado o descartado). El pegamento DOM (canvas/exec.ts) es una cáscara fina
 * sobre estas.
 */

// peerColor es una función PURA (hash → token de la paleta de marca) y no arrastra
// DOM ni estado, así que reutilizarla aquí respeta el carácter puro de este núcleo
// y deja UNA sola fuente de verdad para «autor → color»: el mismo autor sale del
// mismo color en su caret, su avatar de cabecera y sus marcas de control de cambios.
import { peerColor } from './state/cursors';

/** Autor de una marca de rastreo: nombre visible + id estable (sub de sesión). */
export interface TrackAuthor {
  name: string;
  id: string;
}

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa datos del usuario antes de inyectarlos en el HTML (anti-XSS). */
function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/**
 * Color de marca estable derivado del id del autor: un token de la paleta de
 * marca (`--sytue-avatar-1..6`), NO un matiz arbitrario. Distingue autores sin
 * registro de identidad y va inline en cada marca para sobrevivir a la recarga.
 * Delega en peerColor para que el color coincida con el caret y el avatar del
 * mismo autor, y para no salirse de Graphite & Brass sobre el folio.
 */
export function authorColor(id: string): string {
  return peerColor(id);
}

/** Atributos comunes de una marca: bandera `data-track`, autoría escapada, sello
 *  de tiempo y color por autor (variable CSS inline que consume editor.css). */
function trackAttrs(author: TrackAuthor, ts: number): string {
  return (
    ` data-track data-author="${escapeHtml(author.name)}"` +
    ` data-author-id="${escapeHtml(author.id)}" data-ts="${ts}"` +
    ` style="--sytue-track:${authorColor(author.id)}"`
  );
}

/** `<ins data-track …>texto</ins>` con el texto escapado; '' si el texto es vacío. */
export function insertionHtml(text: string, author: TrackAuthor, ts: number): string {
  return text === '' ? '' : `<ins${trackAttrs(author, ts)}>${escapeHtml(text)}</ins>`;
}

/** `<del data-track …>texto</del>` con el texto escapado; '' si el texto es vacío. */
export function deletionHtml(text: string, author: TrackAuthor, ts: number): string {
  return text === '' ? '' : `<del${trackAttrs(author, ts)}>${escapeHtml(text)}</del>`;
}

/** `data-author-id` de una cadena de atributos, o null si no lo trae. */
function authorId(attrs: string): string | null {
  return /\sdata-author-id="([^"]*)"/.exec(attrs)?.[1] ?? null;
}

/**
 * Colapsa rachas de marcas de rastreo ADYACENTES (sin nada entre medias) del
 * mismo tipo (`<ins>`/`<ins>` o `<del>`/`<del>`) y mismo autor en una sola marca,
 * conservando los atributos de la PRIMERA (su `data-ts` = inicio de la racha).
 * Es conservador: solo toca marcas `data-track` de contenido de texto plano (las
 * del MVP no anidan elementos); cualquier otra cosa —texto suelto entre marcas,
 * marcas de distinto autor, `<ins>`/`<del>` sin `data-track`, contenido con
 * etiquetas— pasa intacto. Función pura (string→string): la usa el pegamento DOM
 * como especificación y la cubre el self-test.
 */
export function mergeAdjacentTrackMarks(html: string): string {
  const mark = /<(ins|del)\b([^>]*)>([^<]*)<\/\1>/g;
  let out = '';
  let lastIndex = 0;
  // Marca ABIERTA (aún sin emitir) con la que podría fundirse la siguiente si es
  // contigua, del mismo tipo y autor. id=null ⇒ no rastreada (nunca funde).
  let open: { tag: string; attrs: string; id: string | null; content: string } | null = null;
  const flush = (): void => {
    if (open) out += `<${open.tag}${open.attrs}>${open.content}</${open.tag}>`;
    open = null;
  };
  for (let m = mark.exec(html); m !== null; m = mark.exec(html)) {
    const [full, tag, attrs, content] = m as unknown as [string, string, string, string];
    const gap = html.slice(lastIndex, m.index); // texto entre la marca previa y esta
    const id = /\sdata-track\b/.test(attrs) ? authorId(attrs) : null;
    if (open && gap === '' && id !== null && open.tag === tag && open.id === id) {
      open.content += content; // contigua + mismo tipo + mismo autor ⇒ fundir
    } else {
      flush();
      out += gap;
      open = { tag, attrs, id, content };
    }
    lastIndex = m.index + full.length;
  }
  flush();
  return out + html.slice(lastIndex);
}

/* ---------- Persistencia del MODO por documento (round-trip en el HTML) ---------- */

/**
 * Sentinela invisible que marca el documento como «en control de cambios». Va al
 * FRENTE del HTML persistido (comentario HTML: ni se renderiza ni cuenta como
 * palabra) para que el modo VIVA en el contenido —que ES el modelo— y viaje con
 * el autosave, la colaboración y las versiones. NO entra al DOM editable:
 * PageCanvas lo retira al cargar y lo antepone al guardar, así el usuario no lo
 * borra al teclear. Resuelve que el `data-track` del root no sobreviva al
 * innerHTML (los atributos del propio root no se serializan con `.innerHTML`).
 */
export const TRACK_SENTINEL = '<!--sytue-track-->';

/** Antepone la sentinela del modo al HTML cuando el control de cambios está activo. */
export const withTrackFlag = (html: string, on: boolean): string =>
  on ? TRACK_SENTINEL + html : html;

/** Separa la sentinela del modo del frente del HTML persistido (round-trip de `on`). */
export const readTrackFlag = (html: string): { on: boolean; html: string } =>
  html.startsWith(TRACK_SENTINEL)
    ? { on: true, html: html.slice(TRACK_SENTINEL.length) }
    : { on: false, html };

/* ---------- Resolver marcas: aceptar / rechazar (consolidación) ---------- */

/**
 * Barre las marcas de rastreo de `html` EN ORDEN DE DOCUMENTO (una sola pasada,
 * O(n)). `visit` recibe el ordinal 0-based de cada marca rastreada (`data-track`)
 * y sus partes; devuelve el texto con el que sustituirla o `null` para dejarla
 * intacta. Base ÚNICA de listar y de resolver: así el ordinal del panel, el índice
 * de `querySelectorAll` del DOM vivo y el que reescribe el HTML son el MISMO.
 * Conservador como mergeAdjacentTrackMarks: solo cuenta/toca `<ins>/<del>` con
 * `data-track` y contenido de texto plano (las marcas del MVP no anidan);
 * cualquier otra cosa —incluidos `<ins>/<del>` sin rastrear— ni cuenta ni se toca.
 */
function scanTrackMarks(
  html: string,
  visit: (index: number, tag: 'ins' | 'del', attrs: string, content: string) => string | null,
): string {
  let index = -1;
  return html.replace(
    /<(ins|del)\b([^>]*)>([^<]*)<\/\1>/gi,
    (full: string, tag: string, attrs: string, content: string): string => {
      if (!/\sdata-track\b/.test(attrs)) return full; // no rastreada: ni cuenta ni se toca
      index += 1;
      return visit(index, tag.toLowerCase() as 'ins' | 'del', attrs, content) ?? full;
    },
  );
}

/** Texto que SOBREVIVE al resolver una marca: se conserva al ACEPTAR una inserción
 *  o al RECHAZAR un borrado; en los otros dos casos la marca desaparece (''). */
const resolvedContent = (accept: boolean, tag: 'ins' | 'del', content: string): string =>
  accept === (tag === 'ins') ? content : '';

/** Consolida: deja el documento como si los cambios rastreados se hubieran aplicado. */
export const acceptTrackedChanges = (html: string): string =>
  scanTrackMarks(html, (_i, tag, _a, content) => resolvedContent(true, tag, content));

/** Descarta: deja el documento como estaba antes de los cambios rastreados. */
export const rejectTrackedChanges = (html: string): string =>
  scanTrackMarks(html, (_i, tag, _a, content) => resolvedContent(false, tag, content));

/* ---------- Panel de revisión: listar y resolver UNA marca ---------- */

/** Un cambio rastreado localizado en el documento (una fila del panel de revisión). */
export interface TrackedChange {
  /** Ordinal 0-based entre las marcas rastreadas EN ORDEN DE DOCUMENTO. Ancla
   *  estable de una instantánea del HTML: coincide con el índice de la misma marca
   *  en `querySelectorAll('ins[data-track],del[data-track]')` del DOM vivo, así el
   *  panel resuelve/navega/resalta por ese índice sin ids en el HTML. */
  id: number;
  /** Inserción (`<ins>`) o borrado (`<del>`). */
  type: 'insert' | 'delete';
  /** Nombre del autor (des-escapado para pintarlo como texto en la UI). */
  author: string;
  /** Extracto del texto de la marca (des-escapado para pintarlo como texto). */
  text: string;
}

const HTML_UNESCAPES: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
};

/** Inverso de escapeHtml (solo las 5 entidades que produce): el dato escapado del
 *  HTML vuelve a texto llano para pintarlo en la UI (Preact lo re-escapa al render).
 *  Pasada única izquierda→derecha: `&amp;` se resuelve antes de avanzar, sin
 *  doble-desescape (`&amp;lt;` → `&lt;`, no `<`). */
const unescapeHtml = (text: string): string =>
  text.replace(/&(?:amp|lt|gt|quot|#39);/g, (entity) => HTML_UNESCAPES[entity] ?? entity);

/** Valor del atributo `name="…"` en una cadena de atributos, o '' si no está. */
const attrValue = (attrs: string, name: string): string =>
  new RegExp(`\\s${name}="([^"]*)"`).exec(attrs)?.[1] ?? '';

/**
 * Lista los cambios rastreados de `html` EN ORDEN DE DOCUMENTO (autor + tipo +
 * extracto), con `id` = ordinal 0-based. Función pura para el panel de revisión;
 * su longitud es el recuento de cambios. Los ids son contiguos 0..n-1.
 */
export function listTrackedChanges(html: string): TrackedChange[] {
  const changes: TrackedChange[] = [];
  scanTrackMarks(html, (id, tag, attrs, content) => {
    changes.push({
      id,
      type: tag === 'ins' ? 'insert' : 'delete',
      author: unescapeHtml(attrValue(attrs, 'data-author')),
      text: unescapeHtml(content),
    });
    return null; // solo lectura: no sustituye nada
  });
  return changes;
}

/**
 * Resuelve UNA sola marca —la de ordinal `id` (ver TrackedChange.id)— dejando el
 * resto intactas: ACEPTAR conserva el texto de una inserción / elimina un borrado;
 * RECHAZAR hace lo contrario. Identidad si `id` cae fuera de rango. Función pura
 * (string→string) cubierta por el self-test; el pegamento DOM la aplica al folio.
 */
export const resolveTrackedChange = (html: string, id: number, accept: boolean): string =>
  scanTrackMarks(html, (index, tag, _attrs, content) =>
    index === id ? resolvedContent(accept, tag, content) : null,
  );
