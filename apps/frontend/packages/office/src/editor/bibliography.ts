/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bibliografía y citas — AYUDAS PURAS (sin DOM). El pegamento con el folio
 * (leer/escribir las fuentes embebidas, insertar citas, (re)generar la sección)
 * vive en ribbon/tabs/Bibliografia.tsx; aquí solo la lógica no trivial y
 * testeable: formatear una cita en texto «(Autor, año)», una entrada estilo APA
 * básico y (re)construir la lista de bibliografía con las fuentes CITADAS,
 * ordenadas por autor. Los campos de las fuentes son de usuario: se ESCAPAN al
 * volcarlos a HTML (frontera de confianza, anti-XSS), igual que en mailmerge.
 * Ejecutar el self-test:
 *   cd apps/frontend && bun test packages/office/test/bibliography.test.ts
 */

/** Tipo de fuente (define el formateo APA: cursiva del título o de la revista). */
export type SourceType = 'libro' | 'articulo' | 'web';

/** Fuente bibliográfica. Los campos son texto libre del usuario (se escapan al
 *  volcarlos a HTML). `publisher` es editorial (libro) · revista (artículo) ·
 *  sitio (web); `url` solo aplica a `web`. */
export interface Source {
  id: string;
  type: SourceType;
  author: string;
  title: string;
  year: string;
  publisher: string;
  url?: string;
}

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa texto para inyectarlo como HTML (mismo criterio que exec.ts/mailmerge). */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/**
 * Apellido para la cita en texto: lo previo a la 1.ª coma («Apellido, Nombre»),
 * o el autor entero si no hay coma (una entidad, un único nombre).
 * ponytail: un solo autor; «et al.» y varios autores llegarán si se piden.
 */
export function authorSurname(author: string): string {
  const a = author.trim();
  const comma = a.indexOf(',');
  return (comma >= 0 ? a.slice(0, comma) : a).trim();
}

/**
 * Cita en texto «(Autor, año)». Sin autor → título; sin ninguno → «Anónimo».
 * Sin año → «s.f.» (sin fecha, criterio APA).
 */
export function citationText(source: Source): string {
  const who = authorSurname(source.author) || source.title.trim() || 'Anónimo';
  const year = source.year.trim() || 's.f.';
  return `(${who}, ${year})`;
}

/**
 * Entrada de bibliografía en HTML estilo APA básico, con los campos ESCAPADOS:
 * «Autor (Año). Título. Fuente. URL». Va en cursiva el título del libro/web y la
 * revista del artículo. Los segmentos vacíos se omiten; el año siempre aparece.
 */
export function apaEntry(source: Source): string {
  const a = escapeHtml(source.author.trim());
  const y = escapeHtml(source.year.trim() || 's.f.');
  const t = escapeHtml(source.title.trim());
  const p = escapeHtml(source.publisher.trim());
  const u = escapeHtml((source.url ?? '').trim());
  const italicTitle = source.type !== 'articulo'; // libro/web: título en cursiva

  const segs: string[] = [];
  segs.push(a ? `${a} (${y}).` : `(${y}).`);
  if (t) segs.push(italicTitle ? `<i>${t}</i>.` : `${t}.`);
  if (p) segs.push(source.type === 'articulo' ? `<i>${p}</i>.` : `${p}.`);
  if (u) segs.push(u);
  return segs.join(' ');
}

/** Sangría francesa de cada entrada, inline para sobrevivir al export (mismo
 *  criterio que la sangría del TDC y el estilo de las leyendas). */
const ENTRY_STYLE = 'margin:.35em 0;padding-left:2em;text-indent:-2em';

/**
 * (Re)construye el cuerpo de la bibliografía: las fuentes CITADAS (por id),
 * ordenadas por apellido y luego por título, cada una como un `<p>` con sangría
 * francesa y su entrada APA. Cadena vacía si ninguna fuente citada existe (los
 * ids colgantes —citas cuya fuente se borró— se ignoran). Pura: la cubre el
 * self-test. La ordenación no depende del orden de `sources` ni de `citedIds`.
 */
export function buildBibliographyList(sources: Source[], citedIds: readonly string[]): string {
  const cited = new Set(citedIds);
  return sources
    .filter((s) => cited.has(s.id))
    .sort(
      (a, b) =>
        authorSurname(a.author).localeCompare(authorSurname(b.author), 'es') ||
        a.title.localeCompare(b.title, 'es'),
    )
    .map((s) => `<p data-bib-entry style="${ENTRY_STYLE}">${apaEntry(s)}</p>`)
    .join('');
}

/**
 * HTML del marcador de cita (FUENTE ÚNICA del formato, para que insertar y
 * refrescar no diverjan). `contenteditable=false` lo hace una ficha atómica; el
 * id (nuestro) y el texto de la cita van escapados.
 */
export function citationHtml(source: Source): string {
  return `<span data-citation="${escapeHtml(source.id)}" contenteditable="false">${escapeHtml(
    citationText(source),
  )}</span>`;
}
