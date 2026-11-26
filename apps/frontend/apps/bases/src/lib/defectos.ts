/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El valor por defecto de un campo: con el que nace un registro que no lo trae (un alta
 * desde la rejilla, una tarjeta del tablero, una respuesta de formulario). Lo aplica el
 * servicio (`domain.ConDefectos`); aquí solo se decide qué se escribe en el diálogo y qué
 * se envía. Lo de una fecha puede ser `'today'`: el día de cada alta, no uno fijo.
 *
 * Fyne hace lo mismo con los mismos nombres (`apps/fyne/apps/bases/defectos.go`).
 */
import type { BasesField, BasesValue } from '@sytue/api-clients/bases';
import { textoDeValor, valorDeTexto } from './valores.js';

/** El defecto de una fecha que vale el día de cada alta (`domain.DefectoHoy`). */
export const DEFECTO_HOY = 'today';

/** Los tipos que admiten defecto: los que se escriben (no enlaces, fórmulas ni adjuntos). */
export function admiteDefecto(campo: BasesField): boolean {
  return campo.kind !== 'link' && campo.kind !== 'formula' && campo.kind !== 'attachment';
}

/** Lo que lleva escrito el diálogo del defecto. */
export interface BorradorDefecto {
  /** Una fecha que vale el día de cada alta. */
  hoy: boolean;
  /** Lo escrito en un campo de texto, número, fecha u opción. */
  texto: string;
  /** Una casilla marcada por defecto. */
  marcado: boolean;
  /** Las opciones de una selección múltiple. */
  varios: string[];
}

/** El borrador con el que abre el diálogo: el defecto que el campo ya tiene. */
export function borradorDeDefecto(campo: BasesField): BorradorDefecto {
  const v = campo.default ?? null;
  return {
    hoy: campo.kind === 'date' && v === DEFECTO_HOY,
    texto:
      campo.kind === 'checkbox' || campo.kind === 'multiselect' || v === DEFECTO_HOY
        ? ''
        : textoDeValor(campo, v),
    marcado: v === true,
    varios: Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [],
  };
}

/**
 * Lo que se envía: el defecto, `null` para quitarlo, o `undefined` si lo escrito no
 * encaja en el campo (el diálogo no deja guardar).
 */
export function valorDeDefecto(campo: BasesField, b: BorradorDefecto): BasesValue | undefined {
  switch (campo.kind) {
    case 'checkbox':
      return b.marcado ? true : null;
    case 'multiselect':
      return b.varios.length > 0 ? b.varios : null;
    case 'date':
      if (b.hoy) return DEFECTO_HOY;
      return valorDeTexto(campo, b.texto);
    default:
      return valorDeTexto(campo, b.texto);
  }
}
