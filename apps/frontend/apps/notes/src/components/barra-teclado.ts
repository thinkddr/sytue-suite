/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA BARRA PEGADA AL TECLADO DEL TELÉFONO.
 *
 * Escribiendo en el teléfono, la barra de formato deja de estar encima de la nota —donde
 * se va de la vista en cuanto se baja un poco— y se pone justo encima del teclado en
 * pantalla, como la de Confluence o la de Notas: lo que se usa mientras se escribe, donde
 * está el pulgar. Vuelve arriba al irse el teclado o al dejar de escribir. Es lo mismo que
 * hace el editor de Fyne (`barra_teclado.go`).
 *
 * El teclado se ve en el viewport VISUAL: en iOS y en Chrome para Android el de diseño no
 * encoge, el visual sí, y lo que falta entre los dos por abajo es lo que tapa el teclado.
 * La barra va `fixed` a esa altura. Se mira con `observarViewport`, que es lo que mira
 * cada panel flotante de la suite.
 */
import { useEffect, useState } from 'preact/hooks';
import type { RefObject } from 'preact';
import { observarViewport } from '@sytue/ui';

/** Lo que tiene que encoger el viewport visual para que cuente como teclado: la barra de
 *  direcciones al plegarse no llega. */
export const MINIMO_TECLADO = 120;

/** Lo que tapa el teclado desde abajo del viewport de diseño, o null si no está fuera. */
export function tapaDelTeclado(
  altoDiseno: number,
  visual: { height: number; offsetTop: number } | null | undefined,
): number | null {
  if (!visual || altoDiseno - visual.height < MINIMO_TECLADO) return null;
  return Math.max(0, Math.round(altoDiseno - visual.height - visual.offsetTop));
}

/** Lo que es seguir escribiendo aunque el foco salga del campo: un menú o un diálogo de
 *  la barra, que se abren en una capa aparte. */
const HERRAMIENTAS = '[role="menu"], [role="dialog"], [role="listbox"]';

/**
 * Dónde va la barra: `null`, arriba (como siempre); un número, `fixed` a esa altura desde
 * abajo. Solo con el dedo (`pointer: coarse`), con el teclado fuera y escribiendo en
 * `campo` (o en un menú de su barra).
 */
export function useBarraEnTeclado(
  campo: RefObject<HTMLElement>,
  raiz: RefObject<HTMLElement>,
): number | null {
  const [abajo, setAbajo] = useState<number | null>(null);
  useEffect(() => {
    const el = campo.current;
    if (!el || typeof window === 'undefined' || !window.matchMedia?.('(pointer: coarse)').matches)
      return;
    let escribiendo = document.activeElement === el;
    const medir = () =>
      setAbajo(escribiendo ? tapaDelTeclado(window.innerHeight, window.visualViewport) : null);
    // El foco en el campo es escribir; en la barra o en uno de sus menús, seguir
    // escribiendo; en cualquier otro sitio (o en ninguno), haber acabado.
    const foco = (e: FocusEvent) => {
      const a = e.target instanceof Element ? e.target : null;
      escribiendo =
        a === el ||
        (escribiendo && !!a && (!!raiz.current?.contains(a) || !!a.closest(HERRAMIENTAS)));
      medir();
    };
    const sale = (e: FocusEvent) => {
      if (e.relatedTarget === null) {
        escribiendo = false;
        medir();
      }
    };
    document.addEventListener('focusin', foco);
    el.addEventListener('blur', sale);
    const dejar = observarViewport(medir);
    medir();
    return () => {
      document.removeEventListener('focusin', foco);
      el.removeEventListener('blur', sale);
      dejar();
    };
  }, [campo, raiz]);
  return abajo;
}
