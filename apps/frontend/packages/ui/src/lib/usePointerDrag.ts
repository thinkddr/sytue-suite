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
  /** Desplazador de la lista o lienzo: se recorre al acercar el arrastre a sus bordes. */
  scrollContainer?: () => HTMLElement | null;
  /** Lista virtualizada: el gesto sobrevive al desmontaje del asa que lo inició. */
  documentEvents?: boolean;
}

export function usePointerDrag<TData = unknown>(opciones: UsePointerDragOptions<TData>) {
  const dragRef = useRef<DragState<TData> | null>(null);
  const opcionesRef = useRef(opciones);
  const capturaRef = useRef<HTMLElement | null>(null);
  const ultimoRef = useRef<PointerEvent | null>(null);
  const frameRef = useRef<number | null>(null);
  const escapeRef = useRef<((e: KeyboardEvent) => void) | null>(null);
  const documentoRef = useRef(false);
  const quitarEventosRef = useRef<(() => void) | null>(null);
  useEffect(() => {
    opcionesRef.current = opciones;
  }, [opciones]);

  const limpiar = useCallback(() => {
    const drag = dragRef.current;
    dragRef.current = null;
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    frameRef.current = null;
    if (escapeRef.current) document.removeEventListener('keydown', escapeRef.current, true);
    escapeRef.current = null;
    quitarEventosRef.current?.();
    quitarEventosRef.current = null;
    documentoRef.current = false;
    const target = capturaRef.current;
    capturaRef.current = null;
    ultimoRef.current = null;
    if (drag && target?.hasPointerCapture?.(drag.pointerId))
      target.releasePointerCapture(drag.pointerId);
  }, []);

  const cancelarArrastre = useCallback(
    (e?: PointerEvent) => {
      const drag = dragRef.current;
      const evento = e ?? ultimoRef.current;
      if (!drag || !evento || evento.pointerId !== drag.pointerId) return;
      limpiar();
      opcionesRef.current.onCancel?.(drag.data, evento);
    },
    [limpiar],
  );

  useEffect(() => () => cancelarArrastre(), [cancelarArrastre]);

  const notificarMovimiento = useCallback((e: PointerEvent) => {
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

  const recorrerBorde = useCallback(() => {
    let anterior = 0;
    const frame = (ahora: number): void => {
      frameRef.current = null;
      const drag = dragRef.current;
      const e = ultimoRef.current;
      const scroller = opcionesRef.current.scrollContainer?.();
      if (!drag || !e || !scroller) return;
      const rect = scroller.getBoundingClientRect();
      const paso = (anterior ? Math.min(ahora - anterior, 32) : 16) / 1000;
      anterior = ahora;
      const velocidad = (p: number, inicio: number, fin: number): number => {
        const borde = Math.min(40, (fin - inicio) / 3);
        if (borde <= 0 || p < inicio - borde || p > fin + borde) return 0;
        if (p < inicio + borde) return -480 * Math.min(1, (inicio + borde - p) / borde);
        if (p > fin - borde) return 480 * Math.min(1, (p - fin + borde) / borde);
        return 0;
      };
      const x = scroller.scrollLeft;
      const y = scroller.scrollTop;
      // Fuera del panel no se desplaza su contenido: el puntero puede estar ya sobre
      // un destino de otra lista, del árbol lateral o de la tira de días.
      if (e.clientY >= rect.top && e.clientY <= rect.bottom)
        scroller.scrollLeft += velocidad(e.clientX, rect.left, rect.right) * paso;
      if (e.clientX >= rect.left && e.clientX <= rect.right)
        scroller.scrollTop += velocidad(e.clientY, rect.top, rect.bottom) * paso;
      if (x !== scroller.scrollLeft || y !== scroller.scrollTop) notificarMovimiento(e);
      if (dragRef.current) frameRef.current = requestAnimationFrame(frame);
    };
    frameRef.current = requestAnimationFrame(frame);
  }, [notificarMovimiento]);

  const onPointerMove = useCallback(
    (e: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId || ultimoRef.current === e) return;
      ultimoRef.current = e;
      notificarMovimiento(e);
      if (
        opcionesRef.current.scrollContainer &&
        frameRef.current === null &&
        Math.hypot(e.clientX - drag.x0, e.clientY - drag.y0) >= 4
      )
        recorrerBorde();
    },
    [notificarMovimiento, recorrerBorde],
  );

  const onPointerUp = useCallback(
    (e: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      const data = drag.data;
      limpiar();
      opcionesRef.current.onEnd?.(data, e);
    },
    [limpiar],
  );

  const onPointerCancel = useCallback(
    (e: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      limpiar();
      const opts = opcionesRef.current;
      (opts.onCancel ?? opts.onEnd)?.(drag.data, e);
    },
    [limpiar],
  );

  const onLostPointerCapture = useCallback(
    (e: PointerEvent) => {
      if (!documentoRef.current) cancelarArrastre(e);
    },
    [cancelarArrastre],
  );

  const iniciarArrastre = useCallback(
    (e: PointerEvent, data: TData) => {
      if (e.button !== 0 || (e.pointerType && !e.isPrimary) || dragRef.current) return false;
      e.preventDefault();
      e.stopPropagation();
      const target = e.currentTarget as HTMLElement | null;
      documentoRef.current = opcionesRef.current.documentEvents === true;
      if (!documentoRef.current && target && typeof target.setPointerCapture === 'function') {
        try {
          target.setPointerCapture(e.pointerId);
        } catch {
          // La interacción sigue funcionando sin captura en los entornos de prueba.
        }
      }
      capturaRef.current = documentoRef.current ? null : target;
      ultimoRef.current = e;
      dragRef.current = { pointerId: e.pointerId, x0: e.clientX, y0: e.clientY, data };
      if (documentoRef.current) {
        document.addEventListener('pointermove', onPointerMove, true);
        document.addEventListener('pointerup', onPointerUp, true);
        document.addEventListener('pointercancel', onPointerCancel, true);
      }
      const abandonar = (): void => cancelarArrastre();
      const alOcultar = (): void => {
        if (document.hidden) abandonar();
      };
      window.addEventListener('blur', abandonar);
      document.addEventListener('visibilitychange', alOcultar);
      quitarEventosRef.current = () => {
        document.removeEventListener('pointermove', onPointerMove, true);
        document.removeEventListener('pointerup', onPointerUp, true);
        document.removeEventListener('pointercancel', onPointerCancel, true);
        window.removeEventListener('blur', abandonar);
        document.removeEventListener('visibilitychange', alOcultar);
      };
      const alEscape = (key: KeyboardEvent): void => {
        if (key.key !== 'Escape') return;
        key.preventDefault();
        key.stopPropagation();
        cancelarArrastre();
      };
      escapeRef.current = alEscape;
      document.addEventListener('keydown', alEscape, true);
      return true;
    },
    [cancelarArrastre, onPointerMove, onPointerUp, onPointerCancel],
  );

  return {
    iniciarArrastre,
    onPointerMove,
    onPointerUp,
    onPointerCancel,
    onLostPointerCapture,
    cancelarArrastre,
    dragRef,
    estaArrastrando: () => dragRef.current !== null,
  };
}
