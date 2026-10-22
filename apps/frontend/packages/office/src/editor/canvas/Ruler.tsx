/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ruler — regla horizontal de 20 px del lienzo (BLUEPRINT.md §6.5, tarea
 * B7-regla): números en cada centímetro con media marca a 0,5 cm, zonas de
 * margen sombreadas y tres indicadores triangulares ARRASTRABLES de sangría
 * —primera línea, izquierda/francesa y derecha— que aplican text-indent y
 * margin-left/right al bloque de la selección actual. Se dibuja en px de
 * maqueta (zoom 100): PageCanvas la monta dentro del wrapper escalado y el
 * transform la escala junto al folio; los arrastres se convierten a px de
 * maqueta midiendo el ancho REAL del propio nodo, así la sincronía con el
 * zoom es automática se monte donde se monte.
 *
 * Decisiones documentadas (la tarea B7-regla exige arrastre y prevalece sobre
 * el ponytail «marcadores estáticos» de §6.5):
 * - El contrato §3 de exec no tiene comando de sangría en px (indent/outdent
 *   van a saltos fijos de execCommand), así que la regla localiza el bloque
 *   vía getEditorRoot() + Selection, escribe los estilos inline directamente
 *   y emite `input` sobre el root —el mismo camino manual que usan los
 *   comandos Range de exec— para que autosave y estadísticas se enteren.
 * - «Margen» arrastrable = margen del BLOQUE (margin-left/right), como dicta
 *   la tarea («aplican text-indent/margin al bloque»); los márgenes de página
 *   siguen siendo los presets de pageSetup (pestaña Disposición).
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useUltimo } from '@sytue/ui';
import type { EditorContext } from '../types';
import { getEditorRoot } from './exec';
import { RULER_H } from './metrics';
import { margenesPagina, medidasPagina } from '../pagedecor';

/** Ancho mínimo de columna de texto que los topes de arrastre respetan. */
const MIN_TEXT_W = 24;
/** Paso de los marcadores con teclado (flechas ←/→), en cm. */
const STEP_CM = 0.25;

/** Bloques con sangría aplicable (los de exec sin celdas: los márgenes no rigen en td/th). */
const INDENT_SELECTOR = 'p,h1,h2,h3,li,blockquote';

type MarkerId = 'primera' | 'francesa' | 'derecha';

/** Sangrías del bloque en px de maqueta: left/right = margin, first = text-indent. */
interface Indents {
  left: number;
  first: number;
  right: number;
}

const ZERO_INDENTS: Indents = { left: 0, first: 0, right: 0 };

/* ---------- utilidades de bloque (camino manual, patrón de exec) ---------- */

/**
 * Bloque de texto donde arranca la selección actual, acotado al root de exec.
 * ponytail: se aplica a UN bloque («el bloque de la selección», tarea);
 * multi-bloque cuando exec exponga blocksInSelection.
 */
function selectedBlock(): HTMLElement | null {
  const root = getEditorRoot();
  if (!root) return null;
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const node = sel.getRangeAt(0).startContainer;
  if (!root.contains(node)) return null;
  const el = node instanceof Element ? node : node.parentElement;
  const block = el?.closest<HTMLElement>(INDENT_SELECTOR) ?? null;
  return block && root.contains(block) ? block : null;
}

/** Lee las sangrías inline del bloque. ponytail: solo px (es lo que escribe la propia regla y el indent de execCommand con styleWithCSS); otras unidades cuentan como 0. */
function readIndents(block: HTMLElement | null): Indents {
  if (!block) return ZERO_INDENTS;
  return {
    left: parseFloat(block.style.marginLeft) || 0,
    first: parseFloat(block.style.textIndent) || 0,
    right: parseFloat(block.style.marginRight) || 0,
  };
}

/** Escribe las sangrías como estilos inline (cadena vacía = atributo fuera: HTML limpio). */
function applyIndents(block: HTMLElement, v: Indents): void {
  block.style.marginLeft = v.left ? `${v.left}px` : '';
  block.style.textIndent = v.first ? `${v.first}px` : '';
  block.style.marginRight = v.right ? `${v.right}px` : '';
}

/** El camino manual no emite `input` nativo: se emite a mano, como hace exec (§3). */
function dispatchInput(): void {
  getEditorRoot()?.dispatchEvent(new Event('input', { bubbles: true }));
}

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), Math.max(min, max));
}

function sameIndents(a: Indents, b: Indents): boolean {
  return a.left === b.left && a.first === b.first && a.right === b.right;
}

/** Formato «1,27 cm» para aria-valuetext (coma decimal en español). */
function cmText(v: number): string {
  return `${v.toFixed(2).replace('.', ',')} cm`;
}

/* ---------- componente ---------- */

