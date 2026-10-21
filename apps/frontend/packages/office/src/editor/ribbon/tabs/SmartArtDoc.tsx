/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «SmartArt» de la pestaña Insertar (solo kind document). Un único botón
 * abre un panel sobrio que, según el contexto, INSERTA un diagrama (eligiendo
 * disposición) o EDITA el que está bajo el caret. El diagrama vive como un
 * bloque `<figure data-smartart>` en el HTML del folio —que ES el modelo (§6.5)—
 * con `data-layout`/`data-items` como fuente de verdad y el SVG horneado; toda la
 * lógica pura (serializado, geometría reutilizada de slides, volcado a SVG) está
 * en ../../smartart. Mismo patrón que notas/leyendas/índice: mutación DOM directa
 * sobre getEditorRoot() que emite `input` para el autosave.
 *
 * ponytail: alta/edición DOM directa fuera del undo nativo (como el resto de
 * generadores de Referencias); historial propio con la migración a modelo Range.
 */
import type { JSX } from 'preact';
import { useMobileBack } from '@sytue/ui';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import { useT } from '../../context';
import type { DocsT } from '../../../i18n/index';
import { useEffect, useRef, useState } from 'preact/hooks';
import type { SmartArtItem, SmartArtLayout } from '../../types';
import { getEditorRoot } from '../../canvas/exec';
import { EditorIcon } from '../../icons';
import { POPOVER_SHELL, RibbonGroup, RibbonSelect, useBloqueado } from '../primitives';
import { SMARTART_LAYOUTS } from '../../slides/SlidesCanvas';
import {
  DEFAULT_SMARTART_ITEMS,
  parseSmartArtItems,
  sanitizeSmartArtLayout,
  serializeSmartArtItems,
  smartArtFigureHtml,
  smartArtSvg,
} from '../../smartart';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

const TRIGGER_CLASS =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

const ROW_BTN = `inline-flex h-6 w-6 items-center justify-center rounded-sm text-text-muted hover:bg-selected disabled:opacity-30 ${FOCUS_RING}`;

/** Opciones del select de disposición (reusa la fuente única de slides). */
// Las opciones se construyen con la `t` activa, no a nivel de módulo: un array
// creado al importar se congelaría con el idioma de la primera carga.
const layoutOptions = (t: DocsT) =>
  SMARTART_LAYOUTS.map((l) => ({ value: l.id, label: t(l.labelKey) }));

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));

/* ---------- localización de la figura activa ---------- */

/**
 * `<figure data-smartart>` bajo el caret: dentro de la figura (caret en ella) o
 * adyacente al bloque atómico (contenteditable=false → el caret cae en el folio
 * apuntando al hijo). null si el caret no vive en el folio o no hay figura.
 */
const figureFromSelection = (root: HTMLElement): HTMLElement | null => {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (!root.contains(range.commonAncestorContainer)) return null;
  const el =
    range.startContainer instanceof Element
      ? range.startContainer
      : range.startContainer.parentElement;
  const inside = el?.closest<HTMLElement>('figure[data-smartart]') ?? null;
  if (inside && root.contains(inside)) return inside;
  if (range.startContainer instanceof Element) {
    const kids = range.startContainer.childNodes;
    for (const idx of [range.startOffset, range.startOffset - 1]) {
      const cand = kids[idx];
      if (cand instanceof HTMLElement && cand.matches('figure[data-smartart]')) return cand;
    }
  }
  return null;
};

/** Range vivo clonado si cae en el folio (para insertar donde estaba el caret). */
const captureRange = (root: HTMLElement): Range | null => {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  return root.contains(range.commonAncestorContainer) ? range.cloneRange() : null;
};

/* ---------- el grupo ---------- */

/** Marca temporal para reencontrar la figura recién insertada en el DOM. Solo se llama
 *  desde el manejador de «insertar», nunca en el render. */
const idTemporal = (): string => `sa-${Math.random().toString(36).slice(2, 10)}`;

