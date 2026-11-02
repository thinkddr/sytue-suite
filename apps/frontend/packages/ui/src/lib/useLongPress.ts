/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { useCallback, useEffect, useRef } from 'preact/hooks';
import { useUltimo } from './useUltimo.js';

/** Mantener para seleccionar. No captura el puntero ni impide desplazar la lista. */
export function useLongPress<T>(onHold?: (data: T) => void) {
  const callbackRef = useUltimo(onHold);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);
  const omitirClickRef = useRef(false);
  const cancelar = useCallback(() => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = null;
    cleanupRef.current?.();
    cleanupRef.current = null;
  }, []);
  useEffect(() => () => cancelar(), [cancelar]);

  const iniciar = useCallback(
    (e: PointerEvent, data: T): void => {
      omitirClickRef.current = false;
      if (!callbackRef.current || e.button !== 0 || !['touch', 'pen'].includes(e.pointerType))
        return;
      if (!e.isPrimary) return;
      const root = e.currentTarget as HTMLElement;
      const target =
        e.target instanceof Element
          ? e.target.closest('button,a,input,select,textarea,[contenteditable="true"]')
          : null;
      if (target && target !== root) return;
      cancelar();
      const accion = callbackRef.current; // El origen queda fijado aunque la lista se recicle.
      const { clientX: x, clientY: y, pointerId } = e;
      const mover = (p: PointerEvent): void => {
        if (p.pointerId === pointerId && Math.hypot(p.clientX - x, p.clientY - y) > 8) cancelar();
      };
      const terminar = (p: PointerEvent): void => {
        if (p.pointerId === pointerId) cancelar();
      };
      const segundoPuntero = (p: PointerEvent): void => {
        if (p.pointerId !== pointerId) cancelar();
      };
      document.addEventListener('pointerdown', segundoPuntero, true);
      document.addEventListener('pointermove', mover, true);
      document.addEventListener('pointerup', terminar, true);
      document.addEventListener('pointercancel', terminar, true);
      window.addEventListener('blur', cancelar);
      cleanupRef.current = () => {
        document.removeEventListener('pointerdown', segundoPuntero, true);
        document.removeEventListener('pointermove', mover, true);
        document.removeEventListener('pointerup', terminar, true);
        document.removeEventListener('pointercancel', terminar, true);
        window.removeEventListener('blur', cancelar);
      };
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        omitirClickRef.current = true;
        accion(data);
      }, 450);
    },
    [cancelar, callbackRef],
  );

  return {
    iniciar,
    cancelar,
    onClickCapture: (e: MouseEvent): void => {
      if (!omitirClickRef.current || e.detail === 0) return;
      omitirClickRef.current = false;
      e.preventDefault();
      e.stopImmediatePropagation();
    },
    onContextMenuCapture: (e: MouseEvent): void => {
      if (omitirClickRef.current) e.preventDefault();
    },
  };
}
