/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Buscar» (BLUEPRINT.md §6.4): búsqueda de texto sobre el lienzo con
 * resaltado amarillo nativo (CSS Custom Highlight API — no muta el HTML del
 * folio, que ES el modelo persistido), navegación siguiente/anterior con
 * contador «n de m». Exporta además las utilidades de rangos/resaltado que
 * reutiliza CommentsPanel (fichero del mismo lote del carril izquierdo).
 *
 * Con una hoja no se busca en el DOM (que sería la celda enfocada): la hoja da un
 * `SearchTarget` (sheets/search.ts) que busca en sus celdas y escribe por el modelo.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Input, IconButton, useUltimo } from '@sytue/ui';
import { getEditorRoot, matchIndices, replaceRange } from '../canvas/exec';
import type { MatchOptions } from '../canvas/exec';
import { EditorIcon } from '../icons';
import { HIT_ACTIVE_BG, HIT_BG, HIT_INK, getSearchTarget } from '../sheets/search';
import type { EditorContext } from '../types';

/* ---------- Utilidades compartidas de rangos y resaltado ---------- */

/** Tope de coincidencias pintadas. */
// ponytail: 500 sobra para folios reales; subir (o paginar) si aparecen documentos-libro.
const MAX_MATCHES = 500;

/**
 * Rangos de todas las apariciones de `query` en los nodos de texto de `root`, en
 * orden de documento. El cotejo (mayúsculas, palabra completa) lo decide
 * `matchIndices` (canvas/exec, puro y con self-test); aquí solo se materializan
 * los rangos DOM. Sin `opts`: insensible a mayúsculas y sin palabra completa
 * (comportamiento previo; lo que espera CommentsPanel al re-anclar).
 */
// ponytail: indexOf por nodo de texto (§6.4): no ve coincidencias partidas
// entre nodos (p. ej. mitad en negrita); un matcher entre nodos llegará con la RGA.
export function findTextRanges(
  root: HTMLElement,
  query: string,
  limit = MAX_MATCHES,
  opts?: MatchOptions,
): Range[] {
  const out: Range[] = [];
  if (query === '') return out;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (
    let node = walker.nextNode();
    node !== null && out.length < limit;
    node = walker.nextNode()
  ) {
    for (const [start, end] of matchIndices(node.nodeValue ?? '', query, opts)) {
      if (out.length >= limit) break;
      const range = document.createRange();
      range.setStart(node, start);
      range.setEnd(node, end);
      out.push(range);
    }
  }
  return out;
}

/**
 * Pinta `ranges` bajo el nombre dado con la CSS Custom Highlight API; con la
 * lista vacía elimina ese resaltado. Devuelve `false` si la API no existe
 * (navegador antiguo o SSR) para que quien llama degrade con elegancia.
 */
export function setEditorHighlight(name: string, ranges: readonly Range[]): boolean {
  if (typeof CSS === 'undefined' || !('highlights' in CSS)) return false;
  if (ranges.length === 0) CSS.highlights.delete(name);
  else CSS.highlights.set(name, new Highlight(...ranges));
  return true;
}

/* ---------- Panel ---------- */

const HL_ALL = 'sytue-search';
const HL_ACTIVE = 'sytue-search-active';

/** Alternador de opción de búsqueda (mayúsculas / palabra completa): glifo + aria-pressed. */
const OPT_BTN =
  'inline-flex h-7 min-w-7 shrink-0 items-center justify-center rounded-sm px-1 text-xs font-semibold text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
  'sytue-focus-ring';

const REPLACE_BTN =
  'inline-flex h-7 items-center rounded-sm border border-border px-2 text-xs text-text ' +
  'hover:bg-bg-subtle disabled:pointer-events-none disabled:opacity-40 ' +
  'sytue-focus-ring';

/** Pinta todas las coincidencias y la activa; sin API nativa, selecciona la activa (§6.4). */
function paint(list: Range[], activeIndex: number): void {
  const activeRange = list[activeIndex];
  const supported = setEditorHighlight(HL_ALL, list);
  setEditorHighlight(HL_ACTIVE, activeRange ? [activeRange] : []);
  if (!supported && activeRange) {
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(activeRange);
  }
}

function scrollTo(range: Range | undefined): void {
  range?.startContainer.parentElement?.scrollIntoView({
    block: 'center',
    behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
  });
}

