/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Importes de la tienda: texto que escribe la persona <-> céntimos.
 *
 * El campo guarda lo que se teclea y solo da formato al salir de él: reformatear en cada
 * pulsación (el `toFixed(2)` de antes) movía el cursor y hacía imposible escribir «12,5».
 */
import { formatNumber, intlTag, type Locale } from '@sytue/i18n';

/** «12,50», «12.5» o «12» -> 1250. Devuelve null si no es un importe válido (>= 0). */
export function parseImporte(texto: string): number | null {
  const limpio = texto.trim().replace(/\s/g, '').replace(',', '.');
  if (!/^\d+(\.\d{0,2})?$/.test(limpio)) return null;
  return Math.round(Number(limpio) * 100);
}

/** 1250 -> «12,50» (según el idioma), sin separador de millares para poder volver a editarlo. */
export function textoImporte(cents: number, locale: Locale): string {
  return formatNumber(locale, cents / 100, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
    useGrouping: false,
  });
}

/** Símbolo de la moneda («€») según el idioma. */
export function simboloMoneda(locale: Locale, moneda = 'EUR'): string {
  const partes = new Intl.NumberFormat(intlTag(locale), {
    style: 'currency',
    currency: moneda,
  }).formatToParts(0);
  return partes.find((p) => p.type === 'currency')?.value ?? moneda;
}
