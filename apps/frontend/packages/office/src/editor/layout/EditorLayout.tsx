/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Rejilla de zonas del editor (BLUEPRINT.md §2 y §6): cabecera · ribbon ·
 * [franja izquierda | lienzo | ajustes] · barra de estado. Layout puro por
 * slots; las alturas fijas viven en cada zona (vars de editor.css §8).
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '@sytue/ui';

export interface EditorLayoutProps {
  header: ComponentChildren;
  ribbon: ComponentChildren;
  /** Franja de aviso entre la cinta y el lienzo (hoy: solo lectura). */
  aviso?: ComponentChildren;
  /** Franja izquierda (incluye su panel desplegable). */
  rail: ComponentChildren;
  /** Zona de lienzo con scroll propio (regla + página). */
  main: ComponentChildren;
  /** Panel derecho de ajustes; null = oculto. */
  settings: ComponentChildren | null;
  status: ComponentChildren;
  /** ¿Hay algún panel lateral desplegado? En <768px el lienzo les cede la
   *  pantalla (ver abajo); en escritorio no cambia nada. */
  panelOpen?: boolean;
}

export function EditorLayout(props: EditorLayoutProps): JSX.Element {
  return (
    <div
      data-suelo-tactil
      class="sytue-editor sytue-sheet-corners bg-surface text-text flex h-full min-h-0 flex-col"
    >
      {/* Las zonas de altura fija no se comprimen aunque el lienzo desborde. */}
      <div class="shrink-0">{props.header}</div>
      <div class="shrink-0">{props.ribbon}</div>
      {/* Franja de solo lectura. Va DEBAJO de la cinta y encima del lienzo, que es
          donde se mira antes de empezar a escribir. Sin ella, un lector pulsa
          negrita, no pasa nada y concluye que la aplicación está rota — el
          silencio es peor que la restricción. */}
      {props.aviso ? <div class="shrink-0">{props.aviso}</div> : null}
      <div class="flex min-h-0 flex-1">
        {props.rail}
        {/* Un panel mide 288 px y las dos franjas 40 cada una: en un teléfono de
            360 eso ya son 368, así que al abrir Buscar (o Comentarios, o
            Revisión…) el lienzo se quedaba en 0 px de ancho Y la pantalla
            desbordaba de lado. En <768px el lienzo se retira mientras haya un
            panel abierto y el panel ocupa el hueco entero; cerrarlo lo devuelve.
            Mismo criterio que el panel lateral de Rooms en móvil. */}
        {/* `data-editor-main`: el destino del F6 de la cinta (ribbon/banda.ts). */}
        <div
          data-editor-main
          tabIndex={-1}
          class={cx('flex min-h-0 min-w-0 flex-1 flex-col', props.panelOpen && 'max-md:hidden')}
        >
          {props.main}
        </div>
        {props.settings}
      </div>
      <div class="shrink-0">{props.status}</div>
    </div>
  );
}