export function Ruler({ ctx }: { ctx: EditorContext }): JSX.Element | null {
  const t = ctx.t;
  const [indents, setIndents] = useState<Indents>(ZERO_INDENTS);
  const [dragging, setDragging] = useState<MarkerId | null>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  /* Refs espejo: los manejadores de puntero llegan más deprisa que el render. */
  const indentsRef = useUltimo(indents);
  const dragRef = useRef<{ marker: MarkerId; block: HTMLElement } | null>(null);

  const pageW = medidasPagina(ctx.pageSetup).w;
  const [, derecho, , margin] = margenesPagina(ctx.pageSetup);
  const textW = pageW - margin - derecho;
  /** px por cm (§6.5): a 96 dpi, 96/2,54 en cualquier papel. */
  const pxPerCm = 96 / 2.54;

  /* Marcadores en sincronía con el bloque bajo el cursor; contentEpoch
   * refresca tras cargas, adopciones remotas y restauraciones (DOM reescrito). */
  useEffect(() => {
    const sync = (): void => {
      if (dragRef.current) return; // durante un arrastre manda el puntero
      const next = readIndents(selectedBlock());
      setIndents((prev) => (sameIndents(prev, next) ? prev : next));
    };
    document.addEventListener('selectionchange', sync);
    sync();
    return () => document.removeEventListener('selectionchange', sync);
  }, [ctx.contentEpoch]);

  /** Posición del marcador sobre el folio (px de maqueta desde su borde izquierdo). */
  const markerX = (marker: MarkerId, v: Indents): number =>
    marker === 'primera'
      ? margin + v.left + v.first
      : marker === 'francesa'
        ? margin + v.left
        : pageW - derecho - v.right;

  /** Sangrías resultantes de llevar un marcador a la posición x, con topes. */
  const withMarkerAt = (marker: MarkerId, x: number, cur: Indents): Indents => {
    if (marker === 'primera') {
      // text-indent negativo = primera línea a la izquierda del cuerpo (francesa).
      const first = Math.round(
        clamp(x - margin - cur.left, -cur.left, textW - cur.left - cur.right - MIN_TEXT_W),
      );
      return { ...cur, first };
    }
    if (marker === 'francesa') {
      const left = Math.round(clamp(x - margin, 0, textW - cur.right - MIN_TEXT_W));
      // Comportamiento clásico: la primera línea conserva su posición visual;
      // mover solo este marcador crea (o deshace) la sangría francesa.
      const first = Math.round(
        clamp(cur.first + cur.left - left, -left, textW - left - cur.right - MIN_TEXT_W),
      );
      return { ...cur, left, first };
    }
    const right = Math.round(clamp(pageW - derecho - x, 0, textW - cur.left - MIN_TEXT_W));
    return { ...cur, right };
  };

  /** Lleva el marcador a x: aplica al bloque y refleja el estado (sin `input` aún). */
  const moveMarker = (marker: MarkerId, x: number, block: HTMLElement): void => {
    const next = withMarkerAt(marker, x, indentsRef.current);
    if (sameIndents(next, indentsRef.current)) return;
    indentsRef.current = next; // los pointermove pueden adelantar al render
    applyIndents(block, next);
    setIndents(next);
  };

  const startDrag = (marker: MarkerId, event: PointerEvent): void => {
    // preventDefault: el folio conserva foco y selección (mismo truco que el ribbon §5).
    event.preventDefault();
    const block = selectedBlock();
    if (!block) return; // sin bloque seleccionado no hay dónde aplicar la sangría
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    dragRef.current = { marker, block };
    setDragging(marker);
  };

  const onDragMove = (event: PointerEvent): void => {
    const drag = dragRef.current;
    const track = trackRef.current;
    if (!drag || !track) return;
    const rect = track.getBoundingClientRect();
    if (rect.width === 0) return;
    // Ancho real ÷ ancho de maqueta = escala efectiva (incluye el zoom del wrapper).
    const x = (event.clientX - rect.left) / (rect.width / pageW);
    moveMarker(drag.marker, x, drag.block);
  };

  const endDrag = (): void => {
    if (!dragRef.current) return;
    dragRef.current = null;
    setDragging(null);
    dispatchInput(); // un único `input` por arrastre: autosave + estadísticas
  };

  /* Operable con teclado (AA): las flechas mueven el marcador EN PANTALLA
   * (← izquierda, → derecha) a pasos de 0,25 cm; para «derecha», → reduce la
   * sangría — se prima la coherencia visual sobre el patrón slider estricto. */
  const nudgeMarker = (marker: MarkerId, event: KeyboardEvent): void => {
    const dir = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
    if (dir === 0) return;
    event.preventDefault();
    const block = selectedBlock();
    if (!block) return;
    moveMarker(marker, markerX(marker, indentsRef.current) + dir * STEP_CM * pxPerCm, block);
    dispatchInput();
  };

  /* Marcas ancladas al margen izquierdo (0 = inicio del área de texto); en los
   * márgenes se rotula la distancia absoluta, como en los procesadores clásicos. */
  const labels: JSX.Element[] = [];
  const halfTicks: JSX.Element[] = [];
  for (let i = Math.ceil((2 - margin) / (pxPerCm / 2)); ; i += 1) {
    const x = margin + (i * pxPerCm) / 2;
    if (x > pageW - 2) break;
    if (i === 0) continue; // el 0 es la frontera del margen: ya la marca el sombreado
    if (i % 2 === 0) {
      labels.push(
        <text key={i} x={x} y={13} text-anchor="middle" font-size="8">
          {Math.abs(i / 2)}
        </text>,
      );
    } else {
      halfTicks.push(<line key={i} x1={x} y1={8} x2={x} y2={12} />);
    }
  }

  const markers: {
    id: MarkerId;
    label: string;
    x: number;
    top: boolean;
    value: number;
    max: number;
  }[] = [
    {
      id: 'primera',
      label: t('ruler.firstLine'),
      x: clamp(markerX('primera', indents), 0, pageW),
      top: true,
      value: (indents.left + indents.first) / pxPerCm,
      max: (textW - indents.right - MIN_TEXT_W) / pxPerCm,
    },
    {
      id: 'francesa',
      label: t('ruler.hanging'),
      x: clamp(markerX('francesa', indents), 0, pageW),
      top: false,
      value: indents.left / pxPerCm,
      max: (textW - indents.right - MIN_TEXT_W) / pxPerCm,
    },
    {
      id: 'derecha',
      label: t('ruler.right'),
      x: clamp(markerX('derecha', indents), 0, pageW),
      top: false,
      value: indents.right / pxPerCm,
      max: (textW - indents.left - MIN_TEXT_W) / pxPerCm,
    },
  ];

  /* Guarda doble tras los hooks (orden estable): quien monta ya condiciona por
   * ui.ruler && kind==='document' (§6.5), pero devolver null lo hace a prueba
   * de integradores distraídos. */
  if (!ctx.ui.ruler || ctx.kind !== 'document') return null;

  return (
    <div
      ref={trackRef}
      role="group"
      aria-label={t('ruler.label')}
      class="relative mx-auto border-b select-none"
      style={{
        width: `${pageW}px`,
        height: 'var(--editor-ruler-h)',
        background: 'var(--editor-page-bg)',
        // La regla es mobiliario del PAPEL, no del cromo: su tinta y su línea salen de
        // tokens fijos (§8), no de los del tema. Con tokens de tema, en oscuro se pintaba
        // gris claro sobre papel blanco. Los ticks, los rótulos y los marcadores heredan
        // este color por `currentColor`.
        borderColor: 'var(--editor-ruler-border)',
        color: 'var(--editor-ruler-ink)',
      }}
    >
      <svg
        aria-hidden="true"
        class="pointer-events-none absolute inset-0"
        width={pageW}
        height={RULER_H}
        viewBox={`0 0 ${pageW} ${RULER_H}`}
      >
        {/* Zonas de margen sombreadas (token local §8: --editor-ruler-margin). */}
        <rect x={0} y={0} width={margin} height={RULER_H} fill="var(--editor-ruler-margin)" />
        <rect
          x={pageW - derecho}
          y={0}
          width={derecho}
          height={RULER_H}
          fill="var(--editor-ruler-margin)"
        />
        <g stroke="currentColor" stroke-width={1} shape-rendering="crispEdges" opacity={0.7}>
          {halfTicks}
        </g>
        <g fill="currentColor">{labels}</g>
      </svg>

      {markers.map((m) => (
        <div
          key={m.id}
          role="slider"
          tabIndex={0}
          aria-label={m.label}
          title={m.label}
          aria-orientation="horizontal"
          aria-valuemin={0}
          aria-valuemax={Math.round(m.max * 100) / 100}
          aria-valuenow={Math.round(m.value * 100) / 100}
          aria-valuetext={cmText(m.value)}
          class={`sytue-focus-ring absolute flex h-[9px] w-3.5 -translate-x-1/2 cursor-ew-resize items-center justify-center hover:text-[var(--editor-ruler-ink-hover)] ${
            m.top ? 'top-0' : 'bottom-0'
          }`}
          style={{
            left: `${m.x}px`,
            touchAction: 'none',
            // El latón de marca (Graphite & Brass) vive en --sytue-primary; --sytue-accent (slate) de reserva.
            ...(dragging === m.id
              ? { color: 'var(--sytue-primary, var(--sytue-accent))' }
              : undefined),
          }}
          onPointerDown={(e) => startDrag(m.id, e)}
          onPointerMove={onDragMove}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
          onKeyDown={(e) => nudgeMarker(m.id, e)}
        >
          {/* Triángulo: hacia abajo = primera línea (arriba); hacia arriba = izquierda/derecha (abajo). */}
          <svg aria-hidden="true" width={10} height={7} viewBox="0 0 10 7">
            <path
              d={m.top ? 'M1 1h8L5 6Z' : 'M5 1l4 5H1Z'}
              fill="currentColor"
              stroke="currentColor"
              stroke-linejoin="round"
            />
          </svg>
        </div>
      ))}
    </div>
  );
}
