/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Overlay contextual de IMAGEN (kind document): al seleccionar un `<img>` del
 * folio muestra un marco con TIRADORES de esquina (redimensión con proporción) y
 * una mini-toolbar flotante con ajuste de texto, alineación, tamaño exacto,
 * recorte y texto alternativo. La lógica pura (style/recorte/float) vive en
 * ../images.ts y el pegamento DOM en ./imageDom.ts; aquí solo la interacción.
 *
 * Vive FUERA del wrapper con transform scale del lienzo (contenedor `fixed` a
 * viewport, como el menú de campos de PageCanvas): un ancestro transformado sería
 * el bloque contenedor de un `fixed` y descuadraría getBoundingClientRect. Las
 * coordenadas de arrastre se dividen por el zoom para pasar de píxeles de pantalla
 * a píxeles de maqueta (los que persisten en el modelo).
 */
import type { JSX, TargetedMouseEvent } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import type { EditorContext } from '../types';
import type { DocsKey } from '../../i18n';
import { Toolbar, useFitInViewport, useMobileBack } from '@sytue/ui';
import { EditorIcon } from '../icons';
import {
  IMAGE_WRAPS,
  isAbsoluteWrap,
  isFloatWrap,
  type ImageAlign,
  type ImageWrap,
} from '../images';
import { getEditorRoot } from './exec';
import { margenesPagina } from '../pagedecor';
import { applyImage, readModel, removeImage, setSelectedImage, writeModel } from './imageDom';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';
const SEL_CLASS =
  'border-border bg-surface text-text h-7 rounded-sm border px-1 text-xs ' + FOCUS_RING;
const NUM_CLASS =
  'border-border bg-surface text-text h-7 w-14 rounded-sm border px-1 text-xs tabular-nums ' +
  FOCUS_RING;
const BTN_CLASS =
  'text-text-muted hover:bg-bg-subtle hover:text-text inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
  'aria-pressed:bg-selected aria-pressed:text-text ' +
  FOCUS_RING;

/** Lado mínimo de una imagen redimensionada (px de maqueta). */
const MIN_SIZE = 24;

/** Esquinas del marco: fracción de posición (0/1) y signo con que crece el ancho. */
const CORNERS = [
  { id: 'nw', cx: 0, cy: 0, dir: -1, cursor: 'nwse-resize' },
  { id: 'ne', cx: 1, cy: 0, dir: 1, cursor: 'nesw-resize' },
  { id: 'sw', cx: 0, cy: 1, dir: -1, cursor: 'nesw-resize' },
  { id: 'se', cx: 1, cy: 1, dir: 1, cursor: 'nwse-resize' },
] as const;

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
const round2 = (n: number): number => Math.round(n * 100) / 100;

/** Opciones de alineación según el ajuste (float: i/d · bloque: i/c/d). Devuelve
 *  CLAVES: el rótulo lo resuelve la `t` del componente en cada render. */
const alignOptions = (wrap: ImageWrap): { value: ImageAlign; labelKey: DocsKey }[] =>
  isFloatWrap(wrap)
    ? [
        { value: 'left', labelKey: 'image.align.left' },
        { value: 'right', labelKey: 'image.align.right' },
      ]
    : [
        { value: 'left', labelKey: 'image.align.left' },
        { value: 'center', labelKey: 'image.align.center' },
        { value: 'right', labelKey: 'image.align.right' },
      ];

/** Lado del recorte → clave de su etiqueta accesible. */
const CROP_SIDE_KEY: Record<'t' | 'r' | 'b' | 'l', DocsKey> = {
  t: 'image.crop.top',
  r: 'image.crop.right',
  b: 'image.crop.bottom',
  l: 'image.crop.left',
};

