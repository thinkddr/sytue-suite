/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { useState } from 'preact/hooks';
import { usePointerDrag } from './usePointerDrag.js';

export interface ListReorderOptions {
  ids: readonly string[];
  container: () => HTMLElement | null;
  /** Índices finales, una sola vez al soltar. Puede devolver el id nuevo si usa índices como ids. */
  onReorder: (desde: number, hasta: number) => string | void;
  disabled?: boolean;
  axis?: 'x' | 'y' | 'auto';
}

/** Asa de lista: dedo, ratón, flechas y cancelación comparten el mismo movimiento. */
export function useListReorder({
  ids,
  container,
  onReorder,
  disabled = false,
  axis = 'y',
}: ListReorderOptions) {
  const [destino, setDestino] = useState<{ id: string; despues: boolean } | null>(null);
  const [origen, setOrigen] = useState<string | null>(null);
  const [eje, setEje] = useState<'x' | 'y'>(axis === 'auto' ? 'y' : axis);
  const ejeActual = (): 'x' | 'y' => {
    if (axis !== 'auto') return axis;
    const el = container();
    return el && getComputedStyle(el).flexDirection.startsWith('row') ? 'x' : 'y';
  };
  const en = (x: number, y: number): { id: string; despues: boolean } | null => {
    const root = container();
    const el = document
      .elementsFromPoint(x, y)
      .map((e) => e.closest<HTMLElement>('[data-reorder-id]'))
      .find((e) => e && root?.contains(e));
    const id = el?.dataset.reorderId;
    if (!el || id === undefined || !ids.includes(id)) return null;
    const r = el.getBoundingClientRect();
    return {
      id,
      despues: ejeActual() === 'x' ? x > r.left + r.width / 2 : y > r.top + r.height / 2,
    };
  };
  const limpiar = (): void => {
    setDestino(null);
    setOrigen(null);
  };
  const drag = usePointerDrag<{ id: string; x0: number; y0: number }>({
    scrollContainer: container,
    onMove: ({ data, clientX, clientY, dxRaw, dyRaw }) => {
      if (Math.hypot(dxRaw, dyRaw) < 4) return;
      setOrigen(data.id);
      setEje(ejeActual());
      const siguiente = en(clientX, clientY);
      setDestino((actual) =>
        actual?.id === siguiente?.id && actual?.despues === siguiente?.despues ? actual : siguiente,
      );
    },
    onCancel: limpiar,
    onEnd: ({ id, x0, y0 }, e) => {
      limpiar();
      if (disabled || !ids.includes(id) || Math.hypot(e.clientX - x0, e.clientY - y0) < 4) return;
      const fin = en(e.clientX, e.clientY);
      if (fin && fin.id !== id) {
        const desde = ids.indexOf(id);
        const objetivo = ids.indexOf(fin.id);
        const hasta = objetivo + (fin.despues ? 1 : 0) - (desde < objetivo ? 1 : 0);
        if (hasta !== desde) onReorder(desde, hasta);
      }
    },
  });
  return {
    cancelar: drag.cancelarArrastre,
    itemProps: (id: string) => ({
      'data-reorder-id': id,
      'data-reorder-axis': eje,
      'data-reorder-over':
        !disabled && origen !== id && destino?.id === id
          ? destino.despues
            ? 'after'
            : 'before'
          : undefined,
      'data-reorder-dragging': origen === id ? 'true' : undefined,
    }),
    handleProps: (id: string) => ({
      'data-reorder-handle': 'true',
      onPointerDown: (e: PointerEvent): void => {
        if (!disabled && ids.includes(id))
          drag.iniciarArrastre(e, { id, x0: e.clientX, y0: e.clientY });
      },
      onPointerMove: drag.onPointerMove,
      onPointerUp: drag.onPointerUp,
      onPointerCancel: drag.onPointerCancel,
      onLostPointerCapture: drag.onLostPointerCapture,
      onClick: (e: MouseEvent): void => e.stopPropagation(),
      onKeyDown: (e: KeyboardEvent): void => {
        if (disabled) return;
        const i = ids.indexOf(id);
        if (i < 0) return;
        const actual = ejeActual();
        const atras = actual === 'x' ? 'ArrowLeft' : 'ArrowUp';
        const delante = actual === 'x' ? 'ArrowRight' : 'ArrowDown';
        const j =
          e.key === atras
            ? i - 1
            : e.key === delante
              ? i + 1
              : e.key === 'Home'
                ? 0
                : e.key === 'End'
                  ? ids.length - 1
                  : -1;
        if (![atras, delante, 'Home', 'End'].includes(e.key)) return;
        e.preventDefault();
        e.stopPropagation();
        const objetivo = ids[j];
        if (objetivo !== undefined && j !== i) {
          const siguiente = onReorder(i, j) ?? id;
          requestAnimationFrame(() => {
            const fila = Array.from(
              container()?.querySelectorAll<HTMLElement>('[data-reorder-id]') ?? [],
            ).find((e) => e.dataset.reorderId === siguiente);
            fila?.querySelector<HTMLElement>('[data-reorder-handle]')?.focus();
          });
        }
      },
    }),
  };
}
