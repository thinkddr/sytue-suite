/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El resumen de la barra de hojas, a la carta, y la barra de fórmulas desplegada.
 *
 * El resumen de la selección enseñaba siempre recuento, suma y promedio: tres cifras
 * que se comían la barra en un portátil y que casi nunca se miran juntas. Ahora cada
 * cual elige cuáles ver —como al pulsar con el derecho la barra de estado de Excel—,
 * con la suma sola por defecto, que es la que se busca casi siempre. Lo elegido y si la
 * barra de fórmulas está desplegada son preferencias de QUIEN mira, no del libro: se
 * recuerdan por dispositivo (localStorage) y no viajan con el documento. La nativa
 * hace lo mismo con las preferencias de Fyne (`calcus/resumen.go`).
 */
import type { DocsT } from '../../i18n';
import { formatCellValue } from './format';
import type { SelectionStats } from './format';

/** Lo que puede enseñar el resumen. */
export type MedidaResumen = 'average' | 'count' | 'numbers' | 'min' | 'max' | 'sum';

/** Todas, en el orden en que se pintan y se ofrecen: el de la barra de estado de Excel. */
export const MEDIDAS_RESUMEN: readonly MedidaResumen[] = [
  'average',
  'count',
  'numbers',
  'min',
  'max',
  'sum',
];

/** Lo que se ve sin haber elegido nada. */
export const RESUMEN_POR_DEFECTO: readonly MedidaResumen[] = ['sum'];

const CLAVE_RESUMEN = 'sytue.calcus.resumen';
const CLAVE_FX_DESPLEGADA = 'sytue.calcus.fxDesplegada';

/**
 * Lee lo guardado («sum,min»): lo que no se entiende se ignora, los repetidos cuentan
 * una vez y el orden es siempre el de `MEDIDAS_RESUMEN`. Si no queda nada —primera vez,
 * almacenamiento corrupto— vale el de por defecto: un resumen vacío no se podría ni
 * pulsar para volver a elegir.
 */
export function leerMedidas(raw: string | null | undefined): MedidaResumen[] {
  const pedidas = new Set((raw ?? '').split(','));
  const out = MEDIDAS_RESUMEN.filter((m) => pedidas.has(m));
  return out.length > 0 ? out : [...RESUMEN_POR_DEFECTO];
}

/** Pone o quita una medida. Quitar la última no hace nada, por lo mismo de arriba. */
export function alternarMedida(
  actual: readonly MedidaResumen[],
  m: MedidaResumen,
): MedidaResumen[] {
  const quedan = actual.includes(m) ? actual.filter((x) => x !== m) : [...actual, m];
  return quedan.length > 0 ? MEDIDAS_RESUMEN.filter((x) => quedan.includes(x)) : [...actual];
}

/** El texto de una medida: «Suma: 6», «Recuento numérico: 3». */
export function textoMedida(t: DocsT, m: MedidaResumen, s: SelectionStats): string {
  switch (m) {
    case 'average':
      return t('sheet.stats.average', { value: formatCellValue(s.avg) });
    case 'count':
      return t('sheet.stats.count', { count: s.count });
    case 'numbers':
      return t('sheet.stats.numbers', { count: s.numbers });
    case 'min':
      return t('sheet.stats.min', { value: formatCellValue(s.min) });
    case 'max':
      return t('sheet.stats.max', { value: formatCellValue(s.max) });
    case 'sum':
      return t('sheet.stats.sum', { value: formatCellValue(s.sum) });
  }
}

/** El resumen entero, con el «·» de siempre entre medidas. */
export function textoResumen(
  t: DocsT,
  medidas: readonly MedidaResumen[],
  s: SelectionStats,
): string {
  return medidas.map((m) => textoMedida(t, m, s)).join(' · ');
}

/* ---------- Preferencias del dispositivo ---------- */

/* Todo con try: en una ventana privada o con el almacenamiento bloqueado,
 * `localStorage` lanza al tocarlo, y la preferencia dura entonces lo que la página. */

export function medidasRecordadas(): MedidaResumen[] {
  try {
    return leerMedidas(localStorage.getItem(CLAVE_RESUMEN));
  } catch {
    return [...RESUMEN_POR_DEFECTO];
  }
}

export function recordarMedidas(m: readonly MedidaResumen[]): void {
  try {
    const def =
      m.length === RESUMEN_POR_DEFECTO.length && m.every((x, i) => x === RESUMEN_POR_DEFECTO[i]);
    if (def) localStorage.removeItem(CLAVE_RESUMEN);
    else localStorage.setItem(CLAVE_RESUMEN, m.join(','));
  } catch {
    // sin almacenamiento la preferencia dura lo que la página
  }
}

export function fxDesplegadaRecordada(): boolean {
  try {
    return localStorage.getItem(CLAVE_FX_DESPLEGADA) === '1';
  } catch {
    return false;
  }
}

export function recordarFxDesplegada(desplegada: boolean): void {
  try {
    if (desplegada) localStorage.setItem(CLAVE_FX_DESPLEGADA, '1');
    else localStorage.removeItem(CLAVE_FX_DESPLEGADA);
  } catch {
    // sin almacenamiento la preferencia dura lo que la página
  }
}