export function ImageOverlay({ ctx }: { ctx: EditorContext }): JSX.Element | null {
  const t = ctx.t;
  const [sel, setSel] = useState<HTMLImageElement | null>(null);
  // Contador para recolocar el overlay al hacer scroll/redimensionar/editar.
  // OJO: sus actualizadores son `(n) => n + 1`, NUNCA `(t) => t + 1`: eso
  // ensombrecería la `t` de traducción de arriba (trampa repetida en este repo).
  const [, setTick] = useState(0);
  const [cropOpen, setCropOpen] = useState(false);
  const [altOpen, setAltOpen] = useState(false);
  const toolbarRef = useRef<HTMLDivElement>(null);
  useFitInViewport(toolbarRef, sel !== null, { vertical: true });

  const zoom = ctx.ui.zoom / 100;

  const select = (img: HTMLImageElement): void => {
    setSel(img);
    setSelectedImage(img);
    // Empuja un selectionchange para que el ribbon refleje imageSelected (§3.3).
    document.dispatchEvent(new Event('selectionchange'));
  };
  const deselect = (): void => {
    setSel(null);
    setSelectedImage(null);
    setCropOpen(false);
    setAltOpen(false);
    document.dispatchEvent(new Event('selectionchange'));
  };
  useMobileBack(sel !== null, deselect);

  /** Ancho útil del folio (px de maqueta): tope de la redimensión. */
  const maxWidth = (): number => {
    const page = getEditorRoot();
    if (!page) return 2000;
    const [, derecho, , izquierdo] = margenesPagina(ctx.pageSetup);
    return Math.max(MIN_SIZE, page.clientWidth - derecho - izquierdo);
  };

  /* Selección por clic, arrastre de imágenes absolutas y recolocación. */
  useEffect(() => {
    const onClick = (e: MouseEvent): void => {
      const target = e.target as HTMLElement | null;
      const page = getEditorRoot();
      const img = target?.closest?.('img');
      if (img instanceof HTMLImageElement && page?.contains(img)) {
        select(img);
        return;
      }
      // Clic en otro punto del folio: deseleccionar. Fuera del folio (ribbon,
      // toolbar, cromo): se conserva la selección para poder usar sus controles.
      if (page && target && page.contains(target)) deselect();
    };

    // Arrastre de una imagen anclada (detrás/delante): reposiciona x/y en el folio.
    const onDown = (e: MouseEvent): void => {
      const s = sel;
      if (!s || e.target !== s) return;
      const model = readModel(s);
      if (!isAbsoluteWrap(model.wrap)) return;
      e.preventDefault();
      const sx = e.clientX;
      const sy = e.clientY;
      const ox = model.x ?? 0;
      const oy = model.y ?? 0;
      let last = model;
      const move = (ev: MouseEvent): void => {
        last = {
          ...model,
          x: round2(ox + (ev.clientX - sx) / zoom),
          y: round2(oy + (ev.clientY - sy) / zoom),
        };
        writeModel(s, last, true);
        setTick((n) => n + 1);
      };
      const up = (): void => {
        document.removeEventListener('mousemove', move);
        document.removeEventListener('mouseup', up);
        writeModel(s, last, false);
      };
      document.addEventListener('mousemove', move);
      document.addEventListener('mouseup', up);
    };

    const onReflow = (): void => {
      if (!sel) return; // sin imagen seleccionada no hay overlay que recolocar
      if (!sel.isConnected) deselect();
      else setTick((n) => n + 1);
    };

    document.addEventListener('click', onClick);
    document.addEventListener('mousedown', onDown);
    document.addEventListener('input', onReflow, true);
    window.addEventListener('scroll', onReflow, true);
    window.addEventListener('resize', onReflow);
    return () => {
      document.removeEventListener('click', onClick);
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('input', onReflow, true);
      window.removeEventListener('scroll', onReflow, true);
      window.removeEventListener('resize', onReflow);
    };
  }, [sel, zoom, ctx.pageSetup]);

  /* El contenido se re-aplica al DOM (carga/versión/remoto): la imagen previa ya
   * no existe. Escape también deselecciona. */
  useEffect(() => {
    deselect();
  }, [ctx.contentEpoch]);

  useEffect(() => {
    if (!sel) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') deselect();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [sel]);

  if (!sel) return null;
  const rect = sel.getBoundingClientRect();
  const model = readModel(sel);
  const w = Math.round(model.w ?? sel.clientWidth) || MIN_SIZE;
  const h = Math.round(model.h ?? sel.clientHeight) || MIN_SIZE;

  /** Redimensión por tirador de esquina (mantiene la proporción desde el ancho). */
  const onResizeDown = (e: TargetedMouseEvent<HTMLButtonElement>, dir: number): void => {
    e.preventDefault();
    e.stopPropagation();
    const model0 = readModel(sel);
    const startW = model0.w ?? (Math.round(sel.clientWidth) || MIN_SIZE);
    const startH = model0.h ?? (Math.round(sel.clientHeight) || MIN_SIZE);
    const aspect = startW / startH || 1;
    const sx = e.clientX;
    const cap = maxWidth();
    let last = model0;
    const move = (ev: MouseEvent): void => {
      const nw = clamp(startW + ((ev.clientX - sx) / zoom) * dir, MIN_SIZE, cap);
      last = { ...model0, w: round2(nw), h: round2(nw / aspect) };
      writeModel(sel, last, true);
      setTick((n) => n + 1);
    };
    const up = (): void => {
      document.removeEventListener('mousemove', move);
      document.removeEventListener('mouseup', up);
      writeModel(sel, last, false);
    };
    document.addEventListener('mousemove', move);
    document.addEventListener('mouseup', up);
  };

  const setWidth = (value: string): void => {
    const nw = clamp(Number(value) || 0, MIN_SIZE, maxWidth());
    const aspect = w / h || 1;
    applyImage(sel, { w: round2(nw), h: round2(nw / aspect) });
    setTick((n) => n + 1);
  };
  const setHeight = (value: string): void => {
    const nh = clamp(Number(value) || 0, MIN_SIZE, 4000);
    const aspect = w / h || 1;
    applyImage(sel, { h: round2(nh), w: round2(nh * aspect) });
    setTick((n) => n + 1);
  };

  const setCropSide = (side: 't' | 'r' | 'b' | 'l', pct: string): void => {
    const frac = clamp(Number(pct) || 0, 0, 90) / 100;
    applyImage(sel, { crop: { ...model.crop, [side]: frac } });
    setTick((n) => n + 1);
  };

  // Toolbar: encima de la imagen si hay hueco; si no, debajo. Acotada al viewport.
  const below = rect.top < 140;
  const barTop = below ? rect.bottom + 10 : rect.top - 10;
  const barLeft = clamp(rect.left, 8, Math.max(8, window.innerWidth - 340));
  const wrapLabel = (id: ImageWrap): string => {
    const wrap = IMAGE_WRAPS.find((x) => x.id === id);
    return wrap ? t(wrap.labelKey) : id;
  };
  const showAlign = isFloatWrap(model.wrap) || model.wrap === 'topBottom';

  return (
    // Capa a pantalla completa sin capturar el ratón; cada hijo lo reactiva.
    <div
      data-image-toolbar
      class="fixed inset-0 z-[var(--z-popover)]"
      style={{ pointerEvents: 'none' }}
    >
      {/* Marco de selección (azul de selección: la imagen está sobre el papel, blanco en los dos temas). */}
      <div
        aria-hidden="true"
        class="absolute border border-[var(--editor-select)]"
        style={{
          left: `${rect.left}px`,
          top: `${rect.top}px`,
          width: `${rect.width}px`,
          height: `${rect.height}px`,
        }}
      />
      {/* Tiradores de esquina. */}
      {CORNERS.map((c) => (
        <button
          key={c.id}
          type="button"
          tabIndex={-1}
          aria-hidden="true"
          class="absolute h-2.5 w-2.5 rounded-[2px] border border-[var(--editor-select)] bg-[var(--editor-page-bg)]"
          style={{
            left: `${rect.left + c.cx * rect.width}px`,
            top: `${rect.top + c.cy * rect.height}px`,
            transform: 'translate(-50%, -50%)',
            cursor: c.cursor,
            pointerEvents: 'auto',
          }}
          onMouseDown={(e) => onResizeDown(e, c.dir)}
        />
      ))}

      {/* Mini-toolbar contextual. */}
      <Toolbar
        innerRef={toolbarRef}
        aria-label={t('image.toolbar')}
        class="sytue-popover-surface border-border absolute flex flex-col gap-1 rounded-lg border p-1 shadow-[var(--shadow-md)]"
        style={{
          left: `${barLeft}px`,
          top: `${barTop}px`,
          transform: below ? undefined : 'translateY(-100%)',
          pointerEvents: 'auto',
        }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div class="flex items-center gap-1">
          <select
            class={SEL_CLASS}
            aria-label={t('image.wrap')}
            title={t('image.wrap')}
            value={model.wrap}
            onChange={(e) => {
              applyImage(sel, { wrap: e.currentTarget.value as ImageWrap });
              setTick((n) => n + 1);
            }}
          >
            {IMAGE_WRAPS.map((wpt) => (
              <option key={wpt.id} value={wpt.id}>
                {wrapLabel(wpt.id)}
              </option>
            ))}
          </select>
          {showAlign && (
            <select
              class={SEL_CLASS}
              aria-label={t('image.align')}
              title={t('image.align')}
              value={model.align}
              onChange={(e) => {
                applyImage(sel, { align: e.currentTarget.value as ImageAlign });
                setTick((n) => n + 1);
              }}
            >
              {alignOptions(model.wrap).map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.labelKey)}
                </option>
              ))}
            </select>
          )}
          <label class="text-text-muted flex items-center gap-1 text-xs">
            {t('image.width.short')}
            <input
              class={NUM_CLASS}
              type="number"
              min={MIN_SIZE}
              aria-label={t('image.width')}
              value={w}
              onChange={(e) => setWidth(e.currentTarget.value)}
            />
          </label>
          <label class="text-text-muted flex items-center gap-1 text-xs">
            {t('image.height.short')}
            <input
              class={NUM_CLASS}
              type="number"
              min={MIN_SIZE}
              aria-label={t('image.height')}
              value={h}
              onChange={(e) => setHeight(e.currentTarget.value)}
            />
          </label>
          <button
            type="button"
            class={BTN_CLASS}
            aria-label={t('image.crop')}
            title={t('image.crop')}
            aria-pressed={cropOpen}
            onClick={() => setCropOpen((v) => !v)}
          >
            <EditorIcon name="crop" />
          </button>
          <button
            type="button"
            class={
              'text-text-muted hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text inline-flex h-7 items-center rounded-sm px-1.5 text-xs ' +
              FOCUS_RING
            }
            aria-label={t('image.alt')}
            title={t('image.alt')}
            aria-pressed={altOpen}
            onClick={() => setAltOpen((v) => !v)}
          >
            Alt
          </button>
          <button
            type="button"
            class={BTN_CLASS}
            aria-label={t('image.remove')}
            title={t('image.remove')}
            onClick={() => {
              removeImage(sel);
              deselect();
            }}
          >
            <EditorIcon name="trash" />
          </button>
        </div>

        {cropOpen && (
          <div class="flex items-center gap-1">
            <span class="sytue-meta-dense w-12">{t('image.crop.percent')}</span>
            {(['t', 'r', 'b', 'l'] as const).map((side) => (
              <input
                key={side}
                class={NUM_CLASS + ' w-12'}
                type="number"
                min={0}
                max={90}
                aria-label={t(CROP_SIDE_KEY[side])}
                value={Math.round((model.crop[side] ?? 0) * 100)}
                onChange={(e) => setCropSide(side, e.currentTarget.value)}
              />
            ))}
            <button
              type="button"
              class={
                'text-text-muted hover:bg-bg-subtle hover:text-text inline-flex h-7 items-center rounded-sm px-1.5 text-xs ' +
                FOCUS_RING
              }
              onClick={() => {
                applyImage(sel, { crop: { t: 0, r: 0, b: 0, l: 0 } });
                setTick((n) => n + 1);
              }}
            >
              {t('image.crop.reset')}
            </button>
          </div>
        )}

        {altOpen && (
          <input
            class={
              'border-border bg-surface text-text placeholder:text-text-subtle h-7 w-full rounded-sm border px-1.5 text-xs ' +
              FOCUS_RING
            }
            type="text"
            aria-label={t('image.alt.label')}
            placeholder={t('image.alt.placeholder')}
            value={model.alt}
            onChange={(e) => {
              applyImage(sel, { alt: e.currentTarget.value });
              setTick((n) => n + 1);
            }}
          />
        )}
      </Toolbar>
    </div>
  );
}
