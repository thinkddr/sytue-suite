/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El pie de la rejilla: lo que se escribe bajo cada columna con el resumen que calcula
 * el servicio sobre TODO lo que pasa la vista (`GET /tables/{id}/summary`), no sobre la
 * ventana cargada. Sumar en el navegador las cien filas que se ven daría un total que
 * parece cierto y no lo es.
 *
 * Qué se dice de cada columna, en una línea corta: la suma en las numéricas (con la
 * media, el mínimo y el máximo como detalle), cuántas marcadas en una casilla y cuántas
 * con valor en el resto. Una columna sin ninguna, nada: un «0 con valor» bajo cada
 * columna vacía es ruido. Fyne escribe lo mismo (`apps/fyne/apps/bases/resumen.go`).
 */
import type { BasesField, BasesFieldSummary, BasesSummary } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { efectivo } from './formulas.js';
import { textoVisible } from './valores.js';

/** Lo que va bajo una columna: el texto corto y, si lo hay, el detalle (su `title`). */
export interface PieDeColumna {
  texto: string;
  detalle?: string;
}

/** El clave con la que se recuerda, por persona y navegador, si el pie está a la vista. */
export const CLAVE_PIE = 'sytue-bases-resumen';

/** Una cifra como la escribe la columna: en su moneda si la tiene, si no con su idioma. */
function cifra(campo: BasesField, n: number, idioma: string | undefined): string {
  if (campo.kind === 'currency') return textoVisible(campo, n, idioma);
  return new Intl.NumberFormat(idioma, { maximumFractionDigits: 2 }).format(n);
}

/** El pie de una columna, o null si no hay nada que decir de ella. */
export function pieDeColumna(
  campo: BasesField,
  r: BasesFieldSummary | undefined,
  t: T,
  idioma?: string,
): PieDeColumna | null {
  if (!r || r.filled === 0) return null;
  if (campo.kind === 'rating' && r.avg !== undefined) {
    // Sumar estrellas no dice nada: de una valoración se quiere la media.
    return { texto: t('summary.average', { value: cifra(campo, r.avg, idioma) }) };
  }
  const tipo = efectivo(campo).kind;
  if (tipo === 'number' && r.sum !== undefined) {
    return {
      texto: t('summary.sum', { value: cifra(campo, r.sum, idioma) }),
      ...(r.avg !== undefined && r.min !== undefined && r.max !== undefined
        ? {
            detalle: t('summary.detail', {
              avg: cifra(campo, r.avg, idioma),
              min: cifra(campo, r.min, idioma),
              max: cifra(campo, r.max, idioma),
            }),
          }
        : {}),
    };
  }
  if (tipo === 'checkbox') return { texto: t('summary.checked', { n: r.filled }) };
  return { texto: t('summary.filled', { n: r.filled }) };
}

/** El rótulo de la primera columna del pie: cuántos registros pasan la vista. */
export function recuento(r: BasesSummary, t: T): string {
  return t('summary.count', { n: r.count });
}
