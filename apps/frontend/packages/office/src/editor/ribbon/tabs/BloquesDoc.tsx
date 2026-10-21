/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupos «Formas» y «Gráfico» de la pestaña Insertar (solo kind document). Cada
 * uno abre un panel sobrio que, según el contexto, INSERTA un bloque o EDITA el
 * que está bajo el caret. El bloque vive como un `<figure data-shape>` /
 * `<figure data-chart>` en el HTML del folio —que ES el modelo (§6.5)— con su
 * configuración en `data-*` como fuente de verdad y el SVG horneado; toda la
 * lógica pura (saneado, serializado, geometría reutilizada de slides/sheets,
 * volcado a SVG) vive en ../../docblocks. Mismo patrón que SmartArtDoc: mutación
 * DOM directa sobre getEditorRoot() que emite `input` para el autosave.
 *
 * ponytail: alta/edición DOM directa fuera del undo nativo (como el resto de
 * generadores de bloques); historial propio con la migración a modelo Range.
 * ponytail: redimensionado por campos de tamaño en el panel; arrastre con
 * tiradores = upgrade (reutilizar el overlay de imagen, canvas/ImageOverlay).
 */
import type { JSX } from 'preact';
import { useT } from '../../context';
import { useRef, useState } from 'preact/hooks';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import { getEditorRoot } from '../../canvas/exec';
import { EditorIcon } from '../../icons';
import {
  CONTENT_PALETTE,
  RibbonColorSwatch,
  RibbonGroup,
  RibbonSelect,
  usePopover,
  POPOVER_SHELL,
  FIELD_BASE,
  FIELD_CLASS as RIBBON_FIELD,
  useBloqueado,
} from '../primitives';
import type { SlideChartType } from '../../types';
import {
  chartFigureHtml,
  chartSvg,
  DOC_CHART_TYPES,
  DOC_SHAPES,
  newChartGrid,
  parseChartGrid,
  sanitizeChartType,
  sanitizeShapeConfig,
  sanitizeSize,
  serializeChartGrid,
  shapeFigureHtml,
  shapeSvg,
} from '../../docblocks';
import type { DocShapeConfig } from '../../docblocks';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

const TRIGGER_CLASS =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

const ROW_BTN = `inline-flex h-6 w-6 items-center justify-center rounded-sm text-text-muted hover:bg-selected disabled:opacity-30 ${FOCUS_RING}`;

const FIELD_CLASS = `${RIBBON_FIELD} min-w-0`;

/** Relleno: la paleta de contenido + «sin relleno» (transparent). */
const FILL_PALETTE = [...CONTENT_PALETTE, 'transparent'] as const;

/* ---------- localización de la figura activa (mismo criterio que SmartArtDoc) ---------- */

/** `<figure>` que cumple `selector` bajo el caret: dentro de ella (caret dentro) o
 *  adyacente al bloque atómico (contenteditable=false → el caret cae en el folio
 *  apuntando al hijo). null si el caret no vive en el folio o no hay figura. */
export const figureFromSelection = (root: HTMLElement, selector: string): HTMLElement | null => {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (!root.contains(range.commonAncestorContainer)) return null;
  const el =
    range.startContainer instanceof Element
      ? range.startContainer
      : range.startContainer.parentElement;
  const inside = el?.closest<HTMLElement>(selector) ?? null;
  if (inside && root.contains(inside)) return inside;
  if (range.startContainer instanceof Element) {
    const kids = range.startContainer.childNodes;
    for (const idx of [range.startOffset, range.startOffset - 1]) {
      const cand = kids[idx];
      if (cand instanceof HTMLElement && cand.matches(selector)) return cand;
    }
  }
  return null;
};

/** Range vivo clonado si cae en el folio (para insertar donde estaba el caret). */
export const captureRange = (root: HTMLElement): Range | null => {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  return root.contains(range.commonAncestorContainer) ? range.cloneRange() : null;
};

export const tmpId = (): string => `blk-${Math.random().toString(36).slice(2, 10)}`;

/**
 * Inserta el HTML de un bloque en el caret capturado + un párrafo de escape (como
 * tabla/SmartArt, §3.1) y devuelve la figura enganchada (sin la marca temporal).
 * Emite `input` para el autosave. null si no se pudo insertar.
 */
export const insertBlock = (
  captured: Range | null,
  html: string,
  id: string,
): HTMLElement | null => {
  const root = getEditorRoot();
  if (!root) return null;
  if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
  const sel = document.getSelection();
  if (sel && captured && root.contains(captured.commonAncestorContainer)) {
    sel.removeAllRanges();
    sel.addRange(captured);
  }
  try {
    executeEditingCommand('insertHTML', `${html}<p><br></p>`);
  } catch {
    return null; // navegador sin soporte: no-op
  }
  const fig = root.querySelector<HTMLElement>(`[data-blk-tmp="${id}"]`);
  if (!fig) return null;
  fig.removeAttribute('data-blk-tmp');
  fig.scrollIntoView({ block: 'nearest' });
  root.dispatchEvent(new Event('input', { bubbles: true })); // persiste ya sin data-blk-tmp
  return fig;
};