export function SmartArtDocGroup(): JSX.Element {
  const bloqueado = useBloqueado('smartart');
  const t = useT();
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [editing, setEditing] = useState(false);
  const [layout, setLayout] = useState<SmartArtLayout>('list');
  const [items, setItems] = useState<SmartArtItem[]>([]);
  const rootRef = useRef<HTMLDivElement>(null);
  const figRef = useRef<HTMLElement | null>(null);
  const capturedRef = useRef<Range | null>(null);

  // Cierre con Escape y clic fuera (patrón §5).
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  /** Re-hornea la figura activa (data-* fuente de verdad + SVG) y emite input. */
  const rebake = (fig: HTMLElement, l: SmartArtLayout, its: SmartArtItem[]): void => {
    const root = getEditorRoot();
    if (!root || !root.contains(fig)) return;
    fig.setAttribute('data-layout', l); // setAttribute escapa el valor (anti-XSS)
    fig.setAttribute('data-items', serializeSmartArtItems(its));
    fig.innerHTML = smartArtSvg(l, its);
    root.dispatchEvent(new Event('input', { bubbles: true }));
  };

  /** Aplica un cambio: refleja en el estado del panel y en la figura activa. */
  const update = (l: SmartArtLayout, its: SmartArtItem[]): void => {
    setLayout(l);
    setItems(its);
    if (figRef.current) rebake(figRef.current, l, its);
  };

  const openPanel = (): void => {
    if (open) {
      setOpen(false);
      return;
    }
    const root = getEditorRoot();
    const fig = root ? figureFromSelection(root) : null;
    capturedRef.current = root ? captureRange(root) : null;
    figRef.current = fig;
    if (fig) {
      setLayout(sanitizeSmartArtLayout(fig.getAttribute('data-layout')));
      setItems(parseSmartArtItems(fig.getAttribute('data-items')));
      setEditing(true);
    } else {
      setEditing(false);
    }
    setOpen(true);
  };

  /** Inserta un diagrama por defecto en el caret y pasa a modo edición. */
  const insert = (l: SmartArtLayout): void => {
    const root = getEditorRoot();
    if (!root) return;
    const its = DEFAULT_SMARTART_ITEMS[l].map((it) => ({ ...it }));
    const tmpId = idTemporal();
    if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
    const sel = document.getSelection();
    const cap = capturedRef.current;
    if (sel && cap && root.contains(cap.commonAncestorContainer)) {
      sel.removeAllRanges();
      sel.addRange(cap);
    }
    try {
      // Párrafo de escape tras la figura (como tabla/salto de página, §3.1): deja
      // dónde poner el caret y seguir escribiendo si el diagrama cierra el folio.
      executeEditingCommand('insertHTML', `${smartArtFigureHtml(l, its, tmpId)}<p><br></p>`);
    } catch {
      return; // navegador sin soporte: no-op
    }
    const fig = root.querySelector<HTMLElement>(`[data-sa-tmp="${tmpId}"]`);
    if (!fig) return;
    fig.removeAttribute('data-sa-tmp');
    figRef.current = fig;
    setLayout(l);
    setItems(its);
    setEditing(true);
    fig.scrollIntoView({ block: 'nearest' });
    root.dispatchEvent(new Event('input', { bubbles: true })); // persiste ya sin data-sa-tmp
  };

  /* ----- edición de elementos (misma semántica que el panel de slides) ----- */

  const setText = (i: number, text: string): void =>
    update(
      layout,
      items.map((it, k) => (k === i ? { ...it, text } : it)),
    );

  const addItem = (): void => {
    const level = items[items.length - 1]?.level ?? 0;
    const item: SmartArtItem =
      layout === 'hierarchy' && level > 0 ? { text: '', level } : { text: '' };
    update(layout, [...items, item]);
  };

  const removeItem = (i: number): void => {
    if (items.length <= 1) return; // siempre queda al menos un elemento
    update(
      layout,
      items.filter((_, k) => k !== i),
    );
  };

  const move = (i: number, dir: -1 | 1): void => {
    const j = i + dir;
    if (j < 0 || j >= items.length) return;
    const next = [...items];
    [next[i], next[j]] = [next[j]!, next[i]!];
    update(layout, next);
  };

  const indent = (i: number, delta: -1 | 1): void => {
    if (i === 0) return; // el primer elemento es la raíz: sin sangría
    const next = items.map((it) => ({ ...it }));
    const prevLevel = next[i - 1]!.level ?? 0;
    const curLevel = next[i]!.level ?? 0;
    const lvl = clamp(curLevel + delta, 0, prevLevel + 1);
    if (lvl === curLevel) return;
    if (lvl === 0) delete next[i]!.level;
    else next[i]!.level = lvl;
    update(layout, next);
  };

  const isHier = layout === 'hierarchy';

  return (
    <RibbonGroup label={t('smartart.title')}>
      <div ref={rootRef} class="relative">
        <button
          type="button"
          class={TRIGGER_CLASS}
          aria-label={t('smartart.insertOrEdit')}
          title={t('smartart.diagram')}
          aria-haspopup="true"
          aria-expanded={open}
          disabled={bloqueado}
          onMouseDown={(e) => e.preventDefault()} // conserva el caret del folio
          onClick={openPanel}
        >
          <EditorIcon name="smartList" />
        </button>

        {open && (
          <div
            role="group"
            aria-label={t('smartart.title')}
            class={`${POPOVER_SHELL} left-0 w-72 p-2`}
          >
            {editing ? (
              <div class="flex flex-col gap-1.5">
                <label class="text-text-muted flex items-center justify-between gap-2 text-xs">
                  {t('smartart.layout')}
                  <RibbonSelect
                    label={t('smartart.layout')}
                    width={140}
                    value={layout}
                    options={layoutOptions(t)}
                    onSelect={(v) => update(sanitizeSmartArtLayout(v), items)}
                  />
                </label>
                <p class="sytue-meta-dense m-0">
                  {t(isHier ? 'smartart.hint.hierarchy' : 'smartart.hint.items')}
                </p>
                <ul class="m-0 flex max-h-56 list-none flex-col gap-0.5 overflow-y-auto p-0">
                  {items.map((it, i) => {
                    const level = it.level ?? 0;
                    const maxLevel = (items[i - 1]?.level ?? 0) + 1;
                    return (
                      <li key={i} class="flex items-center gap-1">
                        {isHier && (
                          <>
                            <button
                              type="button"
                              class={ROW_BTN}
                              aria-label={t('item.outdent', { n: i + 1 })}
                              title={t('indent.decreaseLabel')}
                              disabled={i === 0 || level === 0}
                              onClick={() => indent(i, -1)}
                            >
                              <EditorIcon name="chevronLeft" size="md" />
                            </button>
                            <button
                              type="button"
                              class={ROW_BTN}
                              aria-label={t('item.indent', { n: i + 1 })}
                              title={t('indent.increaseLabel')}
                              disabled={i === 0 || level >= maxLevel}
                              onClick={() => indent(i, 1)}
                            >
                              <EditorIcon name="chevronRight" size="md" />
                            </button>
                          </>
                        )}
                        <input
                          class={`border-border bg-surface text-text placeholder:text-text-subtle h-7 min-w-0 flex-1 rounded-sm border px-1.5 text-xs ${FOCUS_RING}`}
                          style={isHier ? { marginLeft: `${level * 12}px` } : undefined}
                          value={it.text}
                          aria-label={t('item.text', { n: i + 1 })}
                          placeholder={t('slides.smartart.itemText')}
                          onInput={(e) => setText(i, e.currentTarget.value)}
                        />
                        <button
                          type="button"
                          class={ROW_BTN}
                          aria-label={t('item.moveUp', { n: i + 1 })}
                          title={t('common.moveUp')}
                          disabled={i === 0}
                          onClick={() => move(i, -1)}
                        >
                          <EditorIcon name="chevronUp" size="md" />
                        </button>
                        <button
                          type="button"
                          class={ROW_BTN}
                          aria-label={t('item.moveDown', { n: i + 1 })}
                          title={t('common.moveDown')}
                          disabled={i === items.length - 1}
                          onClick={() => move(i, 1)}
                        >
                          <EditorIcon name="chevronDown" size="md" />
                        </button>
                        <button
                          type="button"
                          class={ROW_BTN}
                          aria-label={t('item.remove', { n: i + 1 })}
                          title={t('slides.smartart.removeItem')}
                          disabled={items.length <= 1}
                          onClick={() => removeItem(i)}
                        >
                          <EditorIcon name="close" size="md" />
                        </button>
                      </li>
                    );
                  })}
                </ul>
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 w-max items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`}
                  onClick={addItem}
                >
                  <EditorIcon name="plus" size="sm" /> {t('smartart.addItem')}
                </button>
              </div>
            ) : (
              <div class="flex flex-col gap-1.5">
                <p class="text-text-muted m-0 text-xs">{t('smartart.chooseLayout')}</p>
                <div class="grid grid-cols-2 gap-1">
                  {SMARTART_LAYOUTS.map((l) => (
                    <button
                      key={l.id}
                      type="button"
                      class={`border-border bg-surface text-text hover:bg-bg-subtle flex items-center gap-2 rounded-sm border px-2 py-1.5 text-left text-xs ${FOCUS_RING}`}
                      onClick={() => insert(l.id)}
                    >
                      <EditorIcon name={l.icon} size="md" />
                      {t(l.labelKey)}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </RibbonGroup>
  );
}
