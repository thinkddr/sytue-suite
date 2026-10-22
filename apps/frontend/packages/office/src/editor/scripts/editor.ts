/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cableado del anfitrión de scripts con el editor abierto: despacha por los MISMOS
 * dispatch que las macros (exec, dispatchSheetCommand, dispatchSlidesCommand), de modo que
 * la protección de hoja y de libro del editor y la del servidor (`/ops`) se aplican como a
 * cualquier acción de la persona, y lee los datos que el manifiesto declara.
 *
 * Lo que se lee del folio y de las diapositivas es el ÚLTIMO CONTENIDO SINCRONIZADO
 * (`ctx.content`), no lo que se acaba de teclear; las celdas y la selección, sí en vivo.
 * ponytail: lectura en vivo de folio y diapositivas cuando el lienzo exponga un lector.
 */
import type { Anfitrion } from './anfitrion';
import { diapositivasTexto, escalares, textoPlano } from './lectores';
import { exec } from '../canvas/exec';
import {
  dispatchSheetCommand,
  readSheetName,
  readSheetState,
  readSheetValues,
} from '../sheets/SheetCanvas';
import type { SheetCommand } from '../sheets/SheetCanvas';
import { dispatchSlidesCommand } from '../slides/SlidesCanvas';
import type { SlidesCommand } from '../slides/SlidesCanvas';
import type { EditorCommand, EditorContext } from '../types';
import type { MacroStep } from '../macros';

const despachar = (step: MacroStep): void => {
  if (step.editor === 'spreadsheet') dispatchSheetCommand(step.cmd as SheetCommand, step.value);
  else if (step.editor === 'presentation')
    dispatchSlidesCommand(step.cmd as SlidesCommand, step.value);
  else exec(step.cmd as EditorCommand, step.value);
};

export function anfitrionDeEditor(ctx: EditorContext): Anfitrion {
  const seleccion = (): { hoja: string; ref: string } | null => {
    const hoja = readSheetName();
    const ref = readSheetState()?.selection;
    return hoja && ref ? { hoja, ref } : null;
  };
  return {
    despachar,
    ...(ctx.kind === 'spreadsheet' ? { seleccion } : {}),
    leer: (que, hoja, ref, rango) => {
      switch (que) {
        case 'title':
          return ctx.title;
        case 'text':
          return ctx.kind === 'document' ? textoPlano(ctx.content) : undefined;
        case 'slides':
          return ctx.kind === 'presentation'
            ? (diapositivasTexto(ctx.content) ?? undefined)
            : undefined;
        case 'selection':
          return ctx.kind === 'spreadsheet' ? (seleccion() ?? undefined) : undefined;
        case 'cells': {
          if (ctx.kind !== 'spreadsheet' || !hoja || !ref) return undefined;
          const v = readSheetValues(ref, hoja);
          if (!v) return undefined;
          const m = escalares(v);
          return rango ? m : (m[0]?.[0] ?? null);
        }
      }
    },
  };
}
