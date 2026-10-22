/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los dos ganchos de la banda de grupos de la cinta (Ribbon.tsx): saber si sus grupos
 * caben en una línea y el salto F6 entre la cinta y el lienzo.
 */
import type { RefObject } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';

/**
 * ¿Desborda en horizontal este contenedor? Se vuelve a medir cuando cambia su tamaño, el
 * de cualquiera de sus hijos (un grupo gana un botón) o su lista de hijos (otra pestaña).
 *
 * `dep` es lo que, al cambiar, obliga a medir otra vez sin que el tamaño cambie (la
 * pestaña activa con los mismos grupos de ancho).
 *
 * `pausado`: no medir mientras es verdadero y CONSERVAR la última medida. La banda
 * desplegada con «Más» envuelve en varias líneas y no desborda; si se midiera entonces,
 * «Más» (que solo se pinta si algo no cabe) se desmontaba al plegar con Escape y se llevaba
 * el foco con él, que caía al `<body>`, hasta la medición siguiente.
 */
export function useDesborda(ref: RefObject<HTMLElement>, dep?: unknown, pausado = false): boolean {
  const [desborda, setDesborda] = useState(false);
  // Un ref y no una dependencia del efecto: plegar no debe recrear los observadores, solo
  // dejar que el siguiente aviso del navegador (que llega al cambiar el tamaño) mida.
  const pausadoRef = useRef(pausado);
  // ResizeObserver avisa antes de pintar. Un efecto diferido llegaba después y
  // medía la banda expandida como si siguiera plegada: al cerrar desmontaba «Más»
  // y perdía el foco. La pausa debe reflejar el DOM ya confirmado en ese instante.
  useLayoutEffect(() => {
    pausadoRef.current = pausado;
  });
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === 'undefined') return;
    // +1: los anchos con decimales redondean y un píxel de más no es un desbordamiento.
    const medir = (): void => {
      if (!pausadoRef.current) setDesborda(el.scrollWidth > el.clientWidth + 1);
    };
    const ro = new ResizeObserver(medir);
    const observar = (): void => {
      ro.disconnect();
      ro.observe(el);
      for (const hijo of Array.from(el.children)) ro.observe(hijo);
      medir();
    };
    const mo = new MutationObserver(observar);
    mo.observe(el, { childList: true });
    observar();
    return () => {
      ro.disconnect();
      mo.disconnect();
    };
  }, [ref, dep]);
  return desborda;
}

/** Los controles que reciben el foco con tabulador. */
const ENFOCABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * Lleva el foco al lienzo: el texto editable del folio o de una caja, o el primer control
 * enfocable de la zona (la rejilla de la hoja, el lienzo de diapositivas); si no hay
 * ninguno, la propia zona, que lleva `tabindex="-1"` para poder recibirlo.
 */
export function enfocarLienzo(): boolean {
  const zona = document.querySelector<HTMLElement>('[data-editor-main]');
  if (!zona) return false;
  const destino =
    zona.querySelector<HTMLElement>('[contenteditable="true"]') ??
    zona.querySelector<HTMLElement>(ENFOCABLE) ??
    zona;
  destino.focus({ preventScroll: true });
  return true;
}

/**
 * F6 (y Mayús+F6) salta entre las dos zonas del editor: desde la cinta, al lienzo; desde
 * cualquier otro sitio, a `alCinta` (la pestaña activa). Es la tecla de «ir a la
 * siguiente zona» de las aplicaciones de escritorio; sin ella, llegar de un lienzo a la
 * cinta cuesta tantas tabulaciones como controles haya en medio.
 */
export function useSaltoF6(cinta: RefObject<HTMLElement>, alCinta: () => void): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key !== 'F6' || e.ctrlKey || e.metaKey || e.altKey) return;
      const dentro = cinta.current?.contains(document.activeElement) ?? false;
      e.preventDefault();
      if (dentro) enfocarLienzo();
      else alCinta();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [cinta, alCinta]);
}
