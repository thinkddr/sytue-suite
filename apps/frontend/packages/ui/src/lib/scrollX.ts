/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La fila de barra que en móvil NO se pliega: SE DESPLAZA EN HORIZONTAL, como
 * en Outlook o en Docs. Con `flex-wrap` a secas, las barras con muchos controles
 * se repartían en dos, tres o cinco filas —la de una nota de Sytue Notes llegó a
 * ocupar la mitad del teléfono— y el área de trabajo quedaba en un par de
 * líneas. En móvil la fila es UNA y lo que no cabe se alcanza deslizando;
 * desde `md` (768px) cada barra vuelve a su comportamiento de siempre.
 *
 * `FILA_UNICA_EN_MOVIL` es la parte estructural y `MASCARA_SCROLL_X` + `useScrollRestante`
 * el aviso de que queda contenido: en iOS no hay barra de desplazamiento que lo
 * insinúe y «no se ven las opciones» es el defecto que se reporta. Quien monta la
 * barra añade TAMBIÉN el atributo `data-scroll-x`: es lo que el barrido E2E
 * (test/e2e/detector-desbordamiento.mjs) lee para no contar como defecto un
 * desbordamiento deliberado — una barra que desborda a propósito y otra que
 * desborda por error se ven igual desde fuera.
 *
 * El desplazamiento NO recorta los desplegables: `Dropdown` y `Popover` pintan
 * su panel en un portal con posición fija, fuera del árbol que recorta. Era la
 * condición que ponía la cinta de Docs para poder desplazarse (docs/design.md §4)
 * y por eso la cinta de grupos sigue plegándose en vez de deslizarse.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import type { RefObject } from 'preact';

/**
 * Una fila en móvil: nada se envuelve y lo que no cabe se desplaza de lado.
 * Componerla SOBRE las clases de la barra —el `flex flex-wrap` que ya tenga—,
 * que en `md` y hacia arriba manda lo que la barra ya hiciera.
 */
export const FILA_UNICA_EN_MOVIL =
  'max-md:flex-nowrap max-md:overflow-x-auto max-md:[&>*]:shrink-0';

/** Difuminado de los últimos 24 px. Aplicar SOLO cuando `queda` es cierto. */
export const MASCARA_SCROLL_X =
  '[mask-image:linear-gradient(to_right,#000_calc(100%-24px),transparent)]';

/**
 * Observa la fila marcada con `ref` y dice si queda contenido a la derecha.
 *
 * `depende` es para las barras que se montan CONDICIONALMENTE —la de lote solo
 * existe con selección, y su efecto a secas correría con el ref vacío y no se
 * volvería a enlazar nunca—: cambia de falso a verdadero cuando la barra acaba
 * de aparecer, y el efecto entonces sí encuentra el nodo. Quien monta siempre
 * su fila no lo pasa.
 */
export function useScrollRestante<T extends HTMLElement>(
  depende?: unknown,
): {
  ref: RefObject<T>;
  queda: boolean;
} {
  const ref = useRef<T>(null);
  const [queda, setQueda] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const medir = (): void => setQueda(Math.ceil(el.scrollLeft + el.clientWidth) < el.scrollWidth);
    medir();
    el.addEventListener('scroll', medir, { passive: true });
    // Cambia con el ancho de la ventana y con el propio contenido (el número de
    // acciones depende de permisos, de la selección…); el observer cubre los dos
    // casos sin un listener de resize aparte.
    const ro = new ResizeObserver(medir);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', medir);
      ro.disconnect();
    };
  }, [depende]);
  return { ref, queda };
}