export function SearchPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const [query, setQuery] = useState('');
  const [replaceText, setReplaceText] = useState('');
  const [count, setCount] = useState(0);
  const [active, setActive] = useState(0);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [wholeWord, setWholeWord] = useState(false);
  /** Rangos vivos de la búsqueda actual (objetos DOM: fuera del estado Preact). */
  const rangesRef = useRef<Range[]>([]);
  /** Opciones de cotejo por referencia: las re-búsquedas silenciosas (guardado,
   *  época) las leen siempre frescas, sin depender de la clausura del efecto. */
  const optsRef = useUltimo<MatchOptions>({ caseSensitive, wholeWord });

  /** Recalcula las coincidencias con las opciones vigentes; con `jump` salta a la primera. */
  const runSearch = useCallback(
    (rawQuery: string, jump: boolean): void => {
      const root = getEditorRoot();
      const q = rawQuery.trim();
      const target = getSearchTarget();
      if (target) {
        const n = target.find(q, optsRef.current);
        setCount(n);
        setActive(0);
        if (jump && n > 0) target.goTo(0);
        return;
      }
      const list =
        root !== null && q !== '' ? findTextRanges(root, q, MAX_MATCHES, optsRef.current) : [];
      rangesRef.current = list;
      setCount(list.length);
      setActive(0);
      paint(list, 0);
      if (jump) scrollTo(list[0]);
    },
    [optsRef],
  );

  /** Navega a la coincidencia `index` (con vuelta circular). */
  const goTo = (index: number): void => {
    const target = getSearchTarget();
    if (target) {
      const n = target.count();
      if (n === 0) return;
      const next = ((index % n) + n) % n;
      setActive(next);
      target.goTo(next);
      return;
    }
    const list = rangesRef.current;
    if (list.length === 0) return;
    const next = ((index % list.length) + list.length) % list.length;
    setActive(next);
    paint(list, next);
    scrollTo(list[next]);
  };

  /** Re-busca y activa la primera coincidencia en/ tras `caret` (evita re-emparejar
   *  el texto recién insertado si el reemplazo contiene el término buscado). */
  const researchFromCaret = (caret: Range | null): void => {
    const root = getEditorRoot();
    const q = query.trim();
    const list =
      root !== null && q !== '' ? findTextRanges(root, q, MAX_MATCHES, optsRef.current) : [];
    rangesRef.current = list;
    setCount(list.length);
    let idx = 0;
    if (caret && list.length > 0) {
      const found = list.findIndex(
        (r) => r.compareBoundaryPoints(Range.START_TO_START, caret) >= 0,
      );
      idx = found === -1 ? 0 : found; // sin coincidencia posterior → vuelta al principio
    }
    setActive(idx);
    paint(list, idx);
    scrollTo(list[idx]);
  };

  /** Reemplaza la coincidencia activa y avanza a la siguiente (tras el caret). */
  const replaceOne = (): void => {
    const target = getSearchTarget();
    if (target) {
      const next = target.replace(active, replaceText);
      const n = target.count();
      setCount(n);
      setActive(n === 0 ? 0 : next);
      if (n > 0) target.goTo(next);
      return;
    }
    const range = rangesRef.current[active];
    if (!range) return;
    replaceRange(range, replaceText);
    const sel = window.getSelection();
    researchFromCaret(sel && sel.rangeCount > 0 ? sel.getRangeAt(0).cloneRange() : null);
  };

  /** Reemplaza TODAS las coincidencias en un pase inverso (offsets previos válidos). */
  const replaceAll = (): void => {
    const root = getEditorRoot();
    const q = query.trim();
    const target = getSearchTarget();
    if (target) {
      if (q === '') return;
      target.find(q, optsRef.current);
      target.replaceAll(replaceText);
      runSearch(q, false);
      return;
    }
    if (root === null || q === '') return;
    // ponytail: hasta MAX_MATCHES por clic (mismo tope que el pintado); si un doc
    // supera 500 apariciones, un segundo clic barre las restantes.
    const list = findTextRanges(root, q, MAX_MATCHES, optsRef.current);
    for (let idx = list.length - 1; idx >= 0; idx -= 1) {
      const range = list[idx];
      if (range) replaceRange(range, replaceText);
    }
    runSearch(q, false);
  };

  // Debounce de 300 ms sobre lo tecleado y sobre el cambio de opciones (§6.4).
  useEffect(() => {
    const timer = window.setTimeout(() => runSearch(query, query.trim() !== ''), 300);
    return () => window.clearTimeout(timer);
  }, [query, caseSensitive, wholeWord, runSearch]);

  // Re-búsqueda silenciosa cuando el contenido cambia por debajo: los rangos
  // caducan con el DOM (guardado confirmado, adopción remota o restauración).
  const prevSaveRef = useRef(ctx.save);
  useEffect(() => {
    const before = prevSaveRef.current;
    prevSaveRef.current = ctx.save;
    if (ctx.save === 'saved' && before !== 'saved' && query.trim() !== '') runSearch(query, false);
  }, [ctx.save, query, runSearch]);
  useEffect(() => {
    if (query.trim() !== '') runSearch(query, false);
  }, [ctx.contentEpoch, query, runSearch]);

  // Al cerrar el panel no queda pintura en el lienzo (ni en las celdas de la hoja).
  useEffect(
    () => () => {
      setEditorHighlight(HL_ALL, []);
      setEditorHighlight(HL_ACTIVE, []);
      getSearchTarget()?.find('', optsRef.current);
    },
    [optsRef],
  );

  // La hoja en solo lectura no deja reemplazar: los dos botones se apagan, como en nativo.
  const noReplace = count === 0 || getSearchTarget()?.editable() === false;

  const counterText =
    query.trim() === ''
      ? ''
      : count === 0
        ? t('find.noMatches')
        : t('find.position', { index: active + 1, total: count });

  return (
    <div class="flex flex-col gap-2 p-3">
      {/* Amarillo #FEF08A y melocotón #FED7AA de HIGHLIGHT_PALETTE y negro #0A0A0A de
          CONTENT_PALETTE (§5): colores de CONTENIDO sobre el papel blanco, no tokens de UI. */}
      <style>{`
        ::highlight(${HL_ALL}) { background-color: ${HIT_BG}; color: ${HIT_INK}; }
        ::highlight(${HL_ACTIVE}) { background-color: ${HIT_ACTIVE_BG}; color: ${HIT_INK}; }
      `}</style>

      <Input
        type="search"
        // Id estable: con el panel YA abierto, Ctrl/Cmd+F vuelve a este campo
        // (EditorApp), como el menú Archivo enfoca #editor-title-input.
        id="editor-find-input"
        // El panel solo existe mientras está abierto, y se abre para buscar: el
        // foco entra en el campo, se llegue por el carril o por Ctrl/Cmd+F. Sin
        // esto, el atajo abría el panel y lo tecleado seguía yendo al documento.
        // oxlint-disable-next-line jsx-a11y/no-autofocus
        autofocus
        placeholder={t('find.placeholder')}
        aria-label={t('find.label')}
        leadingIcon={<EditorIcon name="search" size="md" />}
        value={query}
        onInput={(event) => setQuery(event.currentTarget.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault();
            goTo(event.shiftKey ? active - 1 : active + 1);
          }
        }}
      />

      <div class="flex items-center gap-1">
        <p class="text-text-muted min-w-0 flex-1 truncate text-xs" aria-live="polite">
          {counterText}
        </p>
        <button
          type="button"
          class={OPT_BTN}
          aria-label={t('find.matchCase')}
          title={t('find.matchCase')}
          aria-pressed={caseSensitive}
          onClick={() => setCaseSensitive((v) => !v)}
        >
          Aa
        </button>
        <button
          type="button"
          class={OPT_BTN}
          aria-label={t('find.wholeWord')}
          title={t('find.wholeWord')}
          aria-pressed={wholeWord}
          onClick={() => setWholeWord((v) => !v)}
        >
          «ab»
        </button>
        <span aria-hidden="true" class="bg-border mx-0.5 h-5 w-px" />
        <IconButton
          size="md"
          label={t('find.previous')}
          showTitle={false}
          title={t('find.previous.title')}
          disabled={count === 0}
          onClick={() => goTo(active - 1)}
        >
          <EditorIcon name="chevronUp" size="md" />
        </IconButton>
        <IconButton
          size="md"
          label={t('find.next')}
          showTitle={false}
          title={t('find.next.title')}
          disabled={count === 0}
          onClick={() => goTo(active + 1)}
        >
          <EditorIcon name="chevronDown" size="md" />
        </IconButton>
      </div>

      <Input
        type="text"
        placeholder={t('find.replace.placeholder')}
        aria-label={t('find.replace.label')}
        value={replaceText}
        onInput={(event) => setReplaceText(event.currentTarget.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault();
            replaceOne();
          }
        }}
      />

      <div class="flex items-center justify-end gap-1">
        <button type="button" class={REPLACE_BTN} disabled={noReplace} onClick={replaceOne}>
          {t('find.replace.one')}
        </button>
        <button type="button" class={REPLACE_BTN} disabled={noReplace} onClick={replaceAll}>
          {t('find.replace.all')}
        </button>
      </div>
    </div>
  );
}
