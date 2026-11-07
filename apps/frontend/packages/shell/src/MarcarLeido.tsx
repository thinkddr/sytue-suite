/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El punto azul de un aviso sin leer, hecho BOTÓN: «Marcar como leído» sin abrirlo.
 *
 * Antes el punto era un `<span>` y la única forma de quitarlo de uno solo era abrir el
 * aviso —que navega— o «Marcar todas», que se lleva por delante los demás. Va FUERA del
 * enlace de la fila, como hermano: un botón dentro de un `<a>` no es HTML válido y el
 * clic subiría hasta la navegación.
 *
 * La diana es de 24 px (WCAG 2.5.8): el punto de 8 con `p-2` alrededor. El nombre
 * accesible es la acción; que el aviso está sin leer lo dice la propia fila.
 *
 * Al marcar, el aviso pasa a leído y este botón DESAPARECE: el foco que tenía se iría a
 * `<body>` y el siguiente Tab empezaría desde arriba de la página. Antes de que se vaya,
 * el foco pasa a la fila (el hermano anterior, el enlace o botón del aviso), que sigue.
 *
 * Con el filtro «Sin leer» la fila TAMBIÉN se va: el aviso leído sale de la lista. Por eso
 * se apunta antes la fila del aviso siguiente (o la del anterior, si era el último) y,
 * cuando la lista ya se ha repintado, si la propia ya no está, el foco va a esa.
 */
import type { JSX } from 'preact';

export interface MarcarLeidoProps {
  /** «Marcar como leído», ya traducido: es el nombre accesible y el `title`. */
  rotulo: string;
  alMarcar: () => void;
  class?: string;
}

export function MarcarLeido({ rotulo, alMarcar, class: clase }: MarcarLeidoProps): JSX.Element {
  return (
    <button
      type="button"
      aria-label={rotulo}
      title={rotulo}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        const fila = e.currentTarget.previousElementSibling;
        const aviso = e.currentTarget.parentElement;
        const vecina = (aviso?.nextElementSibling ?? aviso?.previousElementSibling)
          ?.firstElementChild;
        alMarcar();
        if (fila instanceof HTMLElement) fila.focus();
        // El repintado de la lista va después de este evento: se mira en la tarea
        // siguiente, cuando el aviso ya ha salido o se ha quedado.
        setTimeout(() => {
          if (fila?.isConnected === false && vecina instanceof HTMLElement && vecina.isConnected) {
            vecina.focus();
          }
        }, 0);
      }}
      class={
        'hover:bg-chrome-hover sytue-focus-ring inline-flex shrink-0 rounded-full p-2 transition-colors' +
        (clase ? ` ${clase}` : '')
      }
    >
      <span aria-hidden="true" class="bg-primary block h-2 w-2 rounded-full" />
    </button>
  );
}
