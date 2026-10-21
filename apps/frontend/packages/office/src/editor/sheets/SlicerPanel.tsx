/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Segmentación de datos (slicer): tarjeta flotante sobre la rejilla que filtra
 * una columna del autofiltro de la hoja. Lista los valores ÚNICOS del campo como
 * botones; los SELECCIONADOS (visibles) quedan resaltados y los excluidos, en
 * gris tachado. Clic = alternar; «Borrar filtro» reselecciona todo. La tarjeta se
 * arrastra por su cabecera (Pointer Events) y persiste x/y al soltar, igual que
 * los gráficos (sheets/Chart.tsx). El filtrado en vivo lo hace el autofiltro: la
 * selección se traduce a `SheetAutoFilter.excl` (excludedFromSelection) y el
 * lienzo oculta las filas (filteredRows) — cero lógica de filtrado propia.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import type { Slicer } from '../types';
import { useT } from '../context';
import { EditorIcon } from '../icons';
import { CHART_POS_MAX } from './format';
import type { SlicerEntry } from './slicer';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

export interface SheetSlicerProps {
  slicer: Slicer;
  /** Nombre del campo (cabecera de la columna) o su letra si no hay cabecera. */
  field: string;
  /** Valores únicos de la columna con su estado de exclusión. */
  entries: SlicerEntry[];
  /** Hay un autofiltro que cubre la columna, o la tabla dinámica sigue en la hoja (si
   *  no, la tarjeta queda inerte). */
  active: boolean;
  /** Factor del zoom CSS de la rejilla (ctx.ui.zoom/100): corrige los deltas del puntero. */
  zoom: number;
  onMove: (x: number, y: number) => void;
  /** Aplica una nueva selección (visibles) sobre el conjunto de valores únicos. */
  onApply: (selected: string[], unique: string[]) => void;
  onRemove: () => void;
}

export function SheetSlicer({
  slicer,
  field,
  entries,
  active,
  zoom,
  onMove,
  onApply,
  onRemove,
}: SheetSlicerProps): JSX.Element {
  const t = useT();
  /** Desplazamiento del arrastre en curso (transform de previsualización). */
  const [drag, setDrag] = useState<{ dx: number; dy: number } | null>(null);
  const dragRef = useRef<{ x0: number; y0: number; dx: number; dy: number } | null>(null);

  const x = slicer.x ?? 96;
  const y = slicer.y ?? 48;

  const onPointerDown = (e: PointerEvent): void => {
    e.preventDefault(); // ni selección de texto ni foco robado a la rejilla
    dragRef.current = { x0: e.clientX, y0: e.clientY, dx: 0, dy: 0 };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e: PointerEvent): void => {
    const d = dragRef.current;
    if (!d) return;
    d.dx = (e.clientX - d.x0) / zoom;
    d.dy = (e.clientY - d.y0) / zoom;
    setDrag({ dx: d.dx, dy: d.dy });
  };
  const onPointerEnd = (commit: boolean): void => {
    const d = dragRef.current;
    dragRef.current = null;
    setDrag(null);
    if (commit && d && (d.dx !== 0 || d.dy !== 0)) {
      onMove(
        Math.min(CHART_POS_MAX, Math.max(0, Math.round(x + d.dx))),
        Math.min(CHART_POS_MAX, Math.max(0, Math.round(y + d.dy))),
      );
    }
  };

  const unique = entries.map((e) => e.value);
  const selected = entries.filter((e) => !e.hidden).map((e) => e.value);
  const anyHidden = entries.some((e) => e.hidden);

  /** Alterna un valor en la selección y reaplica (multi-selección). */
  const toggle = (value: string): void => {
    const set = new Set(selected);
    if (set.has(value)) set.delete(value);
    else set.add(value);
    onApply([...set], unique);
  };

  return (
    <section
      class="sytue-papel border-border bg-surface absolute z-10 w-44 overflow-hidden rounded-lg border shadow-[var(--shadow-md)] select-none"
      style={{
        left: `${x}px`,
        top: `${y}px`,
        transform: drag ? `translate(${drag.dx}px, ${drag.dy}px)` : undefined,
      }}
      aria-label={t('slicer.card.label', { field })}
    >
      {/* Cabecera: asa de arrastre + borrar filtro + eliminar. */}
      <div
        class="border-border text-text-muted flex h-7 cursor-move items-center gap-1.5 border-b px-2"
        style={{ touchAction: 'none' }}
        title={t('chart.drag')}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={() => onPointerEnd(true)}
        onPointerCancel={() => onPointerEnd(false)}
      >
        <EditorIcon name="slicer" size="sm" />
        <span class="text-text min-w-0 flex-1 truncate text-xs font-medium" title={field}>
          {field}
        </span>
        <button
          type="button"
          class={`hover:bg-bg-subtle inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm disabled:pointer-events-none disabled:opacity-40 ${FOCUS_RING}`}
          aria-label={t('slicer.clear')}
          title={t('slicer.clear')}
          disabled={!anyHidden}
          onPointerDown={(e) => e.stopPropagation()}
          onClick={() => onApply(unique, unique)}
        >
          <EditorIcon name="filterOff" size="sm" />
        </button>
        <button
          type="button"
          class={`hover:bg-bg-subtle inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ${FOCUS_RING}`}
          aria-label={t('slicer.remove')}
          title={t('slicer.remove')}
          onPointerDown={(e) => e.stopPropagation()}
          onClick={onRemove}
        >
          <EditorIcon name="close" size="sm" />
        </button>
      </div>

      {!active ? (
        <div class="sytue-meta-dense px-2 py-3">
          {t(slicer.pivot ? 'slicer.pivotInactive' : 'slicer.inactive')}
        </div>
      ) : entries.length === 0 ? (
        <div class="sytue-meta-dense px-2 py-3">{t('slicer.noValues')}</div>
      ) : (
        <>
          <ul
            role="listbox"
            aria-label={t('slicer.values.label', { field })}
            aria-multiselectable={true}
            class="max-h-56 overflow-auto p-1"
          >
            {entries.map((en) => {
              const isSel = !en.hidden;
              const label = en.label ?? (en.value === '' ? t('slicer.blanks') : en.value);
              return (
                <li key={en.value}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={isSel}
                    class={`flex w-full items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-left text-xs ${FOCUS_RING} ${
                      isSel ? 'bg-selected text-text' : 'text-text-subtle hover:bg-bg-subtle'
                    }`}
                    title={label}
                    onClick={() => toggle(en.value)}
                  >
                    <span class="min-w-0 flex-1 truncate">{label}</span>
                    {isSel && <EditorIcon name="check" size="sm" />}
                  </button>
                </li>
              );
            })}
          </ul>
          {/* Estado del filtro: n de m seleccionados (m = valores únicos). */}
          <div class="border-border sytue-meta-dense border-t px-2 py-1">
            {t('slicer.count', { selected: selected.length, total: entries.length })}
          </div>
        </>
      )}
    </section>
  );
}
