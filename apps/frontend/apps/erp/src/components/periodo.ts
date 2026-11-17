/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El PERÍODO del libro diario y del sumas y saldos: los cortes que pide quien lleva
 * la contabilidad —este mes, el anterior, el trimestre, el año— sin teclear fechas.
 * El servicio ya acotaba por `desde`/`hasta`; aquí se calcula el rango. Espejo en
 * `apps/fyne/apps/erp/periodo.go`, mismos nombres.
 */
import type { ErpMessageKey } from '../i18n/index.js';

export const PERIODOS = [
  'todo',
  'mes',
  'mesAnterior',
  'trimestre',
  'anio',
  'anioAnterior',
] as const;
export type Periodo = (typeof PERIODOS)[number];

export const ROTULO_PERIODO: Record<Periodo, ErpMessageKey> = {
  todo: 'ledger.periodAll',
  mes: 'ledger.periodMonth',
  mesAnterior: 'ledger.periodLastMonth',
  trimestre: 'ledger.periodQuarter',
  anio: 'ledger.periodYear',
  anioAnterior: 'ledger.periodLastYear',
};

/** `YYYY-MM-DD` de un día del calendario LOCAL: el mes de quien mira, no el de UTC. */
function iso(anio: number, mes: number, dia: number): string {
  // `Date` normaliza: el día 0 de un mes es el último del anterior, y el mes 12 es enero.
  const d = new Date(anio, mes, dia);
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${dd}`;
}

/** El rango de un período, con los dos extremos INCLUIDOS; `todo` no acota. */
export function rangoDePeriodo(
  p: Periodo,
  hoy: Date = new Date(),
): { desde?: string; hasta?: string } {
  const a = hoy.getFullYear();
  const m = hoy.getMonth();
  switch (p) {
    case 'mes':
      return { desde: iso(a, m, 1), hasta: iso(a, m + 1, 0) };
    case 'mesAnterior':
      return { desde: iso(a, m - 1, 1), hasta: iso(a, m, 0) };
    case 'trimestre': {
      const t = m - (m % 3);
      return { desde: iso(a, t, 1), hasta: iso(a, t + 3, 0) };
    }
    case 'anio':
      return { desde: iso(a, 0, 1), hasta: iso(a, 11, 31) };
    case 'anioAnterior':
      return { desde: iso(a - 1, 0, 1), hasta: iso(a - 1, 11, 31) };
    default:
      return {};
  }
}

/**
 * Las fechas que se proponen al CERRAR el libro: el último día del mes, del trimestre
 * y del año que ya terminaron. Cerrar es declarar acabado lo que pasó —el servicio
 * rechaza una fecha futura—, así que ninguna propuesta cae después de hoy. En enero
 * las tres coinciden (31 de diciembre) y en abril las dos primeras: se dejan una vez,
 * con el rótulo de la más amplia, que es lo que de verdad se está cerrando.
 */
export const PROPUESTAS_CIERRE = ['mesAnterior', 'trimestreAnterior', 'anioAnterior'] as const;
export type PropuestaCierre = (typeof PROPUESTAS_CIERRE)[number];

export const ROTULO_PROPUESTA: Record<PropuestaCierre, ErpMessageKey> = {
  mesAnterior: 'close.endOfLastMonth',
  trimestreAnterior: 'close.endOfLastQuarter',
  anioAnterior: 'close.endOfLastYear',
};

export function propuestasDeCierre(
  hoy: Date = new Date(),
): { propuesta: PropuestaCierre; hasta: string }[] {
  const a = hoy.getFullYear();
  const m = hoy.getMonth();
  const fechas: Record<PropuestaCierre, string> = {
    mesAnterior: iso(a, m, 0),
    trimestreAnterior: iso(a, m - (m % 3), 0),
    anioAnterior: iso(a, 0, 0),
  };
  const fuera = new Set<string>();
  const out: { propuesta: PropuestaCierre; hasta: string }[] = [];
  // De la más amplia a la más corta para quedarse con su rótulo, y luego en su orden.
  for (const p of [...PROPUESTAS_CIERRE].reverse()) {
    if (fuera.has(fechas[p])) continue;
    fuera.add(fechas[p]);
    out.unshift({ propuesta: p, hasta: fechas[p] });
  }
  return out;
}
