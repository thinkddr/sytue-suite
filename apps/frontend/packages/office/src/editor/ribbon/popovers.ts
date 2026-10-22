/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Popovers de la cinta en `position: fixed`.
 *
 * La banda de grupos se desplaza en horizontal cuando no cabe (`overflow-x: auto`), y un
 * contenedor con `overflow-x` computa `overflow-y: auto`: un popover `absolute` colgado
 * de un botón de la banda queda RECORTADO por ella —paletas, rejilla de tabla,
 * comentario— y es lo que la cinta evitó durante meses envolviendo en varias líneas (ver
 * `Ribbon.tsx`). Un popover `fixed` no lo recorta ningún ancestro; a cambio, no sigue a su
 * disparador cuando algo se mueve, y de eso se ocupa este módulo.
 *
 * UN solo mecanismo para los treinta y pico paneles de la cinta, no uno por panel: los
 * paneles llevan la clase `ribbon-popover` (la trae `POPOVER_SHELL`) y la banda, con un
 * `MutationObserver`, los coloca en cuanto aparecen —antes de pintar— y de nuevo cuando la
 * banda se desplaza o la ventana cambia de tamaño. Sin él cada panel tendría que saber
 * dónde está su disparador.
 */
import type { RefObject } from 'preact';
import { useEffect } from 'preact/hooks';
import { limitesViewport, observarViewport } from '@sytue/ui';

/** Clase que marca un panel como popover de la cinta. */
export const CLASE_POPOVER = 'ribbon-popover';

/** Hueco entre el panel y el borde de la pantalla, y entre el panel y su disparador. */
const MARGEN = 8;
const SEPARACION = 4;

/**
 * Coloca un panel `fixed` bajo el elemento del que cuelga (su `parentElement`, el
 * contenedor `relative` del disparador), dentro de la pantalla: se corre a la izquierda
 * si se saldría por la derecha, y se acota en alto —con scroll propio— si no cabe debajo.
 * Respeta el `max-height` que ya traiga el panel por clase.
 */
export function colocarPopover(panel: HTMLElement): void {
  const ancla = panel.parentElement;
  if (!ancla) return;
  const r = ancla.getBoundingClientRect();
  const v = limitesViewport(MARGEN);
  panel.style.maxWidth = `${v.width}px`;
  // Medir sin el tope que dejó la colocación anterior, conservando el de la clase.
  panel.style.maxHeight = '';
  const deClase = parseFloat(getComputedStyle(panel).maxHeight);
  const alto = Math.min(
    panel.scrollHeight,
    Number.isFinite(deClase) ? deClase : Infinity,
    v.height,
  );
  const ancho = panel.offsetWidth;
  const arriba = Math.max(v.top, Math.min(r.bottom + SEPARACION, v.bottom - alto));
  const izquierda = Math.max(v.left, Math.min(r.left, v.right - ancho));
  panel.style.left = `${izquierda}px`;
  panel.style.top = `${arriba}px`;
  if (panel.scrollHeight > alto) {
    panel.style.maxHeight = `${alto}px`;
    panel.style.overflowY = 'auto';
  }
}

/** Coloca todos los popovers abiertos dentro de `raiz`. */
export function colocarPopovers(raiz: ParentNode): void {
  for (const p of raiz.querySelectorAll<HTMLElement>(`.${CLASE_POPOVER}`)) colocarPopover(p);
}

/**
 * Mantiene colocados los popovers de la cinta que cuelgan de `raiz`: al aparecer, al
 * desplazarse la banda y al cambiar el tamaño de la ventana.
 */
export function usePopoversFijos(raiz: RefObject<HTMLElement>): void {
  useEffect(() => {
    const el = raiz.current;
    if (!el) return;
    const colocar = (): void => colocarPopovers(el);
    // childList + subtree: un panel que aparece, y uno que cambia de contenido (cambia su alto).
    const mo = new MutationObserver(colocar);
    mo.observe(el, { childList: true, subtree: true });
    el.addEventListener('scroll', colocar, { passive: true });
    const dejarDeObservar = observarViewport(colocar);
    return () => {
      mo.disconnect();
      el.removeEventListener('scroll', colocar);
      dejarDeObservar();
    };
  }, [raiz]);
}
