/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hook para interacciones de arrastre con Pointer Events y setPointerCapture.
 *
 * Gestiona captura de puntero, cálculo de deltas con escala y desacoplamiento
 * de onMove (render rápido sin persistir) y onEnd (commit / autosave).
 */

import { useRef, useCallback, useEffect } from 'preact/hooks';

export interface DragState<TData = unknown> {
  pointerId: number;
  x0: number;
  y0: number;
  data: TData;
}

export interface DragMoveEvent<TData = unknown> {
  data: TData;
  dxRaw: number;
  dyRaw: number;
  clientX: number;
  clientY: number;
  x0: number;
  y0: number;
}

export interface UsePointerDragOptions<TData = unknown> {
  /** Escala visual actual del lienzo o escenario (1 por defecto). */
  escala?: number | (() => number);
  /** Dimensiones de referencia si el delta se normaliza en porcentaje o coordenadas lógicas. */
  escenario?: { w: number; h: number } | (() => { w: number; h: number });
  /** Notificación de movimiento mientras se arrastra (no persiste). */
  onMove?: (estado: DragMoveEvent<TData>, e: PointerEvent) => void;
  /** Notificación al soltar o cancelar el puntero (hace commit). */
  onEnd?: (data: TData, e: PointerEvent) => void;
  /** Cancelación sin guardar; si no se declara, conserva el comportamiento de onEnd. */
  onCancel?: (data: TData, e: PointerEvent) => void;
}

export function usePointerDrag<TData = unknown>(opciones: UsePointerDragOptions<TData>) {
  const dragRef = useRef<DragState<TData> | null>(null);
  const opcionesRef = useRef(opciones);
  useEffect(
    () => () => {
      dragRef.current = null;
    },
    [],
  );
  useEffect(() => {
    opcionesRef.current = opciones;
  }, [opciones]);

  const iniciarArrastre = useCallback((e: PointerEvent, data: TData) => {
    e.preventDefault();
    e.stopPropagation();
    const target = e.currentTarget as HTMLElement | null;
    if (target && typeof target.setPointerCapture === 'function') {
      try {
        target.setPointerCapture(e.pointerId);
      } catch {
        // En navegadores antiguos o entornos de prueba
      }
    }
    dragRef.current = {
      pointerId: e.pointerId,
      x0: e.clientX,
      y0: e.clientY,
      data,
    };
  }, []);

  const onPointerMove = useCallback((e: PointerEvent) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;

    const opts = opcionesRef.current;
    const s = typeof opts.escala === 'function' ? opts.escala() : (opts.escala ?? 1);
    const esc = typeof opts.escenario === 'function' ? opts.escenario() : opts.escenario;

    let dxRaw = e.clientX - drag.x0;
    let dyRaw = e.clientY - drag.y0;

    if (esc && esc.w > 0 && esc.h > 0) {
      dxRaw = (dxRaw / (esc.w * s)) * 100;
      dyRaw = (dyRaw / (esc.h * s)) * 100;
    } else if (s > 0 && s !== 1) {
      dxRaw = dxRaw / s;
      dyRaw = dyRaw / s;
    }

    opts.onMove?.(
      {
        data: drag.data,
        dxRaw,
        dyRaw,
        clientX: e.clientX,
        clientY: e.clientY,
        x0: drag.x0,
        y0: drag.y0,
      },
      e,
    );
  }, []);

  const onPointerUp = useCallback((e: PointerEvent) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;
    const data = drag.data;
    dragRef.current = null;
    opcionesRef.current.onEnd?.(data, e);
  }, []);

  const onPointerCancel = useCallback((e: PointerEvent) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;
    dragRef.current = null;
    const opts = opcionesRef.current;
    (opts.onCancel ?? opts.onEnd)?.(drag.data, e);
  }, []);

  return {
    iniciarArrastre,
    onPointerMove,
    onPointerUp,
    onPointerCancel,
    dragRef,
    estaArrastrando: () => dragRef.current !== null,
  };
}