/* ---------- Grupo «Formas» ---------- */

export function FormasDocGroup(): JSX.Element {
  const bloqueado = useBloqueado('figure');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [editing, setEditing] = useState(false);
  const [cfg, setCfg] = useState<DocShapeConfig>(() => sanitizeShapeConfig({ shape: 'rect' }));
  const figRef = useRef<HTMLElement | null>(null);
  const capturedRef = useRef<Range | null>(null);

  /** Re-hornea la figura activa (data-* fuente de verdad + SVG + ancho) y emite input. */
  const rebake = (fig: HTMLElement, next: DocShapeConfig): void => {
    const root = getEditorRoot();
    if (!root || !root.contains(fig)) return;
    fig.setAttribute('data-shape-type', next.shape);
    fig.setAttribute('data-w', String(next.w));
    fig.setAttribute('data-h', String(next.h));
    fig.setAttribute('data-fill', next.fill);
    fig.setAttribute('data-stroke', next.stroke);
    fig.setAttribute('data-text', next.text);
    fig.style.width = `${next.w}px`;
    fig.innerHTML = shapeSvg(next);
    root.dispatchEvent(new Event('input', { bubbles: true }));
  };

  /** Aplica un parche: refleja en el estado del panel y en la figura activa. */
  const update = (patch: Partial<DocShapeConfig>): void => {
    const next = { ...cfg, ...patch };
    setCfg(next);
    if (figRef.current) rebake(figRef.current, next);
  };

  const openPanel = (): void => {
    if (open) {
      setOpen(false);
      return;
    }
    const root = getEditorRoot();
    const fig = root ? figureFromSelection(root, 'figure[data-shape]') : null;
    capturedRef.current = root ? captureRange(root) : null;
    figRef.current = fig;
    if (fig) {
      setCfg(
        sanitizeShapeConfig({
          shape: fig.getAttribute('data-shape-type'),
          w: fig.getAttribute('data-w'),
          h: fig.getAttribute('data-h'),
          fill: fig.getAttribute('data-fill'),
          stroke: fig.getAttribute('data-stroke'),
          text: fig.getAttribute('data-text'),
        }),
      );
      setEditing(true);
    } else {
      setEditing(false);
    }
    setOpen(true);
  };

  /** Inserta una forma por defecto en el caret y pasa a modo edición. */
  const insert = (shape: DocShapeConfig['shape']): void => {
    const next = sanitizeShapeConfig({ shape });
    const id = tmpId();
    const fig = insertBlock(capturedRef.current, shapeFigureHtml(next, id), id);
    if (!fig) return;
    figRef.current = fig;
    setCfg(next);
    setEditing(true);
  };

  const onShape = (shape: DocShapeConfig['shape']): void =>
    editing ? update({ shape }) : insert(shape);

  const textShape = cfg.shape !== 'line' && cfg.shape !== 'arrow';

  return (
    <RibbonGroup label={t('blocks.shapes')}>
      <div ref={rootRef} class="relative">
        <button
          type="button"
          class={TRIGGER_CLASS}
          aria-label={t('blocks.shape.insert')}
          title={t('blocks.shapes')}
          aria-haspopup="true"
          aria-expanded={open}
          disabled={bloqueado}
          onMouseDown={(e) => e.preventDefault()} // conserva el caret del folio
          onClick={openPanel}
        >
          <EditorIcon name="rect" />
        </button>

        {open && (
          <div
            role="group"
            aria-label={t('blocks.shapes')}
            class={`${POPOVER_SHELL} left-0 w-64 p-2`}
          >
            <p class="text-text-muted m-0 mb-1 text-xs">
              {editing ? t('shape.editHint') : t('shape.pickHint')}
            </p>
            <div class="grid grid-cols-6 gap-0.5">
              {DOC_SHAPES.map((s) => {
                const active = editing && cfg.shape === s.id;
                return (
                  <button
                    key={s.id}
                    type="button"
                    class={
                      'inline-flex h-8 w-8 items-center justify-center rounded-sm ' +
                      (active
                        ? 'bg-selected text-text'
                        : 'text-text-muted hover:bg-bg-subtle hover:text-text') +
                      ' ' +
                      FOCUS_RING
                    }
                    aria-label={
                      editing
                        ? t('shape.changeTo', { kind: t(s.labelKey).toLowerCase() })
                        : t('shape.insert', { kind: t(s.labelKey).toLowerCase() })
                    }
                    title={t(s.labelKey)}
                    aria-pressed={active}
                    onClick={() => onShape(s.id)}
                  >
                    <EditorIcon name={s.id} />
                  </button>
                );
              })}
            </div>

            {editing && (
              <div class="mt-2 flex flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <span class="text-text-muted text-2xs w-12">Estilo</span>
                  <RibbonColorSwatch
                    icon="highlight"
                    label={t('shape.fill')}
                    color={cfg.fill}
                    palette={FILL_PALETTE}
                    onPick={(c) => update({ fill: c })}
                  />
                  <RibbonColorSwatch
                    icon="textColor"
                    label={t('border.color.label')}
                    color={cfg.stroke}
                    palette={CONTENT_PALETTE}
                    onPick={(c) => update({ stroke: c })}
                  />
                </div>
                <div class="flex items-center gap-1.5">
                  <span class="text-text-muted text-2xs w-12">{t('common.size')}</span>
                  <input
                    class={`${FIELD_CLASS} w-16`}
                    type="number"
                    min={24}
                    max={1600}
                    value={cfg.w}
                    aria-label={t('blocks.shape.width')}
                    onChange={(e) => update({ w: sanitizeSize(e.currentTarget.value, cfg.w) })}
                  />
                  <span class="sytue-meta-dense" aria-hidden="true">
                    ×
                  </span>
                  <input
                    class={`${FIELD_CLASS} w-16`}
                    type="number"
                    min={24}
                    max={1600}
                    value={cfg.h}
                    aria-label={t('blocks.shape.height')}
                    onChange={(e) => update({ h: sanitizeSize(e.currentTarget.value, cfg.h) })}
                  />
                </div>
                {textShape && (
                  <div class="flex items-center gap-1.5">
                    <span class="text-text-muted text-2xs w-12">Texto</span>
                    <input
                      class={`${FIELD_CLASS} flex-1`}
                      type="text"
                      value={cfg.text}
                      placeholder={t('blocks.shape.innerHint')}
                      aria-label={t('blocks.shape.innerText')}
                      onInput={(e) => update({ text: e.currentTarget.value })}
                    />
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </RibbonGroup>
  );
}

/* ---------- Grupo «Gráfico» ---------- */

const CHART_MAX_ROWS = 60;
const CHART_MAX_COLS = 12;
const CHART_TYPE_OPTIONS = DOC_CHART_TYPES.map((t) => ({ value: t.id, label: t.label }));

export function GraficoDocGroup(): JSX.Element {
  const bloqueado = useBloqueado('grafico');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [editing, setEditing] = useState(false);
  const [type, setType] = useState<SlideChartType>('bar');
  const [grid, setGrid] = useState<string[][]>(() => newChartGrid());
  const figRef = useRef<HTMLElement | null>(null);
  const capturedRef = useRef<Range | null>(null);

  /** Re-hornea la figura activa (data-* fuente de verdad + SVG) y emite input. */
  const rebake = (fig: HTMLElement, t: SlideChartType, g: string[][]): void => {
    const root = getEditorRoot();
    if (!root || !root.contains(fig)) return;
    fig.setAttribute('data-chart-type', t);
    fig.setAttribute('data-chart-grid', serializeChartGrid(g));
    fig.innerHTML = chartSvg(t, g);
    root.dispatchEvent(new Event('input', { bubbles: true }));
  };

  const update = (t: SlideChartType, g: string[][]): void => {
    setType(t);
    setGrid(g);
    if (figRef.current) rebake(figRef.current, t, g);
  };

  const openPanel = (): void => {
    if (open) {
      setOpen(false);
      return;
    }
    const root = getEditorRoot();
    const fig = root ? figureFromSelection(root, 'figure[data-chart]') : null;
    capturedRef.current = root ? captureRange(root) : null;
    figRef.current = fig;
    if (fig) {
      setType(sanitizeChartType(fig.getAttribute('data-chart-type')));
      setGrid(parseChartGrid(fig.getAttribute('data-chart-grid')));
      setEditing(true);
    } else {
      setEditing(false);
    }
    setOpen(true);
  };

  /** Inserta un gráfico por defecto en el caret y pasa a modo edición. */
  const insert = (t: SlideChartType): void => {
    const g = newChartGrid();
    const id = tmpId();
    const fig = insertBlock(capturedRef.current, chartFigureHtml(t, g, id), id);
    if (!fig) return;
    figRef.current = fig;
    setType(t);
    setGrid(g);
    setEditing(true);
  };

  const setCell = (r: number, c: number, val: string): void =>
    update(
      type,
      grid.map((row, ri) => (ri === r ? row.map((cell, ci) => (ci === c ? val : cell)) : row)),
    );

  const cols = grid[0]?.length ?? 1;
  const addRow = (): void => {
    if (grid.length >= CHART_MAX_ROWS) return;
    update(type, [...grid, Array.from({ length: cols }, () => '')]);
  };
  const addCol = (): void => {
    if (cols >= CHART_MAX_COLS) return;
    update(
      type,
      grid.map((r) => [...r, '']),
    );
  };
  const delRow = (): void => {
    if (grid.length <= 2) return; // deja cabecera + una fila de datos
    update(type, grid.slice(0, -1));
  };
  const delCol = (): void => {
    if (cols <= 2) return; // deja rótulos + una serie
    update(
      type,
      grid.map((r) => r.slice(0, -1)),
    );
  };

  return (
    <RibbonGroup label={t('blocks.chart')}>
      <div ref={rootRef} class="relative">
        <button
          type="button"
          class={TRIGGER_CLASS}
          aria-label={t('blocks.chart.insert')}
          title={t('blocks.chart')}
          aria-haspopup="true"
          aria-expanded={open}
          disabled={bloqueado}
          onMouseDown={(e) => e.preventDefault()}
          onClick={openPanel}
        >
          <EditorIcon name="chart" />
        </button>

        {open &&
          (editing ? (
            <div
              role="group"
              aria-label={t('blocks.chart.edit')}
              class={`${POPOVER_SHELL} left-0 w-80 max-w-[calc(100vw-16px)] p-2`}
            >
              <label class="text-text-muted mb-1.5 flex items-center justify-between gap-2 text-xs">
                Tipo
                <RibbonSelect
                  label={t('chart.type')}
                  width={140}
                  value={type}
                  options={CHART_TYPE_OPTIONS}
                  onSelect={(v) => update(sanitizeChartType(v), grid)}
                />
              </label>
              <p class="sytue-meta-dense m-0 mb-1">{t('chart.data.hint')}</p>
              <div class="max-h-56 overflow-auto">
                <table class="border-collapse">
                  <tbody>
                    {grid.map((row, r) => (
                      <tr key={r}>
                        {row.map((cell, c) => (
                          <td key={c} class="p-0.5">
                            <input
                              class={`${FIELD_BASE} h-6 w-16`}
                              type="text"
                              value={cell}
                              aria-label={t('table.rowCol', { row: r + 1, col: c + 1 })}
                              onInput={(e) => setCell(r, c, e.currentTarget.value)}
                            />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div class="mt-1.5 flex items-center gap-1.5">
                <span class="text-text-muted text-2xs mr-auto">{t('common.rows')}</span>
                <button
                  type="button"
                  class={ROW_BTN}
                  aria-label={t('blocks.row.remove')}
                  title={t('blocks.row.remove')}
                  disabled={grid.length <= 2}
                  onClick={delRow}
                >
                  <EditorIcon name="close" size="md" />
                </button>
                <button
                  type="button"
                  class={ROW_BTN}
                  aria-label={t('blocks.row.add')}
                  title={t('blocks.row.add')}
                  disabled={grid.length >= CHART_MAX_ROWS}
                  onClick={addRow}
                >
                  <EditorIcon name="plus" size="md" />
                </button>
                <span class="text-text-muted text-2xs ml-1.5">{t('layout.columns')}</span>
                <button
                  type="button"
                  class={ROW_BTN}
                  aria-label={t('blocks.col.remove')}
                  title={t('blocks.col.remove')}
                  disabled={cols <= 2}
                  onClick={delCol}
                >
                  <EditorIcon name="close" size="md" />
                </button>
                <button
                  type="button"
                  class={ROW_BTN}
                  aria-label={t('blocks.col.add')}
                  title={t('blocks.col.add')}
                  disabled={cols >= CHART_MAX_COLS}
                  onClick={addCol}
                >
                  <EditorIcon name="plus" size="md" />
                </button>
              </div>
            </div>
          ) : (
            <div
              role="group"
              aria-label={t('blocks.chart')}
              class={`${POPOVER_SHELL} left-0 w-52 p-2`}
            >
              <p class="text-text-muted m-0 mb-1.5 text-xs">{t('blocks.shape.chooseType')}</p>
              <div class="flex flex-col gap-1">
                {DOC_CHART_TYPES.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    class={`border-border bg-surface text-text hover:bg-bg-subtle flex items-center gap-2 rounded-sm border px-2 py-1.5 text-left text-xs ${FOCUS_RING}`}
                    onClick={() => insert(t.id)}
                  >
                    <EditorIcon name="chart" size="md" />
                    {t.label}
                  </button>
                ))}
              </div>
            </div>
          ))}
      </div>
    </RibbonGroup>
  );
}
