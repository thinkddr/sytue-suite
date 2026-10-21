/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Índice alfabético — AYUDAS PURAS (sin DOM). Hermana de la tabla de contenidos,
 * pero al revés: en vez de listar los títulos en orden de documento, recopila los
 * TÉRMINOS marcados por el usuario y los lista ORDENADOS alfabéticamente, agrupando
 * subentradas. El pegamento con el folio (marcar la selección con
 * <span data-index-entry>, numerar las apariciones y (re)generar la
 * <section data-index>) vive en ribbon/tabs/Referencias.tsx; aquí solo la lógica no
 * trivial y testeable: parsear «término:subtérmino», AGRUPAR+ORDENAR las entradas en
 * la estructura del índice y volcarla a HTML. Los términos son texto de usuario: se
 * ESCAPAN al volcarlos a HTML (frontera de confianza, anti-XSS), igual que en
 * bibliography.ts / mailmerge.ts. Ejecutar el self-test:
 *   cd apps/frontend && bun test packages/office/test/alphaindex.test.ts
 */

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa texto para inyectarlo como HTML (mismo criterio que exec.ts/bibliography.ts). */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/** Ancla de una aparición marcada: id del <span> y destino del enlace del índice.
 *  Fuente ÚNICA del formato para que el DOM (id del span) y el HTML del índice
 *  (href) no diverjan. */
export const indexAnchorId = (ref: string): string => `idx-${ref}`;

/**
 * Una aparición marcada del cuerpo, ya numerada. `ref` = número de aparición en
 * orden de documento (el nº de página real exige paginación; sin ella se usa el
 * orden). `sub` vacío = entrada de primer nivel.
 */
export interface IndexEntry {
  term: string;
  sub: string;
  ref: string;
}

/** Subentrada agrupada: el subtérmino y sus referencias en orden de aparición. */
export interface IndexSub {
  sub: string;
  refs: string[];
}

/** Término del índice: sus referencias directas (sin subentrada) y sus subentradas. */
export interface IndexTerm {
  term: string;
  refs: string[];
  subs: IndexSub[];
}

/**
 * Parsea el texto del prompt de «marcar entrada» en {término, subtérmino}. La
 * subentrada es opcional y se separa con el PRIMER dos puntos («Animales:Perros»);
 * los dos puntos posteriores quedan dentro del subtérmino («A:B:C» → sub «B:C»).
 * Devuelve null si no queda término principal (cadena vacía o «:algo»).
 */
export function parseIndexTerm(raw: string): { term: string; sub: string } | null {
  const s = raw.trim();
  if (s === '') return null;
  const colon = s.indexOf(':');
  if (colon < 0) return { term: s, sub: '' };
  const term = s.slice(0, colon).trim();
  const sub = s.slice(colon + 1).trim();
  return term === '' ? null : { term, sub };
}

/** Añade `ref` a `refs` si no está ya (idempotente: marcar dos veces el mismo punto no duplica). */
function pushUnique(refs: string[], ref: string): void {
  if (!refs.includes(ref)) refs.push(ref);
}

/**
 * AGRUPA las apariciones por término (y por subtérmino dentro de cada uno) y
 * ORDENA los términos y las subentradas alfabéticamente (localeCompare 'es':
 * pliega acentos y mayúsculas). Las REFERENCIAS conservan el orden de aparición
 * en el documento (las entradas llegan en ese orden), así que salen ascendentes.
 * Las entradas sin término se descartan. Pura: la cubre el self-test.
 * ponytail: agrupa por término EXACTO (el usuario lo teclea); «Café» y «café» son
 * grupos distintos. Plegado por may/min si se pide paridad total con Word.
 */
export function buildIndex(entries: readonly IndexEntry[]): IndexTerm[] {
  // Acumula con mapas (término→refs directas + subtérmino→refs) para que la
  // agrupación sea O(n) también con muchos términos/subentradas (documentos largos).
  interface Acc {
    term: string;
    refs: string[];
    subs: Map<string, string[]>;
  }
  const byTerm = new Map<string, Acc>();
  for (const e of entries) {
    const term = e.term.trim();
    if (term === '') continue; // sin término no hay entrada
    let g = byTerm.get(term);
    if (!g) {
      g = { term, refs: [], subs: new Map() };
      byTerm.set(term, g);
    }
    const sub = e.sub.trim();
    if (sub === '') {
      pushUnique(g.refs, e.ref);
    } else {
      let refs = g.subs.get(sub);
      if (!refs) {
        refs = [];
        g.subs.set(sub, refs);
      }
      pushUnique(refs, e.ref);
    }
  }
  return Array.from(byTerm.values(), (g) => ({
    term: g.term,
    refs: g.refs,
    subs: Array.from(g.subs, ([sub, refs]) => ({ sub, refs })).sort((a, b) =>
      a.sub.localeCompare(b.sub, 'es'),
    ),
  })).sort((a, b) => a.term.localeCompare(b.term, 'es'));
}

/** Sangría de las subentradas, inline para sobrevivir al export (mismo criterio que el TDC). */
const LIST_STYLE = 'list-style:none;padding-left:0;margin:.25em 0';
const SUB_STYLE = 'list-style:none;padding-left:1.5em;margin:0';

/** «<a href="#idx-1">1</a>, <a href="#idx-5">5</a>»: enlaces a cada aparición. */
function refsHtml(refs: readonly string[]): string {
  return refs
    .map((r) => `<a href="#${escapeHtml(indexAnchorId(r))}">${escapeHtml(r)}</a>`)
    .join(', ');
}

/**
 * Vuelca la estructura del índice a HTML (cuerpo de la <section data-index>):
 * una lista sin viñetas de términos ordenados, cada uno con sus referencias y una
 * sublista de subentradas. Términos/subtérminos ESCAPADOS (anti-XSS). Cadena vacía
 * si no hay entradas (el llamador pone entonces un aviso). Pura: la cubre el self-test.
 */
export function buildIndexHtml(entries: readonly IndexEntry[]): string {
  const groups = buildIndex(entries);
  if (groups.length === 0) return '';
  const items = groups.map((g) => {
    const direct = g.refs.length > 0 ? ` ${refsHtml(g.refs)}` : '';
    const subs =
      g.subs.length > 0
        ? `<ul style="${SUB_STYLE}">${g.subs
            .map((s) => `<li data-index-sub>${escapeHtml(s.sub)} ${refsHtml(s.refs)}</li>`)
            .join('')}</ul>`
        : '';
    return `<li data-index-term>${escapeHtml(g.term)}${direct}${subs}</li>`;
  });
  return `<ul style="${LIST_STYLE}">${items.join('')}</ul>`;
}
