/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * formatBytes — un tamaño de fichero, escrito como lo escribe cada idioma.
 *
 * Estaba copiado SIETE veces (Mail, Drive, Console y cuatro sitios de Cloud), y
 * las copias no coincidían: cuatro usaban `toFixed`, que siempre pone el punto
 * decimal del inglés. En español, francés, alemán e italiano el separador es la
 * COMA, así que «1.5 MB» se lee como mil quinientos megas en cuatro de los cinco
 * idiomas de producto. Por eso el número pasa por `formatNumber` (Intl) y solo la
 * unidad se concatena a mano: B, KB, MB… son símbolos y no se traducen.
 *
 * Vive en @sytue/ui —y no en @sytue/i18n, que sería su casa natural— porque quien
 * lo necesita es la interfaz, y toda app que pinta bytes ya importa este paquete.
 * ponytail: si aparece un consumidor sin DOM (BFF, script), mudarlo a
 * @sytue/i18n/format y reexportarlo desde aquí.
 */
import { formatNumber, type Locale } from '@sytue/i18n';

const UNIDADES = ['KB', 'MB', 'GB', 'TB'] as const;

export function formatBytes(locale: Locale, n: number): string {
  // Un `NaN` o un negativo vienen de una cuota que el backend no supo calcular:
  // se dice que no se sabe, no «-1 B».
  if (!Number.isFinite(n) || n < 0) return '—';
  if (n < 1024) return `${formatNumber(locale, n)} B`;
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < UNIDADES.length - 1) {
    v /= 1024;
    i += 1;
  }
  // Un decimal por debajo de 10 («1,5 GB»), ninguno por encima («512 MB»): el
  // segundo decimal no cambia ninguna decisión y alarga una cifra que se lee de
  // reojo en el pie de un menú lateral.
  const digits = v < 10 ? 1 : 0;
  const value = formatNumber(locale, v, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  return `${value} ${UNIDADES[i]}`;
}
